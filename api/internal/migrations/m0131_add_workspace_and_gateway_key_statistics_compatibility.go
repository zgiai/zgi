package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0131_add_workspace_and_gateway_key_statistics_compatibility() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260309000131",
		Migrate: func(tx *gorm.DB) error {
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
