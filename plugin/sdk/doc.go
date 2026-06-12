// Package sdk defines the plugin development kit for Sion.
//
// All plugins (built-in and third-party) implement the Plugin interface and
// receive a PluginContext with injected port interfaces.
//
// Design rule: plugins NEVER import each other. Cross-plugin communication
// goes exclusively through the EventBus.
//
// Files:
//   plugin.go     — Plugin interface (Info/Init/Start/Stop/IsRunning)
//   context.go    — PluginContext struct (injected port interfaces)
//   lifecycle.go  — BasePlugin, PluginRegistry with dependency-aware InitAll/StartAll/StopAll
//   function.go   — FunctionProvider interface for AI-callable tools
//   ui.go         — UIProvider interface for settings panel components
//   errors.go     — PluginError and sentinel errors

package sdk
