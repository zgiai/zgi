package logger

import (
	"context"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Module wires the logger into the Fx graph and flushes it during shutdown.
var Module = fx.Options(
	fx.Provide(New),
	fx.Invoke(register),
)

// New builds a production zap logger.
func New() (*zap.Logger, error) {
	return zap.NewProduction()
}

func register(lc fx.Lifecycle, log *zap.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return log.Sync()
		},
	})
}
