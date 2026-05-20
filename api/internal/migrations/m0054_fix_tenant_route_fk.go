package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0054_fix_tenant_route_fk fixes the foreign key constraint on user_credential_id
// to reference llm_tenant_credentials instead of llm_credentials
func M0054_fix_tenant_route_fk() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251216000054",
		Migrate: func(tx *gorm.DB) error {
			// Check if llm_tenant_routes table exists
			var exists bool
			err := tx.Raw(`
				SELECT EXISTS (
					SELECT FROM information_schema.tables 
					WHERE table_schema = CURRENT_SCHEMA() 
					AND table_name = 'llm_tenant_routes'
				)
			`).Scan(&exists).Error
			if err != nil {
				return err
			}

			// Skip if table doesn't exist
			if !exists {
				return nil
			}

			// Check if user_credential_id column exists
			var colExists bool
			err = tx.Raw(`
				SELECT EXISTS (
					SELECT FROM information_schema.columns 
					WHERE table_schema = CURRENT_SCHEMA() 
					AND table_name = 'llm_tenant_routes'
					AND column_name = 'user_credential_id'
				)
			`).Scan(&colExists).Error
			if err != nil {
				return err
			}

			// Skip if column doesn't exist
			if !colExists {
				return nil
			}

			sqls := []string{
				// Drop old foreign key constraint (if exists)
				`ALTER TABLE llm_tenant_routes DROP CONSTRAINT IF EXISTS llm_tenant_routes_user_credential_id_fkey`,
				`ALTER TABLE llm_tenant_routes DROP CONSTRAINT IF EXISTS fk_route_credential`,

				// Add new foreign key constraint to llm_tenant_credentials
				`ALTER TABLE llm_tenant_routes ADD CONSTRAINT fk_tenant_route_credential
					FOREIGN KEY (user_credential_id) REFERENCES llm_tenant_credentials(id) ON DELETE SET NULL`,
			}
			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE llm_tenant_routes DROP CONSTRAINT IF EXISTS fk_tenant_route_credential`).Error
		},
	}
}
