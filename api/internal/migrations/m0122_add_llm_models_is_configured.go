package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0122_add_llm_models_is_configured adds the missing is_configured column
// required by llm model sync queries.
func M0122_add_llm_models_is_configured() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260306000122",
		Migrate: func(tx *gorm.DB) error {
			sqls := []string{
				`ALTER TABLE IF EXISTS llm_models
					ADD COLUMN IF NOT EXISTS is_configured BOOLEAN DEFAULT false`,
				`UPDATE llm_models
					SET is_configured = false
					WHERE is_configured IS NULL`,
				`CREATE INDEX IF NOT EXISTS idx_llm_models_is_configured ON llm_models(is_configured)`,
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
