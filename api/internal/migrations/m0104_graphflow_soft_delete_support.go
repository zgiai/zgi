package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0104_graphflow_soft_delete_support adds is_deleted and deleted_at columns to graphflow tables
func M0104_graphflow_soft_delete_support() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260125232400", // Current timestamp
		Migrate: func(tx *gorm.DB) error {
			// Helper function to check if table exists
			tableExists := func(tableName string) bool {
				var count int64
				tx.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_name = ?", tableName).Scan(&count)
				return count > 0
			}

			// 1. document_segments
			if tableExists("document_segments") {
				if err := tx.Exec(`
					ALTER TABLE document_segments
					ADD COLUMN IF NOT EXISTS is_deleted BOOLEAN DEFAULT false,
					ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
				`).Error; err != nil {
					return err
				}

				if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_document_segments_deleted ON document_segments(is_deleted) WHERE is_deleted = false;`).Error; err != nil {
					return err
				}
			}

			// 2. kb_entity_mentions
			if tableExists("kb_entity_mentions") {
				if err := tx.Exec(`
					ALTER TABLE kb_entity_mentions
					ADD COLUMN IF NOT EXISTS is_deleted BOOLEAN DEFAULT false,
					ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
				`).Error; err != nil {
					return err
				}

				if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_entity_mentions_deleted ON kb_entity_mentions(is_deleted) WHERE is_deleted = false;`).Error; err != nil {
					return err
				}
			}

			// 3. kb_triple_mentions
			if tableExists("kb_triple_mentions") {
				if err := tx.Exec(`
					ALTER TABLE kb_triple_mentions
					ADD COLUMN IF NOT EXISTS is_deleted BOOLEAN DEFAULT false,
					ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
				`).Error; err != nil {
					return err
				}

				if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_triple_mentions_deleted ON kb_triple_mentions(is_deleted) WHERE is_deleted = false;`).Error; err != nil {
					return err
				}
			}

			// 4. kb_relationships
			if tableExists("kb_relationships") {
				if err := tx.Exec(`
					ALTER TABLE kb_relationships
					ADD COLUMN IF NOT EXISTS is_deleted BOOLEAN DEFAULT false,
					ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
				`).Error; err != nil {
					return err
				}

				if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_relationships_deleted ON kb_relationships(is_deleted) WHERE is_deleted = false;`).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			tables := []string{
				"document_segments",
				"kb_entity_mentions",
				"kb_triple_mentions",
				"kb_relationships",
			}

			for _, table := range tables {
				if err := tx.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS is_deleted", table)).Error; err != nil {
					return err
				}
				if err := tx.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS deleted_at", table)).Error; err != nil {
					return err
				}
			}

			return nil
		},
	}
}
