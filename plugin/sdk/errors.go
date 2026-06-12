package sdk

import (
	"errors"
	"fmt"
)

var (
	ErrDuplicatePlugin = errors.New("plugin already registered")
	ErrPluginNotFound  = errors.New("plugin not found")
	ErrNotRunning      = errors.New("plugin is not running")
)

// PluginError wraps an error with plugin name and operation context.
type PluginError struct {
	Plugin string
	Op     string // "register"|"init"|"start"|"stop"
	Err    error
}

func (e *PluginError) Error() string {
	return fmt.Sprintf("plugin %s %s: %v", e.Plugin, e.Op, e.Err)
}

func (e *PluginError) Unwrap() error { return e.Err }
