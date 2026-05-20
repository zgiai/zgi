package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0102_refactor_dataset_child_tables_ids() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601280099",
		Migrate: func(tx *gorm.DB) error {
			// List of tables to migrate
			tables := []struct {
				Name      string
				IndexName string
				NewIndex  string
			}{
				{Name: "documents", IndexName: "documents_tenant_id_idx", NewIndex: "document_organization_id_idx"},
				{Name: "document_segments", IndexName: "document_segments_tenant_id_idx", NewIndex: "document_segment_organization_id_idx"},
				{Name: "document_segment_questions", IndexName: "document_segment_questions_tenant_id_idx", NewIndex: "document_segment_question_organization_id_idx"},
				{Name: "child_chunks", IndexName: "child_chunks_tenant_id_dataset_id_idx", NewIndex: "child_chunk_organization_id_idx"},
				{Name: "batch_hit_testing_tasks", IndexName: "batch_hit_testing_tasks_tenant_id_idx", NewIndex: "idx_batch_hit_testing_tasks_organization_id"},
			}

			for _, table := range tables {
				// 1. Check if tenant_id column exists
				var tenantIdCount int64
				if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = 'tenant_id'", table.Name).Scan(&tenantIdCount).Error; err != nil {
					return err
				}

				// 2. Check if organization_id column exists
				var organizationIdCount int64
				if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = 'organization_id'", table.Name).Scan(&organizationIdCount).Error; err != nil {
					return err
				}

				// 3. Rename tenant_id to organization_id
				if tenantIdCount > 0 && organizationIdCount == 0 {
					if err := tx.Exec(fmt.Sprintf("ALTER TABLE %s RENAME COLUMN tenant_id TO organization_id", table.Name)).Error; err != nil {
						return err
					}
				} else if tenantIdCount == 0 && organizationIdCount == 0 {
					// If neither exists, add organization_id
					if err := tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS organization_id UUID NOT NULL", table.Name)).Error; err != nil {
						return err
					}
				}

				// 4. Handle indexes
				// Rename index if old index exists
				var oldIndexCount int64
				if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = ? AND indexname = ?", table.Name, table.IndexName).Scan(&oldIndexCount).Error; err != nil {
					return err
				}

				if oldIndexCount > 0 {
					if err := tx.Exec(fmt.Sprintf("ALTER INDEX %s RENAME TO %s", table.IndexName, table.NewIndex)).Error; err != nil {
						return err
					}
				} else {
					// If old index doesn't exist, try to create the new one if it doesn't exist
					if err := tx.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (organization_id)", table.NewIndex, table.Name)).Error; err != nil {
						return err
					}
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			tables := []struct {
				Name      string
				IndexName string
				NewIndex  string
			}{
				{Name: "documents", IndexName: "documents_tenant_id_idx", NewIndex: "document_organization_id_idx"},
				{Name: "document_segments", IndexName: "document_segments_tenant_id_idx", NewIndex: "document_segment_organization_id_idx"},
				{Name: "document_segment_questions", IndexName: "document_segment_questions_tenant_id_idx", NewIndex: "document_segment_question_organization_id_idx"},
				{Name: "child_chunks", IndexName: "child_chunks_tenant_id_dataset_id_idx", NewIndex: "child_chunk_organization_id_idx"},
				{Name: "batch_hit_testing_tasks", IndexName: "batch_hit_testing_tasks_tenant_id_idx", NewIndex: "idx_batch_hit_testing_tasks_organization_id"},
			}

			for _, table := range tables {
				// 1. Rename organization_id back to tenant_id
				var count int64
				if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = 'organization_id'", table.Name).Scan(&count).Error; err != nil {
					return err
				}

				if count > 0 {
					if err := tx.Exec(fmt.Sprintf("ALTER TABLE %s RENAME COLUMN organization_id TO tenant_id", table.Name)).Error; err != nil {
						return err
					}
				}

				// 2. Rename index back
				var newIndexCount int64
				if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = ? AND indexname = ?", table.Name, table.NewIndex).Scan(&newIndexCount).Error; err != nil {
					return err
				}

				if newIndexCount > 0 {
					if err := tx.Exec(fmt.Sprintf("ALTER INDEX %s RENAME TO %s", table.NewIndex, table.IndexName)).Error; err != nil {
						return err
					}
				} else {
					// Fallback: drop if exists (cleanup)
					if err := tx.Exec(fmt.Sprintf("DROP INDEX IF EXISTS %s", table.NewIndex)).Error; err != nil {
						return err
					}
				}
			}

			return nil
		},
	}
}
