package config

import "go.uber.org/fx"

// Module exposes the config loader to Fx.
var Module = fx.Provide(Load)
