package port

import "context"

// ── Chat Tool Provider ──

// ChatToolProvider is the minimal tool-execution interface ChatOrchestrator needs.
// It hides the full ToolRegistry behind a narrow contract.
type ChatToolProvider interface {
	// ToolCount returns the number of registered tools (>0 means tools are available).
	ToolCount() int

	// ExecuteTools attempts a tool-assisted response. If the LLM doesn't call any tool,
	// it returns a direct text response with resultCount=0.
	ExecuteTools(
		ctx context.Context,
		executor LLMExecutor,
		systemPrompt string,
		userMsg string,
	) (response string, resultCount int, err error)
}
