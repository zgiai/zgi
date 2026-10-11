package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationCreateUserRelationshipMemoriesID = "202609081200000000_create_user_relationship_memories"

func init() {
	registerSchemaMigration(
		migrationCreateUserRelationshipMemoriesID,
		upCreateUserRelationshipMemories,
		downCreateUserRelationshipMemories,
	)
}

func upCreateUserRelationshipMemories(schema *mschema.Builder) error {
	if err := schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.user_memory_subjects (
			id uuid PRIMARY KEY,
			kind varchar(32) NOT NULL,
			account_id uuid,
			issuer_type varchar(32) NOT NULL DEFAULT '',
			issuer_id uuid,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT ck_user_memory_subject_kind CHECK (kind IN ('account', 'api_end_user')),
			CONSTRAINT ck_user_memory_subject_identity CHECK (
				(kind = 'account' AND account_id = id AND issuer_id IS NULL) OR
				(kind = 'api_end_user' AND account_id IS NULL AND issuer_type = 'agent_api' AND issuer_id IS NOT NULL)
			)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_user_memory_subject_account
			ON public.user_memory_subjects (account_id) WHERE account_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_user_memory_subject_issuer
			ON public.user_memory_subjects (issuer_type, issuer_id, id) WHERE issuer_id IS NOT NULL;

		CREATE TABLE IF NOT EXISTS public.user_memory_relationships (
			id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
			organization_id uuid NOT NULL,
			workspace_id uuid NOT NULL,
			subject_id uuid NOT NULL REFERENCES public.user_memory_subjects(id) ON DELETE RESTRICT,
			consumer_type varchar(32) NOT NULL,
			consumer_id uuid NOT NULL,
			consumer_name_snapshot varchar(255) NOT NULL DEFAULT '',
			status varchar(24) NOT NULL DEFAULT 'active',
			memory_enabled boolean NOT NULL DEFAULT true,
			auto_capture_enabled boolean NOT NULL DEFAULT false,
			settings_revision bigint NOT NULL DEFAULT 1,
			memory_epoch bigint NOT NULL DEFAULT 0,
			extraction_cutoff_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			archived_at timestamptz,
			CONSTRAINT ck_user_memory_consumer_type CHECK (consumer_type IN ('agent', 'digital_human')),
			CONSTRAINT ck_user_memory_relationship_status CHECK (status IN ('active', 'archived')),
			CONSTRAINT idx_user_memory_relationship_scope UNIQUE (workspace_id, subject_id, consumer_type, consumer_id)
		);
		CREATE INDEX IF NOT EXISTS idx_user_memory_relationship_org_subject
			ON public.user_memory_relationships (organization_id, subject_id, updated_at DESC);
		CREATE INDEX IF NOT EXISTS idx_user_memory_relationship_consumer
			ON public.user_memory_relationships (workspace_id, consumer_type, consumer_id);

		CREATE TABLE IF NOT EXISTS public.user_memory_items (
			id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
			relationship_id uuid NOT NULL REFERENCES public.user_memory_relationships(id) ON DELETE RESTRICT,
			memory_key varchar(64) NOT NULL,
			display_name varchar(80) NOT NULL DEFAULT '',
			category varchar(32) NOT NULL DEFAULT '',
			content_payload text NOT NULL DEFAULT '',
			content_encoding varchar(24) NOT NULL DEFAULT 'plain',
			content_key_id varchar(128),
			revision bigint NOT NULL DEFAULT 1,
			source_kind varchar(32) NOT NULL DEFAULT 'explicit',
			source_conversation_id uuid,
			source_message_id uuid,
			source_completed_at timestamptz,
			extractor_version varchar(64) NOT NULL DEFAULT '',
			last_operation_id uuid,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT idx_user_memory_item_key UNIQUE (relationship_id, memory_key),
			CONSTRAINT ck_user_memory_content_encoding CHECK (content_encoding IN ('plain')),
			CONSTRAINT ck_user_memory_source_kind CHECK (source_kind IN ('legacy', 'explicit', 'automatic', 'manager'))
		);
		CREATE INDEX IF NOT EXISTS idx_user_memory_items_relationship_updated
			ON public.user_memory_items (relationship_id, updated_at DESC);

		CREATE TABLE IF NOT EXISTS public.user_memory_events (
			id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
			operation_id uuid,
			relationship_id uuid NOT NULL REFERENCES public.user_memory_relationships(id) ON DELETE RESTRICT,
			memory_key varchar(64) NOT NULL DEFAULT '',
			action varchar(32) NOT NULL,
			actor_type varchar(32) NOT NULL DEFAULT 'system',
			actor_id uuid,
			source varchar(32) NOT NULL DEFAULT 'api',
			source_conversation_id uuid,
			source_message_id uuid,
			before_revision bigint,
			after_revision bigint,
			result varchar(32) NOT NULL DEFAULT 'success',
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_user_memory_events_operation
			ON public.user_memory_events (operation_id) WHERE operation_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_user_memory_events_relationship
			ON public.user_memory_events (relationship_id, created_at DESC);

		CREATE TABLE IF NOT EXISTS public.user_memory_undo_records (
			operation_id uuid PRIMARY KEY,
			relationship_id uuid NOT NULL REFERENCES public.user_memory_relationships(id) ON DELETE RESTRICT,
			memory_key varchar(64) NOT NULL,
			previous_exists boolean NOT NULL DEFAULT false,
			previous_item_id uuid,
			previous_display_name varchar(80) NOT NULL DEFAULT '',
			previous_category varchar(32) NOT NULL DEFAULT '',
			previous_content_payload text NOT NULL DEFAULT '',
			previous_content_encoding varchar(24) NOT NULL DEFAULT 'plain',
			previous_content_key_id varchar(128),
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
		CREATE INDEX IF NOT EXISTS idx_user_memory_undo_relationship
			ON public.user_memory_undo_records (relationship_id, expires_at);

		CREATE TABLE IF NOT EXISTS public.user_memory_extraction_jobs (
			id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
			relationship_id uuid NOT NULL REFERENCES public.user_memory_relationships(id) ON DELETE RESTRICT,
			conversation_id uuid NOT NULL,
			message_watermark_id uuid NOT NULL,
			memory_epoch bigint NOT NULL DEFAULT 0,
			config_revision varchar(64) NOT NULL DEFAULT '',
			definition_snapshot jsonb NOT NULL DEFAULT '[]'::jsonb,
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
		CREATE INDEX IF NOT EXISTS idx_user_memory_jobs_due
			ON public.user_memory_extraction_jobs (status, scheduled_at);
		CREATE INDEX IF NOT EXISTS idx_user_memory_jobs_relationship
			ON public.user_memory_extraction_jobs (relationship_id, created_at DESC);
	`); err != nil {
		return err
	}

	return schema.DataFix("install consumer lifecycle fences and migrate legacy Agent user memory", func(db *gorm.DB) error {
		return db.Exec(`
		-- Consumer lifecycle is enforced in the database so every deletion path,
		-- including maintenance tools, archives relationships without deleting
		-- user-owned memory.
		CREATE OR REPLACE FUNCTION public.archive_user_memory_consumer_relationships()
		RETURNS trigger AS $$
		DECLARE
			consumer_kind varchar(32);
			workspace_value uuid;
		BEGIN
			IF TG_OP = 'UPDATE' AND (OLD.deleted_at IS NOT NULL OR NEW.deleted_at IS NULL) THEN
				RETURN NEW;
			END IF;
			IF TG_TABLE_NAME = 'agents' THEN
				consumer_kind := 'agent';
				workspace_value := OLD.tenant_id;
			ELSE
				consumer_kind := 'digital_human';
				workspace_value := OLD.workspace_id;
			END IF;
			UPDATE public.user_memory_relationships
			SET status = 'archived', archived_at = CURRENT_TIMESTAMP,
				memory_enabled = false, auto_capture_enabled = false,
				memory_epoch = memory_epoch + 1,
				extraction_cutoff_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			WHERE workspace_id = workspace_value AND consumer_type = consumer_kind
				AND consumer_id = OLD.id AND status = 'active';
			UPDATE public.user_memory_extraction_jobs jobs
			SET status = 'cancelled', error_code = 'consumer_archived',
				finished_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			FROM public.user_memory_relationships relationships
			WHERE jobs.relationship_id = relationships.id
				AND relationships.workspace_id = workspace_value
				AND relationships.consumer_type = consumer_kind
				AND relationships.consumer_id = OLD.id
				AND jobs.status IN ('pending', 'queued', 'running', 'failed');
			IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS trg_archive_agent_user_memory ON public.agents;
		CREATE TRIGGER trg_archive_agent_user_memory
		AFTER UPDATE OF deleted_at OR DELETE ON public.agents
		FOR EACH ROW EXECUTE FUNCTION public.archive_user_memory_consumer_relationships();
		DROP TRIGGER IF EXISTS trg_archive_digital_human_user_memory ON public.digital_humans;
		CREATE TRIGGER trg_archive_digital_human_user_memory
		AFTER UPDATE OF deleted_at OR DELETE ON public.digital_humans
		FOR EACH ROW EXECUTE FUNCTION public.archive_user_memory_consumer_relationships();

		-- Expand/backfill. No raw API user value is read or persisted: legacy
		-- user_id already contains the deterministic opaque subject UUID.
		WITH legacy_subjects AS (
			SELECT DISTINCT user_id, user_scope, agent_id FROM public.agent_memory_values
			UNION SELECT DISTINCT user_id, user_scope, agent_id FROM public.agent_memory_subject_states
			UNION SELECT DISTINCT user_id, user_scope, agent_id FROM public.agent_memory_extraction_jobs
			UNION SELECT DISTINCT user_id, user_scope, agent_id FROM public.agent_memory_undo_records
		)
		INSERT INTO public.user_memory_subjects (id, kind, account_id, issuer_type, issuer_id)
		SELECT user_id,
			CASE WHEN user_scope = 'account' THEN 'account' ELSE 'api_end_user' END,
			CASE WHEN user_scope = 'account' THEN user_id ELSE NULL END,
			CASE WHEN user_scope = 'account' THEN '' ELSE 'agent_api' END,
			CASE WHEN user_scope = 'account' THEN NULL ELSE agent_id END
		FROM legacy_subjects
		ON CONFLICT (id) DO NOTHING;

		WITH legacy_relationships AS (
			SELECT DISTINCT workspace_id, agent_id, user_scope, user_id FROM public.agent_memory_values
			UNION SELECT DISTINCT workspace_id, agent_id, user_scope, user_id FROM public.agent_memory_subject_states
			UNION SELECT DISTINCT workspace_id, agent_id, user_scope, user_id FROM public.agent_memory_extraction_jobs
			UNION SELECT DISTINCT workspace_id, agent_id, user_scope, user_id FROM public.agent_memory_undo_records
		), states AS (
			SELECT workspace_id, agent_id, user_scope, user_id, MAX(memory_epoch) AS memory_epoch, MAX(extraction_cutoff_at) AS extraction_cutoff_at
			FROM public.agent_memory_subject_states GROUP BY workspace_id, agent_id, user_scope, user_id
		), published_config AS (
			SELECT DISTINCT ON (agent_id) agent_id, config_snapshot
			FROM public.agent_published_versions
			WHERE deleted_at IS NULL
			ORDER BY agent_id, created_at DESC
		)
		INSERT INTO public.user_memory_relationships (
			organization_id, workspace_id, subject_id, consumer_type, consumer_id,
			consumer_name_snapshot, status, memory_enabled, auto_capture_enabled, memory_epoch,
			extraction_cutoff_at, created_at, updated_at, archived_at
		)
		SELECT w.organization_id::uuid, pair.workspace_id, pair.user_id, 'agent', pair.agent_id,
			COALESCE(a.name, ''),
			CASE WHEN a.id IS NULL OR a.deleted_at IS NOT NULL THEN 'archived' ELSE 'active' END,
			CASE WHEN a.id IS NULL OR a.deleted_at IS NOT NULL THEN false ELSE true END,
			CASE WHEN a.id IS NULL OR a.deleted_at IS NOT NULL THEN false ELSE
			COALESCE(config.config_snapshot->>'agent_memory_enabled', 'false') = 'true'
				AND COALESCE(config.config_snapshot->>'agent_memory_auto_extraction_enabled', 'false') = 'true'
			END,
			COALESCE(state.memory_epoch, 0), state.extraction_cutoff_at,
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP,
			CASE WHEN a.id IS NULL THEN CURRENT_TIMESTAMP ELSE a.deleted_at END
		FROM legacy_relationships pair
		JOIN public.workspaces w ON w.id::text = pair.workspace_id::text
		LEFT JOIN public.agents a ON a.id = pair.agent_id
		LEFT JOIN published_config config ON config.agent_id = pair.agent_id
		LEFT JOIN states state ON state.workspace_id = pair.workspace_id AND state.agent_id = pair.agent_id
			AND state.user_scope = pair.user_scope AND state.user_id = pair.user_id
		WHERE w.organization_id IS NOT NULL
		ON CONFLICT (workspace_id, subject_id, consumer_type, consumer_id) DO NOTHING;

		INSERT INTO public.user_memory_items (
			id, relationship_id, memory_key, display_name, category, content_payload,
			content_encoding, revision, source_kind, source_conversation_id,
			source_message_id, source_completed_at, extractor_version, last_operation_id,
			created_at, updated_at
		)
		SELECT value.id, relationship.id, value.slot_key, COALESCE(slot.name, value.slot_key), '', value.content,
			'plain', value.revision, value.source_kind, value.source_conversation_id,
			value.source_message_id, value.source_completed_at, value.extractor_version, value.last_operation_id,
			value.created_at, value.updated_at
		FROM public.agent_memory_values value
		JOIN public.user_memory_relationships relationship
			ON relationship.workspace_id = value.workspace_id AND relationship.consumer_type = 'agent'
			AND relationship.consumer_id = value.agent_id AND relationship.subject_id = value.user_id
		LEFT JOIN public.agent_memory_slots slot
			ON slot.workspace_id = value.workspace_id AND slot.agent_id = value.agent_id AND slot.key = value.slot_key
		ON CONFLICT DO NOTHING;

		INSERT INTO public.user_memory_events (
			id, operation_id, relationship_id, memory_key, action, actor_type, source,
			source_conversation_id, source_message_id, before_revision, after_revision, result, created_at
		)
		SELECT event.id, event.operation_id, relationship.id, event.slot_key, event.action,
			event.actor_type, event.source, event.source_conversation_id, event.source_message_id,
			event.before_revision, event.after_revision, event.result, event.created_at
		FROM public.agent_memory_events event
		JOIN public.user_memory_relationships relationship
			ON relationship.workspace_id = event.workspace_id AND relationship.consumer_type = 'agent'
			AND relationship.consumer_id = event.agent_id AND relationship.subject_id = event.user_id
		WHERE event.user_id IS NOT NULL AND event.action LIKE 'value%'
		ON CONFLICT DO NOTHING;

		INSERT INTO public.user_memory_undo_records (
			operation_id, relationship_id, memory_key, previous_exists, previous_item_id,
			previous_display_name, previous_category, previous_content_payload,
			previous_content_encoding, previous_revision, previous_source_kind,
			previous_conversation_id, previous_message_id, previous_source_completed_at,
			previous_extractor_version, resulting_revision, expires_at, created_at
		)
		SELECT undo.operation_id, relationship.id, undo.slot_key, undo.previous_exists,
			current_item.id, COALESCE(current_item.display_name, undo.slot_key),
			COALESCE(current_item.category, ''), undo.previous_content, 'plain',
			undo.previous_revision, undo.previous_source_kind,
			undo.previous_conversation_id, undo.previous_message_id, undo.previous_source_completed_at,
			undo.previous_extractor_version, undo.resulting_revision, undo.expires_at, undo.created_at
		FROM public.agent_memory_undo_records undo
		JOIN public.user_memory_relationships relationship
			ON relationship.workspace_id = undo.workspace_id AND relationship.consumer_type = 'agent'
			AND relationship.consumer_id = undo.agent_id AND relationship.subject_id = undo.user_id
		LEFT JOIN public.user_memory_items current_item
			ON current_item.relationship_id = relationship.id AND current_item.memory_key = undo.slot_key
		ON CONFLICT (operation_id) DO NOTHING;

		INSERT INTO public.user_memory_extraction_jobs (
			id, relationship_id, conversation_id, message_watermark_id, memory_epoch,
			config_revision, definition_snapshot, extractor_version, idempotency_key, status,
			attempt_count, error_code, scheduled_at, force_at, started_at, finished_at, created_at, updated_at
		)
		SELECT job.id, relationship.id, job.conversation_id, job.message_watermark_id, job.memory_epoch,
			job.config_revision, job.runtime_slots, job.extractor_version, job.idempotency_key, job.status,
			job.attempt_count, job.error_code, job.scheduled_at, job.force_at, job.started_at, job.finished_at,
			job.created_at, job.updated_at
		FROM public.agent_memory_extraction_jobs job
		JOIN public.user_memory_relationships relationship
			ON relationship.workspace_id = job.workspace_id AND relationship.consumer_type = 'agent'
			AND relationship.consumer_id = job.agent_id AND relationship.subject_id = job.user_id
		ON CONFLICT DO NOTHING;
		`).Error
	})
}

func downCreateUserRelationshipMemories(schema *mschema.Builder) error {
	if err := schema.Raw(`
		DROP TRIGGER IF EXISTS trg_archive_agent_user_memory ON public.agents;
		DROP TRIGGER IF EXISTS trg_archive_digital_human_user_memory ON public.digital_humans;
		DROP FUNCTION IF EXISTS public.archive_user_memory_consumer_relationships();
	`); err != nil {
		return err
	}
	if err := schema.DropIfExists("user_memory_extraction_jobs"); err != nil {
		return err
	}
	if err := schema.DropIfExists("user_memory_undo_records"); err != nil {
		return err
	}
	if err := schema.DropIfExists("user_memory_events"); err != nil {
		return err
	}
	if err := schema.DropIfExists("user_memory_items"); err != nil {
		return err
	}
	if err := schema.DropIfExists("user_memory_relationships"); err != nil {
		return err
	}
	return schema.DropIfExists("user_memory_subjects")
}
