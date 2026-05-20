package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0020_llm_models_channel_update adds is_official column and backfills data, and adds type column to llm_models
func M0020_llm_models_channel_update() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251127000001",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add is_official column to llm_tenant_channels
			if err := tx.Exec(`ALTER TABLE llm_tenant_channels ADD COLUMN IF NOT EXISTS is_official BOOLEAN NOT NULL DEFAULT false`).Error; err != nil {
				return err
			}

			// 2. Add index for is_official
			if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_tenant_channel_is_official ON llm_tenant_channels(is_official)`).Error; err != nil {
				return err
			}

			// 3. Add type column to llm_models
			if err := tx.Exec(`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS type VARCHAR(50) NOT NULL DEFAULT 'llm'`).Error; err != nil {
				return err
			}

			// 4. Add index for type
			if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_llm_models_type ON llm_models(type)`).Error; err != nil {
				return err
			}

			// 5. Backfill official channels for existing shadow tenants only
			// Create "AGI" official channel for shadow tenants (enterprise groups) that don't have one
			return tx.Exec(`
				INSERT INTO llm_tenant_channels (
					id, tenant_id, name, provider, models, api_base_url, 
					priority, weight, is_official, is_enabled, 
					created_at, updated_at,
					auto_ban, balance, currency,
					model_maps, param_override, header_override, status_code_maps, tags
				)
				SELECT 
					uuid_generate_v4(), eg.id, 'AGI', 'System', '[]'::jsonb, '',
					1, 1, true, true, 
					CURRENT_TIMESTAMP, CURRENT_TIMESTAMP,
					false, 0, 'USD',
					'{}'::jsonb, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb, '[]'::jsonb
				FROM enterprise_groups eg
				WHERE NOT EXISTS (
					SELECT 1 FROM llm_tenant_channels c 
					WHERE c.tenant_id = eg.id AND c.is_official = true
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE llm_tenant_channels DROP COLUMN IF EXISTS is_official`).Error; err != nil {
				return err
			}
			return tx.Exec(`ALTER TABLE llm_models DROP COLUMN IF EXISTS type`).Error
		},
	}
}
