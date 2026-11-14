package server

import "context"

// Runnable blocks until its work has ended. The service manager owns its context.
type Runnable interface {
	Run(ctx context.Context) error
}

type shutdowner interface {
	Shutdown(ctx context.Context) error
}
