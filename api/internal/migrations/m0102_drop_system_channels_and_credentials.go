package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0102_drop_system_channels_and_credentials removes system channels and credentials tables
// These have been migrated to Console-API and are no longer used in ZGI-API
func M0102_drop_system_channels_and_credentials() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260203000102",
		Migrate: func(tx *gorm.DB) error {
			// Step 1: Drop foreign key constraint from llm_routes
			if err := tx.Exec(`
				ALTER TABLE llm_routes 
				DROP CONSTRAINT IF EXISTS llm_tenant_routes_system_channel_id_fkey;
			`).Error; err != nil {
				return err
			}

			// Step 2: Drop system_channel_id column from llm_routes
			if err := tx.Exec(`
				ALTER TABLE llm_routes 
				DROP COLUMN IF EXISTS system_channel_id;
			`).Error; err != nil {
				return err
			}

			// Step 3: Drop llm_system_channels table
			if err := tx.Exec(`
				DROP TABLE IF EXISTS llm_system_channels CASCADE;
			`).Error; err != nil {
				return err
			}

			// Step 4: Drop llm_system_credentials table
			if err := tx.Exec(`
				DROP TABLE IF EXISTS llm_system_credentials CASCADE;
			`).Error; err != nil {
				return err
			}

			// Step 5: Update check constraint to remove system_channel_id reference
			if err := tx.Exec(`
				ALTER TABLE llm_routes 
				DROP CONSTRAINT IF EXISTS chk_system_ref;
			`).Error; err != nil {
				return err
			}

			// Step 6: Add new simplified check constraint
			if err := tx.Exec(`
				ALTER TABLE llm_routes 
				ADD CONSTRAINT chk_route_ref CHECK (
					(type = 'ZGI_CLOUD' AND is_official = true) OR
					(type = 'PRIVATE' AND user_credential_id IS NOT NULL)
				);
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Note: This rollback recreates the tables but without data
			// In production, you should backup data before running this migration

			// Step 1: Recreate llm_system_credentials table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS llm_system_credentials (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					name VARCHAR(100) NOT NULL,
					provider VARCHAR(50),
					protocol VARCHAR(50),
					api_key_ciphertext TEXT NOT NULL,
					api_base_url VARCHAR(500),
					is_active BOOLEAN DEFAULT true,
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					deleted_at TIMESTAMPTZ
				);
			`).Error; err != nil {
				return err
			}

			// Step 2: Recreate llm_system_channels table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS llm_system_channels (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					credential_id UUID NOT NULL REFERENCES llm_system_credentials(id) ON DELETE CASCADE,
					name VARCHAR(100) NOT NULL,
					provider VARCHAR(50),
					protocol VARCHAR(50),
					models JSONB DEFAULT '[]',
					api_base_url VARCHAR(500),
					default_priority INTEGER DEFAULT 10,
					default_weight INTEGER DEFAULT 50,
					is_active BOOLEAN DEFAULT true,
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					deleted_at TIMESTAMPTZ
				);
			`).Error; err != nil {
				return err
			}

			// Step 3: Add system_channel_id column back to llm_routes
			if err := tx.Exec(`
				ALTER TABLE llm_routes 
				ADD COLUMN IF NOT EXISTS system_channel_id UUID;
			`).Error; err != nil {
				return err
			}

			// Step 4: Add foreign key constraint
			if err := tx.Exec(`
				ALTER TABLE llm_routes 
				ADD CONSTRAINT llm_tenant_routes_system_channel_id_fkey 
				FOREIGN KEY (system_channel_id) REFERENCES llm_system_channels(id) ON DELETE CASCADE;
			`).Error; err != nil {
				return err
			}

			// Step 5: Restore old check constraint
			if err := tx.Exec(`
				ALTER TABLE llm_routes 
				DROP CONSTRAINT IF EXISTS chk_route_ref;
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				ALTER TABLE llm_routes 
				ADD CONSTRAINT chk_system_ref CHECK (
					(type = 'ZGI_CLOUD' AND (system_channel_id IS NOT NULL OR is_official = true)) OR
					(type = 'PRIVATE' AND user_credential_id IS NOT NULL)
				);
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
