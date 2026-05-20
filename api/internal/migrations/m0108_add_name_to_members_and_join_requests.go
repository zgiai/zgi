package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0108_add_name_to_members_and_join_requests() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026020200106",
		Migrate: func(tx *gorm.DB) error {
			// Add name column to members table
			if !tx.Migrator().HasColumn("members", "name") {
				if err := tx.Exec("ALTER TABLE members ADD COLUMN name VARCHAR(255)").Error; err != nil {
					return err
				}
			}

			// Add name column to organization_join_requests table
			if !tx.Migrator().HasColumn("organization_join_requests", "name") {
				if err := tx.Exec("ALTER TABLE organization_join_requests ADD COLUMN name VARCHAR(255)").Error; err != nil {
					return err
				}
			}

			// Backfill data: update members.name from accounts.name where empty
			if err := tx.Exec(`
				UPDATE members 
				SET name = accounts.name 
				FROM accounts 
				WHERE members.account_id = accounts.id 
				AND (members.name IS NULL OR members.name = '')
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop name column from members table
			if tx.Migrator().HasColumn("members", "name") {
				if err := tx.Exec("ALTER TABLE members DROP COLUMN name").Error; err != nil {
					return err
				}
			}

			// Drop name column from organization_join_requests table
			if tx.Migrator().HasColumn("organization_join_requests", "name") {
				if err := tx.Exec("ALTER TABLE organization_join_requests DROP COLUMN name").Error; err != nil {
					return err
				}
			}

			return nil
		},
	}
}
