package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0081_rename_tenants_to_workspaces() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601160081",
		Migrate: func(tx *gorm.DB) error {
			// 1. Rename table tenants -> workspaces
			if err := tx.Exec("ALTER TABLE tenants RENAME TO workspaces").Error; err != nil {
				return err
			}

			// 2. Create view tenants for backward compatibility
			// This allows existing code querying "tenants" to work against "workspaces" table
			if err := tx.Exec("CREATE VIEW tenants AS SELECT * FROM workspaces").Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 1. Drop view
			if err := tx.Exec("DROP VIEW IF EXISTS tenants").Error; err != nil {
				return err
			}

			// 2. Rename table back to tenants
			if err := tx.Exec("ALTER TABLE workspaces RENAME TO tenants").Error; err != nil {
				return err
			}

			return nil
		},
	}
}
