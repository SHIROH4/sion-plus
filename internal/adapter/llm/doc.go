// Package llm implements port.LLMExecutor, port.EmbeddingService, port.LLMProviderRegistry, port.TokenUsageTracker.
//
// Files:
//   openai_gateway.go    — OpenAI-compatible Chat/ChatWithTools/ChatStream (port.LLMExecutor)
//   provider_registry.go — multi-provider management with fallback chains (port.LLMProviderRegistry)
//   ollama_embed.go      — Ollama local embedding service (port.EmbeddingService)
//   rate_limiter.go      — token bucket rate limiter
//   retry.go             — retry with backoff
//   token_tracker.go     — non-blocking token usage recorder
//   tracked_executor.go  — executor wrapper with token tracking

package llm
