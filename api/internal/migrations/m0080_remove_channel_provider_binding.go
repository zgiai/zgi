package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0080_remove_channel_provider_binding removes the provider binding constraint from channels
// and migrates to a model-based approach where channels only declare supported models.
//
// Changes:
// 1. Ensure models field exists (JSONB array)
// 2. Migrate data: populate models from provider if empty
// 3. Make provider field nullable (keep for backward compatibility)
// 4. Add GIN indexes for JSONB queries
//
// Rationale:
// - Channel should only care about "what models it supports"
// - Provider is a property of Model, not Channel
// - Supports aggregated services like Together AI, OpenRouter
func M0080_remove_channel_provider_binding() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260131000080",
		Migrate: func(tx *gorm.DB) error {
			// Step 1: Ensure models field exists in both tables
			if err := tx.Exec(`
				ALTER TABLE llm_system_channels 
				ADD COLUMN IF NOT EXISTS models JSONB DEFAULT '[]'
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				ADD COLUMN IF NOT EXISTS models JSONB DEFAULT '[]'
			`).Error; err != nil {
				return err
			}

			// Step 2: Data migration - populate models from provider
			// For system channels
			if err := tx.Exec(`
				UPDATE llm_system_channels sc
				SET models = (
					SELECT COALESCE(jsonb_agg(m.name), '[]'::jsonb)
					FROM llm_models m
					WHERE m.provider = sc.provider
					AND m.is_active = true
					AND m.deleted_at IS NULL
				)
				WHERE (models = '[]' OR models IS NULL)
				AND provider IS NOT NULL
				AND provider != ''
			`).Error; err != nil {
				return err
			}

			// For tenant routes
			if err := tx.Exec(`
				UPDATE llm_tenant_routes tr
				SET models = (
					SELECT COALESCE(jsonb_agg(m.name), '[]'::jsonb)
					FROM llm_models m
					WHERE m.provider = tr.provider
					AND m.is_active = true
					AND m.deleted_at IS NULL
				)
				WHERE (models = '[]' OR models IS NULL)
				AND provider IS NOT NULL
				AND provider != ''
			`).Error; err != nil {
				return err
			}

			// Step 3: Make provider field nullable (keep for backward compatibility)
			if err := tx.Exec(`
				ALTER TABLE llm_system_channels 
				ALTER COLUMN provider DROP NOT NULL
			`).Error; err != nil {
				// Ignore error if constraint doesn't exist
				// This is safe because we're making it more permissive
			}

			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				ALTER COLUMN provider DROP NOT NULL
			`).Error; err != nil {
				// Ignore error if constraint doesn't exist
			}

			// Step 4: Add GIN indexes for JSONB queries (improves performance)
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_system_channels_models 
				ON llm_system_channels USING gin(models)
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_tenant_routes_models 
				ON llm_tenant_routes USING gin(models)
			`).Error; err != nil {
				return err
			}

			// Step 5: Add comments for documentation
			if err := tx.Exec(`
				COMMENT ON COLUMN llm_system_channels.models IS 
				'JSONB array of supported model names. This is the primary field for model support.'
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				COMMENT ON COLUMN llm_system_channels.provider IS 
				'DEPRECATED: Use models field instead. Kept for backward compatibility only.'
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				COMMENT ON COLUMN llm_tenant_routes.models IS 
				'JSONB array of supported model names. This is the primary field for model support.'
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				COMMENT ON COLUMN llm_tenant_routes.provider IS 
				'DEPRECATED: Use models field instead. Kept for backward compatibility only.'
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Rollback: Remove indexes and comments
			// Do NOT remove models field as it may contain data
			
			tx.Exec(`DROP INDEX IF EXISTS idx_system_channels_models`)
			tx.Exec(`DROP INDEX IF EXISTS idx_tenant_routes_models`)
			
			tx.Exec(`COMMENT ON COLUMN llm_system_channels.models IS NULL`)
			tx.Exec(`COMMENT ON COLUMN llm_system_channels.provider IS NULL`)
			tx.Exec(`COMMENT ON COLUMN llm_tenant_routes.models IS NULL`)
			tx.Exec(`COMMENT ON COLUMN llm_tenant_routes.provider IS NULL`)
			
			// Note: We don't restore NOT NULL constraint on provider
			// as that could break existing data
			
			return nil
		},
	}
}
