package fxapp

import (
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

type filteredFXEventLogger struct {
	log *zap.Logger
}

func newFXEventLogger(log *zap.Logger) fxevent.Logger {
	return &filteredFXEventLogger{log: log}
}

func (l *filteredFXEventLogger) LogEvent(event fxevent.Event) {
	switch e := event.(type) {
	case *fxevent.OnStartExecuted:
		if e.Err != nil {
			l.log.Error("fx start hook failed",
				zap.String("hook", e.FunctionName),
				zap.String("caller", e.CallerName),
				zap.Error(e.Err),
				zap.String("log_type", "app"),
			)
		}
	case *fxevent.OnStopExecuted:
		if e.Err != nil {
			l.log.Error("fx stop hook failed",
				zap.String("hook", e.FunctionName),
				zap.String("caller", e.CallerName),
				zap.Error(e.Err),
				zap.String("log_type", "app"),
			)
		}
	case *fxevent.Invoked:
		if e.Err != nil {
			l.log.Error("fx invoke failed",
				zap.String("function", e.FunctionName),
				zap.Error(e.Err),
				zap.String("stack", e.Trace),
				zap.String("log_type", "app"),
			)
		}
	case *fxevent.RollingBack:
		l.log.Error("fx start failed, rolling back",
			zap.Error(e.StartErr),
			zap.String("log_type", "app"),
		)
	case *fxevent.RolledBack:
		if e.Err != nil {
			l.log.Error("fx rollback failed",
				zap.Error(e.Err),
				zap.String("log_type", "app"),
			)
		}
	case *fxevent.Started:
		if e.Err != nil {
			l.log.Error("fx start failed",
				zap.Error(e.Err),
				zap.String("log_type", "app"),
			)
		}
	case *fxevent.Stopped:
		if e.Err != nil {
			l.log.Error("fx stop failed",
				zap.Error(e.Err),
				zap.String("log_type", "app"),
			)
		}
	case *fxevent.LoggerInitialized:
		if e.Err != nil {
			l.log.Error("fx custom logger initialization failed",
				zap.Error(e.Err),
				zap.String("log_type", "app"),
			)
		}
	}
}
