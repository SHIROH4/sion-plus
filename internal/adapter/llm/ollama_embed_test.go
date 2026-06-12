package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SHIROH4/sion-plus/internal/port"
)

func newTestOllamaServer(t *testing.T, handler http.HandlerFunc) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	return srv.URL, srv.Close
}

func TestOllamaVectorize(t *testing.T) {
	url, close := newTestOllamaServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/embeddings" {
			json.NewEncoder(w).Encode(ollamaEmbedResponse{
				Embedding: []float32{0.1, 0.2, 0.3},
			})
			return
		}
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer close()

	emb := NewOllamaEmbedding(url, "test-model", 3, nil)
	vec, err := emb.Vectorize(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Vectorize: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3 dims, got %d", len(vec))
	}
	if vec[0] != 0.1 || vec[2] != 0.3 {
		t.Errorf("unexpected vector: %v", vec)
	}
}

func TestOllamaBatchVectorize(t *testing.T) {
	url, close := newTestOllamaServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/embeddings" {
			var body ollamaEmbedRequest
			json.NewDecoder(r.Body).Decode(&body)
			// Return different embeddings based on prompt length
			json.NewEncoder(w).Encode(ollamaEmbedResponse{
				Embedding: []float32{float32(len(body.Prompt)), 0.5, 0.0},
			})
			return
		}
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer close()

	emb := NewOllamaEmbedding(url, "test", 3, nil)
	vecs, err := emb.BatchVectorize(context.Background(), []string{"a", "bb", "ccc"})
	if err != nil {
		t.Fatalf("BatchVectorize: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("expected 3 vectors, got %d", len(vecs))
	}
	if vecs[0][0] != 1.0 || vecs[2][0] != 3.0 {
		t.Errorf("unexpected batch results: %v", vecs)
	}
}

func TestOllamaDimension(t *testing.T) {
	emb := NewOllamaEmbedding("http://localhost:11434", "bge-small", 512, nil)
	if emb.Dimension() != 512 {
		t.Errorf("Dimension: got %d, want 512", emb.Dimension())
	}
}

func TestOllamaIsAvailable(t *testing.T) {
	url, close := newTestOllamaServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer close()

	emb := NewOllamaEmbedding(url, "test", 3, nil)
	if !emb.IsAvailable() {
		t.Error("server should be available")
	}
}

func TestOllamaIsAvailableDown(t *testing.T) {
	url, close := newTestOllamaServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer close()

	emb := NewOllamaEmbedding(url, "test", 3, nil)
	if emb.IsAvailable() {
		t.Error("server should appear unavailable on 5xx")
	}
}

func TestOllamaFallbackToRemote(t *testing.T) {
	// Local Ollama is down, should fall back to remote
	localURL, localClose := newTestOllamaServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer localClose()

	remoteURL, remoteClose := newTestOllamaServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/embeddings" {
			json.NewEncoder(w).Encode(ollamaEmbedResponse{
				Embedding: []float32{9.9, 8.8, 7.7},
			})
			return
		}
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer remoteClose()

	remoteEmb := NewOllamaEmbedding(remoteURL, "remote-model", 3, nil)
	localEmb := NewOllamaEmbedding(localURL, "local-model", 3, remoteEmb)

	vec, err := localEmb.Vectorize(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Vectorize with fallback: %v", err)
	}
	if vec[0] != 9.9 {
		t.Errorf("expected fallback result, got %v", vec)
	}
}

func TestOllamaErrorWithoutFallback(t *testing.T) {
	url, close := newTestOllamaServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer close()

	emb := NewOllamaEmbedding(url, "test", 3, nil)
	_, err := emb.Vectorize(context.Background(), "hello")
	if err == nil {
		t.Error("expected error when ollama is down and no fallback")
	}
}

func TestDefaultOllamaEmbedConfig(t *testing.T) {
	baseURL, model, dim := DefaultOllamaEmbedConfig()
	if baseURL != "http://localhost:11434" {
		t.Errorf("baseURL: %s", baseURL)
	}
	if model != "bge-small-zh-v1.5" {
		t.Errorf("model: %s", model)
	}
	if dim != 512 {
		t.Errorf("dim: %d", dim)
	}
}

func TestOllamaCompileCheck(t *testing.T) {
	var _ port.EmbeddingService = (*OllamaEmbedding)(nil)
}
