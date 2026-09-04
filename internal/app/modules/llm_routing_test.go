package modules

import (
	"context"
	"testing"

	"github.com/SHIROH4/sion-plus/internal/adapter/llm"
	"github.com/SHIROH4/sion-plus/internal/port"
)

type routeRecordingRegistry struct {
	lastRoute string
	executor  port.LLMExecutor
}

func (r *routeRecordingRegistry) GetExecutor(taskType string) (port.LLMExecutor, string, error) {
	r.lastRoute = taskType
	return r.executor, "test", nil
}
func (*routeRecordingRegistry) Reload([]port.LLMProviderConfig, port.LLMRoutes) error { return nil }
func (*routeRecordingRegistry) ListHealthy() []string                                 { return []string{"test"} }
func (*routeRecordingRegistry) MarkUnhealthy(string)                                  {}
func (*routeRecordingRegistry) MarkHealthy(string)                                    {}
func (*routeRecordingRegistry) StartHealthCheck(context.Context)                      {}

type routeTestExecutor struct{}

func (routeTestExecutor) Chat(context.Context, string, []port.LLMMessage) (string, error) {
	return "ok", nil
}
func (routeTestExecutor) ChatWithTools(context.Context, string, []port.LLMMessage, []port.ToolDef, func(string, string) string, int, string) (string, error) {
	return "ok", nil
}
func (routeTestExecutor) ChatStream(_ context.Context, _ string, _ []port.LLMMessage, onChunk func(string) error) error {
	return onChunk("ok")
}
func (routeTestExecutor) IsAvailable(context.Context) bool { return true }

func TestRoutedExecutorUsesContextRoute(t *testing.T) {
	registry := &routeRecordingRegistry{executor: routeTestExecutor{}}
	executor := &routedExecutor{
		registry: registry,
		tracker:  llm.NewTokenTracker(t.TempDir()),
		taskType: "chat",
		label:    "chat",
	}
	ctx := port.WithLLMCallMetadata(context.Background(), "memory", "fact_extract")

	if _, err := executor.Chat(ctx, "prompt", nil); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if registry.lastRoute != "memory" {
		t.Fatalf("route=%q, want memory", registry.lastRoute)
	}
}

func TestRoutedExecutorFallsBackToDefaultRoute(t *testing.T) {
	registry := &routeRecordingRegistry{executor: routeTestExecutor{}}
	executor := &routedExecutor{
		registry: registry,
		tracker:  llm.NewTokenTracker(t.TempDir()),
		taskType: "chat",
		label:    "chat",
	}

	if _, err := executor.Chat(context.Background(), "prompt", nil); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if registry.lastRoute != "chat" {
		t.Fatalf("route=%q, want chat", registry.lastRoute)
	}
}
