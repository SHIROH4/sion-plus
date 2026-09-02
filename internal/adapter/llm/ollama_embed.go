package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SHIROH4/sion-plus/internal/port"
)

// OllamaEmbedding implements port.EmbeddingService via Ollama's local HTTP API.
// Default model: bge-small-zh-v1.5 (512 dimensions).
// If Ollama is unavailable, falls back to the configured remote embedding provider.
type OllamaEmbedding struct {
	baseURL   string
	model     string
	dimension int
	client    *http.Client
	remote    port.EmbeddingService // fallback when Ollama is down
}

var _ port.EmbeddingService = (*OllamaEmbedding)(nil)

// DefaultOllamaEmbedConfig returns sensible defaults for local Ollama.
func DefaultOllamaEmbedConfig() (baseURL, model string, dimension int) {
	return "http://localhost:11434", "bge-small-zh-v1.5", 512
}

// NewOllamaEmbedding creates an embedding service backed by local Ollama.
// Pass nil for remote to operate without fallback.
func NewOllamaEmbedding(baseURL, model string, dimension int, remote port.EmbeddingService) *OllamaEmbedding {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "bge-small-zh-v1.5"
	}
	if dimension <= 0 {
		dimension = 512
	}
	return &OllamaEmbedding{
		baseURL:   strings.TrimRight(baseURL, "/"),
		model:     model,
		dimension: dimension,
		remote:    remote,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Vectorize computes a single embedding.
func (e *OllamaEmbedding) Vectorize(ctx context.Context, text string) ([]float32, error) {
	vecs, err := e.BatchVectorize(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}
	return vecs[0], nil
}

// BatchVectorize computes embeddings for multiple texts in one request.
func (e *OllamaEmbedding) BatchVectorize(ctx context.Context, texts []string) ([][]float32, error) {
	if !e.IsAvailable() {
		if e.remote != nil {
			return e.remote.BatchVectorize(ctx, texts)
		}
		return nil, fmt.Errorf("ollama unavailable and no remote fallback configured")
	}

	results := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vec, err := e.vectorizeOne(ctx, text)
		if err != nil {
			// Degrade to remote for this single text
			if e.remote != nil {
				fallback, ferr := e.remote.Vectorize(ctx, text)
				if ferr != nil {
					return nil, fmt.Errorf("ollama error (%w) + remote fallback error (%w)", err, ferr)
				}
				results = append(results, fallback)
				continue
			}
			return nil, err
		}
		results = append(results, vec)
	}
	return results, nil
}

func (e *OllamaEmbedding) vectorizeOne(ctx context.Context, text string) ([]float32, error) {
	body := ollamaEmbedRequest{Model: e.model, Prompt: text}
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/api/embeddings", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("ollama returned empty embedding")
	}
	return result.Embedding, nil
}

// Dimension returns the expected output dimension.
func (e *OllamaEmbedding) Dimension() int {
	return e.dimension
}

// IsAvailable probes the Ollama HTTP endpoint.
func (e *OllamaEmbedding) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", e.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// ── JSON types ────────────────────────────────────────────────────

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}
