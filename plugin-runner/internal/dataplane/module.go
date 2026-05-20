package dataplane

import "go.uber.org/fx"

// Module wires data-plane dependencies (DB + Redis) into the Fx graph.
var Module = fx.Options(
	fx.Provide(NewConnections),
	fx.Invoke(runMigrations),
)
