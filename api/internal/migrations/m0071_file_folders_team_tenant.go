package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0071_file_folders_team_tenant() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260108000071",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE file_folders
				ADD COLUMN IF NOT EXISTS team_tenant_id UUID
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS file_folders_team_tenant_id_idx ON file_folders(team_tenant_id)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE file_folders
				DROP COLUMN IF EXISTS team_tenant_id
			`).Error
		},
	}
}
