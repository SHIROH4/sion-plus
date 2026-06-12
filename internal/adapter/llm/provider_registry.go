package llm

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/SHIROH4/sion-plus/internal/port"
)

// ProviderRegistry implements port.LLMProviderRegistry.
// Manages multiple providers with task-type routing and fallback chains.
// Health checking runs in a background goroutine.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]*providerEntry // keyed by name
	routes    port.LLMRoutes
}

type providerEntry struct {
	config  port.LLMProviderConfig
	gateway *OpenAIGateway
	healthy bool
}

var _ port.LLMProviderRegistry = (*ProviderRegistry)(nil)

// NewProviderRegistry creates an empty registry. Call Reload() to populate.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]*providerEntry),
	}
}

// ── Routing ───────────────────────────────────────────────────────

// GetExecutor returns the LLMExecutor for a task type, with fallback.
// taskType maps to LLMRoutes fields: "chat", "emotion", "memory", etc.
// If the preferred provider is unhealthy, falls back to the first healthy provider.
func (r *ProviderRegistry) GetExecutor(taskType string) (port.LLMExecutor, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Resolve task type → provider name
	name := r.resolveRoute(taskType)
	if name == "" {
		// No route configured, try any healthy provider
		for n, e := range r.providers {
			if e.healthy {
				return e.gateway, n, nil
			}
		}
		return nil, "", fmt.Errorf("no healthy provider available for task %q", taskType)
	}

	entry, ok := r.providers[name]
	if !ok {
		// Provider not in pool (disabled / removed): fall back to first healthy
		for n, e := range r.providers {
			if e.healthy {
				return e.gateway, n, nil
			}
		}
		return nil, "", fmt.Errorf("provider %q not found for task %q", name, taskType)
	}

	if entry.healthy {
		return entry.gateway, name, nil
	}

	// Fallback: find first healthy alternative
	for n, e := range r.providers {
		if e.healthy && n != name {
			return e.gateway, n, nil
		}
	}
	return nil, "", fmt.Errorf("provider %q is unhealthy and no fallback available", name)
}

func (r *ProviderRegistry) resolveRoute(taskType string) string {
	switch taskType {
	case "chat":
		if r.routes.Chat != "" {
			return r.routes.Chat
		}
	case "emotion":
		if r.routes.Emotion != "" {
			return r.routes.Emotion
		}
	case "memory":
		if r.routes.Memory != "" {
			return r.routes.Memory
		}
	case "vision":
		if r.routes.Vision != "" {
			return r.routes.Vision
		}
	case "summary":
		if r.routes.Summary != "" {
			return r.routes.Summary
		}
	case "signal":
		if r.routes.Signal != "" {
			return r.routes.Signal
		}
	case "search":
		if r.routes.Search != "" {
			return r.routes.Search
		}
	}
	return r.routes.Default
}

// ── Reload ────────────────────────────────────────────────────────

func (r *ProviderRegistry) Reload(configs []port.LLMProviderConfig, routes port.LLMRoutes) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	newProviders := make(map[string]*providerEntry)
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		gw := NewOpenAIGateway(GatewayConfig{
			BaseURL:    cfg.BaseURL,
			APIKey:     cfg.APIKey,
			Model:      cfg.ChatModel,
			MaxRetries: cfg.MaxRetries,
		})
		if cfg.TimeoutSec > 0 {
			gw.client.Timeout = time.Duration(cfg.TimeoutSec) * time.Second
		}
		newProviders[cfg.Name] = &providerEntry{
			config:  cfg,
			gateway: gw,
			healthy: true, // optimistic: will be verified by health check
		}
	}

	r.providers = newProviders
	r.routes = routes
	log.Printf("[ProviderRegistry] Reload: loaded %d providers", len(newProviders))
	return nil
}

// ── Health ────────────────────────────────────────────────────────

func (r *ProviderRegistry) ListHealthy() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var names []string
	for name, e := range r.providers {
		if e.healthy {
			names = append(names, name)
		}
	}
	log.Printf("[ProviderRegistry] ListHealthy: %d total, %d healthy", len(r.providers), len(names))
	return names
}

func (r *ProviderRegistry) MarkUnhealthy(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.providers[name]; ok {
		if e.healthy {
			log.Printf("[ProviderRegistry] %s → unhealthy", name)
		}
		e.healthy = false
	}
}

func (r *ProviderRegistry) MarkHealthy(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.providers[name]; ok {
		if !e.healthy {
			log.Printf("[ProviderRegistry] %s → healthy", name)
		}
		e.healthy = true
	}
}

// ── Health Check Loop ──────────────────────────────────────────────

// StartHealthCheck launches a background goroutine that probes each provider.
// Probe interval: 30s.
//   - 3 consecutive failures → mark unhealthy
//   - 2 consecutive successes → mark healthy
func (r *ProviderRegistry) StartHealthCheck(ctx context.Context) {
	go r.healthCheckLoop(ctx)
}

func (r *ProviderRegistry) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Track consecutive results per provider
	failures := make(map[string]int)
	successes := make(map[string]int)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.RLock()
			snapshot := make([]struct {
				name    string
				gateway *OpenAIGateway
				healthy bool
			}, 0, len(r.providers))
			for name, e := range r.providers {
				snapshot = append(snapshot, struct {
					name    string
					gateway *OpenAIGateway
					healthy bool
				}{name, e.gateway, e.healthy})
			}
			r.mu.RUnlock()

			for _, p := range snapshot {
				probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				available := p.gateway.IsAvailable(probeCtx)
				cancel()

				if available {
					failures[p.name] = 0
					successes[p.name]++
					if successes[p.name] >= 2 && !p.healthy {
						r.MarkHealthy(p.name)
						successes[p.name] = 0
					}
				} else {
					successes[p.name] = 0
					failures[p.name]++
					if failures[p.name] >= 3 && p.healthy {
						r.MarkUnhealthy(p.name)
						failures[p.name] = 0
					}
				}
			}
		}
	}
}
