package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0099_refactor_file_folder_permissions_ids() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601270096",
		Migrate: func(tx *gorm.DB) error {
			var wsColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'file_folder_permissions' AND column_name = 'workspace_id'").Scan(&wsColCount).Error; err != nil {
				return err
			}
			var tenantColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'file_folder_permissions' AND column_name = 'tenant_id'").Scan(&tenantColCount).Error; err != nil {
				return err
			}
			if tenantColCount > 0 && wsColCount == 0 {
				if err := tx.Exec("ALTER TABLE file_folder_permissions RENAME COLUMN tenant_id TO workspace_id").Error; err != nil {
					return err
				}
			} else if tenantColCount == 0 && wsColCount == 0 {
				if err := tx.Exec("ALTER TABLE file_folder_permissions ADD COLUMN IF NOT EXISTS workspace_id UUID").Error; err != nil {
					return err
				}
			}

			var idxCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'file_folder_permissions' AND indexname = 'file_folder_permission_tenant_idx'").Scan(&idxCount).Error; err != nil {
				return err
			}
			if idxCount > 0 {
				if err := tx.Exec("ALTER INDEX IF EXISTS file_folder_permission_tenant_idx RENAME TO file_folder_permission_workspace_idx").Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			var wsColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'file_folder_permissions' AND column_name = 'workspace_id'").Scan(&wsColCount).Error; err != nil {
				return err
			}
			if wsColCount > 0 {
				_ = tx.Exec("ALTER INDEX IF EXISTS file_folder_permission_workspace_idx RENAME TO file_folder_permission_tenant_idx").Error
				if err := tx.Exec("ALTER TABLE file_folder_permissions RENAME COLUMN workspace_id TO tenant_id").Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
