package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationAddDigitalHumanTaskVisibilityID = "202610201010000000_add_digital_human_task_visibility"

// DigitalHuman tasks become owned by the DigitalHuman with requester-based
// visibility (ADR 0005). The existing owner columns keep identifying the
// requester, so builds without visibility support keep working; visibility
// stays nullable for rows they create and is derived from owner_kind.
func init() {
	registerSchemaMigration(migrationAddDigitalHumanTaskVisibilityID, upAddDigitalHumanTaskVisibility, downAddDigitalHumanTaskVisibility)
}

func upAddDigitalHumanTaskVisibility(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.automation_tasks
			ADD COLUMN IF NOT EXISTS visibility varchar(16),
			ADD COLUMN IF NOT EXISTS confirmed_by_type varchar(32),
			ADD COLUMN IF NOT EXISTS confirmed_by_ref varchar(255),
			ADD COLUMN IF NOT EXISTS confirmed_at timestamptz;
	`); err != nil {
		return err
	}
	if err := schema.DataFix("derive DigitalHuman task visibility from the existing owner", func(db *gorm.DB) error {
		return db.Exec(`
			UPDATE public.automation_tasks SET visibility = CASE owner_kind
				WHEN 'user_subject' THEN 'requester'
				WHEN 'conversation' THEN 'conversation'
				ELSE 'workspace'
			END
			WHERE task_kind = 'digital_human' AND visibility IS NULL`).Error
	}); err != nil {
		return err
	}
	return schema.Raw(`
		ALTER TABLE public.automation_tasks DROP CONSTRAINT IF EXISTS automation_tasks_visibility_check;
		ALTER TABLE public.automation_tasks ADD CONSTRAINT automation_tasks_visibility_check CHECK (
			(task_kind = 'automation' AND visibility IS NULL AND confirmed_by_type IS NULL AND confirmed_by_ref IS NULL AND confirmed_at IS NULL) OR
			(task_kind = 'digital_human' AND (visibility IS NULL OR visibility IN ('requester', 'conversation', 'workspace', 'restricted')))
		);
		CREATE INDEX IF NOT EXISTS idx_automation_tasks_digital_human_visibility
			ON public.automation_tasks (organization_id, workspace_id, subject_id, visibility, work_status, updated_at DESC)
			WHERE task_kind = 'digital_human';
	`)
}

func downAddDigitalHumanTaskVisibility(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_automation_tasks_digital_human_visibility;
		ALTER TABLE public.automation_tasks
			DROP CONSTRAINT IF EXISTS automation_tasks_visibility_check,
			DROP COLUMN IF EXISTS confirmed_at,
			DROP COLUMN IF EXISTS confirmed_by_ref,
			DROP COLUMN IF EXISTS confirmed_by_type,
			DROP COLUMN IF EXISTS visibility;
	`)
}
