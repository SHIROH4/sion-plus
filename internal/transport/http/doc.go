// Package http implements the REST API transport layer.
//
// Design rules:
//   1. Handler files ONLY do: parse request → validate → call service → serialize response
//   2. No business logic in handlers — all logic lives in app/modules
//   3. No direct DB access — all data access goes through port interfaces
//
// Files:
//   router.go           — mux registration, middleware chain
//   middleware.go        — CORS, request logging (method/path/duration), ready check
//   config_handler.go   — GET/POST /api/config
//   chat_handler.go     — POST /api/chat/send, GET /api/chat/history
//   memory_handler.go   — GET /api/memories, DELETE /api/memories/{id}, GET /api/diaries
//   cognition_handler.go — GET /api/features/current, /api/strategies
//   emotion_handler.go  — GET /api/emotion
//   plugin_handler.go   — GET/POST /api/plugins/{name}, PATCH /api/plugins/{name}/toggle
//   identity_handler.go — GET/POST /api/identity, POST /api/identity/self-update
//   stats_handler.go    — GET /api/stats, /api/learning/overview
//   proactive_handler.go — GET /api/proactive/poll
//   token_handler.go    — GET /api/tokens/today
//   health_handler.go   — GET /api/health

package http

// TODO (module 24): Implement HTTP router and all handlers
//   - Use Go 1.22+ ServeMux with method-based routing (mux.HandleFunc("GET /api/config", ...))
//   - Each handler receives its dependencies via constructor (not global variables)
//   - Common response helpers: writeJSON(w, status, v), writeError(w, status, msg)
