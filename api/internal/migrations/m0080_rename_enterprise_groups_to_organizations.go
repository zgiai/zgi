package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0080_rename_enterprise_groups_to_organizations() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601160080",
		Migrate: func(tx *gorm.DB) error {
			// 1. Rename table enterprise_groups -> organizations
			if err := tx.Exec("ALTER TABLE enterprise_groups RENAME TO organizations").Error; err != nil {
				return err
			}

			// 2. Create view enterprise_groups for backward compatibility
			// This allows existing code querying "enterprise_groups" to work against "organizations" table
			if err := tx.Exec("CREATE VIEW enterprise_groups AS SELECT * FROM organizations").Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 1. Drop view
			if err := tx.Exec("DROP VIEW IF EXISTS enterprise_groups").Error; err != nil {
				return err
			}

			// 2. Rename table back to enterprise_groups
			if err := tx.Exec("ALTER TABLE organizations RENAME TO enterprise_groups").Error; err != nil {
				return err
			}

			return nil
		},
	}
}
