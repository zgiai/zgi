package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"gorm.io/gorm"
)

const migrationAddDigitalHumanTaskOwnershipID = "202609101110000000_add_digital_human_task_ownership"

func init() {
	registerSchemaMigration(migrationAddDigitalHumanTaskOwnershipID, upAddDigitalHumanTaskOwnership, downAddDigitalHumanTaskOwnership)
}

func upAddDigitalHumanTaskOwnership(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.automation_tasks
			ADD COLUMN IF NOT EXISTS owner_kind varchar(32),
			ADD COLUMN IF NOT EXISTS owner_subject_kind varchar(32),
			ADD COLUMN IF NOT EXISTS owner_subject_id uuid,
			ADD COLUMN IF NOT EXISTS owner_conversation_id uuid,
			ADD COLUMN IF NOT EXISTS origin_surface varchar(32);
	`); err != nil {
		return err
	}
	if err := schema.DataFix("assign existing DigitalHuman tasks to the shared owner", func(db *gorm.DB) error {
		return db.Exec(
			"UPDATE public.automation_tasks SET owner_kind = ?, origin_surface = ? WHERE task_kind = ?",
			automationmodel.AutomationTaskOwnerDigitalHuman,
			"digital_human_console",
			automationmodel.AutomationTaskKindDigitalHuman,
		).Error
	}); err != nil {
		return err
	}
	return schema.Raw(`
		ALTER TABLE public.automation_tasks
			DROP CONSTRAINT IF EXISTS automation_tasks_owner_check,
			DROP CONSTRAINT IF EXISTS automation_tasks_owner_subject_fk;
		ALTER TABLE public.automation_tasks
			ADD CONSTRAINT automation_tasks_owner_subject_fk
				FOREIGN KEY (owner_subject_id) REFERENCES public.user_subjects(id) ON DELETE RESTRICT,
			ADD CONSTRAINT automation_tasks_owner_check CHECK (
				(task_kind = 'automation' AND owner_kind IS NULL AND owner_subject_kind IS NULL AND owner_subject_id IS NULL AND owner_conversation_id IS NULL AND origin_surface IS NULL) OR
				(task_kind = 'digital_human' AND origin_surface IS NOT NULL AND btrim(origin_surface) <> '' AND (
					(owner_kind = 'digital_human' AND owner_subject_kind IS NULL AND owner_subject_id IS NULL AND owner_conversation_id IS NULL) OR
					(owner_kind = 'user_subject' AND owner_subject_kind IN ('account', 'api_end_user') AND owner_subject_id IS NOT NULL AND owner_conversation_id IS NULL) OR
					(owner_kind = 'conversation' AND owner_subject_kind IS NULL AND owner_subject_id IS NULL AND owner_conversation_id IS NOT NULL)
				))
			);

		DROP INDEX IF EXISTS public.idx_automation_tasks_subject_operation;
		CREATE INDEX IF NOT EXISTS idx_automation_tasks_digital_human_owner_status
			ON public.automation_tasks (organization_id, workspace_id, subject_id, owner_kind, owner_subject_id, owner_conversation_id, work_status, updated_at DESC)
			WHERE task_kind = 'digital_human';
		CREATE UNIQUE INDEX IF NOT EXISTS idx_automation_tasks_shared_operation
			ON public.automation_tasks (organization_id, workspace_id, subject_id, source_operation_id)
			WHERE task_kind = 'digital_human' AND owner_kind = 'digital_human' AND source_operation_id IS NOT NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_automation_tasks_user_operation
			ON public.automation_tasks (organization_id, workspace_id, subject_id, owner_subject_id, source_operation_id)
			WHERE task_kind = 'digital_human' AND owner_kind = 'user_subject' AND source_operation_id IS NOT NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_automation_tasks_conversation_operation
			ON public.automation_tasks (organization_id, workspace_id, subject_id, owner_conversation_id, source_operation_id)
			WHERE task_kind = 'digital_human' AND owner_kind = 'conversation' AND source_operation_id IS NOT NULL;
	`)
}

func downAddDigitalHumanTaskOwnership(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_automation_tasks_conversation_operation;
		DROP INDEX IF EXISTS public.idx_automation_tasks_user_operation;
		DROP INDEX IF EXISTS public.idx_automation_tasks_shared_operation;
		DROP INDEX IF EXISTS public.idx_automation_tasks_digital_human_owner_status;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_automation_tasks_subject_operation
			ON public.automation_tasks (organization_id, workspace_id, subject_id, source_operation_id)
			WHERE source_operation_id IS NOT NULL;
		ALTER TABLE public.automation_tasks
			DROP CONSTRAINT IF EXISTS automation_tasks_owner_check,
			DROP CONSTRAINT IF EXISTS automation_tasks_owner_subject_fk,
			DROP COLUMN IF EXISTS origin_surface,
			DROP COLUMN IF EXISTS owner_conversation_id,
			DROP COLUMN IF EXISTS owner_subject_id,
			DROP COLUMN IF EXISTS owner_subject_kind,
			DROP COLUMN IF EXISTS owner_kind;
	`)
}
