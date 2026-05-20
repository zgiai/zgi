package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0097_refactor_upload_files_ids() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601270094",
		Migrate: func(tx *gorm.DB) error {
			var orgColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'upload_files' AND column_name = 'organization_id'").Scan(&orgColCount).Error; err != nil {
				return err
			}
			var tenantColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'upload_files' AND column_name = 'tenant_id'").Scan(&tenantColCount).Error; err != nil {
				return err
			}

			if tenantColCount > 0 && orgColCount == 0 {
				if err := tx.Exec("ALTER TABLE upload_files RENAME COLUMN tenant_id TO organization_id").Error; err != nil {
					return err
				}
			} else if tenantColCount == 0 && orgColCount == 0 {
				if err := tx.Exec("ALTER TABLE upload_files ADD COLUMN IF NOT EXISTS organization_id UUID").Error; err != nil {
					return err
				}
			}

			var wsColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'upload_files' AND column_name = 'workspace_id'").Scan(&wsColCount).Error; err != nil {
				return err
			}
			var teamTenantColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'upload_files' AND column_name = 'team_tenant_id'").Scan(&teamTenantColCount).Error; err != nil {
				return err
			}
			if teamTenantColCount > 0 && wsColCount == 0 {
				if err := tx.Exec("ALTER TABLE upload_files RENAME COLUMN team_tenant_id TO workspace_id").Error; err != nil {
					return err
				}
			} else if teamTenantColCount == 0 && wsColCount == 0 {
				if err := tx.Exec("ALTER TABLE upload_files ADD COLUMN IF NOT EXISTS workspace_id UUID").Error; err != nil {
					return err
				}
			}

			var groupColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'upload_files' AND column_name = 'group_id'").Scan(&groupColCount).Error; err != nil {
				return err
			}
			if groupColCount > 0 {
				_ = tx.Exec("DROP INDEX IF EXISTS upload_files_group_id_idx").Error
				if err := tx.Exec("ALTER TABLE upload_files DROP COLUMN IF EXISTS group_id").Error; err != nil {
					return err
				}
			}

			var idx1Count int64
			if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'upload_files' AND indexname = 'upload_files_tenant_id_idx'").Scan(&idx1Count).Error; err != nil {
				return err
			}
			if idx1Count > 0 {
				if err := tx.Exec("ALTER INDEX IF EXISTS upload_files_tenant_id_idx RENAME TO upload_files_organization_id_idx").Error; err != nil {
					return err
				}
			}

			var idx2Count int64
			if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'upload_files' AND indexname = 'idx_upload_files_tenant_archived'").Scan(&idx2Count).Error; err != nil {
				return err
			}
			if idx2Count > 0 {
				if err := tx.Exec("ALTER INDEX IF EXISTS idx_upload_files_tenant_archived RENAME TO idx_upload_files_organization_archived").Error; err != nil {
					return err
				}
			}

			var idx3Count int64
			if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'upload_files' AND indexname = 'idx_upload_files_tenant_archived_created'").Scan(&idx3Count).Error; err != nil {
				return err
			}
			if idx3Count > 0 {
				if err := tx.Exec("ALTER INDEX IF EXISTS idx_upload_files_tenant_archived_created RENAME TO idx_upload_files_organization_archived_created").Error; err != nil {
					return err
				}
			}

			var idx4Count int64
			if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'upload_files' AND indexname = 'upload_file_tenant_idx'").Scan(&idx4Count).Error; err != nil {
				return err
			}
			if idx4Count > 0 {
				if err := tx.Exec("ALTER INDEX IF EXISTS upload_file_tenant_idx RENAME TO upload_file_organization_idx").Error; err != nil {
					return err
				}
			}

			var idx5Count int64
			if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'upload_files' AND indexname = 'upload_files_team_tenant_id_idx'").Scan(&idx5Count).Error; err != nil {
				return err
			}
			if idx5Count > 0 {
				if err := tx.Exec("ALTER INDEX IF EXISTS upload_files_team_tenant_id_idx RENAME TO upload_files_workspace_id_idx").Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			var orgColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'upload_files' AND column_name = 'organization_id'").Scan(&orgColCount).Error; err != nil {
				return err
			}
			if orgColCount > 0 {
				_ = tx.Exec("ALTER INDEX IF EXISTS upload_files_organization_id_idx RENAME TO upload_files_tenant_id_idx").Error
				_ = tx.Exec("ALTER INDEX IF EXISTS idx_upload_files_organization_archived RENAME TO idx_upload_files_tenant_archived").Error
				_ = tx.Exec("ALTER INDEX IF EXISTS idx_upload_files_organization_archived_created RENAME TO idx_upload_files_tenant_archived_created").Error
				_ = tx.Exec("ALTER INDEX IF EXISTS upload_file_organization_idx RENAME TO upload_file_tenant_idx").Error
				if err := tx.Exec("ALTER TABLE upload_files RENAME COLUMN organization_id TO tenant_id").Error; err != nil {
					return err
				}
			}

			var wsColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'upload_files' AND column_name = 'workspace_id'").Scan(&wsColCount).Error; err != nil {
				return err
			}
			if wsColCount > 0 {
				_ = tx.Exec("ALTER INDEX IF EXISTS upload_files_workspace_id_idx RENAME TO upload_files_team_tenant_id_idx").Error
				if err := tx.Exec("ALTER TABLE upload_files RENAME COLUMN workspace_id TO team_tenant_id").Error; err != nil {
					return err
				}
			}

			var groupColCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'upload_files' AND column_name = 'group_id'").Scan(&groupColCount).Error; err != nil {
				return err
			}
			if groupColCount == 0 {
				if err := tx.Exec("ALTER TABLE upload_files ADD COLUMN IF NOT EXISTS group_id UUID").Error; err != nil {
					return err
				}
				_ = tx.Exec("CREATE INDEX IF NOT EXISTS upload_files_group_id_idx ON upload_files(group_id)").Error
			}

			return nil
		},
	}
}
