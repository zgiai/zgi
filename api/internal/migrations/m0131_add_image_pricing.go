package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0131_add_image_pricing() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260310000131",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS image_prices JSONB DEFAULT '[]'::jsonb;
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE llm_models DROP COLUMN IF EXISTS image_prices;
			`).Error
		},
	}
}
