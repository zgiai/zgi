package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0078_remove_account_roles removes the legacy account_roles table
func M0078_remove_account_roles() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601150078",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS "public"."account_roles";`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
