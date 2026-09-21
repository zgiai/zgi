package fxapp

import (
	"context"
	"go.uber.org/fx"
)

type connectorOnboardingWorker interface{ Run(context.Context) }

func RegisterConnectorOnboardingLifecycle(lc fx.Lifecycle, worker connectorOnboardingWorker, check func(context.Context) error) {
	var cancel context.CancelFunc
	var done chan struct{}
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) error {
		if err := check(ctx); err != nil {
			return err
		}
		var running context.Context
		running, cancel = context.WithCancel(context.Background())
		done = make(chan struct{})
		go func() { defer close(done); worker.Run(running) }()
		return nil
	}, OnStop: func(ctx context.Context) error {
		if cancel == nil {
			return nil
		}
		cancel()
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}})
}
