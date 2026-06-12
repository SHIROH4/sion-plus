package tool

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SHIROH4/sion-plus/internal/port"
)

// ToolHandler is the function signature for tool execution.
// Receives parsed JSON arguments, returns result text or error.
type ToolHandler func(ctx context.Context, args map[string]any) (string, error)

// ToolDef is a registered tool with its schema and handler.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema for the arguments
	Handler     ToolHandler
	Dangerous   bool // requires user confirmation before execution
	Sandboxed   bool // runs in restricted environment
}

// ToPort converts to port.ToolDef (OpenAI-compatible format).
func (t *ToolDef) ToPort() port.ToolDef {
	return port.ToolDef{
		Name:        t.Name,
		Description: t.Description,
		Parameters:  t.Parameters,
	}
}

// ToolResult is the structured output of a tool execution.
type ToolResult struct {
	ToolName  string `json:"tool_name"`
	Success   bool   `json:"success"`
	Output    string `json:"output"`
	Error     string `json:"error,omitempty"`
	Duration  string `json:"duration"`
	Truncated bool   `json:"truncated,omitempty"`
}

// toolCacheEntry holds a cached tool result for deduplication.
type toolCacheEntry struct {
	result    string
	expiresAt time.Time
}

// ToolRegistry manages tool registration, discovery, and execution.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]*ToolDef

	// Task dedup: cache recent tool results by (toolName, task hash)
	resultCache map[string]toolCacheEntry

	// Stats
	totalCalls    int
	totalErrors   int
	totalDeduped  int
	lastCallAt    time.Time
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:        make(map[string]*ToolDef),
		resultCache:  make(map[string]toolCacheEntry),
	}
}

// Register adds or replaces a tool.
func (r *ToolRegistry) Register(tool *ToolDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name] = tool
	log.Printf("[ToolRegistry] registered %q (dangerous=%v)", tool.Name, tool.Dangerous)
}

// Unregister removes a tool by name.
func (r *ToolRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// Get returns a tool by name.
func (r *ToolRegistry) Get(name string) *ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// List returns all registered tools.
func (r *ToolRegistry) List() []*ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// Specs returns all tools as port.ToolDef for LLM function calling.
func (r *ToolRegistry) Specs() []port.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]port.ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.ToPort())
	}
	return out
}

// SpecsByNames returns specific tools by name for selective function calling.
func (r *ToolRegistry) SpecsByNames(names []string) []port.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]port.ToolDef, 0, len(names))
	for _, name := range names {
		if t, ok := r.tools[name]; ok {
			out = append(out, t.ToPort())
		}
	}
	return out
}

// Execute runs a tool by name with the given arguments.
// Deduplicates: returns cached result if same (name, args) was executed within 60s.
func (r *ToolRegistry) Execute(ctx context.Context, name string, args map[string]any) *ToolResult {
	// Generate cache key from tool name + args
	cacheKey := name + ":" + hashArgs(args)

	// Check cache (only for expensive tools with arguments)
	if isExpensiveTool(name) && len(args) > 0 {
		r.mu.RLock()
		if cached, ok := r.resultCache[cacheKey]; ok && time.Now().Before(cached.expiresAt) {
			r.mu.RUnlock()
			r.mu.Lock()
			r.totalDeduped++
			r.mu.Unlock()
			log.Printf("[ToolRegistry] dedup hit: %s", cacheKey)
			return &ToolResult{ToolName: name, Success: true, Output: cached.result, Duration: "0s"}
		}
		r.mu.RUnlock()
	}

	r.mu.Lock()
	r.totalCalls++
	r.lastCallAt = time.Now()
	r.mu.Unlock()

	start := time.Now()
	result := &ToolResult{ToolName: name}

	tool := r.Get(name)
	if tool == nil {
		r.mu.Lock()
		r.totalErrors++
		r.mu.Unlock()
		result.Success = false
		result.Error = fmt.Sprintf("tool %q not found", name)
		result.Duration = time.Since(start).String()
		return result
	}

	output, err := tool.Handler(ctx, args)
	result.Duration = time.Since(start).String()

	if err != nil {
		r.mu.Lock()
		r.totalErrors++
		r.mu.Unlock()
		result.Success = false
		result.Error = err.Error()
		result.Output = output // partial output even on error
		return result
	}

	result.Success = true
	result.Output = truncateOutput(output)
	if len(output) > maxOutputLen {
		result.Truncated = true
	}

	// Cache expensive tool results for 60s
	if isExpensiveTool(name) && len(args) > 0 {
		r.mu.Lock()
		r.resultCache[cacheKey] = toolCacheEntry{result: output, expiresAt: time.Now().Add(60 * time.Second)}
		r.mu.Unlock()
	}

	return result
}

// isExpensiveTool returns true for tools with high latency/cost that benefit from dedup.
func isExpensiveTool(name string) bool {
	switch name {
	case "web_search", "browser", "computer_use":
		return true
	}
	return false
}

func hashArgs(args map[string]any) string {
	if len(args) == 0 {
		return "empty"
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(fmt.Sprint(args[k]))
		b.WriteString(";")
	}
	return b.String()
}

// ── port.ChatToolProvider ─────────────────────────────────────────

// ToolCount returns the number of registered tools.
func (r *ToolRegistry) ToolCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// TryToolCall wraps the full TryToolCall signature for port.ChatToolProvider.
// It always passes nil for approveFn (no dangerous-tool confirmation in chat).
func (r *ToolRegistry) ExecuteTools(
	ctx context.Context,
	executor port.LLMExecutor,
	systemPrompt string,
	userMsg string,
) (string, int, error) {
	resp, results, err := r.TryToolCall(ctx, executor, systemPrompt, userMsg, nil)
	return resp, len(results), err
}

var _ port.ChatToolProvider = (*ToolRegistry)(nil)

// Stats returns registry statistics.
func (r *ToolRegistry) Stats() (calls, errors int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.totalCalls, r.totalErrors
}

// ── Helpers ──────────────────────────────────────────────────────

const maxOutputLen = 8000 // characters before truncation

func truncateOutput(s string) string {
	if len(s) <= maxOutputLen {
		return s
	}
	return s[:maxOutputLen] + fmt.Sprintf("\n... (truncated, %d total chars)", len(s))
}
