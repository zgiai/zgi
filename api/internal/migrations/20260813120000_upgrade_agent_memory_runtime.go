package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationUpgradeAgentMemoryRuntimeID = "20260813120000_upgrade_agent_memory_runtime"

func init() {
	registerSchemaMigration(
		migrationUpgradeAgentMemoryRuntimeID,
		upUpgradeAgentMemoryRuntime,
		downUpgradeAgentMemoryRuntime,
	)
}

func upUpgradeAgentMemoryRuntime(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.agent_memory_values
			ADD COLUMN IF NOT EXISTS revision bigint NOT NULL DEFAULT 1,
			ADD COLUMN IF NOT EXISTS source_kind varchar(32) NOT NULL DEFAULT 'legacy',
			ADD COLUMN IF NOT EXISTS source_conversation_id uuid,
			ADD COLUMN IF NOT EXISTS source_message_id uuid,
			ADD COLUMN IF NOT EXISTS source_completed_at timestamptz,
			ADD COLUMN IF NOT EXISTS extractor_version varchar(64) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS last_operation_id uuid;
		-- PostgreSQL applies constant defaults to existing rows without rewriting
		-- the table. A compatibility trigger advances revisions for writes made by
		-- an older binary that does not know about the new columns.
		CREATE OR REPLACE FUNCTION public.agent_memory_value_legacy_write_guard()
		RETURNS trigger AS $$
		BEGIN
			IF NEW.revision IS NOT DISTINCT FROM OLD.revision THEN
				NEW.revision := OLD.revision + 1;
				NEW.source_kind := 'legacy';
				NEW.source_conversation_id := NULL;
				NEW.source_message_id := NULL;
				NEW.source_completed_at := NULL;
				NEW.extractor_version := '';
				NEW.last_operation_id := NULL;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		DROP TRIGGER IF EXISTS trg_agent_memory_value_legacy_write_guard ON public.agent_memory_values;
		CREATE TRIGGER trg_agent_memory_value_legacy_write_guard
			BEFORE UPDATE ON public.agent_memory_values
			FOR EACH ROW EXECUTE FUNCTION public.agent_memory_value_legacy_write_guard();
		DO $$ BEGIN
			ALTER TABLE public.agent_memory_values
				ADD CONSTRAINT ck_agent_memory_values_source_kind
				CHECK (source_kind IN ('legacy', 'explicit', 'automatic', 'manager'));
		EXCEPTION WHEN duplicate_object THEN NULL; END $$;
		CREATE INDEX IF NOT EXISTS idx_agent_memory_values_source_message
			ON public.agent_memory_values (source_message_id)
			WHERE source_message_id IS NOT NULL;

		ALTER TABLE public.agent_memory_events
			ADD COLUMN IF NOT EXISTS before_revision bigint,
			ADD COLUMN IF NOT EXISTS after_revision bigint,
			ADD COLUMN IF NOT EXISTS result varchar(32) NOT NULL DEFAULT 'success',
			ADD COLUMN IF NOT EXISTS operation_id uuid;
		-- Keep mixed-version nodes from adding new body snapshots. Existing rows
		-- are scrubbed in bounded batches by the runtime maintenance worker.
		CREATE OR REPLACE FUNCTION public.agent_memory_event_content_guard()
		RETURNS trigger AS $$
		BEGIN
			NEW.before_snapshot := NULL;
			NEW.after_snapshot := NULL;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		DROP TRIGGER IF EXISTS trg_agent_memory_event_content_guard ON public.agent_memory_events;
		CREATE TRIGGER trg_agent_memory_event_content_guard
			BEFORE INSERT OR UPDATE ON public.agent_memory_events
			FOR EACH ROW EXECUTE FUNCTION public.agent_memory_event_content_guard();
		CREATE INDEX IF NOT EXISTS idx_agent_memory_events_retention
			ON public.agent_memory_events (created_at);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_memory_events_operation
			ON public.agent_memory_events (operation_id)
			WHERE operation_id IS NOT NULL;

		CREATE TABLE IF NOT EXISTS public.agent_memory_subject_states (
			id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
			workspace_id uuid NOT NULL,
			agent_id uuid NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
			user_scope varchar(32) NOT NULL,
			user_id uuid NOT NULL,
			memory_epoch bigint NOT NULL DEFAULT 0,
			extraction_cutoff_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT idx_agent_memory_subject_scope UNIQUE (workspace_id, agent_id, user_scope, user_id)
		);

		CREATE TABLE IF NOT EXISTS public.agent_memory_agent_states (
			id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
			workspace_id uuid NOT NULL,
			agent_id uuid NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
			draft_config_revision varchar(64) NOT NULL DEFAULT '',
			published_config_revision varchar(64) NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT idx_agent_memory_agent_scope UNIQUE (workspace_id, agent_id)
		);

		CREATE TABLE IF NOT EXISTS public.agent_memory_extraction_jobs (
			id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
			workspace_id uuid NOT NULL,
			agent_id uuid NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
			user_scope varchar(32) NOT NULL,
			user_id uuid NOT NULL,
			conversation_id uuid NOT NULL,
			message_watermark_id uuid NOT NULL,
			memory_epoch bigint NOT NULL DEFAULT 0,
			config_scope varchar(16) NOT NULL DEFAULT '',
			config_revision varchar(64) NOT NULL DEFAULT '',
			runtime_slots jsonb NOT NULL DEFAULT '[]'::jsonb,
			extractor_version varchar(64) NOT NULL,
			idempotency_key varchar(128) NOT NULL UNIQUE,
			status varchar(24) NOT NULL DEFAULT 'pending',
			attempt_count integer NOT NULL DEFAULT 0,
			error_code varchar(64) NOT NULL DEFAULT '',
			scheduled_at timestamptz NOT NULL,
			force_at timestamptz NOT NULL,
			started_at timestamptz,
			finished_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_agent_memory_jobs_due
			ON public.agent_memory_extraction_jobs (workspace_id, status, scheduled_at);
		CREATE INDEX IF NOT EXISTS idx_agent_memory_jobs_status_due
			ON public.agent_memory_extraction_jobs (status, scheduled_at);
		CREATE INDEX IF NOT EXISTS idx_agent_memory_jobs_terminal_cleanup
			ON public.agent_memory_extraction_jobs (status, finished_at);
		CREATE INDEX IF NOT EXISTS idx_agent_memory_jobs_subject
			ON public.agent_memory_extraction_jobs (workspace_id, agent_id, user_scope, user_id, created_at);

		CREATE TABLE IF NOT EXISTS public.agent_memory_undo_records (
			operation_id uuid PRIMARY KEY,
			workspace_id uuid NOT NULL,
			agent_id uuid NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
			user_scope varchar(32) NOT NULL,
			user_id uuid NOT NULL,
			slot_key varchar(64) NOT NULL,
			previous_exists boolean NOT NULL DEFAULT false,
			previous_content text NOT NULL DEFAULT '',
			previous_revision bigint NOT NULL DEFAULT 0,
			previous_source_kind varchar(32) NOT NULL DEFAULT '',
			previous_conversation_id uuid,
			previous_message_id uuid,
			previous_source_completed_at timestamptz,
			previous_extractor_version varchar(64) NOT NULL DEFAULT '',
			resulting_revision bigint NOT NULL,
			expires_at timestamptz NOT NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_agent_memory_undo_scope
			ON public.agent_memory_undo_records (workspace_id, agent_id, user_scope, user_id);
		CREATE INDEX IF NOT EXISTS idx_agent_memory_undo_expires
			ON public.agent_memory_undo_records (expires_at);
	`)
}

func downUpgradeAgentMemoryRuntime(schema *mschema.Builder) error {
	// Value columns intentionally remain during rollback so an older binary can
	// ignore them without losing migrated state. Only new standalone tables are removed.
	if err := schema.DropIfExists("agent_memory_undo_records"); err != nil {
		return err
	}
	if err := schema.DropIfExists("agent_memory_extraction_jobs"); err != nil {
		return err
	}
	if err := schema.DropIfExists("agent_memory_subject_states"); err != nil {
		return err
	}
	return schema.DropIfExists("agent_memory_agent_states")
}
