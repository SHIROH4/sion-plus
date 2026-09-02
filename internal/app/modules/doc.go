// Package modules contains the application-layer service orchestrators.
// Each service is a Module (Init/Start/Stop/Health) that composes port interfaces
// into business workflows.

package modules

import "context"

// Module is the standard lifecycle interface for all services.
type Module interface {
	Name() string
	Init(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health(ctx context.Context) error
}
