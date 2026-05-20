package server

import (
	"context"

	"go.uber.org/fx"
)

// Module wires the HTTP server into Fx.
var Module = fx.Options(
	fx.Provide(New),
	fx.Invoke(registerHooks),
)

func registerHooks(lc fx.Lifecycle, srv *HTTPServer) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return srv.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			return srv.Stop(ctx)
		},
	})
}
