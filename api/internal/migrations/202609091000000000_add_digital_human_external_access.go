package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationAddDigitalHumanExternalAccessID = "202609091000000000_add_digital_human_external_access"

func init() {
	registerSchemaMigration(migrationAddDigitalHumanExternalAccessID, upAddDigitalHumanExternalAccess, downAddDigitalHumanExternalAccess)
}

func upAddDigitalHumanExternalAccess(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.digital_humans
			ADD COLUMN IF NOT EXISTS web_app_id uuid;

		ALTER TABLE public.digital_human_versions
			ADD COLUMN IF NOT EXISTS presentation_policy jsonb NOT NULL DEFAULT '{}'::jsonb;

		ALTER TABLE public.digital_human_runs
			ADD COLUMN IF NOT EXISTS principal_type varchar(32) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS principal_id uuid,
			ADD COLUMN IF NOT EXISTS user_subject_kind varchar(32) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS user_subject_id uuid,
			ADD COLUMN IF NOT EXISTS surface varchar(32) NOT NULL DEFAULT 'digital_human_console',
			ADD COLUMN IF NOT EXISTS request_fingerprint varchar(71) NOT NULL DEFAULT '';

		ALTER TABLE public.user_memory_subjects DROP CONSTRAINT IF EXISTS ck_user_memory_subject_identity;
		ALTER TABLE public.user_memory_subjects ADD CONSTRAINT ck_user_memory_subject_identity CHECK (
			(kind = 'account' AND account_id = id AND issuer_id IS NULL) OR
			(kind = 'api_end_user' AND account_id IS NULL AND issuer_type IN ('agent_api', 'digital_human_api') AND issuer_id IS NOT NULL)
		);

		CREATE TABLE IF NOT EXISTS public.digital_human_api_keys (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE CASCADE,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
			key_hash varchar(64) NOT NULL UNIQUE,
			key_prefix varchar(20) NOT NULL,
			name varchar(255) NOT NULL,
			status varchar(20) NOT NULL DEFAULT 'active',
			expires_at timestamptz,
			usage_count bigint NOT NULL DEFAULT 0,
			last_used_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT ck_digital_human_api_key_status CHECK (status IN ('active', 'revoked'))
		);
		CREATE INDEX IF NOT EXISTS idx_digital_human_api_keys_owner
			ON public.digital_human_api_keys (digital_human_id, workspace_id, created_at DESC);
	`); err != nil {
		return err
	}

	if err := schema.DataFix("backfill DigitalHuman external access identity", func(db *gorm.DB) error {
		if err := db.Exec(`
			UPDATE public.digital_humans
			SET web_app_id = public.uuid_generate_v4()
			WHERE web_app_id IS NULL
		`).Error; err != nil {
			return err
		}

		if err := db.Exec(`
			UPDATE public.digital_human_runs
			SET principal_type = 'account', principal_id = actor_id,
				user_subject_kind = 'account', user_subject_id = actor_id,
				surface = 'digital_human_console'
			WHERE actor_id IS NOT NULL
				AND COALESCE(invocation_event->'origin'->>'kind', 'manual') = 'manual'
		`).Error; err != nil {
			return err
		}

		return db.Exec(`
			UPDATE public.digital_human_runs
			SET principal_type = COALESCE(NULLIF(invocation_event->'sender'->>'type', ''), 'system'),
				user_subject_kind = '', user_subject_id = NULL,
				surface = CASE COALESCE(invocation_event->'origin'->>'kind', '')
					WHEN 'channel' THEN 'channel'
					WHEN 'schedule' THEN 'digital_human_task'
					WHEN 'api' THEN 'digital_human_api'
					ELSE surface END
			WHERE COALESCE(invocation_event->'origin'->>'kind', 'manual') <> 'manual'
		`).Error
	}); err != nil {
		return err
	}

	return schema.Raw(`
		ALTER TABLE public.digital_humans
			ALTER COLUMN web_app_id SET DEFAULT public.uuid_generate_v4(),
			ALTER COLUMN web_app_id SET NOT NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_digital_humans_web_app_id ON public.digital_humans (web_app_id);
	`)
}

func downAddDigitalHumanExternalAccess(schema *mschema.Builder) error {
	if err := schema.DropIfExists("digital_human_api_keys"); err != nil {
		return err
	}
	return schema.Raw(`
		ALTER TABLE public.user_memory_subjects DROP CONSTRAINT IF EXISTS ck_user_memory_subject_identity;
		ALTER TABLE public.user_memory_subjects ADD CONSTRAINT ck_user_memory_subject_identity CHECK (
			(kind = 'account' AND account_id = id AND issuer_id IS NULL) OR
			(kind = 'api_end_user' AND account_id IS NULL AND issuer_type = 'agent_api' AND issuer_id IS NOT NULL)
		);
		ALTER TABLE public.digital_human_runs
			DROP COLUMN IF EXISTS request_fingerprint,
			DROP COLUMN IF EXISTS surface,
			DROP COLUMN IF EXISTS user_subject_id,
			DROP COLUMN IF EXISTS user_subject_kind,
			DROP COLUMN IF EXISTS principal_id,
			DROP COLUMN IF EXISTS principal_type;
		ALTER TABLE public.digital_human_versions DROP COLUMN IF EXISTS presentation_policy;
		DROP INDEX IF EXISTS public.idx_digital_humans_web_app_id;
		ALTER TABLE public.digital_humans DROP COLUMN IF EXISTS web_app_id;
	`)
}
