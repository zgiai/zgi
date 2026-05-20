package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0146ID = "20260418000146"

// M0146_add_bootstrap_locks creates the lock table used by shared bootstrap.
func M0146_add_bootstrap_locks() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0146ID,
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
