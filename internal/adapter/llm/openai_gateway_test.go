package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SHIROH4/sion-plus/internal/port"
)

func newTestGateway(t *testing.T, handler http.HandlerFunc) *OpenAIGateway {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewOpenAIGateway(GatewayConfig{
		BaseURL:    srv.URL,
		APIKey:     "test-key",
		Model:      "test-model",
		Timeout:    5e9,
		MaxRetries: 1,
	})
}

func TestChat(t *testing.T) {
	gw := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []chatChoice{{Message: chatMsg{Content: "Hello, world!"}}},
		})
	})

	ctx := context.Background()
	resp, err := gw.Chat(ctx, "You are helpful", []port.LLMMessage{
		{Role: "user", Content: "Hi"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "Hello, world!" {
		t.Errorf("got %q, want %q", resp, "Hello, world!")
	}
}

func TestChatStream(t *testing.T) {
	gw := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("server does not support flush")
		}
		w.Header().Set("Content-Type", "text/event-stream")

		chunks := []string{"Hello", " world", "!"}
		for _, c := range chunks {
			chunk := chatStreamChunk{}
			chunk.Choices = append(chunk.Choices, struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			}{})
			chunk.Choices[0].Delta.Content = c
			data, _ := json.Marshal(chunk)
			w.Write([]byte("data: " + string(data) + "\n\n"))
			flusher.Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
	})

	ctx := context.Background()
	var parts []string
	err := gw.ChatStream(ctx, "", []port.LLMMessage{{Role: "user", Content: "Hi"}}, func(chunk string) error {
		parts = append(parts, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	got := strings.Join(parts, "")
	if got != "Hello world!" {
		t.Errorf("got %q, want %q", got, "Hello world!")
	}
}

func TestChatWithTools(t *testing.T) {
	callCount := 0
	gw := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		var body chatRequest
		json.NewDecoder(r.Body).Decode(&body)

		callCount++
		if callCount == 1 {
			// First call: return tool call
			json.NewEncoder(w).Encode(chatResponse{
				Choices: []chatChoice{{Message: chatMsg{
					Content: "",
					ToolCalls: []openaiToolCall{{
						ID:   "call_1",
						Type: "function",
						Function: openaiToolFuncCall{
							Name:      "get_weather",
							Arguments: `{"city":"Tokyo"}`,
						},
					}},
				}}},
			})
		} else {
			// Second call: final response
			json.NewEncoder(w).Encode(chatResponse{
				Choices: []chatChoice{{Message: chatMsg{Content: "Tokyo is 22°C"}}},
			})
		}
	})

	ctx := context.Background()
	resp, err := gw.ChatWithTools(ctx, "", []port.LLMMessage{{Role: "user", Content: "weather?"}},
		[]port.ToolDef{{
			Name:        "get_weather",
			Description: "Get weather",
			Parameters:  map[string]any{"type": "object"},
		}},
		func(name, argsJSON string) string {
			return `{"temp": 22}`
		},
		5, "auto",
	)
	if err != nil {
		t.Fatalf("ChatWithTools: %v", err)
	}
	if resp != "Tokyo is 22°C" {
		t.Errorf("got %q, want %q", resp, "Tokyo is 22°C")
	}
}

func TestIsAvailable(t *testing.T) {
	gw := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	if !gw.IsAvailable(context.Background()) {
		t.Error("server should be available")
	}
}

func TestIsAvailableDown(t *testing.T) {
	gw := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if gw.IsAvailable(context.Background()) {
		t.Error("server should appear unavailable (5xx)")
	}
}

func TestRetryOnServerError(t *testing.T) {
	attempts := 0
	gw := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []chatChoice{{Message: chatMsg{Content: "finally!"}}},
		})
	})
	// Override maxRetries for this test
	gw.maxRetries = 3

	ctx := context.Background()
	resp, err := gw.Chat(ctx, "", []port.LLMMessage{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "finally!" {
		t.Errorf("got %q", resp)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestGatewayCompileCheck(t *testing.T) {
	var _ port.LLMExecutor = (*OpenAIGateway)(nil)
}
