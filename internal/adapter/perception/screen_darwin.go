package perception

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/SHIROH4/sion-plus/internal/port"
)

// ScreenObserver implements port.ScreenObserver for macOS.
// Uses osascript for window info, ioreg for idle time, screencapture for screenshots.
type ScreenObserver struct {
	classifier    *AppClassifier
	lastApp       string
	lastTitle     string
	lastSwitchAt  time.Time
	switchCount   int
	switchWindow  time.Duration // 5-minute window for switch counting
	available     bool
}

var _ port.ScreenObserver = (*ScreenObserver)(nil)

func NewScreenObserver(classifier *AppClassifier) *ScreenObserver {
	return &ScreenObserver{
		classifier:   classifier,
		switchWindow: 5 * time.Minute,
		available:    runtime.GOOS == "darwin",
	}
}

func (o *ScreenObserver) IsAvailable() bool { return o.available }

func (o *ScreenObserver) Observe(ctx context.Context) (*port.ScreenObservation, error) {
	if !o.available {
		return nil, fmt.Errorf("ScreenObserver: not on macOS")
	}

	appName, err := o.frontmostApp(ctx)
	if err != nil {
		appName = "unknown"
	}

	windowTitle, err := o.windowTitle(ctx)
	if err != nil {
		windowTitle = ""
	}

	cls := o.classifier.Classify(appName, windowTitle)

	// Track app switches for the state machine
	o.trackSwitch(appName, windowTitle)

	return &port.ScreenObservation{
		AppName:     appName,
		AppCategory: cls.Primary,
		WindowTitle: windowTitle,
		Timestamp:   time.Now().Unix(),
		// IdleSec and SwitchCount can be used by the state machine
	}, nil
}

// ── osascript wrappers ─────────────────────────────────────────────

func (o *ScreenObserver) runAppleScript(ctx context.Context, script string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// selfProcessNames lists process names that belong to Sion itself.
// When these are frontmost, we skip them and look at the app behind.
var selfProcessNames = map[string]bool{
	"Electron": true,
	"sion":     true,
	"Sion":     true,
}

func (o *ScreenObserver) frontmostApp(ctx context.Context) (string, error) {
	name, err := o.runAppleScript(ctx,
		`tell application "System Events" to get name of first application process whose frontmost is true`)
	if err != nil {
		return "", err
	}
	// If the frontmost app is Sion itself, try to get the second frontmost.
	if selfProcessNames[name] {
		second, err2 := o.runAppleScript(ctx,
			`tell application "System Events"
				set allProcs to (every process whose visible is true)
				if (count of allProcs) >= 2 then
					set secondProc to item 2 of allProcs
					return name of secondProc
				end if
				return ""
			end tell`)
		if err2 == nil && second != "" && !selfProcessNames[second] {
			return second, nil
		}
		// Fallback: return the self-name but mark it clearly
		return "Sion (自分)", nil
	}
	return name, nil
}

func (o *ScreenObserver) windowTitle(ctx context.Context) (string, error) {
	s, err := o.runAppleScript(ctx,
		`tell application "System Events" to get title of front window of first application process whose frontmost is true`)
	if err != nil {
		return "", err
	}
	return s, nil
}

// ── Idle time ──────────────────────────────────────────────────────

func (o *ScreenObserver) idleTime(ctx context.Context) float64 {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ioreg", "-c", "IOHIDSystem")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0
	}

	// Parse ioreg output for HIDIdleTime in nanoseconds
	for _, line := range strings.Split(out.String(), "\n") {
		if !strings.Contains(line, "HIDIdleTime") {
			continue
		}
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "=" && i+1 < len(fields) {
				ns, err := strconv.ParseInt(fields[i+1], 10, 64)
				if err == nil {
					return float64(ns) / 1e9 // ns → seconds
				}
			}
		}
	}
	return 0
}

// ── Screenshot (low priority) ──────────────────────────────────────

func (o *ScreenObserver) CaptureScreenshot(ctx context.Context) ([]byte, error) {
	if !o.available {
		return nil, fmt.Errorf("CaptureScreenshot: not on macOS")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	tmpFile := "/tmp/sion_screenshot.jpg"
	cmd := exec.CommandContext(ctx, "screencapture", "-x", "-t", "jpg", tmpFile)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("screencapture: %w", err)
	}
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("read screenshot: %w", err)
	}
	os.Remove(tmpFile)
	if len(data) == 0 {
		return nil, fmt.Errorf("screencapture: empty output")
	}
	return data, nil
}

// ── App switch tracking ─────────────────────────────────────────────

func (o *ScreenObserver) trackSwitch(app, title string) {
	if app != o.lastApp {
		now := time.Now()
		if now.Sub(o.lastSwitchAt) < o.switchWindow {
			o.switchCount++
		} else {
			o.switchCount = 1
		}
		o.lastSwitchAt = now
		o.lastApp = app
	}
	o.lastTitle = title
}

// SwitchCount returns how many app switches occurred in the last window.
func (o *ScreenObserver) SwitchCount() int { return o.switchCount }

// ── Idle seconds (public accessor for state machine) ────────────────

func (o *ScreenObserver) IdleSeconds(ctx context.Context) float64 {
	return o.idleTime(ctx)
}

// ── Debug ───────────────────────────────────────────────────────────

func (o *ScreenObserver) Debug(ctx context.Context) {
	state, err := o.Observe(ctx)
	if err != nil {
		log.Printf("[ScreenObserver] error: %v", err)
		return
	}
	log.Printf("[ScreenObserver] app=%q cat=%s title=%q switches=%d",
		state.AppName, state.AppCategory, state.WindowTitle, o.switchCount)
}
