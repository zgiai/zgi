package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0005_file_management creates file management related tables
func M0005_file_management() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251110100400",
		Migrate: func(tx *gorm.DB) error {
			// Create file_favorites table
			if err := tx.Exec(`
				CREATE TABLE file_favorites (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					file_id UUID NOT NULL,
					account_id UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create file_folder_joins table
			if err := tx.Exec(`
				CREATE TABLE file_folder_joins (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					file_id UUID NOT NULL,
					folder_id UUID NOT NULL,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create file_folder_permissions table
			if err := tx.Exec(`
				CREATE TABLE file_folder_permissions (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					folder_id UUID NOT NULL,
					tenant_id UUID NOT NULL,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create file_folders table
			if err := tx.Exec(`
				CREATE TABLE file_folders (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					name VARCHAR(255) NOT NULL,
					description TEXT,
					parent_id UUID,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_by UUID,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					icon_type VARCHAR(255),
					icon VARCHAR(255),
					icon_background VARCHAR(255),
					position INTEGER NOT NULL DEFAULT 0,
					permission VARCHAR(255) NOT NULL DEFAULT 'only_me',
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create upload_files table
			if err := tx.Exec(`
				CREATE TABLE upload_files (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					storage_type VARCHAR(255) NOT NULL,
					key VARCHAR(255) NOT NULL,
					name VARCHAR(255) NOT NULL,
					size INTEGER NOT NULL,
					extension VARCHAR(255) NOT NULL,
					mime_type VARCHAR(255),
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					used BOOLEAN NOT NULL DEFAULT false,
					used_by UUID,
					used_at TIMESTAMPTZ,
					hash VARCHAR(255),
					created_by_role VARCHAR(255) NOT NULL DEFAULT 'account',
					source_url TEXT NOT NULL DEFAULT '',
					content_text TEXT,
					is_archived BOOLEAN DEFAULT false,
					archived_at TIMESTAMPTZ,
					archived_by UUID,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create indexes
			indexes := []string{
				`CREATE INDEX file_favorites_file_id_idx ON file_favorites(file_id)`,
				`CREATE INDEX file_favorites_account_id_idx ON file_favorites(account_id)`,
				`CREATE UNIQUE INDEX file_folder_joins_file_id_unique ON file_folder_joins(file_id)`,
				`CREATE INDEX file_folder_assoc_file_idx ON file_folder_joins(file_id)`,
				`CREATE INDEX file_folder_assoc_folder_idx ON file_folder_joins(folder_id)`,
				`CREATE INDEX file_folder_permission_folder_idx ON file_folder_permissions(folder_id)`,
				`CREATE INDEX file_folder_permission_tenant_idx ON file_folder_permissions(tenant_id)`,
				`CREATE INDEX file_folder_parent_idx ON file_folders(parent_id)`,
				`CREATE INDEX file_folder_tenant_idx ON file_folders(tenant_id)`,
				`CREATE INDEX upload_files_tenant_id_idx ON upload_files(tenant_id)`,
				`CREATE INDEX upload_files_created_by_idx ON upload_files(created_by)`,
			}

			for _, indexSQL := range indexes {
				if err := tx.Exec(indexSQL).Error; err != nil {
					return err
				}
			}

			// Create additional indexes for upload_files table
			additionalIndexes := []string{
				`CREATE INDEX idx_upload_files_tenant_archived ON upload_files(tenant_id, is_archived)`,
				`CREATE INDEX idx_upload_files_tenant_archived_created ON upload_files(tenant_id, is_archived, created_at)`,
				`CREATE INDEX upload_file_tenant_idx ON upload_files(tenant_id)`,
			}

			for _, indexSQL := range additionalIndexes {
				if err := tx.Exec(indexSQL).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(
				"file_favorites",
				"file_folder_joins",
				"file_folder_permissions",
				"file_folders",
				"upload_files",
			)
		},
	}
}
