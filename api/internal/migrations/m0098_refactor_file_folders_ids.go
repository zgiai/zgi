package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0098_refactor_file_folders_ids() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601270095",
		Migrate: func(tx *gorm.DB) error {
			var orgColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'file_folders' AND column_name = 'organization_id'").Scan(&orgColCount).Error; err != nil {
				return err
			}
			var tenantColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'file_folders' AND column_name = 'tenant_id'").Scan(&tenantColCount).Error; err != nil {
				return err
			}

			if tenantColCount > 0 && orgColCount == 0 {
				if err := tx.Exec("ALTER TABLE file_folders RENAME COLUMN tenant_id TO organization_id").Error; err != nil {
					return err
				}
			} else if tenantColCount == 0 && orgColCount == 0 {
				if err := tx.Exec("ALTER TABLE file_folders ADD COLUMN IF NOT EXISTS organization_id UUID").Error; err != nil {
					return err
				}
			}

			var wsColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'file_folders' AND column_name = 'workspace_id'").Scan(&wsColCount).Error; err != nil {
				return err
			}
			var teamTenantColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'file_folders' AND column_name = 'team_tenant_id'").Scan(&teamTenantColCount).Error; err != nil {
				return err
			}

			if teamTenantColCount > 0 && wsColCount == 0 {
				if err := tx.Exec("ALTER TABLE file_folders RENAME COLUMN team_tenant_id TO workspace_id").Error; err != nil {
					return err
				}
			} else if teamTenantColCount == 0 && wsColCount == 0 {
				if err := tx.Exec("ALTER TABLE file_folders ADD COLUMN IF NOT EXISTS workspace_id UUID").Error; err != nil {
					return err
				}
			}

			var idx1Count int64
			if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'file_folders' AND indexname = 'file_folder_tenant_idx'").Scan(&idx1Count).Error; err != nil {
				return err
			}
			if idx1Count > 0 {
				if err := tx.Exec("ALTER INDEX IF EXISTS file_folder_tenant_idx RENAME TO file_folder_organization_idx").Error; err != nil {
					return err
				}
			}

			var idx2Count int64
			if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'file_folders' AND indexname = 'file_folder_team_tenant_idx'").Scan(&idx2Count).Error; err != nil {
				return err
			}
			if idx2Count > 0 {
				if err := tx.Exec("ALTER INDEX IF EXISTS file_folder_team_tenant_idx RENAME TO file_folder_workspace_idx").Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			var orgColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'file_folders' AND column_name = 'organization_id'").Scan(&orgColCount).Error; err != nil {
				return err
			}
			if orgColCount > 0 {
				_ = tx.Exec("ALTER INDEX IF EXISTS file_folder_organization_idx RENAME TO file_folder_tenant_idx").Error
				if err := tx.Exec("ALTER TABLE file_folders RENAME COLUMN organization_id TO tenant_id").Error; err != nil {
					return err
				}
			}

			var wsColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'file_folders' AND column_name = 'workspace_id'").Scan(&wsColCount).Error; err != nil {
				return err
			}
			if wsColCount > 0 {
				_ = tx.Exec("ALTER INDEX IF EXISTS file_folder_workspace_idx RENAME TO file_folder_team_tenant_idx").Error
				if err := tx.Exec("ALTER TABLE file_folders RENAME COLUMN workspace_id TO team_tenant_id").Error; err != nil {
					return err
				}
			}

			return nil
		},
	}
}
