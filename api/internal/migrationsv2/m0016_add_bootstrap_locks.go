package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0016_add_bootstrap_locks() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddBootstrapLocksID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS zgi_bootstrap_locks (
					key VARCHAR(64) PRIMARY KEY,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)
			`).Error; err != nil {
				return fmt.Errorf("create zgi_bootstrap_locks: %w", err)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP TABLE IF EXISTS zgi_bootstrap_locks`).Error; err != nil {
				return fmt.Errorf("drop zgi_bootstrap_locks: %w", err)
			}

			return nil
		},
	}
}
