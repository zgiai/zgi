package registry

import "go.uber.org/fx"

// Module wires the registry into Fx.
var Module = fx.Provide(New)
