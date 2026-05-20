package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0033_add_workflow_test_version_scope() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddWorkflowTestVersionScopeID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`ALTER TABLE public.workflow_test_batches
					ADD COLUMN IF NOT EXISTS workflow_version_mode VARCHAR(40) NOT NULL DEFAULT 'draft'`,
				`ALTER TABLE public.workflow_test_batches
					ADD COLUMN IF NOT EXISTS workflow_version_uuid UUID NULL`,
				`ALTER TABLE public.workflow_test_batches
					ADD COLUMN IF NOT EXISTS workflow_version_label VARCHAR(160) NOT NULL DEFAULT 'current_draft'`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("add workflow test version scope fields: %w", err)
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			statements := []string{
				`ALTER TABLE public.workflow_test_batches DROP COLUMN IF EXISTS workflow_version_label`,
				`ALTER TABLE public.workflow_test_batches DROP COLUMN IF EXISTS workflow_version_uuid`,
				`ALTER TABLE public.workflow_test_batches DROP COLUMN IF EXISTS workflow_version_mode`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("rollback workflow test version scope fields: %w", err)
				}
			}
			return nil
		},
	}
}
