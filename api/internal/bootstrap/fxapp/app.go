package fxapp

import (
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

var Module = fx.Options(
	fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
		return newFXEventLogger(log)
	}),
	baseModule,
	infraModule,
	legacyGlobalsModule,
	legacyContainerModule,
	taskRuntimeModule,
	schedulerModule,
	graphFlowModule,
	workflowModule,
	runtimeModule,
)

// NewApp builds the server application.
func NewApp() *fx.App {
	return fx.New(Module)
}
