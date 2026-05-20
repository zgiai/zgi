package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0020_add_account_super_admin adds the persisted system-level admin marker.
func M0020_add_account_super_admin() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddAccountSuperAdminID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE accounts
				ADD COLUMN IF NOT EXISTS is_super_admin BOOLEAN NOT NULL DEFAULT false
			`).Error; err != nil {
				return fmt.Errorf("add accounts.is_super_admin: %w", err)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE accounts
				DROP COLUMN IF EXISTS is_super_admin
			`).Error; err != nil {
				return fmt.Errorf("drop accounts.is_super_admin: %w", err)
			}

			return nil
		},
	}
}
