package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0082_inline_organization_id() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601160082",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add columns to workspaces table
			if err := tx.Exec(`
				ALTER TABLE workspaces 
				ADD COLUMN IF NOT EXISTS organization_id UUID,
				ADD COLUMN IF NOT EXISTS department_id UUID,
				ADD COLUMN IF NOT EXISTS api_key_id UUID
			`).Error; err != nil {
				return err
			}

			// 2. Backfill data from enterprise_group_tenant_joins
			// Postgres syntax for UPDATE with JOIN
			// Check if table exists before trying to update
			if tx.Migrator().HasTable("enterprise_group_tenant_joins") {
				if err := tx.Exec(`
					UPDATE workspaces
					SET organization_id = CAST(j.group_id AS UUID),
					    department_id = CAST(j.department_id AS UUID),
					    api_key_id = CAST(j.api_key_id AS UUID)
					FROM enterprise_group_tenant_joins j
					WHERE workspaces.id = j.tenant_id
				`).Error; err != nil {
					return err
				}
			}

			// 3. Add index for organization_id
			if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_workspaces_organization_id ON workspaces(organization_id)`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP INDEX IF EXISTS idx_workspaces_organization_id`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				ALTER TABLE workspaces 
				DROP COLUMN IF EXISTS organization_id,
				DROP COLUMN IF EXISTS department_id,
				DROP COLUMN IF EXISTS api_key_id
			`).Error; err != nil {
				return err
			}
			return nil
		},
	}
}
