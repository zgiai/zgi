package manager

import (
	"context"

	"go.uber.org/fx"
)

// Module exposes the manager to Fx.
var Module = fx.Options(
	fx.Provide(New),
	fx.Invoke(registerHooks),
)

func registerHooks(lc fx.Lifecycle, mgr *Manager) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return mgr.Close(ctx)
		},
	})
}
