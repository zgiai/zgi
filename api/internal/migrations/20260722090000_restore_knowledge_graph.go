package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migrationRestoreKnowledgeGraphID = "20260722090000_restore_knowledge_graph"
	restoreKnowledgeGraphSQL         = `
		ALTER TABLE public.datasets
			ADD COLUMN IF NOT EXISTS graph_status varchar(32) NOT NULL DEFAULT 'disabled',
			ADD COLUMN IF NOT EXISTS graph_revision bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS graph_available_revision bigint,
			ADD COLUMN IF NOT EXISTS graph_projected_revision bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS graph_visibility_revision bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS graph_projected_visibility_revision bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS graph_current_run_id uuid,
			ADD COLUMN IF NOT EXISTS graph_progress integer NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS graph_error_code varchar(128),
			ADD COLUMN IF NOT EXISTS graph_error_message text,
			ADD COLUMN IF NOT EXISTS graph_ready_at timestamptz,
			ADD COLUMN IF NOT EXISTS graph_updated_at timestamptz;

		ALTER TABLE public.data_library_knowledge_base_asset_refs
			ADD COLUMN IF NOT EXISTS retrieval_enabled boolean NOT NULL DEFAULT true,
			ADD COLUMN IF NOT EXISTS graph_run_id uuid,
			ADD COLUMN IF NOT EXISTS graph_sync_status varchar(32);

		CREATE TABLE IF NOT EXISTS public.graphflow_runs (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			organization_id uuid NOT NULL,
			workspace_id uuid,
			dataset_id uuid NOT NULL REFERENCES public.datasets(id) ON DELETE CASCADE,
			document_id uuid,
			source_ref_id uuid,
			sync_run_id uuid,
			asset_generation_no bigint,
			graph_revision bigint NOT NULL,
			embedding_model_provider varchar(255) NOT NULL DEFAULT '',
			embedding_model varchar(255) NOT NULL DEFAULT '',
			embedding_dimension integer NOT NULL DEFAULT 0,
			embedding_fingerprint varchar(512) NOT NULL DEFAULT '',
			trigger varchar(32) NOT NULL,
			mode varchar(32) NOT NULL,
			status varchar(32) NOT NULL DEFAULT 'pending',
			progress integer NOT NULL DEFAULT 0,
			idempotency_key varchar(255) NOT NULL,
			error_code varchar(128),
			error_message text,
			attempt_count integer NOT NULL DEFAULT 0,
			lease_expires_at timestamptz,
			heartbeat_at timestamptz,
			started_at timestamptz,
			finished_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_graphflow_runs_dataset_idempotency
			ON public.graphflow_runs (dataset_id, idempotency_key);
		CREATE INDEX IF NOT EXISTS idx_graphflow_runs_dataset_status
			ON public.graphflow_runs (dataset_id, status, created_at);
		CREATE INDEX IF NOT EXISTS idx_graphflow_runs_source_document
			ON public.graphflow_runs (source_ref_id, document_id);
		CREATE INDEX IF NOT EXISTS idx_graphflow_runs_sync_run
			ON public.graphflow_runs (sync_run_id);

		CREATE TABLE IF NOT EXISTS public.graph_outbox_events (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			organization_id uuid NOT NULL,
			workspace_id uuid,
			dataset_id uuid NOT NULL REFERENCES public.datasets(id) ON DELETE CASCADE,
			run_id uuid REFERENCES public.graphflow_runs(id) ON DELETE SET NULL,
			event_type varchar(64) NOT NULL,
			aggregate_key varchar(512) NOT NULL,
			payload jsonb NOT NULL DEFAULT '{}'::jsonb,
			status varchar(32) NOT NULL DEFAULT 'pending',
			attempt_count integer NOT NULL DEFAULT 0,
			available_at timestamptz NOT NULL DEFAULT now(),
			claimed_at timestamptz,
			confirmed_at timestamptz,
			error_message text,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_graph_outbox_active_aggregate
			ON public.graph_outbox_events (event_type, aggregate_key)
			WHERE status IN ('pending', 'processing');
		CREATE INDEX IF NOT EXISTS idx_graph_outbox_claim
			ON public.graph_outbox_events (status, available_at, created_at);

		ALTER TABLE public.graphflow_tasks
			ADD COLUMN IF NOT EXISTS run_id uuid,
			ADD COLUMN IF NOT EXISTS attempt_no integer NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS lease_expires_at timestamptz,
			ADD COLUMN IF NOT EXISTS heartbeat_at timestamptz,
			ADD COLUMN IF NOT EXISTS error_code varchar(128);
		DROP INDEX IF EXISTS public.idx_graphflow_tasks_run_type;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_graphflow_tasks_run_type
			ON public.graphflow_tasks (run_id, document_id, task_type)
			WHERE run_id IS NOT NULL;

		ALTER TABLE public.kb_entity_mentions
			ADD COLUMN IF NOT EXISTS organization_id uuid,
			ADD COLUMN IF NOT EXISTS source_ref_id uuid,
			ADD COLUMN IF NOT EXISTS document_id uuid,
			ADD COLUMN IF NOT EXISTS run_id uuid,
			ADD COLUMN IF NOT EXISTS evidence_fingerprint varchar(128) NOT NULL DEFAULT '';
		CREATE UNIQUE INDEX IF NOT EXISTS idx_entity_mentions_document_evidence
			ON public.kb_entity_mentions (kb_id, document_id, segment_id, evidence_fingerprint)
			WHERE is_deleted = false AND document_id IS NOT NULL AND evidence_fingerprint <> '';

		ALTER TABLE public.kb_triple_mentions
			ADD COLUMN IF NOT EXISTS organization_id uuid,
			ADD COLUMN IF NOT EXISTS source_ref_id uuid,
			ADD COLUMN IF NOT EXISTS document_id uuid,
			ADD COLUMN IF NOT EXISTS run_id uuid,
			ADD COLUMN IF NOT EXISTS relationship_id uuid,
			ADD COLUMN IF NOT EXISTS evidence_fingerprint varchar(128) NOT NULL DEFAULT '';
		CREATE UNIQUE INDEX IF NOT EXISTS idx_triple_mentions_document_evidence
			ON public.kb_triple_mentions (kb_id, document_id, segment_id, evidence_fingerprint)
			WHERE is_deleted = false AND document_id IS NOT NULL AND evidence_fingerprint <> '';

		UPDATE public.kb_entity_mentions AS mention
		SET document_id = segment.document_id,
			organization_id = segment.organization_id
		FROM public.document_segments AS segment
		WHERE mention.segment_id = segment.id AND mention.document_id IS NULL;
		UPDATE public.kb_triple_mentions AS mention
		SET document_id = segment.document_id,
			organization_id = segment.organization_id
		FROM public.document_segments AS segment
		WHERE mention.segment_id = segment.id AND mention.document_id IS NULL;
		UPDATE public.kb_entity_mentions AS mention
		SET source_ref_id = ref.id
		FROM public.data_library_knowledge_base_asset_refs AS ref
		WHERE mention.document_id = ref.dataset_document_id AND mention.source_ref_id IS NULL AND ref.deleted_at IS NULL;
		UPDATE public.kb_triple_mentions AS mention
		SET source_ref_id = ref.id
		FROM public.data_library_knowledge_base_asset_refs AS ref
		WHERE mention.document_id = ref.dataset_document_id AND mention.source_ref_id IS NULL AND ref.deleted_at IS NULL;
		UPDATE public.kb_triple_mentions AS mention
		SET relationship_id = relationship.id
		FROM public.kb_relationships AS relationship
		WHERE mention.relationship_id IS NULL
			AND mention.kb_id = relationship.kb_id
			AND mention.head_entity_id = relationship.head_entity_id
			AND mention.tail_entity_id = relationship.tail_entity_id
			AND mention.raw_predicate = relationship.relation_type;

		ALTER TABLE public.kb_entities
			ADD COLUMN IF NOT EXISTS active_source_count integer NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS embedding_model_provider varchar(255) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS embedding_model varchar(255) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS embedding_dimension integer NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS embedding_fingerprint varchar(512) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS content_revision bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS visibility_revision bigint NOT NULL DEFAULT 0;

		ALTER TABLE public.kb_relationships
			ADD COLUMN IF NOT EXISTS active_weight integer NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS content_revision bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS visibility_revision bigint NOT NULL DEFAULT 0;

		UPDATE public.datasets
		SET graph_status = CASE WHEN enable_graph_flow THEN 'waiting_content' ELSE 'disabled' END
		WHERE graph_status = 'disabled' AND enable_graph_flow = true;
		UPDATE public.data_library_knowledge_base_asset_refs
		SET retrieval_enabled = (status <> 'disabled')
		WHERE retrieval_enabled IS DISTINCT FROM (status <> 'disabled');

		UPDATE public.kb_entities AS entity
		SET active_source_count = evidence.active_source_count,
			source_count = evidence.source_count
		FROM (
			SELECT mention.entity_id,
				COUNT(DISTINCT mention.source_ref_id)::integer AS source_count,
				COUNT(DISTINCT mention.source_ref_id) FILTER (
					WHERE ref.retrieval_enabled = true
						AND ref.dataset_document_id = mention.document_id
						AND ref.deleted_at IS NULL
				)::integer AS active_source_count
			FROM public.kb_entity_mentions AS mention
			LEFT JOIN public.data_library_knowledge_base_asset_refs AS ref ON ref.id = mention.source_ref_id
			WHERE mention.is_deleted = false AND mention.entity_id IS NOT NULL
			GROUP BY mention.entity_id
		) AS evidence
		WHERE entity.id = evidence.entity_id;

		UPDATE public.kb_relationships AS relationship
		SET active_weight = evidence.active_source_count,
			weight = evidence.source_count
		FROM (
			SELECT mention.relationship_id,
				COUNT(DISTINCT mention.source_ref_id)::integer AS source_count,
				COUNT(DISTINCT mention.source_ref_id) FILTER (
					WHERE ref.retrieval_enabled = true
						AND ref.dataset_document_id = mention.document_id
						AND ref.deleted_at IS NULL
				)::integer AS active_source_count
			FROM public.kb_triple_mentions AS mention
			LEFT JOIN public.data_library_knowledge_base_asset_refs AS ref ON ref.id = mention.source_ref_id
			WHERE mention.is_deleted = false AND mention.relationship_id IS NOT NULL
			GROUP BY mention.relationship_id
		) AS evidence
		WHERE relationship.id = evidence.relationship_id;

		UPDATE public.kb_entities AS entity
		SET embedding_model_provider = dataset.embedding_model_provider,
			embedding_model = dataset.embedding_model,
			embedding_fingerprint = concat_ws(':', dataset.embedding_model_provider, dataset.embedding_model, entity.embedding_dimension),
			vector_state = CASE
				WHEN entity.embedding_model_provider <> '' AND (
					entity.embedding_model_provider IS DISTINCT FROM dataset.embedding_model_provider
					OR entity.embedding_model IS DISTINCT FROM dataset.embedding_model
				) THEN 'pending'
				ELSE entity.vector_state
			END
		FROM public.datasets AS dataset
		WHERE entity.kb_id = dataset.id
			AND dataset.embedding_model_provider IS NOT NULL
			AND dataset.embedding_model IS NOT NULL;

		UPDATE public.datasets
		SET graph_status = 'waiting_content',
			graph_progress = 0,
			graph_error_code = NULL,
			graph_error_message = NULL,
			graph_updated_at = now()
		WHERE enable_graph_flow = true;
	`
)

func init() {
	registerSchemaMigration(
		migrationRestoreKnowledgeGraphID,
		func(schema *mschema.Builder) error { return schema.Raw(restoreKnowledgeGraphSQL) },
		func(schema *mschema.Builder) error {
			return schema.AllowDestructive().Raw(`
				DROP INDEX IF EXISTS public.idx_triple_mentions_document_evidence;
				DROP INDEX IF EXISTS public.idx_entity_mentions_document_evidence;
				DROP INDEX IF EXISTS public.idx_graphflow_tasks_run_type;
				DROP TABLE IF EXISTS public.graph_outbox_events;
				DROP TABLE IF EXISTS public.graphflow_runs
			`)
		},
	)
}
