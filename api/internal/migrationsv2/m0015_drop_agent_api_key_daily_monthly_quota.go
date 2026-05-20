package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0015_drop_agent_api_key_daily_monthly_quota() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2DropAgentAPIKeyQuotaID,
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE IF EXISTS agent_api_keys
					DROP COLUMN IF EXISTS daily_quota,
					DROP COLUMN IF EXISTS monthly_quota,
					DROP COLUMN IF EXISTS daily_usage,
					DROP COLUMN IF EXISTS monthly_usage,
					DROP COLUMN IF EXISTS last_reset_date
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
