package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// ADR 0016 retires the Agent memory slot system: Agents always use memory
// blocks on the relationship store. This migration brings whatever is still
// only in the legacy Agent value store (written while
// ZGI_RELATIONSHIP_MEMORY_ENABLED was off) into the relationship store, so
// no operator step is needed. It replaces the memory-sync-legacy command and
// runs the same statements, frozen here:
//
//   - subjects and relationships are created for every legacy value;
//   - each relationship keeps the higher memory epoch and the later
//     extraction cutoff, so jobs queued before a legacy deletion stay fenced;
//   - a value in both stores keeps the newer of the two, and a legacy value
//     missing from the relationship store is added with its block definition;
//   - a relationship value the legacy store no longer has is deleted only when
//     the legacy store cleared that user's memory after it was written.
//
// It is idempotent: on a deployment that already used the relationship store
// it changes nothing. The legacy tables keep their rows; no runtime path
// writes them any more.
func init() {
	registerSchemaMigration("202610261300000000_agent_memory_into_relationship_store", upAgentMemoryIntoRelationshipStore, downAgentMemoryIntoRelationshipStore)
}

func upAgentMemoryIntoRelationshipStore(schema *mschema.Builder) error {
	return schema.DataFix("move legacy Agent memory into the relationship store", func(db *gorm.DB) error {
		for _, statement := range []string{
			agentMemoryMoveSubjects, agentMemoryMoveRelationships, agentMemoryMoveFences,
			agentMemoryMoveUpdates, agentMemoryMoveInserts, agentMemoryMoveDeletes,
		} {
			if err := db.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// The move is not reversed: the legacy rows are still there, and the
// relationship store keeps what it received.
func downAgentMemoryIntoRelationshipStore(*mschema.Builder) error { return nil }

const agentMemoryMoveSubjects = `
INSERT INTO public.user_subjects (id, kind, account_id, issuer_type, issuer_id, created_at, updated_at)
SELECT DISTINCT ON (value.user_id) value.user_id,
	CASE WHEN value.user_scope = 'account' THEN 'account' ELSE 'api_end_user' END,
	CASE WHEN value.user_scope = 'account' THEN value.user_id ELSE NULL END,
	CASE WHEN value.user_scope = 'account' THEN '' ELSE 'agent_api' END,
	CASE WHEN value.user_scope = 'account' THEN NULL ELSE value.agent_id END,
	CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM public.agent_memory_values value
ON CONFLICT (id) DO NOTHING`

const agentMemoryMoveRelationships = `
WITH pairs AS (
	SELECT DISTINCT workspace_id, agent_id, user_id FROM public.agent_memory_values
), published_config AS (
	SELECT DISTINCT ON (agent_id) agent_id, config_snapshot
	FROM public.agent_published_versions WHERE deleted_at IS NULL
	ORDER BY agent_id, created_at DESC
)
INSERT INTO public.user_memory_relationships (
	id, organization_id, workspace_id, subject_id, consumer_type, consumer_id, consumer_name_snapshot,
	status, memory_enabled, auto_capture_enabled, memory_epoch, settings_revision, created_at, updated_at, archived_at
)
SELECT gen_random_uuid(), w.organization_id::uuid, pair.workspace_id, pair.user_id, 'agent', pair.agent_id, COALESCE(a.name, ''),
	CASE WHEN a.id IS NULL OR a.deleted_at IS NOT NULL THEN 'archived' ELSE 'active' END,
	a.id IS NOT NULL AND a.deleted_at IS NULL,
	a.id IS NOT NULL AND a.deleted_at IS NULL
		AND COALESCE(config.config_snapshot->>'agent_memory_enabled', 'false') = 'true'
		AND COALESCE(config.config_snapshot->>'agent_memory_auto_extraction_enabled', 'false') = 'true',
	0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP,
	CASE WHEN a.id IS NULL THEN CURRENT_TIMESTAMP ELSE a.deleted_at END
FROM pairs pair
JOIN public.workspaces w ON w.id::text = pair.workspace_id::text
LEFT JOIN public.agents a ON a.id = pair.agent_id
LEFT JOIN published_config config ON config.agent_id = pair.agent_id
WHERE w.organization_id IS NOT NULL
ON CONFLICT (workspace_id, subject_id, consumer_type, consumer_id) DO NOTHING`

const agentMemoryMoveFences = `
UPDATE public.user_memory_relationships relationship
SET memory_epoch = GREATEST(relationship.memory_epoch, state.memory_epoch),
	extraction_cutoff_at = CASE
		WHEN relationship.extraction_cutoff_at IS NULL THEN state.extraction_cutoff_at
		WHEN state.extraction_cutoff_at IS NULL THEN relationship.extraction_cutoff_at
		ELSE GREATEST(relationship.extraction_cutoff_at, state.extraction_cutoff_at) END
FROM public.agent_memory_subject_states state
WHERE relationship.consumer_type = 'agent' AND relationship.consumer_id = state.agent_id
	AND relationship.workspace_id = state.workspace_id AND relationship.subject_id = state.user_id
	AND (state.memory_epoch > relationship.memory_epoch
		OR (state.extraction_cutoff_at IS NOT NULL AND (relationship.extraction_cutoff_at IS NULL OR state.extraction_cutoff_at > relationship.extraction_cutoff_at)))`

// agentMemoryMoveLegacyValues pairs each legacy value with its relationship
// and, when its slot still exists, its block definition.
const agentMemoryMoveLegacyValues = `
SELECT value.*, relationship.id AS relationship_id, slot.name AS slot_name, definition.id AS definition_id
FROM public.agent_memory_values value
JOIN public.user_memory_relationships relationship
	ON relationship.workspace_id = value.workspace_id AND relationship.consumer_type = 'agent'
	AND relationship.consumer_id = value.agent_id AND relationship.subject_id = value.user_id
LEFT JOIN public.agent_memory_slots slot
	ON slot.workspace_id = value.workspace_id AND slot.agent_id = value.agent_id AND slot.key = value.slot_key
LEFT JOIN public.memory_block_definitions definition
	ON definition.consumer_type = 'agent' AND definition.consumer_id = value.agent_id AND definition.label = value.slot_key`

const agentMemoryMoveUpdates = `
WITH legacy AS (` + agentMemoryMoveLegacyValues + `)
UPDATE public.user_memory_items item
SET content_payload = legacy.content, content_encoding = 'plain', content_key_id = NULL,
	revision = GREATEST(item.revision, legacy.revision) + 1, source_kind = legacy.source_kind,
	source_conversation_id = legacy.source_conversation_id, source_message_id = legacy.source_message_id,
	source_completed_at = legacy.source_completed_at, extractor_version = legacy.extractor_version,
	last_operation_id = legacy.last_operation_id, definition_id = COALESCE(item.definition_id, legacy.definition_id),
	updated_at = legacy.updated_at
FROM legacy
WHERE item.relationship_id = legacy.relationship_id AND item.memory_key = legacy.slot_key
	AND legacy.updated_at > item.updated_at AND item.content_payload IS DISTINCT FROM legacy.content`

const agentMemoryMoveInserts = `
WITH legacy AS (` + agentMemoryMoveLegacyValues + `)
INSERT INTO public.user_memory_items (
	id, relationship_id, memory_key, display_name, category, content_payload, content_encoding, revision,
	source_kind, source_conversation_id, source_message_id, source_completed_at, extractor_version,
	last_operation_id, definition_id, created_at, updated_at
)
SELECT gen_random_uuid(), legacy.relationship_id, legacy.slot_key, COALESCE(legacy.slot_name, legacy.slot_key), '',
	legacy.content, 'plain', legacy.revision, legacy.source_kind, legacy.source_conversation_id,
	legacy.source_message_id, legacy.source_completed_at, legacy.extractor_version, legacy.last_operation_id,
	legacy.definition_id, legacy.created_at, legacy.updated_at
FROM legacy
ON CONFLICT (relationship_id, memory_key) DO NOTHING`

const agentMemoryMoveDeletes = `
DELETE FROM public.user_memory_items item
USING public.user_memory_relationships relationship, public.agent_memory_subject_states state
WHERE item.relationship_id = relationship.id AND relationship.consumer_type = 'agent'
	AND state.workspace_id = relationship.workspace_id AND state.agent_id = relationship.consumer_id
	AND state.user_id = relationship.subject_id
	AND state.extraction_cutoff_at IS NOT NULL AND state.extraction_cutoff_at > item.updated_at
	AND NOT EXISTS (
		SELECT 1 FROM public.agent_memory_values value
		WHERE value.workspace_id = relationship.workspace_id AND value.agent_id = relationship.consumer_id
			AND value.user_id = relationship.subject_id AND value.slot_key = item.memory_key)`
