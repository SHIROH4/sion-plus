// Package llm implements port.LLMExecutor, port.EmbeddingService, port.LLMProviderRegistry, port.TokenUsageTracker.
//
// Files:
//   openai_gateway.go    — OpenAI-compatible Chat/ChatWithTools/ChatStream (port.LLMExecutor)
//   provider_registry.go — multi-provider management with fallback chains (port.LLMProviderRegistry)
//   ollama_embed.go      — Ollama local embedding service (port.EmbeddingService)
//   token_tracker.go     — non-blocking token usage recorder + JSONL writer (port.TokenUsageTracker)
//   health_check.go      — periodic provider health probe with auto-recovery
//   remote_embed.go      — cloud embedding API fallback when Ollama unavailable

package llm

// TODO (module 10): Implement ProviderRegistry
//   - Reload(): rebuild provider pool from config
//   - GetExecutor(taskType): resolve route → check healthy → return executor, fallback on failure
//   - MarkUnhealthy/MarkHealthy: toggle with logging
//   - StartHealthCheck: periodic probe goroutine

// TODO (module 12): Implement OpenAIGateway
//   - Chat(): POST /v1/chat/completions with system + messages
//   - ChatWithTools(): tool_choice support, round loop, onToolCall callback
//   - ChatStream(): SSE streaming, onChunk callback
//   - IsAvailable(): HEAD request to base_url

// TODO (module 13): Implement OllamaEmbedding
//   - Vectorize(): POST /api/embeddings to local Ollama
//   - Default model: bge-small-zh-v1.5 (512 dims)
//   - IsAvailable(): probe localhost:11434
