package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddWorkflowRuntimeV2ID = "20260717090000_add_workflow_runtime_v2"

func init() {
	registerSchemaMigration(
		migrationAddWorkflowRuntimeV2ID,
		upAddWorkflowRuntimeV2,
		downAddWorkflowRuntimeV2,
	)
}

func upAddWorkflowRuntimeV2(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.workflow_run_logs
			ADD COLUMN IF NOT EXISTS runtime_protocol_version integer NOT NULL DEFAULT 1,
			ADD COLUMN IF NOT EXISTS next_event_sequence bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS execution_generation bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS active_execution_id uuid,
			ADD COLUMN IF NOT EXISTS execution_lease_expires_at timestamptz,
			ADD COLUMN IF NOT EXISTS state_revision bigint NOT NULL DEFAULT 0;

		ALTER TABLE public.workflow_run_pauses
			ADD COLUMN IF NOT EXISTS generation bigint NOT NULL DEFAULT 1,
			ADD COLUMN IF NOT EXISTS status varchar(32) NOT NULL DEFAULT 'paused',
			ADD COLUMN IF NOT EXISTS revision bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS resume_execution_id uuid,
			ADD COLUMN IF NOT EXISTS lease_expires_at timestamptz;
		UPDATE public.workflow_run_pauses
		SET status = CASE WHEN resumed_at IS NULL THEN 'paused' ELSE 'closed' END;

		ALTER TABLE public.workflow_run_pause_reasons
			ADD COLUMN IF NOT EXISTS status varchar(32) NOT NULL DEFAULT 'pending',
			ADD COLUMN IF NOT EXISTS revision bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS submission_event_id uuid,
			ADD COLUMN IF NOT EXISTS completed_at timestamptz;
		CREATE INDEX IF NOT EXISTS idx_workflow_run_pause_reasons_status
			ON public.workflow_run_pause_reasons (pause_id, status);

		ALTER TABLE public.workflow_run_events
			ADD COLUMN IF NOT EXISTS schema_version integer NOT NULL DEFAULT 1,
			ADD COLUMN IF NOT EXISTS category varchar(32) NOT NULL DEFAULT 'execution',
			ADD COLUMN IF NOT EXISTS execution_id uuid,
			ADD COLUMN IF NOT EXISTS pause_id uuid,
			ADD COLUMN IF NOT EXISTS pause_generation bigint,
			ADD COLUMN IF NOT EXISTS idempotency_key varchar(255),
			ADD COLUMN IF NOT EXISTS occurred_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP;

		WITH ranked_events AS (
			SELECT id,
				ROW_NUMBER() OVER (
					PARTITION BY tenant_id, workflow_run_id
					ORDER BY sequence ASC, created_at ASC, id ASC
				) AS repaired_sequence
			FROM public.workflow_run_events
		)
		UPDATE public.workflow_run_events AS events
		SET sequence = ranked_events.repaired_sequence
		FROM ranked_events
		WHERE events.id = ranked_events.id;

		UPDATE public.workflow_run_logs AS runs
		SET next_event_sequence = COALESCE(events.maximum_sequence, 0)
		FROM (
			SELECT workflow_run_id, MAX(sequence) AS maximum_sequence
			FROM public.workflow_run_events
			GROUP BY workflow_run_id
		) AS events
		WHERE runs.id::text = events.workflow_run_id;

		CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_run_events_tenant_run_sequence_unique
			ON public.workflow_run_events (tenant_id, workflow_run_id, sequence);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_run_events_run_idempotency_unique
			ON public.workflow_run_events (workflow_run_id, idempotency_key)
			WHERE idempotency_key IS NOT NULL;

		ALTER TABLE public.agents_messages
			ADD COLUMN IF NOT EXISTS projection_revision bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS execution_generation bigint NOT NULL DEFAULT 0;

		WITH canonical AS (
			SELECT DISTINCT ON (workflow_run_id) workflow_run_id, id
			FROM public.agents_messages
			WHERE workflow_run_id IS NOT NULL AND deleted_at IS NULL
			ORDER BY workflow_run_id, created_at ASC, id ASC
		), latest AS (
			SELECT DISTINCT ON (workflow_run_id)
				workflow_run_id, status, error, message_metadata, updated_at
			FROM public.agents_messages
			WHERE workflow_run_id IS NOT NULL AND deleted_at IS NULL
			ORDER BY workflow_run_id, updated_at DESC, created_at DESC, id DESC
		), latest_answer AS (
			SELECT DISTINCT ON (workflow_run_id) workflow_run_id, answer
			FROM public.agents_messages
			WHERE workflow_run_id IS NOT NULL AND deleted_at IS NULL AND answer <> ''
			ORDER BY workflow_run_id, updated_at DESC, created_at DESC, id DESC
		)
		UPDATE public.agents_messages AS message
		SET answer = COALESCE(latest_answer.answer, message.answer),
			status = latest.status,
			error = latest.error,
			message_metadata = COALESCE(latest.message_metadata, message.message_metadata),
			updated_at = GREATEST(message.updated_at, latest.updated_at)
		FROM canonical
		JOIN latest ON latest.workflow_run_id = canonical.workflow_run_id
		LEFT JOIN latest_answer ON latest_answer.workflow_run_id = canonical.workflow_run_id
		WHERE message.id = canonical.id;

		WITH canonical AS (
			SELECT DISTINCT ON (workflow_run_id) workflow_run_id, id
			FROM public.agents_messages
			WHERE workflow_run_id IS NOT NULL AND deleted_at IS NULL
			ORDER BY workflow_run_id, created_at ASC, id ASC
		)
		DELETE FROM public.agents_messages AS message
		USING canonical
		WHERE message.workflow_run_id = canonical.workflow_run_id
			AND message.deleted_at IS NULL
			AND message.id <> canonical.id;

		CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_messages_active_workflow_run_unique
			ON public.agents_messages (workflow_run_id)
			WHERE workflow_run_id IS NOT NULL AND deleted_at IS NULL;

		ALTER TABLE public.workflow_node_runtime_logs
			ADD COLUMN IF NOT EXISTS parent_execution_id varchar(255),
			ADD COLUMN IF NOT EXISTS container_id varchar(255),
			ADD COLUMN IF NOT EXISTS container_type varchar(32),
			ADD COLUMN IF NOT EXISTS round_index integer,
			ADD COLUMN IF NOT EXISTS attempt integer NOT NULL DEFAULT 1,
			ADD COLUMN IF NOT EXISTS started_event_sequence bigint,
			ADD COLUMN IF NOT EXISTS finished_event_sequence bigint;
		CREATE INDEX IF NOT EXISTS idx_workflow_node_runtime_logs_container_round
			ON public.workflow_node_runtime_logs (workflow_run_id, container_id, round_index);
		WITH duplicate_node_executions AS (
			SELECT id,
				ROW_NUMBER() OVER (
					PARTITION BY workflow_run_id, node_execution_id
					ORDER BY created_at ASC, id ASC
				) AS duplicate_rank
			FROM public.workflow_node_runtime_logs
			WHERE workflow_run_id IS NOT NULL
				AND node_execution_id IS NOT NULL
				AND node_execution_id <> ''
				AND deleted_at IS NULL
		)
		DELETE FROM public.workflow_node_runtime_logs AS logs
		USING duplicate_node_executions
		WHERE logs.id = duplicate_node_executions.id
			AND duplicate_node_executions.duplicate_rank > 1;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_node_runtime_logs_run_execution_unique
			ON public.workflow_node_runtime_logs (workflow_run_id, node_execution_id)
			WHERE workflow_run_id IS NOT NULL AND node_execution_id IS NOT NULL AND deleted_at IS NULL;

		CREATE TABLE IF NOT EXISTS public.workflow_runtime_outbox (
			id uuid PRIMARY KEY,
			tenant_id uuid NOT NULL,
			workflow_run_id uuid NOT NULL,
			pause_id uuid,
			kind varchar(64) NOT NULL,
			idempotency_key varchar(255) NOT NULL,
			payload_json text NOT NULL,
			status varchar(32) NOT NULL DEFAULT 'pending',
			attempts integer NOT NULL DEFAULT 0,
			next_attempt_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			published_at timestamptz,
			last_error text,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_runtime_outbox_idempotency
			ON public.workflow_runtime_outbox (idempotency_key);
		CREATE INDEX IF NOT EXISTS idx_workflow_runtime_outbox_pending
			ON public.workflow_runtime_outbox (status, next_attempt_at)
			WHERE status = 'pending'
	`)
}

func downAddWorkflowRuntimeV2(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_workflow_runtime_outbox_pending;
		DROP INDEX IF EXISTS public.idx_workflow_runtime_outbox_idempotency;
		DROP TABLE IF EXISTS public.workflow_runtime_outbox;
		DROP INDEX IF EXISTS public.idx_workflow_node_runtime_logs_run_execution_unique;
		DROP INDEX IF EXISTS public.idx_workflow_node_runtime_logs_container_round;
		ALTER TABLE public.workflow_node_runtime_logs
			DROP COLUMN IF EXISTS finished_event_sequence,
			DROP COLUMN IF EXISTS started_event_sequence,
			DROP COLUMN IF EXISTS attempt,
			DROP COLUMN IF EXISTS round_index,
			DROP COLUMN IF EXISTS container_type,
			DROP COLUMN IF EXISTS container_id,
			DROP COLUMN IF EXISTS parent_execution_id;
		DROP INDEX IF EXISTS public.idx_agents_messages_active_workflow_run_unique;
		ALTER TABLE public.agents_messages
			DROP COLUMN IF EXISTS execution_generation,
			DROP COLUMN IF EXISTS projection_revision;
		DROP INDEX IF EXISTS public.idx_workflow_run_events_run_idempotency_unique;
		DROP INDEX IF EXISTS public.idx_workflow_run_events_tenant_run_sequence_unique;
		ALTER TABLE public.workflow_run_events
			DROP COLUMN IF EXISTS occurred_at,
			DROP COLUMN IF EXISTS idempotency_key,
			DROP COLUMN IF EXISTS pause_generation,
			DROP COLUMN IF EXISTS pause_id,
			DROP COLUMN IF EXISTS execution_id,
			DROP COLUMN IF EXISTS category,
			DROP COLUMN IF EXISTS schema_version;
		ALTER TABLE public.workflow_run_pauses
			DROP COLUMN IF EXISTS lease_expires_at,
			DROP COLUMN IF EXISTS resume_execution_id,
			DROP COLUMN IF EXISTS revision,
			DROP COLUMN IF EXISTS status,
			DROP COLUMN IF EXISTS generation;
		DROP INDEX IF EXISTS public.idx_workflow_run_pause_reasons_status;
		ALTER TABLE public.workflow_run_pause_reasons
			DROP COLUMN IF EXISTS completed_at,
			DROP COLUMN IF EXISTS submission_event_id,
			DROP COLUMN IF EXISTS revision,
			DROP COLUMN IF EXISTS status;
		ALTER TABLE public.workflow_run_logs
			DROP COLUMN IF EXISTS state_revision,
			DROP COLUMN IF EXISTS execution_lease_expires_at,
			DROP COLUMN IF EXISTS active_execution_id,
			DROP COLUMN IF EXISTS execution_generation,
			DROP COLUMN IF EXISTS next_event_sequence,
			DROP COLUMN IF EXISTS runtime_protocol_version
	`)
}
