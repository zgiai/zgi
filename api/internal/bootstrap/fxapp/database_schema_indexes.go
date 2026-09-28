package fxapp

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/datasource/indexing"
	"go.uber.org/fx"
)

func registerDatabaseSchemaIndexLifecycle(lc fx.Lifecycle, service *indexing.Service) {
	var cancel context.CancelFunc
	var done chan struct{}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			var ctx context.Context
			ctx, cancel = context.WithCancel(context.Background())
			done = make(chan struct{})
			go func() { defer close(done); service.Run(ctx) }()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			cancel()
			select {
			case <-done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
}
