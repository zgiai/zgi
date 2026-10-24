package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// Task reports no longer all wait for review: a task says which do, and the
// rest close as they arrive. Reports already waiting are closed the same way
// under the default, which waits only for a report that did not fully finish,
// and once a newer report of their task exists.
func init() {
	registerSchemaMigration("202610241600000000_task_report_review_modes", upTaskReportReviewModes, downTaskReportReviewModes)
}

func upTaskReportReviewModes(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.digital_human_task_reports DROP CONSTRAINT IF EXISTS digital_human_task_reports_review_check;
		ALTER TABLE public.digital_human_task_reports ADD CONSTRAINT digital_human_task_reports_review_check
			CHECK (review_state IN ('new', 'accepted', 'rework_requested', 'closed'));
	`); err != nil {
		return err
	}
	return schema.DataFix("close task reports that need no review", func(db *gorm.DB) error {
		return db.Exec(`
		UPDATE public.digital_human_task_reports AS reports
		SET review_state = 'closed', reviewed_by_type = 'system', updated_at = now(), reviewed_at = now(),
			reviewed_by_ref = CASE WHEN reports.outcome = 'done' THEN 'not_required' ELSE 'superseded' END
		WHERE reports.final AND reports.review_state = 'new' AND (
			reports.outcome = 'done' OR EXISTS (
				SELECT 1 FROM public.digital_human_task_reports AS newer
				WHERE newer.task_id = reports.task_id AND newer.final AND newer.created_at > reports.created_at
			)
		)`).Error
	})
}

func downTaskReportReviewModes(schema *mschema.Builder) error {
	return schema.Raw(`
		UPDATE public.digital_human_task_reports SET review_state = 'new', reviewed_by_type = NULL, reviewed_by_ref = NULL, reviewed_at = NULL
		WHERE review_state = 'closed';
		ALTER TABLE public.digital_human_task_reports DROP CONSTRAINT IF EXISTS digital_human_task_reports_review_check;
		ALTER TABLE public.digital_human_task_reports ADD CONSTRAINT digital_human_task_reports_review_check
			CHECK (review_state IN ('new', 'accepted', 'rework_requested'));
	`)
}
