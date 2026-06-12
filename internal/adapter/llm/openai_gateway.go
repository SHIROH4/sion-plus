package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shirohania/sion/internal/port"
)

// OpenAIGateway implements port.LLMExecutor via OpenAI-compatible HTTP API.
// Uses net/http only — zero external SDK dependencies.
type OpenAIGateway struct {
	baseURL    string
	apiKey     string
	model      string
	client     *http.Client
	maxRetries int
}

var _ port.LLMExecutor = (*OpenAIGateway)(nil)

// GatewayConfig holds the configuration for an OpenAI-compatible provider.
type GatewayConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	Timeout    time.Duration
	MaxRetries int
}

// NewOpenAIGateway creates a gateway for a single provider.
func NewOpenAIGateway(cfg GatewayConfig) *OpenAIGateway {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	return &OpenAIGateway{
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		maxRetries: cfg.MaxRetries,
		client: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
	}
}

// ── Chat ──────────────────────────────────────────────────────────

func (g *OpenAIGateway) Chat(ctx context.Context, systemPrompt string, msgs []port.LLMMessage) (string, error) {
	messages := g.buildMessages(systemPrompt, msgs)
	body := chatRequest{
		Model:    g.model,
		Messages: messages,
		Stream:   false,
	}
	return g.doChat(ctx, body)
}

// ── ChatWithTools ─────────────────────────────────────────────────

func (g *OpenAIGateway) ChatWithTools(
	ctx context.Context,
	systemPrompt string,
	msgs []port.LLMMessage,
	tools []port.ToolDef,
	onToolCall func(name, argsJSON string) string,
	maxRounds int,
	toolChoice string,
) (string, error) {
	if maxRounds <= 0 {
		maxRounds = 5
	}

	messages := g.buildMessages(systemPrompt, msgs)
	openaiTools := make([]openaiTool, len(tools))
	for i, t := range tools {
		openaiTools[i] = openaiTool{
			Type: "function",
			Function: openaiToolFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}

	for round := 0; round < maxRounds; round++ {
		body := chatRequest{
			Model:      g.model,
			Messages:   messages,
			Tools:      openaiTools,
			ToolChoice: toolChoice,
			Stream:     false,
		}

		resp, err := g.doChatRaw(ctx, body)
		if err != nil {
			return "", err
		}

		choice := resp.Choices[0]
		msg := choice.Message

		// If no tool calls, return content
		if len(msg.ToolCalls) == 0 {
			return msg.Content, nil
		}

		// Append assistant message with tool calls
		messages = append(messages, openaiMessage{
			Role:      "assistant",
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
		})

		// Execute each tool call and append results
		for _, tc := range msg.ToolCalls {
			result := onToolCall(tc.Function.Name, tc.Function.Arguments)
			messages = append(messages, openaiMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return "", fmt.Errorf("ChatWithTools: exceeded max rounds (%d)", maxRounds)
}

// ── ChatStream ────────────────────────────────────────────────────

func (g *OpenAIGateway) ChatStream(ctx context.Context, systemPrompt string, msgs []port.LLMMessage, onChunk func(chunk string) error) error {
	messages := g.buildMessages(systemPrompt, msgs)
	body := chatRequest{
		Model:    g.model,
		Messages: messages,
		Stream:   true,
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", g.baseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	g.setHeaders(req)

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("stream request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return g.httpError(resp)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			return nil
		}

		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				if err := onChunk(c.Delta.Content); err != nil {
					return err
				}
			}
		}
	}
	return scanner.Err()
}

// ── IsAvailable ───────────────────────────────────────────────────

func (g *OpenAIGateway) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "HEAD", g.baseURL, nil)
	if err != nil {
		return false
	}
	g.setHeaders(req)

	resp, err := g.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// ── Internal ──────────────────────────────────────────────────────

func (g *OpenAIGateway) doChat(ctx context.Context, body chatRequest) (string, error) {
	resp, err := g.doChatRaw(ctx, body)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return resp.Choices[0].Message.Content, nil
}

func (g *OpenAIGateway) doChatRaw(ctx context.Context, body chatRequest) (*chatResponse, error) {
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < g.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff(attempt))
		}

		req, err := http.NewRequestWithContext(ctx, "POST", g.baseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		g.setHeaders(req)

		resp, err := g.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, g.httpError(resp)
		}

		var cr chatResponse
		if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
			lastErr = fmt.Errorf("decode response: %w", err)
			continue
		}
		return &cr, nil
	}
	return nil, fmt.Errorf("chat failed after %d retries: %w", g.maxRetries, lastErr)
}

func (g *OpenAIGateway) buildMessages(systemPrompt string, msgs []port.LLMMessage) []openaiMessage {
	messages := make([]openaiMessage, 0, len(msgs)+1)
	if systemPrompt != "" {
		messages = append(messages, openaiMessage{Role: "system", Content: systemPrompt})
	}
	for _, m := range msgs {
		if len(m.ContentParts) > 0 {
			parts := make([]openaiContentPart, len(m.ContentParts))
			for i, p := range m.ContentParts {
				parts[i] = openaiContentPart{Type: p.Type, Text: p.Text}
				if p.ImageURL != "" {
					parts[i].ImageURL = &openaiImageURL{URL: p.ImageURL}
				}
			}
			messages = append(messages, openaiMessage{Role: m.Role, Content: parts})
		} else {
			messages = append(messages, openaiMessage{Role: m.Role, Content: m.Content})
		}
	}
	return messages
}

func (g *OpenAIGateway) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if g.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.apiKey)
	}
}

func (g *OpenAIGateway) httpError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
}

// ── OpenAI-compatible JSON types ──────────────────────────────────

type chatRequest struct {
	Model      string          `json:"model"`
	Messages   []openaiMessage `json:"messages"`
	Stream     bool            `json:"stream,omitempty"`
	Tools      []openaiTool    `json:"tools,omitempty"`
	ToolChoice any             `json:"tool_choice,omitempty"`
}

type openaiMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content,omitempty"` // string or []openaiContentPart
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openaiContentPart struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	ImageURL *openaiImageURL `json:"image_url,omitempty"`
}

type openaiImageURL struct {
	URL string `json:"url"`
}

type openaiTool struct {
	Type     string         `json:"type"`
	Function openaiToolFunc `json:"function"`
}

type openaiToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type openaiToolCall struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function openaiToolFuncCall `json:"function"`
}

type openaiToolFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message chatMsg `json:"message"`
}

type chatMsg struct {
	Content   string           `json:"content"`
	ToolCalls []openaiToolCall `json:"tool_calls,omitempty"`
}

type chatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}
