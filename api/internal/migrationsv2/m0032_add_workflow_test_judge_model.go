package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0032_add_workflow_test_judge_model() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddWorkflowTestJudgeModelID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`ALTER TABLE public.workflow_test_settings
					ADD COLUMN IF NOT EXISTS judge_model_provider VARCHAR(100) NOT NULL DEFAULT ''`,
				`ALTER TABLE public.workflow_test_settings
					ADD COLUMN IF NOT EXISTS judge_model_name VARCHAR(160) NOT NULL DEFAULT ''`,
				`ALTER TABLE public.workflow_test_batches
					ADD COLUMN IF NOT EXISTS judge_model_provider_snapshot VARCHAR(100) NOT NULL DEFAULT ''`,
				`ALTER TABLE public.workflow_test_batches
					ADD COLUMN IF NOT EXISTS judge_model_name_snapshot VARCHAR(160) NOT NULL DEFAULT ''`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("add workflow test judge model fields: %w", err)
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			statements := []string{
				`ALTER TABLE public.workflow_test_batches DROP COLUMN IF EXISTS judge_model_name_snapshot`,
				`ALTER TABLE public.workflow_test_batches DROP COLUMN IF EXISTS judge_model_provider_snapshot`,
				`ALTER TABLE public.workflow_test_settings DROP COLUMN IF EXISTS judge_model_name`,
				`ALTER TABLE public.workflow_test_settings DROP COLUMN IF EXISTS judge_model_provider`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("rollback workflow test judge model fields: %w", err)
				}
			}
			return nil
		},
	}
}
