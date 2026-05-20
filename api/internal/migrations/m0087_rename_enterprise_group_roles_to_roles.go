package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0087_rename_enterprise_group_roles_to_roles renames enterprise_group_roles to roles
func M0087_rename_enterprise_group_roles_to_roles() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601170087",
		Migrate: func(tx *gorm.DB) error {
			// 0. Check if it's already done
			if tx.Migrator().HasTable("roles") && !tx.Migrator().HasTable("enterprise_group_roles") {
				// Check if 'roles' is already the new table (has group_id)
				if tx.Migrator().HasColumn("roles", "group_id") {
					return nil
				}
			}

			// 1. Handle existing 'roles' table if it's not our target
			if tx.Migrator().HasTable("roles") {
				// If it doesn't have group_id, it's a legacy or conflicting table
				if !tx.Migrator().HasColumn("roles", "group_id") {
					// Rename legacy roles to avoid conflict
					if err := tx.Exec("ALTER TABLE roles RENAME TO legacy_roles").Error; err != nil {
						return fmt.Errorf("failed to rename legacy roles table: %w", err)
					}
				}
			}

			// 2. Rename table
			if tx.Migrator().HasTable("enterprise_group_roles") {
				if err := tx.Exec("ALTER TABLE enterprise_group_roles RENAME TO roles").Error; err != nil {
					return err
				}
			}

			// 3. Rename constraints and indexes (best effort)
			_ = tx.Exec("ALTER INDEX IF EXISTS enterprise_group_roles_pkey RENAME TO roles_pkey").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS idx_eg_roles_group_id RENAME TO idx_roles_group_id").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS idx_eg_roles_status RENAME TO idx_roles_status").Error
			// Rename constraint (PostgreSQL specific syntax)
			_ = tx.Exec("ALTER TABLE roles RENAME CONSTRAINT uk_eg_roles_group_name TO uk_roles_group_name").Error
			// Rename underlying index if it wasn't renamed by constraint rename
			_ = tx.Exec("ALTER INDEX IF EXISTS uk_eg_roles_group_name RENAME TO uk_roles_group_name").Error

			// 4. Create backward compatibility view (if it doesn't exist)
			if !tx.Migrator().HasTable("enterprise_group_roles") {
				if err := tx.Exec("CREATE OR REPLACE VIEW enterprise_group_roles AS SELECT * FROM roles").Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop view
			if err := tx.Exec("DROP VIEW IF EXISTS enterprise_group_roles").Error; err != nil {
				return err
			}

			// Rename table back
			if err := tx.Exec("ALTER TABLE IF EXISTS roles RENAME TO enterprise_group_roles").Error; err != nil {
				return err
			}

			// Rename constraints back
			_ = tx.Exec("ALTER INDEX IF EXISTS roles_pkey RENAME TO enterprise_group_roles_pkey").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS idx_roles_group_id RENAME TO idx_eg_roles_group_id").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS idx_roles_status RENAME TO idx_eg_roles_status").Error
			// Rename constraint back
			_ = tx.Exec("ALTER TABLE enterprise_group_roles RENAME CONSTRAINT uk_roles_group_name TO uk_eg_roles_group_name").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS uk_roles_group_name RENAME TO uk_eg_roles_group_name").Error

			return nil
		},
	}
}
