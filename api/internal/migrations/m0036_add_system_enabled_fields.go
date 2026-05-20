package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0036_add_system_enabled_fields adds is_system_enabled field to llm_providers and llm_models tables
// This field controls whether a provider/model is available for tenants (system-level on/off shelf control)
// Different from is_active which controls technical availability
func M0036_add_system_enabled_fields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251214000036",
		Migrate: func(tx *gorm.DB) error {
			// Add is_system_enabled to llm_providers
			if err := tx.Exec(`
				ALTER TABLE llm_providers
				ADD COLUMN IF NOT EXISTS is_system_enabled BOOLEAN NOT NULL DEFAULT true
			`).Error; err != nil {
				return err
			}

			// Create index for is_system_enabled on llm_providers
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_llm_providers_system_enabled ON llm_providers(is_system_enabled)
			`).Error; err != nil {
				return err
			}

			// Add is_system_enabled to llm_models
			if err := tx.Exec(`
				ALTER TABLE llm_models
				ADD COLUMN IF NOT EXISTS is_system_enabled BOOLEAN NOT NULL DEFAULT true
			`).Error; err != nil {
				return err
			}

			// Create index for is_system_enabled on llm_models
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_llm_models_system_enabled ON llm_models(is_system_enabled)
			`).Error; err != nil {
				return err
			}

			// Add comment for clarity
			if err := tx.Exec(`
				COMMENT ON COLUMN llm_providers.is_system_enabled IS 'System-level control: whether this provider is available for tenants to use'
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				COMMENT ON COLUMN llm_models.is_system_enabled IS 'System-level control: whether this model is available for tenants to use'
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			sqls := []string{
				`DROP INDEX IF EXISTS idx_llm_providers_system_enabled`,
				`ALTER TABLE llm_providers DROP COLUMN IF EXISTS is_system_enabled`,
				`DROP INDEX IF EXISTS idx_llm_models_system_enabled`,
				`ALTER TABLE llm_models DROP COLUMN IF EXISTS is_system_enabled`,
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
