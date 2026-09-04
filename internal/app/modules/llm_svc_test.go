package modules

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SHIROH4/sion-plus/internal/port"
)

func TestReloadConfigUpdatesExistingExecutor(t *testing.T) {
	newProvider := func(content string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusOK)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]string{"role": "assistant", "content": content},
				}},
			})
		}))
	}

	first := newProvider("first")
	defer first.Close()
	second := newProvider("second")
	defer second.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewLLMService([]port.LLMProviderConfig{{
		Name: "first", BaseURL: first.URL, ChatModel: "test", Enabled: true,
	}}, port.LLMRoutes{Default: "first", Chat: "first"}, t.TempDir())
	if err := service.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer service.Stop(context.Background())

	executor := service.Executor
	response, err := executor.Chat(context.Background(), "", []port.LLMMessage{{Role: "user", Content: "hello"}})
	if err != nil || response != "first" {
		t.Fatalf("before reload response=%q err=%v", response, err)
	}

	if err := service.ReloadConfig([]port.LLMProviderConfig{{
		Name: "second", BaseURL: second.URL, ChatModel: "test", Enabled: true,
	}}, port.LLMRoutes{Default: "second", Chat: "second"}); err != nil {
		t.Fatalf("ReloadConfig: %v", err)
	}

	response, err = executor.Chat(context.Background(), "", []port.LLMMessage{{Role: "user", Content: "hello"}})
	if err != nil || response != "second" {
		t.Fatalf("same executor after reload response=%q err=%v", response, err)
	}
}
