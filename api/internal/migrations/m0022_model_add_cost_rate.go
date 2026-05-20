package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0022_model_add_cost_rate() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251127000003",
		Migrate: func(tx *gorm.DB) error {
			// Add cost_rate column
			if err := tx.Exec(`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS cost_rate JSONB DEFAULT '{"input":1, "output":1, "image":1, "audio":1}'`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop cost_rate column
			if err := tx.Exec(`ALTER TABLE llm_models DROP COLUMN cost_rate`).Error; err != nil {
				return err
			}
			return nil
		},
	}
}
