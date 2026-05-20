package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0100_refactor_datasets_ids() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601280097",
		Migrate: func(tx *gorm.DB) error {
			// 1. Rename tenant_id to workspace_id
			// Check if workspace_id column exists
			var workspaceIdCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'datasets' AND column_name = 'workspace_id'").Scan(&workspaceIdCount).Error; err != nil {
				return err
			}

			// Check if tenant_id column exists
			var tenantIdCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'datasets' AND column_name = 'tenant_id'").Scan(&tenantIdCount).Error; err != nil {
				return err
			}

			if tenantIdCount > 0 && workspaceIdCount == 0 {
				if err := tx.Exec("ALTER TABLE datasets RENAME COLUMN tenant_id TO workspace_id").Error; err != nil {
					return err
				}
			} else if tenantIdCount == 0 && workspaceIdCount == 0 {
				// If both don't exist, create workspace_id
				if err := tx.Exec("ALTER TABLE datasets ADD COLUMN IF NOT EXISTS workspace_id UUID").Error; err != nil {
					return err
				}
			}

			// Ensure workspace_id is nullable (DROP NOT NULL)
			if err := tx.Exec("ALTER TABLE datasets ALTER COLUMN workspace_id DROP NOT NULL").Error; err != nil {
				return err
			}

			// 2. Add organization_id column
			if err := tx.Exec("ALTER TABLE datasets ADD COLUMN IF NOT EXISTS organization_id UUID").Error; err != nil {
				return err
			}

			// Add index for organization_id
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS dataset_organization_id_idx ON datasets (organization_id)").Error; err != nil {
				return err
			}

			// 3. Data Migration
			// Case A: workspace_id is actually an Organization ID (mixed data)
			// Move to organization_id and set workspace_id to NULL
			// We cast to text/varchar to match organizations.id type
			if err := tx.Exec(`
				UPDATE datasets 
				SET organization_id = CAST(workspace_id AS UUID), 
					workspace_id = NULL 
				WHERE CAST(workspace_id AS UUID) IN (SELECT id FROM organizations)
			`).Error; err != nil {
				return err
			}

			// Case B: workspace_id is a Workspace ID
			// Populate organization_id from the workspaces table
			if err := tx.Exec(`
				UPDATE datasets 
				SET organization_id = (
					SELECT organization_id 
					FROM workspaces 
					WHERE workspaces.id = CAST(datasets.workspace_id AS UUID)
				)
				WHERE workspace_id IS NOT NULL AND organization_id IS NULL
			`).Error; err != nil {
				return err
			}

			// 4. Rename index
			var oldIndexCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'datasets' AND indexname = 'dataset_tenant_idx'").Scan(&oldIndexCount).Error; err != nil {
				return err
			}

			if oldIndexCount > 0 {
				if err := tx.Exec("ALTER INDEX IF EXISTS dataset_tenant_idx RENAME TO dataset_workspace_id_idx").Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 1. Rename index back
			if err := tx.Exec("ALTER INDEX IF EXISTS dataset_workspace_id_idx RENAME TO dataset_tenant_idx").Error; err != nil {
				return err
			}

			// 2. Restore data (Reverse Migration)
			// If workspace_id is NULL, it implies it was an Organization ID. Move organization_id back to workspace_id.
			if err := tx.Exec(`
				UPDATE datasets
				SET workspace_id = organization_id
				WHERE workspace_id IS NULL AND organization_id IS NOT NULL
			`).Error; err != nil {
				return err
			}

			// 3. Drop organization_id
			if err := tx.Exec("ALTER TABLE datasets DROP COLUMN IF EXISTS organization_id").Error; err != nil {
				return err
			}

			// 4. Rename workspace_id back to tenant_id
			var count int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'datasets' AND column_name = 'workspace_id'").Scan(&count).Error; err != nil {
				return err
			}

			if count > 0 {
				if err := tx.Exec("ALTER TABLE datasets RENAME COLUMN workspace_id TO tenant_id").Error; err != nil {
					return err
				}
			}

			return nil
		},
	}
}
