package server

import "context"

// Runnable blocks until its work has ended. The service manager owns its context.
type Runnable interface{ Run(context.Context) error }

type shutdowner interface{ Shutdown(context.Context) error }
