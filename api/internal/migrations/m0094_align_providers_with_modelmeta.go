package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0094_align_providers_with_modelmeta aligns llm_providers table with ModelMeta API schema
// ModelMeta API: https://api.modelmeta.dev/v1/providers
// This migration adds missing fields to match ModelMeta's provider structure
func M0094_align_providers_with_modelmeta() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260127000094",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add ModelMeta standard fields
			if err := tx.Exec(`
				ALTER TABLE llm_providers 
				ADD COLUMN IF NOT EXISTS website VARCHAR(255),
				ADD COLUMN IF NOT EXISTS pricing_url VARCHAR(255),
				ADD COLUMN IF NOT EXISTS tagline VARCHAR(500),
				ADD COLUMN IF NOT EXISTS country_code VARCHAR(10),
				ADD COLUMN IF NOT EXISTS founded_year INTEGER DEFAULT 0
			`).Error; err != nil {
				return err
			}

			// 2. Add field comments for documentation
			comments := []string{
				`COMMENT ON COLUMN llm_providers.website IS 'Provider official website URL (from ModelMeta)'`,
				`COMMENT ON COLUMN llm_providers.pricing_url IS 'Provider pricing page URL (from ModelMeta)'`,
				`COMMENT ON COLUMN llm_providers.tagline IS 'Provider tagline/slogan, supports i18n (from ModelMeta)'`,
				`COMMENT ON COLUMN llm_providers.country_code IS 'Provider country code ISO 3166-1 alpha-2 (from ModelMeta)'`,
				`COMMENT ON COLUMN llm_providers.founded_year IS 'Year the provider company was founded (from ModelMeta)'`,
			}
			for _, comment := range comments {
				if err := tx.Exec(comment).Error; err != nil {
					return err
				}
			}

			// 3. Create index for country_code (for filtering/grouping)
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_llm_providers_country_code 
				ON llm_providers(country_code)
			`).Error; err != nil {
				return err
			}

			// 4. Update existing metadata column comment
			if err := tx.Exec(`
				COMMENT ON COLUMN llm_providers.metadata IS 'JSONB metadata including i18n translations and social links (ModelMeta format)'
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop index first
			if err := tx.Exec(`
				DROP INDEX IF EXISTS idx_llm_providers_country_code
			`).Error; err != nil {
				return err
			}

			// Drop added columns
			if err := tx.Exec(`
				ALTER TABLE llm_providers 
				DROP COLUMN IF EXISTS website,
				DROP COLUMN IF EXISTS pricing_url,
				DROP COLUMN IF EXISTS tagline,
				DROP COLUMN IF EXISTS country_code,
				DROP COLUMN IF EXISTS founded_year
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
