package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shirohania/sion/internal/port"
)

func newHealthyProvider(t *testing.T, name string) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
			return
		}
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []chatChoice{{Message: chatMsg{Content: "response from " + name}}},
		})
	}))
	return srv.URL, srv.Close
}

func newUnhealthyProvider(t *testing.T) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	return srv.URL, srv.Close
}

func TestReloadAndGetExecutor(t *testing.T) {
	url1, close1 := newHealthyProvider(t, "provider-a")
	defer close1()
	url2, close2 := newHealthyProvider(t, "provider-b")
	defer close2()

	reg := NewProviderRegistry()
	err := reg.Reload([]port.LLMProviderConfig{
		{Name: "local-llm", BaseURL: url1, APIKey: "k1", ChatModel: "llama", Enabled: true, Priority: 1},
		{Name: "cloud-llm", BaseURL: url2, APIKey: "k2", ChatModel: "gpt", Enabled: true, Priority: 2},
	}, port.LLMRoutes{
		Default: "local-llm",
		Chat:    "local-llm",
		Emotion: "cloud-llm",
	})
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}

	// Get default executor
	exec, name, err := reg.GetExecutor("chat")
	if err != nil {
		t.Fatalf("GetExecutor(chat): %v", err)
	}
	if name != "local-llm" {
		t.Errorf("expected local-llm, got %s", name)
	}

	resp, err := exec.Chat(context.Background(), "", []port.LLMMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "response from provider-a" {
		t.Errorf("got %q", resp)
	}

	// Get emotion executor (routed to cloud-llm)
	_, name2, err := reg.GetExecutor("emotion")
	if err != nil {
		t.Fatalf("GetExecutor(emotion): %v", err)
	}
	if name2 != "cloud-llm" {
		t.Errorf("expected cloud-llm for emotion, got %s", name2)
	}
}

func TestGetExecutorFallback(t *testing.T) {
	url1, close1 := newUnhealthyProvider(t)
	defer close1()
	url2, close2 := newHealthyProvider(t, "fallback")
	defer close2()

	reg := NewProviderRegistry()
	reg.Reload([]port.LLMProviderConfig{
		{Name: "primary", BaseURL: url1, APIKey: "k1", ChatModel: "m1", Enabled: true},
		{Name: "fallback", BaseURL: url2, APIKey: "k2", ChatModel: "m2", Enabled: true},
	}, port.LLMRoutes{Default: "primary"})

	// Manually mark primary as unhealthy
	reg.MarkUnhealthy("primary")

	exec, name, err := reg.GetExecutor("chat")
	if err != nil {
		t.Fatalf("GetExecutor with fallback: %v", err)
	}
	if name != "fallback" {
		t.Errorf("expected fallback, got %s", name)
	}

	resp, err := exec.Chat(context.Background(), "", []port.LLMMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "response from fallback" {
		t.Errorf("got %q", resp)
	}
}

func TestGetExecutorNoHealthy(t *testing.T) {
	url1, close1 := newUnhealthyProvider(t)
	defer close1()

	reg := NewProviderRegistry()
	reg.Reload([]port.LLMProviderConfig{
		{Name: "only", BaseURL: url1, APIKey: "k", ChatModel: "m", Enabled: true},
	}, port.LLMRoutes{Default: "only"})

	reg.MarkUnhealthy("only")

	_, _, err := reg.GetExecutor("chat")
	if err == nil {
		t.Error("expected error when no healthy provider")
	}
}

func TestListHealthy(t *testing.T) {
	url1, close1 := newHealthyProvider(t, "a")
	defer close1()
	url2, close2 := newUnhealthyProvider(t)
	defer close2()

	reg := NewProviderRegistry()
	reg.Reload([]port.LLMProviderConfig{
		{Name: "healthy", BaseURL: url1, ChatModel: "m", Enabled: true},
		{Name: "unhealthy", BaseURL: url2, ChatModel: "m", Enabled: true},
	}, port.LLMRoutes{})

	reg.MarkUnhealthy("unhealthy")

	healthy := reg.ListHealthy()
	if len(healthy) != 1 || healthy[0] != "healthy" {
		t.Errorf("ListHealthy: got %v", healthy)
	}
}

func TestReloadClearsOldProviders(t *testing.T) {
	url1, close1 := newHealthyProvider(t, "old")
	defer close1()
	url2, close2 := newHealthyProvider(t, "new")
	defer close2()

	reg := NewProviderRegistry()
	reg.Reload([]port.LLMProviderConfig{
		{Name: "old", BaseURL: url1, ChatModel: "m", Enabled: true},
	}, port.LLMRoutes{Default: "old"})

	// Reload with different set
	reg.Reload([]port.LLMProviderConfig{
		{Name: "new", BaseURL: url2, ChatModel: "m", Enabled: true},
	}, port.LLMRoutes{Default: "new"})

	_, _, err := reg.GetExecutor("chat")
	if err != nil {
		t.Errorf("should find new provider: %v", err)
	}
}

func TestDisabledProvider(t *testing.T) {
	url, close := newHealthyProvider(t, "enabled")
	defer close()

	reg := NewProviderRegistry()
	reg.Reload([]port.LLMProviderConfig{
		{Name: "disabled", BaseURL: "http://invalid", ChatModel: "m", Enabled: false},
		{Name: "enabled", BaseURL: url, ChatModel: "m", Enabled: true},
	}, port.LLMRoutes{Default: "disabled"})

	// Disabled providers aren't in the pool, so GetExecutor for "disabled" fails
	// but falls back to any healthy provider
	_, name, err := reg.GetExecutor("chat")
	if err != nil {
		t.Fatalf("GetExecutor should find fallback: %v", err)
	}
	if name != "enabled" {
		t.Errorf("expected fallback to enabled, got %s", name)
	}
}

func TestMarkHealthyUnhealthy(t *testing.T) {
	url, close := newHealthyProvider(t, "p")
	defer close()

	reg := NewProviderRegistry()
	reg.Reload([]port.LLMProviderConfig{
		{Name: "p", BaseURL: url, ChatModel: "m", Enabled: true},
	}, port.LLMRoutes{Default: "p"})

	reg.MarkUnhealthy("p")
	if len(reg.ListHealthy()) != 0 {
		t.Error("expected 0 healthy after MarkUnhealthy")
	}

	reg.MarkHealthy("p")
	if len(reg.ListHealthy()) != 1 {
		t.Error("expected 1 healthy after MarkHealthy")
	}
}

func TestRegistryCompileCheck(t *testing.T) {
	var _ port.LLMProviderRegistry = (*ProviderRegistry)(nil)
}
