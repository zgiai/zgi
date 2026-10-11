package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202608190900000000ID = "202608190900000000_create_digital_human_memory"

const createDigitalHumanMemorySQL = `
	CREATE TABLE IF NOT EXISTS public.digital_human_memory_states (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE RESTRICT,
		memory_epoch bigint NOT NULL DEFAULT 0,
		config_revision varchar(71) NOT NULL,
		auto_extraction_enabled boolean NOT NULL DEFAULT false,
		extraction_cutoff_at timestamptz,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT digital_human_memory_states_epoch_check CHECK (memory_epoch >= 0),
		CONSTRAINT digital_human_memory_states_config_check CHECK (config_revision <> ''),
		CONSTRAINT digital_human_memory_states_subject_unique UNIQUE (organization_id, workspace_id, digital_human_id)
	);

	CREATE TABLE IF NOT EXISTS public.digital_human_memories (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE RESTRICT,
		memory_key varchar(160) NOT NULL,
		memory_class varchar(16) NOT NULL,
		memory_type varchar(24) NOT NULL,
		scope_type varchar(24) NOT NULL DEFAULT '',
		scope_id varchar(255) NOT NULL DEFAULT '',
		content text NOT NULL,
		content_digest varchar(71) NOT NULL,
		visibility varchar(16) NOT NULL DEFAULT 'workspace',
		status varchar(16) NOT NULL DEFAULT 'active',
		confidence numeric(5,4) NOT NULL DEFAULT 1,
		revision bigint NOT NULL DEFAULT 1,
		config_revision varchar(71) NOT NULL,
		memory_epoch bigint NOT NULL DEFAULT 0,
		evidence jsonb NOT NULL DEFAULT '[]'::jsonb,
		source_principal_id varchar(255) NOT NULL DEFAULT '',
		last_operation_id uuid NOT NULL,
		supersedes_id uuid REFERENCES public.digital_human_memories(id) ON DELETE RESTRICT,
		valid_from timestamptz,
		valid_to timestamptz,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT digital_human_memories_class_check CHECK (memory_class IN ('self', 'business')),
		CONSTRAINT digital_human_memories_type_check CHECK (memory_type IN ('identity', 'experience', 'decision', 'fact', 'open_loop', 'reflection')),
		CONSTRAINT digital_human_memories_status_check CHECK (status IN ('active', 'invalidated', 'archived')),
		CONSTRAINT digital_human_memories_visibility_check CHECK (visibility IN ('workspace', 'restricted')),
		CONSTRAINT digital_human_memories_scope_check CHECK (
			(memory_class = 'self' AND scope_type = '' AND scope_id = '') OR
			(memory_class = 'business' AND scope_type IN ('workspace', 'project', 'customer', 'campaign', 'channel') AND scope_id <> '')
		),
		CONSTRAINT digital_human_memories_identity_check CHECK (memory_type <> 'identity' OR memory_class = 'self'),
		CONSTRAINT digital_human_memories_revision_check CHECK (revision >= 1 AND memory_epoch >= 0),
		CONSTRAINT digital_human_memories_confidence_check CHECK (confidence >= 0 AND confidence <= 1),
		CONSTRAINT digital_human_memories_subject_key_unique UNIQUE (
			organization_id, workspace_id, digital_human_id, memory_class, scope_type, scope_id, memory_key
		)
	);
	CREATE INDEX IF NOT EXISTS idx_digital_human_memories_retrieval
		ON public.digital_human_memories (organization_id, workspace_id, digital_human_id, memory_class, scope_type, scope_id, status, updated_at DESC);

	CREATE TABLE IF NOT EXISTS public.digital_human_memory_events (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		operation_id uuid NOT NULL,
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE RESTRICT,
		memory_id uuid REFERENCES public.digital_human_memories(id) ON DELETE RESTRICT,
		action varchar(24) NOT NULL,
		result varchar(24) NOT NULL,
		before_revision bigint,
		after_revision bigint,
		before_snapshot jsonb,
		after_snapshot jsonb,
		evidence jsonb NOT NULL DEFAULT '[]'::jsonb,
		source_principal_id varchar(255) NOT NULL DEFAULT '',
		run_id uuid REFERENCES public.digital_human_runs(id) ON DELETE SET NULL,
		reason_code varchar(128) NOT NULL DEFAULT '',
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT digital_human_memory_events_action_check CHECK (action IN ('upsert', 'clear', 'undo', 'reject', 'expire')),
		CONSTRAINT digital_human_memory_events_result_check CHECK (result IN ('committed', 'rejected', 'idempotent')),
		CONSTRAINT digital_human_memory_events_operation_unique UNIQUE (organization_id, digital_human_id, operation_id)
	);
	CREATE INDEX IF NOT EXISTS idx_digital_human_memory_events_subject
		ON public.digital_human_memory_events (organization_id, workspace_id, digital_human_id, created_at DESC);

	CREATE TABLE IF NOT EXISTS public.digital_human_memory_undo_records (
		operation_id uuid PRIMARY KEY,
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE RESTRICT,
		memory_id uuid NOT NULL REFERENCES public.digital_human_memories(id) ON DELETE RESTRICT,
		previous_snapshot jsonb,
		resulting_revision bigint NOT NULL,
		expires_at timestamptz NOT NULL,
		consumed_at timestamptz,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT digital_human_memory_undo_revision_check CHECK (resulting_revision >= 1)
	);
	CREATE INDEX IF NOT EXISTS idx_digital_human_memory_undo_expires ON public.digital_human_memory_undo_records (expires_at);

	CREATE TABLE IF NOT EXISTS public.digital_human_memory_jobs (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE RESTRICT,
		run_id uuid REFERENCES public.digital_human_runs(id) ON DELETE SET NULL,
		conversation_id uuid,
		message_id uuid,
		artifact_refs jsonb NOT NULL DEFAULT '[]'::jsonb,
		idempotency_key varchar(160) NOT NULL,
		config_revision varchar(71) NOT NULL,
		memory_epoch bigint NOT NULL,
		extractor_version varchar(64) NOT NULL,
		status varchar(24) NOT NULL DEFAULT 'pending',
		attempt_count integer NOT NULL DEFAULT 0,
		error_code varchar(128) NOT NULL DEFAULT '',
		scheduled_at timestamptz NOT NULL,
		started_at timestamptz,
		finished_at timestamptz,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT digital_human_memory_jobs_status_check CHECK (status IN ('pending', 'queued', 'running', 'succeeded', 'failed', 'canceled')),
		CONSTRAINT digital_human_memory_jobs_attempt_check CHECK (attempt_count >= 0 AND memory_epoch >= 0),
		CONSTRAINT digital_human_memory_jobs_key_unique UNIQUE (organization_id, digital_human_id, idempotency_key)
	);
	CREATE INDEX IF NOT EXISTS idx_digital_human_memory_jobs_due
		ON public.digital_human_memory_jobs (status, scheduled_at) WHERE status IN ('pending', 'queued', 'failed');

	CREATE TABLE IF NOT EXISTS public.digital_human_action_ledger (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE RESTRICT,
		run_id uuid REFERENCES public.digital_human_runs(id) ON DELETE SET NULL,
		action_type varchar(48) NOT NULL,
		status varchar(24) NOT NULL,
		memory_operation_id uuid,
		memory_id uuid REFERENCES public.digital_human_memories(id) ON DELETE SET NULL,
		detail jsonb NOT NULL DEFAULT '{}'::jsonb,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT digital_human_action_ledger_status_check CHECK (status IN ('proposed', 'rejected', 'committed', 'reverted', 'completed'))
	);
	CREATE INDEX IF NOT EXISTS idx_digital_human_action_ledger_run ON public.digital_human_action_ledger (run_id, created_at);

	CREATE OR REPLACE FUNCTION public.enforce_digital_human_memory_scope()
	RETURNS trigger AS $$
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM public.digital_humans d
			WHERE d.id = NEW.digital_human_id
			  AND d.organization_id = NEW.organization_id
			  AND d.workspace_id = NEW.workspace_id
			  AND d.deleted_at IS NULL
		) THEN
			RAISE EXCEPTION 'digital human memory scope mismatch' USING ERRCODE = '23514';
		END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;

	CREATE OR REPLACE FUNCTION public.digital_human_memory_event_immutable()
	RETURNS trigger AS $$
	BEGIN
		RAISE EXCEPTION 'digital human memory events are immutable' USING ERRCODE = '23514';
	END;
	$$ LANGUAGE plpgsql;

	DO $$
	DECLARE table_name text;
	BEGIN
		FOREACH table_name IN ARRAY ARRAY[
			'digital_human_memory_states', 'digital_human_memories', 'digital_human_memory_events',
			'digital_human_memory_undo_records', 'digital_human_memory_jobs', 'digital_human_action_ledger'
		] LOOP
			EXECUTE format('DROP TRIGGER IF EXISTS trg_%s_scope ON public.%I', table_name, table_name);
			EXECUTE format('CREATE TRIGGER trg_%s_scope BEFORE INSERT OR UPDATE ON public.%I FOR EACH ROW EXECUTE FUNCTION public.enforce_digital_human_memory_scope()', table_name, table_name);
		END LOOP;
	END $$;
	DROP TRIGGER IF EXISTS trg_digital_human_memory_events_immutable ON public.digital_human_memory_events;
	CREATE TRIGGER trg_digital_human_memory_events_immutable
		BEFORE UPDATE OR DELETE ON public.digital_human_memory_events
		FOR EACH ROW EXECUTE FUNCTION public.digital_human_memory_event_immutable();
`

func init() {
	registerSchemaMigration(migration202608190900000000ID, up202608190900000000, down202608190900000000)
}

func up202608190900000000(schema *mschema.Builder) error {
	return schema.Raw(createDigitalHumanMemorySQL)
}

func down202608190900000000(schema *mschema.Builder) error {
	if err := schema.Raw(`
		DROP TRIGGER IF EXISTS trg_digital_human_memory_events_immutable ON public.digital_human_memory_events;
		DROP FUNCTION IF EXISTS public.digital_human_memory_event_immutable();
		DROP FUNCTION IF EXISTS public.enforce_digital_human_memory_scope() CASCADE;
	`); err != nil {
		return err
	}
	for _, table := range []string{
		"digital_human_action_ledger", "digital_human_memory_jobs", "digital_human_memory_undo_records",
		"digital_human_memory_events", "digital_human_memories", "digital_human_memory_states",
	} {
		if err := schema.DropIfExists(table); err != nil {
			return err
		}
	}
	return nil
}
