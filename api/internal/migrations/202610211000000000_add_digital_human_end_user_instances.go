package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// An end-user instance is one DigitalHuman's relationship with one external
// end user (ADR 0006). It is keyed by the existing user subject, so the raw
// external user value is never stored. Runs record the instance they were
// admitted for.
const digitalHumanEndUserInstancesSQL = `
CREATE TABLE IF NOT EXISTS public.digital_human_end_user_instances (
	id uuid PRIMARY KEY,
	organization_id uuid NOT NULL,
	workspace_id uuid NOT NULL,
	digital_human_id uuid NOT NULL,
	subject_kind varchar(32) NOT NULL,
	subject_id uuid NOT NULL REFERENCES public.user_subjects(id) ON DELETE RESTRICT,
	issuer_type varchar(32) NOT NULL,
	issuer_id uuid,
	status varchar(16) NOT NULL DEFAULT 'active',
	label varchar(128),
	quota_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
	first_seen_at timestamptz NOT NULL,
	last_active_at timestamptz NOT NULL,
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL,
	CONSTRAINT digital_human_end_user_instances_status_check CHECK (status IN ('active', 'suspended', 'erased')),
	CONSTRAINT digital_human_end_user_instances_issuer_check CHECK (issuer_type IN ('digital_human_api', 'digital_human_webapp'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_digital_human_end_user_instances_subject
	ON public.digital_human_end_user_instances (digital_human_id, subject_id);
CREATE INDEX IF NOT EXISTS idx_digital_human_end_user_instances_activity
	ON public.digital_human_end_user_instances (organization_id, workspace_id, digital_human_id, last_active_at DESC);

ALTER TABLE public.digital_human_runs ADD COLUMN IF NOT EXISTS end_user_instance_id uuid;
ALTER TABLE public.digital_human_runs DROP CONSTRAINT IF EXISTS digital_human_runs_end_user_instance_fk;
ALTER TABLE public.digital_human_runs ADD CONSTRAINT digital_human_runs_end_user_instance_fk
	FOREIGN KEY (end_user_instance_id) REFERENCES public.digital_human_end_user_instances(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_digital_human_runs_end_user_instance
	ON public.digital_human_runs (end_user_instance_id, created_at DESC) WHERE end_user_instance_id IS NOT NULL;`

// Historical external end users get an instance from their earliest and
// latest runs, so operators also see end users from before instances existed.
// Their earlier runs keep a NULL instance; the subject key links them.
const digitalHumanEndUserInstancesBackfillSQL = `
INSERT INTO public.digital_human_end_user_instances (
	id, organization_id, workspace_id, digital_human_id, subject_kind, subject_id, issuer_type, issuer_id,
	status, quota_policy, first_seen_at, last_active_at, created_at, updated_at
)
SELECT gen_random_uuid(), first_run.organization_id, first_run.workspace_id, first_run.digital_human_id,
	first_run.user_subject_kind, first_run.user_subject_id, first_run.surface,
	CASE WHEN first_run.surface = 'digital_human_api' THEN first_run.principal_id END,
	'active', '{}'::jsonb, activity.first_seen_at, activity.last_active_at, now(), now()
FROM (
	SELECT DISTINCT ON (digital_human_id, user_subject_id) *
	FROM public.digital_human_runs
	WHERE user_subject_id IS NOT NULL AND surface IN ('digital_human_api', 'digital_human_webapp')
		AND COALESCE(source_trust, 'external_end_user') = 'external_end_user'
	ORDER BY digital_human_id, user_subject_id, created_at
) AS first_run
JOIN (
	SELECT digital_human_id, user_subject_id, min(created_at) AS first_seen_at, max(created_at) AS last_active_at
	FROM public.digital_human_runs
	WHERE user_subject_id IS NOT NULL AND surface IN ('digital_human_api', 'digital_human_webapp')
		AND COALESCE(source_trust, 'external_end_user') = 'external_end_user'
	GROUP BY digital_human_id, user_subject_id
) AS activity USING (digital_human_id, user_subject_id)
WHERE EXISTS (SELECT 1 FROM public.user_subjects WHERE user_subjects.id = first_run.user_subject_id)
ON CONFLICT (digital_human_id, subject_id) DO NOTHING;`

func init() {
	registerSchemaMigration("202610211000000000_add_digital_human_end_user_instances", upAddDigitalHumanEndUserInstances, downAddDigitalHumanEndUserInstances)
}

func upAddDigitalHumanEndUserInstances(schema *mschema.Builder) error {
	if err := schema.Raw(digitalHumanEndUserInstancesSQL); err != nil {
		return err
	}
	return schema.Raw(digitalHumanEndUserInstancesBackfillSQL)
}

func downAddDigitalHumanEndUserInstances(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_digital_human_runs_end_user_instance;
		ALTER TABLE public.digital_human_runs
			DROP CONSTRAINT IF EXISTS digital_human_runs_end_user_instance_fk,
			DROP COLUMN IF EXISTS end_user_instance_id;
		DROP TABLE IF EXISTS public.digital_human_end_user_instances;
	`)
}
