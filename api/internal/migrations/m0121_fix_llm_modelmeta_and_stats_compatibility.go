package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0121_fix_llm_modelmeta_and_stats_compatibility repairs missing schema pieces
// required by current ModelMeta sync and statistics endpoints.
func M0121_fix_llm_modelmeta_and_stats_compatibility() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260306000121",
		Migrate: func(tx *gorm.DB) error {
			sqls := []string{
				// Align llm_providers with current modelmeta sync payload.
				`ALTER TABLE IF EXISTS llm_providers
					ADD COLUMN IF NOT EXISTS website VARCHAR(255),
					ADD COLUMN IF NOT EXISTS pricing_url VARCHAR(255),
					ADD COLUMN IF NOT EXISTS tagline VARCHAR(500),
					ADD COLUMN IF NOT EXISTS country_code VARCHAR(10),
					ADD COLUMN IF NOT EXISTS founded_year INTEGER DEFAULT 0`,
				`CREATE INDEX IF NOT EXISTS idx_llm_providers_country_code ON llm_providers(country_code)`,

				// Align llm_models with current modelmeta sync payload.
				`ALTER TABLE IF EXISTS llm_models
					ADD COLUMN IF NOT EXISTS family_name VARCHAR(200),
					ADD COLUMN IF NOT EXISTS parent_id UUID,
					ADD COLUMN IF NOT EXISTS family_default BOOLEAN DEFAULT false,
					ADD COLUMN IF NOT EXISTS cached_input_price DECIMAL(10,4),
					ADD COLUMN IF NOT EXISTS videos BOOLEAN DEFAULT false,
					ADD COLUMN IF NOT EXISTS image_edit BOOLEAN DEFAULT false,
					ADD COLUMN IF NOT EXISTS translation BOOLEAN DEFAULT false`,
				`CREATE INDEX IF NOT EXISTS idx_models_parent_id ON llm_models(parent_id)`,
				`UPDATE llm_models SET family_name = family WHERE family_name IS NULL AND family IS NOT NULL`,
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Compatibility migration is intentionally one-way.
			return nil
		},
	}
}
