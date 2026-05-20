package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0031_add_workflow_pause_conversation_id() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2WorkflowPauseConversationID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE public.workflow_run_pauses
				ADD COLUMN IF NOT EXISTS conversation_id UUID;

				CREATE INDEX IF NOT EXISTS idx_workflow_run_pauses_conversation_id
				ON public.workflow_run_pauses(conversation_id);

				CREATE INDEX IF NOT EXISTS idx_workflow_run_pauses_active_conversation_reason
				ON public.workflow_run_pauses(conversation_id, app_id, reason)
				WHERE resumed_at IS NULL AND conversation_id IS NOT NULL;
			`).Error; err != nil {
				return fmt.Errorf("add workflow pause conversation id: %w", err)
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DROP INDEX IF EXISTS public.idx_workflow_run_pauses_active_conversation_reason;
				DROP INDEX IF EXISTS public.idx_workflow_run_pauses_conversation_id;
				ALTER TABLE public.workflow_run_pauses DROP COLUMN IF EXISTS conversation_id;
			`).Error; err != nil {
				return fmt.Errorf("drop workflow pause conversation id: %w", err)
			}
			return nil
		},
	}
}
