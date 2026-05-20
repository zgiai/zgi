package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0034_add_workflow_test_expected_result() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddWorkflowTestExpectedResultID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE public.workflow_test_cases
				ADD COLUMN IF NOT EXISTS expected_result TEXT NOT NULL DEFAULT ''`).Error; err != nil {
				return fmt.Errorf("add workflow test expected result: %w", err)
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE public.workflow_test_cases
				DROP COLUMN IF EXISTS expected_result`).Error; err != nil {
				return fmt.Errorf("rollback workflow test expected result: %w", err)
			}
			return nil
		},
	}
}
