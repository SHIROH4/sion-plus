// Package sdk defines the plugin development kit for Sion.
//
// All plugins (built-in and third-party) implement the Plugin interface and
// receive a PluginContext with injected port interfaces.
package sdk

import "context"

// Plugin is the unified interface every Sion plugin must implement.
// Plugins receive dependencies via PluginContext injection at Init time,
// NOT by importing other plugins. Cross-plugin communication goes
// exclusively through the EventBus.
type Plugin interface {
	// Info returns static metadata — used for discovery, UI listing, and
	// dependency resolution.
	Info() PluginInfo

	// Init is called once after construction. The PluginContext provides
	// injected port interfaces; plugins should store references they need
	// and perform one-time setup (e.g., open connections, load config).
	Init(ctx context.Context, pctx *PluginContext) error

	// Start begins the plugin's main work loop. Called after all plugins
	// have been initialized. Long-running plugins should spawn goroutines
	// with ctx-aware cancellation.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the plugin. Plugins must clean up
	// goroutines, close connections, and persist state within the
	// deadline implied by ctx.
	Stop(ctx context.Context) error

	// IsRunning returns true after Start and before Stop completes.
	IsRunning() bool
}

// PluginInfo is static metadata returned by Plugin.Info().
type PluginInfo struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	DependsOn   []string `json:"depends_on,omitempty"` // plugin names this plugin requires
}
