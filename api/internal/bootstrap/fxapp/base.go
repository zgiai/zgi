package fxapp

import (
	"context"

	"github.com/zgiai/zgi/api/config"
	applogger "github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var baseModule = fx.Module("base",
	fx.Provide(
		provideConfig,
		provideLogger,
	),
	fx.Invoke(registerLoggerLifecycle),
)

func provideConfig() (*config.Config, error) {
	if config.GlobalConfig != nil {
		return config.GlobalConfig, nil
	}
	return config.Load()
}

func provideLogger(cfg *config.Config) (*zap.Logger, error) {
	log, err := applogger.New(cfg)
	if err != nil {
		return nil, err
	}

	// Return the zap logger to Fx for DI-managed components, and also
	// set the same instance on the legacy package-level logger bridge.
	applogger.SetLogger(log)
	return log, nil
}

func registerLoggerLifecycle(lc fx.Lifecycle) {
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			applogger.Sync()
			return nil
		},
	})
}
