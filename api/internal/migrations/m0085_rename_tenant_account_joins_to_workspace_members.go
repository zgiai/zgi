package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0085_rename_tenant_account_joins_to_workspace_members renames tenant_account_joins to workspace_members and adds role binding
func M0085_rename_tenant_account_joins_to_workspace_members() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601170085",
		Migrate: func(tx *gorm.DB) error {
			// 1. Rename table
			if err := tx.Exec("ALTER TABLE IF EXISTS tenant_account_joins RENAME TO workspace_members").Error; err != nil {
				return err
			}

			// 2. Rename constraints and indexes (best effort)
			// PK
			_ = tx.Exec("ALTER INDEX IF EXISTS tenant_account_join_pkey RENAME TO workspace_members_pkey").Error
			// Indexes
			_ = tx.Exec("ALTER INDEX IF EXISTS idx_tenant_account_joins_account_current RENAME TO idx_workspace_members_account_current").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS tenant_account_join_account_id_idx RENAME TO idx_workspace_members_account_id").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS tenant_account_join_tenant_id_idx RENAME TO idx_workspace_members_tenant_id").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS unique_tenant_account_join RENAME TO uk_workspace_members_tenant_account").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS idx_tenant_account_role_id RENAME TO idx_workspace_members_role_id").Error

			// 3. Add Foreign Key for role_id (Skipped: enterprise_group_roles refactoring pending)
			// Note: role_id column was added in m0058
			// FK creation deferred until enterprise_group_roles is stable

			// 4. Create backward compatibility view
			if err := tx.Exec("CREATE OR REPLACE VIEW tenant_account_joins AS SELECT * FROM workspace_members").Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop view
			if err := tx.Exec("DROP VIEW IF EXISTS tenant_account_joins").Error; err != nil {
				return err
			}

			// Drop FK (Skipped: creation deferred)
			// _ = tx.Exec("ALTER TABLE workspace_members DROP CONSTRAINT IF EXISTS fk_workspace_members_role").Error

			// Rename table back
			if err := tx.Exec("ALTER TABLE IF EXISTS workspace_members RENAME TO tenant_account_joins").Error; err != nil {
				return err
			}

			// Rename constraints back
			_ = tx.Exec("ALTER INDEX IF EXISTS workspace_members_pkey RENAME TO tenant_account_join_pkey").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS idx_workspace_members_account_current RENAME TO idx_tenant_account_joins_account_current").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS idx_workspace_members_account_id RENAME TO tenant_account_join_account_id_idx").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS idx_workspace_members_tenant_id RENAME TO tenant_account_join_tenant_id_idx").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS uk_workspace_members_tenant_account RENAME TO unique_tenant_account_join").Error
			_ = tx.Exec("ALTER INDEX IF EXISTS idx_workspace_members_role_id RENAME TO idx_tenant_account_role_id").Error

			return nil
		},
	}
}
