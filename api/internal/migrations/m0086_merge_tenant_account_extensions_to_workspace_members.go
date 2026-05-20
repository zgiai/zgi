package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0086_merge_tenant_account_extensions_to_workspace_members merges tenant_account_extensions into workspace_members.extensions
func M0086_merge_tenant_account_extensions_to_workspace_members() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601170086",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add extensions column to workspace_members
			if err := tx.Exec("ALTER TABLE workspace_members ADD COLUMN IF NOT EXISTS extensions jsonb DEFAULT '{}'").Error; err != nil {
				return err
			}

			// 2. Migrate data from tenant_account_extensions to workspace_members
			// jsonb_strip_nulls will remove keys with null values (e.g. if position is null)
			// User instruction: permissions are no longer needed, they are handled by Role logic.
			if err := tx.Exec(`
				UPDATE workspace_members wm
				SET extensions = jsonb_strip_nulls(jsonb_build_object(
					'position', tae.position
				))
				FROM tenant_account_extensions tae
				WHERE wm.id = tae.tenant_account_join_id
			`).Error; err != nil {
				return err
			}

			// 3. Drop tenant_account_extensions table
			if err := tx.Exec("DROP TABLE IF EXISTS tenant_account_extensions").Error; err != nil {
				return err
			}

			// 4. Update view tenant_account_joins to include new columns
			if err := tx.Exec("CREATE OR REPLACE VIEW tenant_account_joins AS SELECT * FROM workspace_members").Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 1. Recreate tenant_account_extensions table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS tenant_account_extensions (
					tenant_account_join_id uuid NOT NULL,
					position varchar(100),
					permissions varchar(32)[] DEFAULT '{}'::varchar[],
					created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (tenant_account_join_id),
					CONSTRAINT fk_tenant_account_extensions_join FOREIGN KEY (tenant_account_join_id) REFERENCES workspace_members(id) ON DELETE CASCADE
				)
			`).Error; err != nil {
				return err
			}

			// 2. Restore data from workspace_members.extensions
			if err := tx.Exec(`
				INSERT INTO tenant_account_extensions (tenant_account_join_id, position, permissions)
				SELECT 
					id, 
					extensions->>'position',
					(SELECT array_agg(elem) FROM jsonb_array_elements_text(extensions->'permissions') AS elem)::varchar[]
				FROM workspace_members
				WHERE extensions IS NOT NULL 
				  AND (extensions ? 'position' OR extensions ? 'permissions')
			`).Error; err != nil {
				return err
			}

			// 3. Drop view before dropping column to avoid dependency error
			if err := tx.Exec("DROP VIEW IF EXISTS tenant_account_joins").Error; err != nil {
				return err
			}

			// 4. Drop extensions column from workspace_members
			if err := tx.Exec("ALTER TABLE workspace_members DROP COLUMN IF EXISTS extensions").Error; err != nil {
				return err
			}

			// 5. Recreate view without extensions column
			if err := tx.Exec("CREATE OR REPLACE VIEW tenant_account_joins AS SELECT * FROM workspace_members").Error; err != nil {
				return err
			}

			return nil
		},
	}
}
