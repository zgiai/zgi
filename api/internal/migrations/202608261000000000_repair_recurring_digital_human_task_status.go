package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migration202608261000000000ID = "202608261000000000_repair_recurring_digital_human_task_status"

const repairRecurringDigitalHumanTaskStatusSQL = `
	WITH repaired AS (
		UPDATE public.automation_tasks
		SET work_status = 'pending',
			revision = revision + 1,
			updated_at = CURRENT_TIMESTAMP
		WHERE task_kind = 'digital_human'
			AND schedule_type = 'cron'
			AND status = 'active'
			AND work_status = 'completed'
		RETURNING id
	)
	INSERT INTO public.automation_task_activities (
		id, task_id, operation_id, actor_type, actor_ref, action, summary, changes, created_at
	)
	SELECT
		gen_random_uuid(), repaired.id, gen_random_uuid(), 'system', 'migration',
		'status_repaired', 'Recurring task restored to active schedule',
		'{"work_status":"pending","reason":"recurring_occurrence_completed"}'::jsonb,
		CURRENT_TIMESTAMP
	FROM repaired;
`

const enforceRecurringDigitalHumanTaskStatusSQL = `
	ALTER TABLE public.automation_tasks
		ADD CONSTRAINT automation_tasks_recurring_completion_check CHECK (
			task_kind <> 'digital_human'
			OR schedule_type <> 'cron'
			OR status <> 'active'
			OR work_status <> 'completed'
		);
`

func init() {
	registerSchemaMigration(migration202608261000000000ID, up202608261000000000, down202608261000000000)
}

func up202608261000000000(schema *mschema.Builder) error {
	if err := schema.DataFix("restore active recurring DigitalHuman tasks after completed occurrences", func(db *gorm.DB) error {
		return db.Exec(repairRecurringDigitalHumanTaskStatusSQL).Error
	}); err != nil {
		return err
	}
	return schema.Raw(enforceRecurringDigitalHumanTaskStatusSQL)
}

func down202608261000000000(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.automation_tasks
			DROP CONSTRAINT IF EXISTS automation_tasks_recurring_completion_check;
	`)
}
