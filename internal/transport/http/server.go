package http

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/SHIROH4/sion-plus/internal/app"
	"github.com/SHIROH4/sion-plus/internal/infra/logbuffer"
	"github.com/SHIROH4/sion-plus/internal/transport/sse"
)

// Server wraps an http.Server with AppRuntime lifecycle integration.
type Server struct {
	srv       *http.Server
	runtime   *app.AppRuntime
	broker    *sse.Broker
	logBuffer *logbuffer.Buffer
}

// NewServer creates an HTTP server bound to addr with the given AppRuntime.
func NewServer(addr string, runtime *app.AppRuntime, broker *sse.Broker, lb *logbuffer.Buffer, frontendDir string) *Server {
	mux := http.NewServeMux()

	h := &handlers{runtime: runtime, broker: broker, logBuffer: lb}
	mux.HandleFunc("POST /api/v1/chat", h.chat)
	mux.HandleFunc("POST /api/v1/chat/stream", h.chatStream)
	mux.HandleFunc("GET /api/v1/chat/history", h.chatHistory)
	mux.HandleFunc("GET /api/v1/health", h.health)
	mux.HandleFunc("GET /api/v1/emotion", h.emotion)
	mux.HandleFunc("GET /api/v1/emotion/history", h.emotionHistory)
	mux.HandleFunc("GET /api/v1/memory/facts", h.memoryFacts)
	mux.HandleFunc("GET /api/v1/memory/topics", h.memoryTopics)
	mux.HandleFunc("GET /api/v1/memory/stats", h.memoryStats)
	mux.HandleFunc("GET /api/v1/tools", h.tools)
	mux.HandleFunc("GET /api/v1/stats", h.stats)
	mux.HandleFunc("GET /api/v1/screen", h.screen)
	mux.HandleFunc("POST /api/v1/proactive/mode", h.proactiveMode)
	mux.HandleFunc("GET /api/v1/proactive/status", h.proactiveStatus)
	mux.HandleFunc("GET /api/v1/proactive/actions", h.proactiveActions)
	mux.HandleFunc("POST /api/v1/proactive/feedback", h.proactiveFeedback)
	mux.HandleFunc("GET /api/v1/proactive/decisions", h.proactiveDecisions)
	mux.HandleFunc("GET /api/v1/proactive/evaluation", h.proactiveEvaluation)
	mux.HandleFunc("GET /api/v1/personality", h.getPersonality)
	mux.HandleFunc("PUT /api/v1/personality", h.savePersonality)
	mux.HandleFunc("GET /api/v1/llm-config", h.getLLMConfig)
	mux.HandleFunc("PUT /api/v1/llm-config", h.saveLLMConfig)
	mux.HandleFunc("GET /api/v1/logs", h.logs)
	mux.HandleFunc("DELETE /api/v1/logs", h.clearLogs)
	mux.HandleFunc("GET /api/events", broker.ServeHTTP)

	if frontendDir != "" {
		fs := http.FileServer(http.Dir(frontendDir))
		mux.Handle("GET /", fs)
	}

	wrapped := withMiddleware(mux)

	return &Server{
		runtime:   runtime,
		broker:    broker,
		logBuffer: lb,
		srv: &http.Server{
			Addr:         addr,
			Handler:      wrapped,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 120 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
	}
}

// ListenAndServe starts the HTTP server. Blocks until stopped.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return err
	}
	log.Printf("[HTTP] listening on http://%s", s.srv.Addr)
	return s.srv.Serve(ln)
}

// Shutdown gracefully stops the server with a timeout.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// Runtime returns the underlying AppRuntime (for testing).
func (s *Server) Runtime() *app.AppRuntime { return s.runtime }
