package sdk

import "github.com/SHIROH4/sion-plus/internal/port"

// PluginContext is the dependency injection container passed to every plugin
// at Init time. It provides access to all port interfaces without importing
// adapter implementations directly.
//
// New fields MUST be added here (and populated by the runtime) — plugins
// must not reach around this context to get dependencies.
type PluginContext struct {
	// EventBus for cross-plugin pub/sub communication.
	EventBus port.EventBus

	// LLMExecutor for AI calls (chat, tools, streaming).
	LLMExecutor port.LLMExecutor

	// MemoryStore for L0-L3 memory read/write.
	MemoryStore port.MemoryStore

	// MemoryRecall for hybrid (BM25+vector) memory search.
	MemoryRecall port.MemoryRecall

	// EmotionStateManager for reading/writing emotion state.
	EmotionStateManager port.EmotionStateManager

	// ConfigManager for reading global configuration.
	ConfigManager port.ConfigManager

	// ToolRegistry for registering AI-callable function tools.
	ToolRegistry ToolRegistry

	// IntentSubmitter for enqueuing proactive intents.
	IntentSubmitter IntentSubmitter

	// DataDir is the path to the Sion data directory (~/.sion).
	DataDir string
}

// ToolRegistry is the subset of port.ToolRegistry that plugins need.
// Defined here to avoid circular imports (plugins import sdk, not adapter/tool).
type ToolRegistry interface {
	Register(tool ToolDef) error
	Unregister(name string) error
}

// ToolDef is a simplified tool definition for plugin registration.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema
	Handler     func(argsJSON string) (string, error)
}

// IntentSubmitter allows plugins to submit proactive intents into the
// cognition pipeline (e.g., a timer plugin firing a reminder).
type IntentSubmitter interface {
	Submit(intent ProactiveIntent) error
}

// ProactiveIntent is a plugin-originated desire for the AI to speak/act.
type ProactiveIntent struct {
	Source      string // "plugin:timer", "plugin:qq", etc.
	Action      string // action name from the action registry
	Message     string // instruction/prompt for the LLM
	Priority    int    // 0-10
	CoalesceKey string // empty = never coalesce
}
