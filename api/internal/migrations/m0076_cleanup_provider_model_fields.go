package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0076_cleanup_provider_model_fields handles the following changes:
// 1. Remove is_system_enabled from llm_models (keep only is_active)
// 2. Remove is_system_enabled from llm_providers (keep only is_active)
// 3. Add is_configured field to llm_models to indicate if model has valid channel configuration
func M0076_cleanup_provider_model_fields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260131000076",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add is_configured column to llm_models
			// This field indicates whether the model has valid channel configuration (credentials, routes, etc.)
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				ADD COLUMN IF NOT EXISTS is_configured BOOLEAN DEFAULT false;
			`).Error; err != nil {
				return err
			}

			// 2. Create index on is_configured for efficient queries
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_llm_models_is_configured 
				ON llm_models(is_configured) WHERE deleted_at IS NULL;
			`).Error; err != nil {
				return err
			}

			// 3. Initialize is_configured based on existing data
			// A model is considered configured if:
			// - It has active system channels, OR
			// - It has active tenant routes
			if err := tx.Exec(`
				UPDATE llm_models m
				SET is_configured = true
				WHERE EXISTS (
					SELECT 1 FROM llm_system_channels sc, jsonb_array_elements_text(sc.models) AS model_name
					WHERE model_name = m.name
					AND sc.is_active = true
					AND sc.deleted_at IS NULL
				)
				OR EXISTS (
					SELECT 1 FROM llm_tenant_routes tr, jsonb_array_elements_text(tr.models) AS model_name
					WHERE model_name = m.name
					AND tr.is_enabled = true
					AND tr.deleted_at IS NULL
				);
			`).Error; err != nil {
				return err
			}

			// 4. Remove is_system_enabled from llm_models
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				DROP COLUMN IF EXISTS is_system_enabled;
			`).Error; err != nil {
				return err
			}

			// 5. Remove is_system_enabled from llm_providers
			if err := tx.Exec(`
				ALTER TABLE llm_providers 
				DROP COLUMN IF EXISTS is_system_enabled;
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Rollback: restore is_system_enabled columns and remove is_configured

			// 1. Restore is_system_enabled to llm_providers
			if err := tx.Exec(`
				ALTER TABLE llm_providers 
				ADD COLUMN IF NOT EXISTS is_system_enabled BOOLEAN DEFAULT true;
			`).Error; err != nil {
				return err
			}

			// 2. Restore is_system_enabled to llm_models
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				ADD COLUMN IF NOT EXISTS is_system_enabled BOOLEAN DEFAULT true;
			`).Error; err != nil {
				return err
			}

			// 3. Remove is_configured from llm_models
			if err := tx.Exec(`
				DROP INDEX IF EXISTS idx_llm_models_is_configured;
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				ALTER TABLE llm_models 
				DROP COLUMN IF EXISTS is_configured;
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
