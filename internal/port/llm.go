package port

import "context"

// LLMCallMetadata describes how one LLM call should be routed and reported.
// It travels with the request context so shared executors can keep global
// rate limiting while still selecting task-specific providers and metrics.
type LLMCallMetadata struct {
	Route    string
	CallType string
}

type llmCallMetadataKey struct{}

// WithLLMCallMetadata annotates an LLM request with an optional provider route
// and a stable call type for token/cost attribution.
func WithLLMCallMetadata(ctx context.Context, route, callType string) context.Context {
	return context.WithValue(ctx, llmCallMetadataKey{}, LLMCallMetadata{Route: route, CallType: callType})
}

// LLMRouteFromContext returns the annotated route, or fallback when absent.
func LLMRouteFromContext(ctx context.Context, fallback string) string {
	if metadata, ok := ctx.Value(llmCallMetadataKey{}).(LLMCallMetadata); ok && metadata.Route != "" {
		return metadata.Route
	}
	return fallback
}

// LLMCallTypeFromContext returns the annotated metric label, or fallback when absent.
func LLMCallTypeFromContext(ctx context.Context, fallback string) string {
	if metadata, ok := ctx.Value(llmCallMetadataKey{}).(LLMCallMetadata); ok && metadata.CallType != "" {
		return metadata.CallType
	}
	return fallback
}

// ── LLM Executor ──

// LLMExecutor is the unified LLM calling interface.
// All LLM calls in the project go through this interface — never call
// provider APIs directly. This enables:
//   - Provider fallback (multi-provider registry)
//   - Token usage tracking (WrapLLMExecutor decorator)
//   - Mocking in tests
type LLMExecutor interface {
	// Chat performs synchronous chat completion.
	Chat(ctx context.Context, systemPrompt string, msgs []LLMMessage) (string, error)

	// ChatWithTools performs chat with function calling (tool_choice support).
	ChatWithTools(
		ctx context.Context,
		systemPrompt string,
		msgs []LLMMessage,
		tools []ToolDef,
		onToolCall func(name, argsJSON string) string,
		maxRounds int,
		toolChoice string,
	) (string, error)

	// ChatStream performs streaming chat for real-time output.
	ChatStream(ctx context.Context, systemPrompt string, msgs []LLMMessage, onChunk func(chunk string) error) error

	// IsAvailable checks if the LLM service is reachable.
	IsAvailable(ctx context.Context) bool
}

// LLMMessage is a single message in a chat conversation.
// When Content is set, it's a simple text message.
// When ContentParts is set, it's a multi-modal message (text + images).
type LLMMessage struct {
	Role         string        `json:"role"` // "system"|"user"|"assistant"|"tool"
	Content      string        `json:"content,omitempty"`
	ContentParts []ContentPart `json:"content_parts,omitempty"`
}

// ContentPart is a single part of a multi-modal message.
type ContentPart struct {
	Type     string `json:"type"` // "text" | "image_url"
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"` // data:image/jpeg;base64,... or https://...
}

// NewVisionMessage creates a multi-modal user message with text + base64 image.
func NewVisionMessage(prompt, base64JPEG string) LLMMessage {
	return LLMMessage{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "text", Text: prompt},
			{Type: "image_url", ImageURL: "data:image/jpeg;base64," + base64JPEG},
		},
	}
}

// ToolDef is an OpenAI-compatible function calling tool definition.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// ── Embedding Service ──

// EmbeddingService computes text embeddings for semantic search.
// Default implementation uses Ollama locally; can fall back to cloud API.
type EmbeddingService interface {
	Vectorize(ctx context.Context, text string) ([]float32, error)
	BatchVectorize(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
	IsAvailable() bool
}

// ── LLM Provider Registry ──

// LLMProviderRegistry manages multiple LLM providers with fallback chains.
type LLMProviderRegistry interface {
	GetExecutor(taskType string) (LLMExecutor, string, error)
	Reload(providers []LLMProviderConfig, routes LLMRoutes) error
	ListHealthy() []string
	MarkUnhealthy(name string)
	MarkHealthy(name string)
	StartHealthCheck(ctx context.Context)
}

type LLMProviderConfig struct {
	Name       string `yaml:"name" json:"name"`
	BaseURL    string `yaml:"base_url" json:"base_url"`
	APIKey     string `yaml:"api_key" json:"api_key"`
	ChatModel  string `yaml:"chat_model" json:"chat_model"`
	EmbedModel string `yaml:"embed_model,omitempty" json:"embed_model,omitempty"`
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	Priority   int    `yaml:"priority" json:"priority"`
	MaxRetries int    `yaml:"max_retries" json:"max_retries"`
	TimeoutSec int    `yaml:"timeout_sec" json:"timeout_sec"`
}

type LLMRoutes struct {
	Default string `yaml:"default" json:"default"`
	Chat    string `yaml:"chat" json:"chat"`
	Emotion string `yaml:"emotion" json:"emotion"`
	Memory  string `yaml:"memory" json:"memory"`
	Vision  string `yaml:"vision" json:"vision"`
	Summary string `yaml:"summary" json:"summary"`
	Signal  string `yaml:"signal" json:"signal"`
	Search  string `yaml:"search" json:"search"`
}
