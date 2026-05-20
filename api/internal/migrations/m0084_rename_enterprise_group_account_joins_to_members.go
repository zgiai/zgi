package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0084_rename_enterprise_group_account_joins_to_members renames enterprise_group_account_joins to members
func M0084_rename_enterprise_group_account_joins_to_members() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601170084",
		Migrate: func(tx *gorm.DB) error {
			// 1. Rename table
			if err := tx.Exec("ALTER TABLE IF EXISTS enterprise_group_account_joins RENAME TO members").Error; err != nil {
				return err
			}

			// 2. Rename constraints and indexes (best effort)
			// PK
			_ = tx.Exec("ALTER INDEX IF EXISTS eg_account_pkey RENAME TO members_pkey").Error
			// Indexes
			_ = tx.Exec("ALTER INDEX IF EXISTS eg_account_account_idx RENAME TO members_account_id_idx").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS eg_account_group_idx RENAME TO members_group_id_idx").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS idx_egaj_account_role RENAME TO idx_members_account_role").Error

			// 3. Create backward compatibility view
			if err := tx.Exec("CREATE OR REPLACE VIEW enterprise_group_account_joins AS SELECT * FROM members").Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop view
			if err := tx.Exec("DROP VIEW IF EXISTS enterprise_group_account_joins").Error; err != nil {
				return err
			}

			// Rename table back
			if err := tx.Exec("ALTER TABLE IF EXISTS members RENAME TO enterprise_group_account_joins").Error; err != nil {
				return err
			}

			// Rename constraints back
			_ = tx.Exec("ALTER INDEX IF EXISTS members_pkey RENAME TO eg_account_pkey").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS members_account_id_idx RENAME TO eg_account_account_idx").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS members_group_id_idx RENAME TO eg_account_group_idx").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS idx_members_account_role RENAME TO idx_egaj_account_role").Error

			return nil
		},
	}
}
