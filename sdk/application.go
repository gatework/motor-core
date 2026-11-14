package sdk

import "github.com/gatework/motor-core/sdk/runtime"

// Runtime is the default application used by package-level host wiring.
// Applications may instead construct and pass their own *runtime.Application.
var Runtime = runtime.New()
