package sdk

// UIProvider is implemented by plugins that want to expose a settings UI
// component in the frontend. The frontend queries all UIProviders to build
// the settings panel dynamically.
type UIProvider interface {
	// UISchema returns the JSON Schema that describes the plugin's
	// configurable settings. The frontend renders a form from this schema.
	UISchema() UISchema

	// GetConfig returns the current plugin configuration as a JSON object.
	GetConfig() (map[string]any, error)

	// SetConfig applies a configuration update. Only the changed keys
	// are present in the map.
	SetConfig(patch map[string]any) error
}

// UISchema describes a plugin's settings UI to the frontend.
type UISchema struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Properties  []UIProperty      `json:"properties"`
	Layout      []UILayoutSection `json:"layout,omitempty"`
}

// UIProperty is a single configurable field.
type UIProperty struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"` // "string"|"number"|"boolean"|"select"|"slider"
	Default     any    `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`

	// For "select" type
	Options []UIOption `json:"options,omitempty"`

	// For "slider" type
	Min  float64 `json:"min,omitempty"`
	Max  float64 `json:"max,omitempty"`
	Step float64 `json:"step,omitempty"`

	// For "number" type
	Unit string `json:"unit,omitempty"`
}

// UIOption is a single option in a select dropdown.
type UIOption struct {
	Label string `json:"label"`
	Value any    `json:"value"`
}

// UILayoutSection groups properties into visual sections in the settings panel.
type UILayoutSection struct {
	Title string   `json:"title"`
	Keys  []string `json:"keys"`
}
