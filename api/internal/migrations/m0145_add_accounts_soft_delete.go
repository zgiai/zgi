package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0145ID = "20260418000145"

func M0145_add_accounts_soft_delete() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0145ID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE accounts
				ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ
			`).Error; err != nil {
				return fmt.Errorf("add accounts.deleted_at: %w", err)
			}

			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_accounts_deleted_at
				ON accounts (deleted_at)
			`).Error; err != nil {
				return fmt.Errorf("create accounts deleted_at index: %w", err)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP INDEX IF EXISTS idx_accounts_deleted_at`).Error; err != nil {
				return fmt.Errorf("drop accounts deleted_at index: %w", err)
			}

			if err := tx.Exec(`ALTER TABLE accounts DROP COLUMN IF EXISTS deleted_at`).Error; err != nil {
				return fmt.Errorf("drop accounts.deleted_at: %w", err)
			}

			return nil
		},
	}
}
