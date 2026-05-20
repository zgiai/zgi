package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0089_rename_account_context_fields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601170089",
		Migrate: func(tx *gorm.DB) error {
			// Rename current_group_id to current_organization_id
			if tx.Migrator().HasColumn(&struct {
				TableName string `gorm:"table_name:account_contexts"`
			}{}, "current_group_id") {
				if err := tx.Migrator().RenameColumn(&struct {
					TableName string `gorm:"table_name:account_contexts"`
				}{}, "current_group_id", "current_organization_id"); err != nil {
					return err
				}
			} else {
				// If column doesn't exist, try raw SQL check to be sure, maybe gorm struct tag issue
				// But more likely, we should check if it's already renamed or if we need to rename it using raw SQL
				// Let's try raw SQL rename if the column exists in information_schema
				var count int64
				tx.Raw("SELECT count(*) FROM information_schema.columns WHERE table_name = 'account_contexts' AND column_name = 'current_group_id'").Scan(&count)
				if count > 0 {
					if err := tx.Exec("ALTER TABLE account_contexts RENAME COLUMN current_group_id TO current_organization_id").Error; err != nil {
						return err
					}
				}
			}

			// Rename current_team_id to current_workspace_id
			if tx.Migrator().HasColumn(&struct {
				TableName string `gorm:"table_name:account_contexts"`
			}{}, "current_team_id") {
				if err := tx.Migrator().RenameColumn(&struct {
					TableName string `gorm:"table_name:account_contexts"`
				}{}, "current_team_id", "current_workspace_id"); err != nil {
					return err
				}
			} else {
				var count int64
				tx.Raw("SELECT count(*) FROM information_schema.columns WHERE table_name = 'account_contexts' AND column_name = 'current_team_id'").Scan(&count)
				if count > 0 {
					if err := tx.Exec("ALTER TABLE account_contexts RENAME COLUMN current_team_id TO current_workspace_id").Error; err != nil {
						return err
					}
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Rename current_organization_id back to current_group_id
			if tx.Migrator().HasColumn(&struct {
				TableName string `gorm:"table_name:account_contexts"`
			}{}, "current_organization_id") {
				if err := tx.Migrator().RenameColumn(&struct {
					TableName string `gorm:"table_name:account_contexts"`
				}{}, "current_organization_id", "current_group_id"); err != nil {
					return err
				}
			}

			// Rename current_workspace_id back to current_team_id
			if tx.Migrator().HasColumn(&struct {
				TableName string `gorm:"table_name:account_contexts"`
			}{}, "current_workspace_id") {
				if err := tx.Migrator().RenameColumn(&struct {
					TableName string `gorm:"table_name:account_contexts"`
				}{}, "current_workspace_id", "current_team_id"); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
