package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// A task report is what one DigitalHuman task run hands back (ADR 0008). The
// DigitalHuman submits it through its report tool, or the scheduler writes one
// when the run ends without it. Only the report that concludes a run chain is
// final: retried attempts keep their reports as history but notify no one.
const digitalHumanTaskReportsSQL = `
CREATE TABLE IF NOT EXISTS public.digital_human_task_reports (
	id uuid PRIMARY KEY,
	organization_id uuid NOT NULL,
	workspace_id uuid NOT NULL,
	digital_human_id uuid NOT NULL,
	task_id uuid NOT NULL REFERENCES public.automation_tasks(id) ON DELETE CASCADE,
	task_run_id uuid NOT NULL REFERENCES public.automation_task_runs(id) ON DELETE CASCADE,
	digital_human_run_id uuid,
	attempt integer NOT NULL DEFAULT 1,
	outcome varchar(16) NOT NULL,
	summary text NOT NULL,
	body text NOT NULL DEFAULT '',
	criteria jsonb NOT NULL DEFAULT '[]'::jsonb,
	issues text NOT NULL DEFAULT '',
	files jsonb NOT NULL DEFAULT '[]'::jsonb,
	source varchar(16) NOT NULL,
	final boolean NOT NULL DEFAULT false,
	review_state varchar(24) NOT NULL DEFAULT 'new',
	review_feedback text,
	reviewed_by_type varchar(32),
	reviewed_by_ref varchar(128),
	reviewed_at timestamptz,
	rework_task_run_id uuid,
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL,
	CONSTRAINT digital_human_task_reports_outcome_check CHECK (outcome IN ('done', 'partial', 'not_done', 'failed')),
	CONSTRAINT digital_human_task_reports_source_check CHECK (source IN ('report_tool', 'system')),
	CONSTRAINT digital_human_task_reports_review_check CHECK (review_state IN ('new', 'accepted', 'rework_requested'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_digital_human_task_reports_task_run
	ON public.digital_human_task_reports (task_run_id);
CREATE INDEX IF NOT EXISTS idx_digital_human_task_reports_task
	ON public.digital_human_task_reports (task_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_digital_human_task_reports_feed
	ON public.digital_human_task_reports (organization_id, workspace_id, digital_human_id, created_at DESC) WHERE final;`

func init() {
	registerSchemaMigration("202610211100000000_add_digital_human_task_reports", upAddDigitalHumanTaskReports, downAddDigitalHumanTaskReports)
}

func upAddDigitalHumanTaskReports(schema *mschema.Builder) error {
	return schema.Raw(digitalHumanTaskReportsSQL)
}

func downAddDigitalHumanTaskReports(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.digital_human_task_reports;`)
}
