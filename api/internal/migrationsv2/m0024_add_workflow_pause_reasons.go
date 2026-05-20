package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0024_add_workflow_pause_reasons backfills the pause reason table for databases that ran an early approval runtime migration.
func M0024_add_workflow_pause_reasons() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2WorkflowPauseReasonsID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS public.workflow_run_pause_reasons (
					id UUID PRIMARY KEY,
					pause_id UUID NOT NULL,
					type VARCHAR(64) NOT NULL,
					node_id VARCHAR(255) NOT NULL DEFAULT '',
					form_id VARCHAR(255) NOT NULL DEFAULT '',
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
				);

				CREATE INDEX IF NOT EXISTS idx_workflow_run_pause_reasons_pause_id ON public.workflow_run_pause_reasons(pause_id);
			`).Error; err != nil {
				return fmt.Errorf("create workflow pause reasons table: %w", err)
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP TABLE IF EXISTS public.workflow_run_pause_reasons;`).Error; err != nil {
				return fmt.Errorf("drop workflow pause reasons table: %w", err)
			}
			return nil
		},
	}
}
