package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0063_upload_files_group_team_tenant adds group/team tenant fields to upload_files
func M0063_upload_files_group_team_tenant() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251225000063",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE upload_files
				ADD COLUMN IF NOT EXISTS group_id UUID,
				ADD COLUMN IF NOT EXISTS team_tenant_id UUID
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS upload_files_group_id_idx ON upload_files(group_id)
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS upload_files_team_tenant_id_idx ON upload_files(team_tenant_id)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE upload_files
				DROP COLUMN IF EXISTS group_id,
				DROP COLUMN IF EXISTS team_tenant_id
			`).Error
		},
	}
}
