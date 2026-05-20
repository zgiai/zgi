package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0106_sync_document_segments_soft_delete ensures document_segments has correctly typed soft delete columns
var M0106_sync_document_segments_soft_delete = &gormigrate.Migration{
	ID: "m0106_sync_document_segments_soft_delete",
	Migrate: func(tx *gorm.DB) error {
		// 1. Add is_deleted column if not exists
		if err := tx.Exec("ALTER TABLE document_segments ADD COLUMN IF NOT EXISTS is_deleted BOOLEAN DEFAULT false").Error; err != nil {
			return err
		}
		// 2. Add deleted_at column if not exists
		if err := tx.Exec("ALTER TABLE document_segments ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE").Error; err != nil {
			return err
		}
		// 3. Create index for performance
		if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_document_segments_is_deleted ON document_segments (is_deleted) WHERE is_deleted = false").Error; err != nil {
			return err
		}
		return nil
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec("ALTER TABLE document_segments DROP COLUMN IF EXISTS is_deleted").Error
	},
}
