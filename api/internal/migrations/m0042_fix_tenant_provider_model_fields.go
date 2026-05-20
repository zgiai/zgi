package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0042_fix_tenant_provider_model_fields adds missing columns to llm_tenant_providers and llm_tenant_models
func M0042_fix_tenant_provider_model_fields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251214000042",
		Migrate: func(tx *gorm.DB) error {
			// 1. Fix llm_tenant_providers
			// Add is_active if not exists
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_providers
				ADD COLUMN IF NOT EXISTS is_active BOOLEAN DEFAULT true
			`).Error; err != nil {
				return err
			}

			// Add sort_order if not exists
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_providers
				ADD COLUMN IF NOT EXISTS sort_order INT DEFAULT 0
			`).Error; err != nil {
				return err
			}

			// Add metadata if not exists
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_providers
				ADD COLUMN IF NOT EXISTS metadata JSONB DEFAULT '{}'
			`).Error; err != nil {
				return err
			}

			// Add api_base_url if not exists (it might be missing in some versions)
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_providers
				ADD COLUMN IF NOT EXISTS api_base_url VARCHAR(255)
			`).Error; err != nil {
				return err
			}

			// Add display_name if not exists
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_providers
				ADD COLUMN IF NOT EXISTS display_name VARCHAR(100)
			`).Error; err != nil {
				return err
			}

			// Add name if not exists (schema might have used provider before)
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_providers
				ADD COLUMN IF NOT EXISTS name VARCHAR(50)
			`).Error; err != nil {
				return err
			}

			// Add logo_url, documentation_url, description, protocol
			cols := []string{
				"logo_url VARCHAR(255)",
				"documentation_url VARCHAR(255)",
				"description TEXT",
				"protocol VARCHAR(50) DEFAULT 'openai'",
			}
			for _, col := range cols {
				if err := tx.Exec(`
					ALTER TABLE llm_tenant_providers
					ADD COLUMN IF NOT EXISTS ` + col).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// We generally don't drop columns in rollback to avoid data loss during dev
			return nil
		},
	}
}
