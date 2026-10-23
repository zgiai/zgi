package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Runtime Workspace end users may hold credits the integrating server
// grants (ADR 0011). Grants are lots that may expire; usage beyond the
// limits that apply is drawn from the lots, soonest expiry first, and what
// no lot covers is debt the next grant pays. Every grant and draw is a
// ledger entry, and each settled usage bill is marked once, so settlement
// can run again safely.
func init() {
	registerSchemaMigration("202610241100000000_runtime_workspace_end_user_credits", upRuntimeWorkspaceEndUserCredits, downRuntimeWorkspaceEndUserCredits)
}

func upRuntimeWorkspaceEndUserCredits(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.runtime_workspace_end_users
			ADD COLUMN IF NOT EXISTS credits_since timestamptz,
			ADD COLUMN IF NOT EXISTS credit_debt bigint NOT NULL DEFAULT 0;

		CREATE TABLE IF NOT EXISTS public.runtime_workspace_end_user_credit_lots (
			id uuid PRIMARY KEY,
			runtime_workspace_id uuid NOT NULL,
			subject_id uuid NOT NULL,
			amount bigint NOT NULL,
			remaining bigint NOT NULL,
			expires_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT ck_runtime_workspace_credit_lots_amounts CHECK (amount > 0 AND remaining >= 0 AND remaining <= amount)
		);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_credit_lots_subject
			ON public.runtime_workspace_end_user_credit_lots (subject_id, expires_at)
			WHERE remaining > 0;
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_credit_lots_workspace
			ON public.runtime_workspace_end_user_credit_lots (runtime_workspace_id);

		CREATE TABLE IF NOT EXISTS public.runtime_workspace_end_user_credit_entries (
			id uuid PRIMARY KEY,
			runtime_workspace_id uuid NOT NULL,
			subject_id uuid NOT NULL,
			kind varchar(16) NOT NULL,
			amount bigint NOT NULL,
			balance_after bigint NOT NULL,
			bill_id uuid,
			external_id varchar(128) NOT NULL DEFAULT '',
			reason varchar(255) NOT NULL DEFAULT '',
			expires_at timestamptz,
			actor_kind varchar(16) NOT NULL DEFAULT '',
			actor_id uuid,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT ck_runtime_workspace_credit_entries_kind CHECK (kind IN ('grant', 'usage'))
		);
		CREATE UNIQUE INDEX IF NOT EXISTS uq_runtime_workspace_credit_entries_external
			ON public.runtime_workspace_end_user_credit_entries (subject_id, external_id)
			WHERE external_id <> '';
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_credit_entries_subject
			ON public.runtime_workspace_end_user_credit_entries (subject_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_credit_entries_workspace
			ON public.runtime_workspace_end_user_credit_entries (runtime_workspace_id);

		CREATE TABLE IF NOT EXISTS public.runtime_workspace_end_user_credit_bills (
			bill_id uuid PRIMARY KEY,
			runtime_workspace_id uuid NOT NULL,
			subject_id uuid NOT NULL,
			charged bigint NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_credit_bills_subject
			ON public.runtime_workspace_end_user_credit_bills (subject_id);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_credit_bills_workspace
			ON public.runtime_workspace_end_user_credit_bills (runtime_workspace_id);
	`)
}

func downRuntimeWorkspaceEndUserCredits(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TABLE IF EXISTS public.runtime_workspace_end_user_credit_bills;
		DROP TABLE IF EXISTS public.runtime_workspace_end_user_credit_entries;
		DROP TABLE IF EXISTS public.runtime_workspace_end_user_credit_lots;
		ALTER TABLE public.runtime_workspace_end_users
			DROP COLUMN IF EXISTS credit_debt,
			DROP COLUMN IF EXISTS credits_since;
	`)
}
