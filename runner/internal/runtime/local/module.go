package local

import (
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/zgiai/zgi/runner/internal/config"
	"github.com/zgiai/zgi/runner/internal/runtime"
)

// Module exposes the local runtime as the default runtime.Runtime implementation.
var Module = fx.Provide(NewRuntime)

// NewRuntime wires Fx dependencies.
func NewRuntime(cfg *config.Config, log *zap.Logger) runtime.Runtime {
	localCfg := Config{
		PythonInterpreter:   cfg.PythonInterpreter,
		NodeInterpreter:     cfg.NodeInterpreter,
		StdoutBufferSize:    cfg.StdoutBufferSize,
		StdoutMaxBufferSize: cfg.StdoutMaxBufferSize,
		ShutdownGracePeriod: cfg.ShutdownTimeout,
		HTTPProxy:           cfg.HTTPProxy,
		HTTPSProxy:          cfg.HTTPSProxy,
		NoProxy:             cfg.NoProxy,
	}
	return New(localCfg, log)
}
