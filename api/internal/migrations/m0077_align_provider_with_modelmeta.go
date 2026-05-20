package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0077_align_provider_with_modelmeta aligns llm_providers table with ModelMeta API standard
// This migration ensures the database structure matches https://api.modelmeta.dev/v1/providers
func M0077_align_provider_with_modelmeta() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260131000077",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add 'object' field (ModelMeta standard)
			// This field is a type identifier, always set to "provider"
			if err := tx.Exec(`
				ALTER TABLE llm_providers 
				ADD COLUMN IF NOT EXISTS object VARCHAR(20) DEFAULT 'provider';
			`).Error; err != nil {
				return err
			}

			// 2. Update existing records to set object = 'provider'
			if err := tx.Exec(`
				UPDATE llm_providers 
				SET object = 'provider' 
				WHERE object IS NULL OR object = '';
			`).Error; err != nil {
				return err
			}

			// 3. Create index on object field for better query performance
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_llm_providers_object 
				ON llm_providers(object);
			`).Error; err != nil {
				return err
			}

			// Note: Field name alignment is handled via JSON tags in the model
			// Database column names remain unchanged for backward compatibility:
			// - name (DB) → provider (JSON)
			// - display_name (DB) → provider_name (JSON)
			// - documentation_url (DB) → api_docs_url (JSON)

			// 4. Clear PostgreSQL cached query plans to prevent "cached plan must not change result type" error
			// This is necessary after adding new columns to ensure existing prepared statements are invalidated
			if err := tx.Exec(`DISCARD PLANS;`).Error; err != nil {
				// DISCARD PLANS failure is not critical, log but continue
				// The error will be resolved on next connection/server restart
				return nil
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Remove the object field and its index
			if err := tx.Exec(`
				DROP INDEX IF EXISTS idx_llm_providers_object;
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				ALTER TABLE llm_providers DROP COLUMN IF EXISTS object;
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
