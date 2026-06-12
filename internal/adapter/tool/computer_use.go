package tool

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/SHIROH4/sion-plus/internal/port"
)

// ── ComputerUse Agent ──────────────────────────────────────────────

type CUAConfig struct {
	MaxSteps        int
	StepTimeout     time.Duration
	ActionTimeout   time.Duration
	WaitAfterAction time.Duration
}

func DefaultCUAConfig() CUAConfig {
	return CUAConfig{
		MaxSteps:        15,
		StepTimeout:     30 * time.Second,
		ActionTimeout:   5 * time.Second,
		WaitAfterAction: 1 * time.Second,
	}
}

type ComputerUseAgent struct {
	executor port.LLMExecutor
	observer cuaObserver
	cfg      CUAConfig

	screenW int
	screenH int
	dimOnce sync.Once
}

type cuaObserver interface {
	CaptureScreenshot(ctx context.Context) ([]byte, error)
	IsAvailable() bool
}

type macOSObserver struct{ available bool }

func NewMacOSObserver() *macOSObserver {
	return &macOSObserver{available: runtime.GOOS == "darwin"}
}
func (o *macOSObserver) IsAvailable() bool        { return o.available }
func (o *macOSObserver) CaptureScreenshot(ctx context.Context) ([]byte, error) {
	if !o.available {
		return nil, fmt.Errorf("not on macOS")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tmpFile := "/tmp/sion_cua_screenshot.jpg"
	cmd := exec.CommandContext(ctx, "screencapture", "-x", "-t", "jpg", tmpFile)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("screencapture: %w", err)
	}
	return osReadFile(tmpFile)
}

func NewComputerUseAgent(executor port.LLMExecutor, observer cuaObserver) *ComputerUseAgent {
	return &ComputerUseAgent{executor: executor, observer: observer, cfg: DefaultCUAConfig()}
}
func (a *ComputerUseAgent) SetConfig(cfg CUAConfig) { a.cfg = cfg }

// ── Types ──────────────────────────────────────────────────────────

type CUAStep struct {
	Step         int            `json:"step"`
	Observation  string         `json:"observation,omitempty"`
	Thought      string         `json:"thought"`
	Action       string         `json:"action"`
	ActionArgs   map[string]any `json:"action_args,omitempty"`
	Success      bool           `json:"success"`
	ActionResult string         `json:"action_result,omitempty"`
	CaptureMs    int64          `json:"capture_ms"`
	LLMMs        int64          `json:"llm_ms"`
}

type CUAResult struct {
	Summary string    `json:"summary"`
	Steps   []CUAStep `json:"steps"`
	Success bool      `json:"success"`
	Error   string    `json:"error,omitempty"`
}

// ── UI Context (name-based, no coordinates) ────────────────────────

type uiContext struct {
	App       string   // frontmost app name
	ElemNames []string // "AXButton: 关闭按钮", etc.
	DockApps  []string // ["访达", "Google Chrome", ...]
}

func enumerateUI(ctx context.Context) *uiContext {
	script := `tell application "System Events"
    set appName to ""
    try
        set appName to name of first process whose frontmost is true
    end try

    set elemNames to {}
    try
        tell process appName
            set elems to UI elements of window 1
            repeat with e in elems
                try
                    set eRole to role of e
                    set eLabel to ""
                    try
                        set eLabel to description of e
                    end try
                    if eLabel is missing value or eLabel is "" then
                        try
                            set eLabel to name of e
                        end try
                    end if
                    if eLabel is not missing value and eLabel is not "" then
                        set end of elemNames to eRole & ": " & eLabel
                    end if
                end try
            end repeat
        end tell
    end try

    set dockApps to {}
    try
        tell process "Dock"
            set dockElems to UI elements of list 1
            repeat with d in dockElems
                try
                    set dName to name of d
                    if dName is not missing value and dName is not "" then
                        set end of dockApps to dName
                    end if
                end try
            end repeat
        end tell
    end try

    return appName & linefeed & AppleScript's text item delimiters & "|||" & elemNames & linefeed & AppleScript's text item delimiters & "|||" & dockApps
end tell`

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return &uiContext{}
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "\n", 3)
	uc := &uiContext{}
	if len(parts) >= 1 {
		uc.App = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 2 && parts[1] != "" {
		uc.ElemNames = strings.Split(parts[1], "|||")
	}
	if len(parts) >= 3 && parts[2] != "" {
		uc.DockApps = strings.Split(parts[2], "|||")
	}
	return uc
}

func (uc *uiContext) format() string {
	if uc == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("## UI Context\n")
	if uc.App != "" {
		b.WriteString(fmt.Sprintf("Frontmost: **%s**\n", uc.App))
	}
	if len(uc.DockApps) > 0 {
		b.WriteString("Dock: " + strings.Join(uc.DockApps, ", ") + "\n")
	}
	if len(uc.ElemNames) > 0 {
		b.WriteString("Clickable: ")
		shown := uc.ElemNames
		if len(shown) > 8 {
			shown = shown[:8]
		}
		b.WriteString(strings.Join(shown, " | ") + "\n")
	}
	return b.String()
}

// ── Coordinate projection ──────────────────────────────────────────

const coordMax = 999

func projectCoord(v int, screenDim int) int {
	if v >= 0 && v <= coordMax {
		return v * screenDim / coordMax
	}
	return v
}

func (a *ComputerUseAgent) cacheScreenSize(jpg []byte) {
	a.dimOnce.Do(func() {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(jpg))
		if err != nil {
			a.screenW, a.screenH = 1920, 1080
			return
		}
		a.screenW = cfg.Width
		a.screenH = cfg.Height
		log.Printf("[CUA] screen: %dx%d", a.screenW, a.screenH)
	})
}

// ── Main loop ──────────────────────────────────────────────────────

func (a *ComputerUseAgent) Run(ctx context.Context, task string) *CUAResult {
	if a.executor == nil {
		return &CUAResult{Error: "ComputerUse: no VLM executor configured"}
	}
	if !a.observer.IsAvailable() {
		return &CUAResult{Error: "ComputerUse: not on macOS"}
	}

	result := &CUAResult{}
	var prevJPG []byte

	for step := 0; step < a.cfg.MaxSteps; step++ {
		select {
		case <-ctx.Done():
			result.Error = "task cancelled"
			return result
		default:
		}

		// 1. Screenshot
		t0 := time.Now()
		jpg, err := a.observer.CaptureScreenshot(ctx)
		if err != nil {
			result.Error = fmt.Sprintf("screenshot step %d: %v", step+1, err)
			return result
		}
		a.cacheScreenSize(jpg)
		captureMs := time.Since(t0).Milliseconds()

		// 2. UI context (element names, not coordinates)
		uiCtx := enumerateUI(ctx)

		// 3. Build prompt
		prompt := buildCUAPrompt(step+1, a.cfg.MaxSteps, task, prevJPG != nil, uiCtx)

		// 4. Build message
		var msg port.LLMMessage
		curB64 := b64Str(jpg)
		if len(prevJPG) > 0 {
			prevB64 := b64Str(compressJPEG(prevJPG, 50*1024))
			msg = port.LLMMessage{
				Role: "user",
				ContentParts: []port.ContentPart{
					{Type: "text", Text: prompt},
					{Type: "image_url", ImageURL: "data:image/jpeg;base64," + prevB64},
					{Type: "image_url", ImageURL: "data:image/jpeg;base64," + curB64},
				},
			}
		} else {
			msg = port.NewVisionMessage(prompt, curB64)
		}

		// 5. Call VLM
		t1 := time.Now()
		vlmCtx, vlmCancel := context.WithTimeout(ctx, a.cfg.StepTimeout)
		resp, err := a.executor.Chat(vlmCtx, "", []port.LLMMessage{msg})
		vlmCancel()
		llmMs := time.Since(t1).Milliseconds()
		if err != nil {
			result.Error = fmt.Sprintf("VLM step %d: %v", step+1, err)
			return result
		}

		// 6. Parse
		cuaStep := parseCUAResponse(resp)
		cuaStep.Step = step + 1
		cuaStep.CaptureMs = captureMs
		cuaStep.LLMMs = llmMs
		if cuaStep.Action == "done" {
			cuaStep.Success = true
			result.Steps = append(result.Steps, cuaStep)
			if s, ok := cuaStep.ActionArgs["summary"].(string); ok {
				result.Summary = s
			}
			result.Success = true
			log.Printf("[CUA] done in %d steps (capture=%dms llm=%dms): %s",
				step+1, captureMs, llmMs, truncateStr(result.Summary, 100))
			return result
		}

		// 7. Execute
		actionResult, err := a.execAction(a.cfg.ActionTimeout, cuaStep.Action, cuaStep.ActionArgs, uiCtx)
		cuaStep.ActionResult = actionResult
		if err != nil {
			cuaStep.Success = false
			log.Printf("[CUA] step %d FAIL: %s → %v (capture=%dms llm=%dms)",
				step+1, cuaStep.Action, err, captureMs, llmMs)
		} else {
			cuaStep.Success = true
			log.Printf("[CUA] step %d  OK: %s (capture=%dms llm=%dms)",
				step+1, cuaStep.Action, captureMs, llmMs)
		}
		result.Steps = append(result.Steps, cuaStep)
		prevJPG = jpg
		time.Sleep(a.cfg.WaitAfterAction)
	}

	result.Error = fmt.Sprintf("max steps (%d) reached without completion", a.cfg.MaxSteps)
	return result
}

// ── Prompt ─────────────────────────────────────────────────────────

const cuaSystemPrompt = `You are an expert macOS GUI automation agent. Use named actions from the UI Context — never guess coordinates.

## Actions

open_app — Click an app in the Dock (use EXACT name from Dock list)
  {"action": "open_app", "args": {"name": "Google Chrome"}}

click_elem — Click an element in the frontmost app by its LABEL from the Clickable list
  {"action": "click_elem", "args": {"label": "关闭按钮"}}

key — Keyboard shortcut
  {"action": "key", "args": {"keys": "cmd+space"}}
  Common: cmd+space, return, escape, cmd+l, cmd+t, cmd+w

type — Type text into focused field
  {"action": "type", "args": {"text": "hello"}}

scroll — Scroll  {"action": "scroll", "args": {"direction": "down"}}
wait — Wait    {"action": "wait", "args": {"seconds": 3}}
done — Task complete. Frontmost app MUST match target.
  {"action": "done", "args": {"summary": "Safari is now focused"}}

## Response Format

## Verification
[Skip step 1. Compare before/after screenshots. Be honest.]

## Observation
[What you see now.]

## Thought
[Which named element to use and why.]

## Action
[One sentence.]

## JSON
` + "```json" + `
{"action": "open_app", "args": {"name": "Safari"}}
` + "```" + `

## Rules
1. Use open_app with exact Dock name to launch apps.
2. Use click_elem with exact label from the Clickable list.
3. Use key for keyboard shortcuts.
4. done ONLY when the target app IS the Frontmost app.
5. NEVER guess coordinates or names — use EXACT values from UI Context.
6. Do not repeat failing actions.`

func buildCUAPrompt(step, maxSteps int, task string, hasHistory bool, uiCtx *uiContext) string {
	verifyHint := "(skip — first step)"
	if hasHistory {
		verifyHint = "Compare image-1 (before) and image-2 (after)."
	}
	uiSection := ""
	if uiCtx != nil {
		uiSection = "\n" + uiCtx.format()
	}
	return fmt.Sprintf("%s%s\n\nTask: %s\nStep %d/%d\n\nVerification: %s",
		cuaSystemPrompt, uiSection, task, step, maxSteps, verifyHint)
}

// ── Response Parser ────────────────────────────────────────────────

func parseCUAResponse(raw string) CUAStep {
	raw = strings.TrimSpace(raw)
	step := CUAStep{
		Thought: truncateStr(extractSection(raw, "Thought"), 200),
		Action:  extractSection(raw, "Action"),
	}
	if obs := extractSection(raw, "Observation"); obs != "" {
		step.Observation = truncateStr(obs, 200)
	}
	jsonStr := extractJSONBlock(raw)
	if jsonStr == "" {
		return CUAStep{Thought: raw, Action: "done", ActionResult: "parse error: no JSON in response"}
	}
	var parsed struct {
		Action string         `json:"action"`
		Args   map[string]any `json:"args"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return CUAStep{Thought: raw, Action: "done", ActionResult: "parse error: " + err.Error()}
	}
	step.Action = parsed.Action
	step.ActionArgs = parsed.Args
	if step.Action == "done" {
		if s, ok := parsed.Args["summary"].(string); ok {
			step.ActionResult = s
		}
	}
	return step
}

func extractSection(text, heading string) string {
	lower := strings.ToLower(text)
	marker := "## " + strings.ToLower(heading)
	idx := strings.Index(lower, marker)
	if idx < 0 {
		return ""
	}
	start := strings.Index(text[idx:], "\n")
	if start < 0 {
		return ""
	}
	start += idx + 1
	remaining := text[start:]
	next := strings.Index(remaining, "\n## ")
	if next < 0 {
		next = len(remaining)
	}
	return strings.TrimSpace(remaining[:next])
}

func extractJSONBlock(text string) string {
	if idx := strings.Index(text, "```json"); idx >= 0 {
		start := idx + 7
		if nl := strings.IndexByte(text[start:], '\n'); nl >= 0 {
			start += nl + 1
		}
		if end := strings.Index(text[start:], "```"); end >= 0 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			return line
		}
	}
	return ""
}

// ── Action Executor (name-based clicking) ──────────────────────────

func (a *ComputerUseAgent) execAction(timeout time.Duration, action string, args map[string]any, uiCtx *uiContext) (string, error) {
	switch action {
	case "open_app":
		return execOpenApp(args)
	case "click_elem":
		return execClickElem(args, uiCtx)
	case "key":
		keys, _ := args["keys"].(string)
		return execKeyCombo(timeout, keys)
	case "type":
		text, _ := args["text"].(string)
		escaped := strings.ReplaceAll(strings.ReplaceAll(text, `\`, `\\`), `"`, `\"`)
		return execAppleScript(timeout, fmt.Sprintf(`tell application "System Events" to keystroke "%s"`, escaped))
	case "scroll":
		dir, _ := args["direction"].(string)
		code := "119"
		if dir == "up" {
			code = "116"
		}
		return execAppleScript(timeout, fmt.Sprintf(`tell application "System Events" to key code %s`, code))
	case "wait":
		sec := intArg(args, "seconds", 2)
		if sec > 10 {
			sec = 10
		}
		time.Sleep(time.Duration(sec) * time.Second)
		return fmt.Sprintf("waited %ds", sec), nil
	case "done":
		summary, _ := args["summary"].(string)
		if summary == "" {
			summary = "completed"
		}
		return summary, nil
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// execOpenApp clicks a Dock icon by name.
func execOpenApp(args map[string]any) (string, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return "", fmt.Errorf("open_app requires 'name'")
	}
	escaped := strings.ReplaceAll(name, `"`, `\"`)
	script := fmt.Sprintf(
		`tell application "System Events" to tell process "Dock" to click UI element "%s" of list 1`,
		escaped)
	res, err := execAppleScript(4*time.Second, script)
	if err != nil {
		// Fallback: Spotlight
		execAppleScript(2*time.Second, `tell application "System Events" to keystroke " " using {command down}`)
		time.Sleep(200 * time.Millisecond)
		escaped2 := strings.ReplaceAll(strings.ReplaceAll(name, `\`, `\\`), `"`, `\"`)
		execAppleScript(2*time.Second, fmt.Sprintf(`tell application "System Events" to keystroke "%s"`, escaped2))
		time.Sleep(200 * time.Millisecond)
		if _, e := execAppleScript(2*time.Second, `tell application "System Events" to keystroke return`); e != nil {
			return "", fmt.Errorf("open_app: %w (dock err: %v)", e, err)
		}
		return fmt.Sprintf("opened %s via Spotlight (dock click failed: %v)", name, err), nil
	}
	return fmt.Sprintf("clicked %s in Dock (%s)", name, res), nil
}

// execClickElem clicks an element in the frontmost app by its description/name.
func execClickElem(args map[string]any, uiCtx *uiContext) (string, error) {
	label, _ := args["label"].(string)
	if label == "" {
		return "", fmt.Errorf("click_elem requires 'label'")
	}
	app := "System Events"
	if uiCtx != nil && uiCtx.App != "" {
		app = uiCtx.App
	}
	escaped := strings.ReplaceAll(label, `"`, `\"`)
	// Click first element whose description or name matches
	script := fmt.Sprintf(
		`tell application "System Events"
    tell process "%s"
        set found to false
        repeat with e in (UI elements of window 1)
            try
                set eDesc to description of e
            on error
                set eDesc to ""
            end try
            try
                set eName to name of e
            on error
                set eName to ""
            end try
            if eDesc contains "%s" or eName contains "%s" then
                click e
                set found to true
                exit repeat
            end if
        end repeat
        return found
    end tell
end tell`, app, escaped, escaped)
	out, err := execAppleScript(4*time.Second, script)
	if err != nil {
		return "", fmt.Errorf("click_elem: %w (output: %s)", err, out)
	}
	return fmt.Sprintf("clicked '%s' in %s", label, app), nil
}

func execAppleScript(timeout time.Duration, script string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("osascript: %w", err)
	}
	s := strings.TrimSpace(out.String())
	if s == "" {
		return "ok", nil
	}
	return s, nil
}

func execKeyCombo(timeout time.Duration, keys string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	parts := strings.Split(strings.ToLower(keys), "+")
	var mainKey string
	var mods []string
	for _, p := range parts {
		switch strings.TrimSpace(p) {
		case "cmd", "command":
			mods = append(mods, "command down")
		case "shift":
			mods = append(mods, "shift down")
		case "option", "alt":
			mods = append(mods, "option down")
		case "ctrl", "control":
			mods = append(mods, "control down")
		default:
			mainKey = p
		}
	}
	var script string
	if len(mods) > 0 {
		script = fmt.Sprintf(`tell application "System Events" to keystroke "%s" using {%s}`, mainKey, strings.Join(mods, ", "))
	} else {
		script = fmt.Sprintf(`tell application "System Events" to keystroke "%s"`, mainKey)
	}
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("key combo: %w", err)
	}
	return outStr(out.String()), nil
}

// ── Helpers ────────────────────────────────────────────────────────

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func outStr(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "ok"
	}
	return s
}

func b64Str(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func compressJPEG(jpg []byte, target int) []byte {
	if len(jpg) <= target {
		return jpg
	}
	img, _, err := image.Decode(bytes.NewReader(jpg))
	if err != nil {
		return jpg
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for scale := 0.9; scale >= 0.4; scale -= 0.15 {
		nw, nh := int(float64(w)*scale), int(float64(h)*scale)
		if nw < 320 {
			nw = 320
		}
		if nh < 180 {
			nh = 180
		}
		scaled := scaleImage(img, nw, nh)
		buf := new(bytes.Buffer)
		if err := jpeg.Encode(buf, scaled, &jpeg.Options{Quality: 60}); err != nil {
			return jpg
		}
		if buf.Len() <= target {
			return buf.Bytes()
		}
	}
	return jpg
}

func scaleImage(src image.Image, w, h int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(x, y, src.At(x*sw/w, y*sh/h))
		}
	}
	return dst
}

var osReadFile = func(path string) ([]byte, error) { return os.ReadFile(path) }

// ── Tool Registration ──────────────────────────────────────────────

func (r *ToolRegistry) RegisterComputerUseTool(agent *ComputerUseAgent) {
	r.Register(&ToolDef{
		Name:        "computer_use",
		Description: "Control the Mac to perform a task. Use for: opening apps, clicking, typing, navigating UI, browser search.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task": map[string]any{"type": "string", "description": "What to do."},
			},
			"required": []string{"task"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			task, _ := args["task"].(string)
			if task == "" {
				return "", fmt.Errorf("task is required")
			}
			result := agent.Run(ctx, task)
			if !result.Success {
				return "", fmt.Errorf("computer_use: %s", result.Error)
			}
			var b strings.Builder
			b.WriteString(fmt.Sprintf("Completed in %d steps:\n", len(result.Steps)))
			for _, s := range result.Steps {
				b.WriteString(fmt.Sprintf("  %d. %s → %s\n", s.Step, s.Action, truncateStr(s.ActionResult, 60)))
			}
			b.WriteString("\nSummary: " + result.Summary)
			return b.String(), nil
		},
		Dangerous: true,
	})
}
