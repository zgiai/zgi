package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0092_add_workspace_id_to_workspace_members() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601250092",
		Migrate: func(tx *gorm.DB) error {
			// Check if workspace_id column exists in workspace_members table
			var workspaceIdCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'workspace_members' AND column_name = 'workspace_id'").Scan(&workspaceIdCount).Error; err != nil {
				return err
			}

			// Check if tenant_id column exists
			var tenantIdCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'workspace_members' AND column_name = 'tenant_id'").Scan(&tenantIdCount).Error; err != nil {
				return err
			}

			if tenantIdCount > 0 && workspaceIdCount == 0 {
				// If tenant_id column exists and workspace_id column does not exist, perform column rename
				if err := tx.Exec("ALTER TABLE workspace_members RENAME COLUMN tenant_id TO workspace_id").Error; err != nil {
					return err
				}
			} else if tenantIdCount == 0 && workspaceIdCount == 0 {
				// If both tenant_id and workspace_id do not exist, partial execution may have occurred
				// Add workspace_id column (if it doesn't exist yet)
				if err := tx.Exec("ALTER TABLE workspace_members ADD COLUMN IF NOT EXISTS workspace_id UUID").Error; err != nil {
					return err
				}
			}
			// If workspace_id column exists, column renaming is complete, skip this step

			// Check and rename indexes
			// Check if old index exists
			var oldIndexCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'workspace_members' AND indexname = 'idx_workspace_members_tenant_id'").Scan(&oldIndexCount).Error; err != nil {
				return err
			}

			if oldIndexCount > 0 {
				if err := tx.Exec("ALTER INDEX IF EXISTS idx_workspace_members_tenant_id RENAME TO idx_workspace_members_workspace_id").Error; err != nil {
					return err
				}
			}

			var oldUniqueIndexCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'workspace_members' AND indexname = 'uk_workspace_members_tenant_account'").Scan(&oldUniqueIndexCount).Error; err != nil {
				return err
			}

			if oldUniqueIndexCount > 0 {
				if err := tx.Exec("ALTER INDEX IF EXISTS uk_workspace_members_tenant_account RENAME TO uk_workspace_members_workspace_account").Error; err != nil {
					return err
				}
			}

			// Check if view needs to be rebuilt
			var viewExists int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.views WHERE table_name = 'tenant_account_joins'").Scan(&viewExists).Error; err != nil {
				return err
			}

			// Always rebuild view to ensure it uses correct column names
			if err := tx.Exec(`DROP VIEW IF EXISTS tenant_account_joins`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				CREATE VIEW tenant_account_joins AS
				SELECT
					id,
					workspace_id AS tenant_id,
					account_id,
					role,
					role_id,
					current,
					created_at,
					updated_at,
					invited_by,
					extensions,
					workspace_id
				FROM workspace_members
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Check if workspace_id column exists in workspace_members table
			var count int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'workspace_members' AND column_name = 'workspace_id'").Scan(&count).Error; err != nil {
				return err
			}

			if count == 0 {
				// If workspace_id column does not exist, rollback has already been executed, skip
				return nil
			}

			if err := tx.Exec("DROP VIEW IF EXISTS tenant_account_joins").Error; err != nil {
				return err
			}

			if err := tx.Exec("ALTER INDEX IF EXISTS uk_workspace_members_workspace_account RENAME TO uk_workspace_members_tenant_account").Error; err != nil {
				return err
			}
			if err := tx.Exec("ALTER INDEX IF EXISTS idx_workspace_members_workspace_id RENAME TO idx_workspace_members_tenant_id").Error; err != nil {
				return err
			}

			if err := tx.Exec("ALTER TABLE workspace_members RENAME COLUMN workspace_id TO tenant_id").Error; err != nil {
				return err
			}

			// Recreate the original view for rollback
			if err := tx.Exec(`
				CREATE VIEW tenant_account_joins AS
				SELECT
					id,
					tenant_id,
					account_id,
					role,
					role_id,
					current,
					created_at,
					updated_at,
					invited_by,
					extensions
				FROM workspace_members
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
