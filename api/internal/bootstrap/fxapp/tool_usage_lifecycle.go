package fxapp

import (
	"context"
	"github.com/zgiai/zgi/api/internal/modules/llm/toolusage"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

func registerToolUsageLifecycle(lc fx.Lifecycle, db *gorm.DB) {
	var cancel context.CancelFunc
	lc.Append(fx.Hook{OnStart: func(context.Context) error {
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		go toolusage.RunExpiryWorker(ctx, db)
		return nil
	}, OnStop: func(context.Context) error {
		if cancel != nil {
			cancel()
		}
		return nil
	}})
}
