package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0096_rename_route_types renames RouteType values from SYSTEM_REF/USER_OWNED to ZGI_CLOUD/PRIVATE
// This migration updates existing data in llm_tenant_routes table to use the new naming convention
func M0096_rename_route_types() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260131000096",
		Migrate: func(tx *gorm.DB) error {
			// Step 1: Drop all related CHECK constraints first
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				DROP CONSTRAINT IF EXISTS chk_route_type
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				DROP CONSTRAINT IF EXISTS chk_route_type_new
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				DROP CONSTRAINT IF EXISTS chk_system_ref
			`).Error; err != nil {
				return err
			}

			// Step 2: Update SYSTEM_REF to ZGI_CLOUD
			if err := tx.Exec(`
				UPDATE llm_tenant_routes 
				SET type = 'ZGI_CLOUD' 
				WHERE type = 'SYSTEM_REF'
			`).Error; err != nil {
				return err
			}

			// Step 3: Update USER_OWNED to PRIVATE
			if err := tx.Exec(`
				UPDATE llm_tenant_routes 
				SET type = 'PRIVATE' 
				WHERE type = 'USER_OWNED'
			`).Error; err != nil {
				return err
			}

			// Step 4: Add new CHECK constraint with updated values
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				ADD CONSTRAINT chk_route_type CHECK (type IN ('ZGI_CLOUD', 'PRIVATE'))
			`).Error; err != nil {
				return err
			}

			// Step 5: Add updated chk_system_ref constraint with new type names
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				ADD CONSTRAINT chk_system_ref CHECK (
					(type = 'ZGI_CLOUD' AND system_channel_id IS NOT NULL) OR
					(type = 'PRIVATE' AND user_credential_id IS NOT NULL)
				)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Step 1: Drop the new CHECK constraint
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				DROP CONSTRAINT IF EXISTS chk_route_type
			`).Error; err != nil {
				return err
			}

			// Step 2: Drop the updated chk_system_ref constraint
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				DROP CONSTRAINT IF EXISTS chk_system_ref
			`).Error; err != nil {
				return err
			}

			// Step 3: Rollback: ZGI_CLOUD back to SYSTEM_REF
			if err := tx.Exec(`
				UPDATE llm_tenant_routes 
				SET type = 'SYSTEM_REF' 
				WHERE type = 'ZGI_CLOUD'
			`).Error; err != nil {
				return err
			}

			// Step 4: Rollback: PRIVATE back to USER_OWNED
			if err := tx.Exec(`
				UPDATE llm_tenant_routes 
				SET type = 'USER_OWNED' 
				WHERE type = 'PRIVATE'
			`).Error; err != nil {
				return err
			}

			// Step 5: Restore old CHECK constraint
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				ADD CONSTRAINT chk_route_type CHECK (type IN ('SYSTEM_REF', 'USER_OWNED'))
			`).Error; err != nil {
				return err
			}

			// Step 6: Restore old chk_system_ref constraint
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				ADD CONSTRAINT chk_system_ref CHECK (
					(type = 'SYSTEM_REF' AND system_channel_id IS NOT NULL) OR
					(type = 'USER_OWNED' AND user_credential_id IS NOT NULL)
				)
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
