package dataplane

import (
	"context"
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func runMigrations(lc fx.Lifecycle, log *zap.Logger, db *gorm.DB) {
	if db == nil {
		log.Info("skip data-plane migrations: database not configured")
		return
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := db.AutoMigrate(
				&PluginRecord{},
				&PluginInstall{},
				&Tenant{},
				&PluginTenantBinding{},
				&PluginRun{},
				&AuditLog{},
			); err != nil {
				return fmt.Errorf("apply data-plane migrations: %w", err)
			}
			log.Info("data-plane schema up to date")
			return nil
		},
	})
}
