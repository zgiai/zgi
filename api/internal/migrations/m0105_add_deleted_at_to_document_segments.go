package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0105AddDeletedAtToDocumentSegments adds deleted_at column to document_segments table
var M0105AddDeletedAtToDocumentSegments = &gormigrate.Migration{
	ID: "m0105_add_deleted_at_to_document_segments",
	Migrate: func(tx *gorm.DB) error {
		// Add deleted_at column
		if err := tx.Exec("ALTER TABLE document_segments ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE").Error; err != nil {
			return err
		}
		// Add index for soft delete
		if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_document_segments_deleted_at ON document_segments (deleted_at)").Error; err != nil {
			return err
		}
		return nil
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec("ALTER TABLE document_segments DROP COLUMN IF EXISTS deleted_at").Error
	},
}
