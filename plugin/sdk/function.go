package sdk

import "context"

// FunctionProvider is implemented by plugins that want to expose AI-callable
// functions (tools) to the LLM agent loop. Each function is registered into
// the global ToolRegistry and can be invoked by the AI during chat.
type FunctionProvider interface {
	// Functions returns the list of functions this plugin provides.
	Functions() []FunctionDef
}

// FunctionDef describes an AI-callable function registered by a plugin.
type FunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
	Handler     FunctionHandler
}

// FunctionHandler is the callback invoked when the AI calls this function.
// argsJSON is the JSON-encoded function arguments from the LLM.
// Returns the result as a JSON string, or an error.
type FunctionHandler func(ctx context.Context, argsJSON string) (string, error)

// FunctionRegistry collects FunctionProviders and registers their functions
// into a ToolRegistry.
type FunctionRegistry struct {
	providers []FunctionProvider
}

// NewFunctionRegistry creates an empty function registry.
func NewFunctionRegistry() *FunctionRegistry {
	return &FunctionRegistry{}
}

// Add registers a FunctionProvider.
func (r *FunctionRegistry) Add(p FunctionProvider) {
	r.providers = append(r.providers, p)
}

// RegisterAll registers all functions from all providers into the given
// ToolRegistry. Returns the count of functions registered.
func (r *FunctionRegistry) RegisterAll(toolReg ToolRegistry) (int, error) {
	count := 0
	for _, p := range r.providers {
		for _, f := range p.Functions() {
			if err := toolReg.Register(ToolDef{
				Name:        f.Name,
				Description: f.Description,
				Parameters:  f.Parameters,
				Handler:     wrapHandler(f.Handler),
			}); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

// wrapHandler adapts a FunctionHandler to the ToolDef.Handler signature.
func wrapHandler(h FunctionHandler) func(argsJSON string) (string, error) {
	return func(argsJSON string) (string, error) {
		return h(context.Background(), argsJSON)
	}
}
