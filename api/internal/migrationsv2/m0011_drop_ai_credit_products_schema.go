package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0011_drop_ai_credit_products_schema() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2DropAICreditProductsID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP TABLE IF EXISTS ai_credit_products CASCADE`).Error; err != nil {
				return err
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
