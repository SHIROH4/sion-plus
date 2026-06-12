package sdk

import (
	"context"
	"sync"
)

// BasePlugin provides a standard lifecycle implementation that most plugins
// can embed to avoid boilerplate. Plugins override the hook methods they need.
type BasePlugin struct {
	mu      sync.Mutex
	running bool
	info    PluginInfo
}

// NewBasePlugin creates a BasePlugin with the given metadata.
func NewBasePlugin(info PluginInfo) BasePlugin {
	return BasePlugin{info: info}
}

func (b *BasePlugin) Info() PluginInfo { return b.info }

func (b *BasePlugin) Init(_ context.Context, _ *PluginContext) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return nil
}

func (b *BasePlugin) Start(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running = true
	return nil
}

func (b *BasePlugin) Stop(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running = false
	return nil
}

func (b *BasePlugin) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

// PluginRegistry manages the set of loaded plugins.
type PluginRegistry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
	order   []string // insertion order for deterministic Start/Stop
}

// NewPluginRegistry creates an empty plugin registry.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{plugins: make(map[string]Plugin)}
}

// Register adds a plugin. Returns an error if a plugin with the same name
// is already registered.
func (r *PluginRegistry) Register(p Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := p.Info().Name
	if _, exists := r.plugins[name]; exists {
		return &PluginError{Plugin: name, Op: "register", Err: ErrDuplicatePlugin}
	}
	r.plugins[name] = p
	r.order = append(r.order, name)
	return nil
}

// Get returns a plugin by name, or nil if not found.
func (r *PluginRegistry) Get(name string) Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.plugins[name]
}

// List returns plugin metadata for all registered plugins.
func (r *PluginRegistry) List() []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infos := make([]PluginInfo, 0, len(r.order))
	for _, name := range r.order {
		infos = append(infos, r.plugins[name].Info())
	}
	return infos
}

// InitAll initializes all plugins in registration order, respecting
// dependency order (plugins are sorted so that dependencies come first).
func (r *PluginRegistry) InitAll(ctx context.Context, pctx *PluginContext) error {
	r.mu.RLock()
	ordered := r.topologicalSort()
	r.mu.RUnlock()

	for _, name := range ordered {
		p := r.Get(name)
		if p == nil {
			continue
		}
		if err := p.Init(ctx, pctx); err != nil {
			return &PluginError{Plugin: name, Op: "init", Err: err}
		}
	}
	return nil
}

// StartAll starts all plugins in registration order.
func (r *PluginRegistry) StartAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, name := range r.order {
		p := r.plugins[name]
		if err := p.Start(ctx); err != nil {
			return &PluginError{Plugin: name, Op: "start", Err: err}
		}
	}
	return nil
}

// StopAll stops all plugins in reverse registration order.
func (r *PluginRegistry) StopAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := len(r.order) - 1; i >= 0; i-- {
		name := r.order[i]
		if err := r.plugins[name].Stop(ctx); err != nil {
			return &PluginError{Plugin: name, Op: "stop", Err: err}
		}
	}
	return nil
}

// topologicalSort returns plugin names sorted so that dependencies come
// before the plugins that depend on them. Uses Kahn's algorithm.
func (r *PluginRegistry) topologicalSort() []string {
	inDegree := make(map[string]int)
	graph := make(map[string][]string)

	for name := range r.plugins {
		inDegree[name] = 0
	}
	for name, p := range r.plugins {
		for _, dep := range p.Info().DependsOn {
			if _, ok := r.plugins[dep]; ok {
				graph[dep] = append(graph[dep], name)
				inDegree[name]++
			}
		}
	}

	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}

	var sorted []string
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		sorted = append(sorted, n)
		for _, neighbor := range graph[n] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}
	return sorted
}
