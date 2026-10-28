package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Memory block definitions and version history (ADR 0013). Definitions mirror
// Agent memory slots under a stable ID per consumer and label; values point at
// their definition; every value change is recorded as a commit with one
// version per changed value. History is removed with its relationship.
func init() {
	registerSchemaMigration("202610251000000000_memory_blocks_and_history", upMemoryBlocksAndHistory, downMemoryBlocksAndHistory)
}

func upMemoryBlocksAndHistory(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.memory_block_definitions (
			id uuid PRIMARY KEY,
			consumer_type varchar(32) NOT NULL,
			consumer_id uuid NOT NULL,
			label varchar(64) NOT NULL,
			name varchar(80) NOT NULL DEFAULT '',
			description text NOT NULL DEFAULT '',
			limit_chars integer NOT NULL CHECK (limit_chars > 0),
			partition varchar(16) NOT NULL CHECK (partition IN ('shared', 'conversation', 'person')),
			owner varchar(16) NOT NULL CHECK (owner IN ('user', 'consumer')),
			audience varchar(16) NOT NULL DEFAULT '' CHECK (audience IN ('', 'everyone', 'members')),
			read_only boolean NOT NULL DEFAULT false,
			auto_write boolean NOT NULL DEFAULT true,
			retention_sec bigint,
			enabled boolean NOT NULL DEFAULT true,
			sort_order integer NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL,
			updated_at timestamptz NOT NULL,
			deleted_at timestamptz,
			CONSTRAINT idx_memory_block_definitions_label UNIQUE (consumer_type, consumer_id, label)
		);

		ALTER TABLE public.user_memory_items ADD COLUMN IF NOT EXISTS definition_id uuid
			REFERENCES public.memory_block_definitions(id) ON DELETE SET NULL;
		CREATE INDEX IF NOT EXISTS idx_user_memory_items_definition ON public.user_memory_items (definition_id) WHERE definition_id IS NOT NULL;

		CREATE TABLE IF NOT EXISTS public.memory_commits (
			id uuid PRIMARY KEY,
			relationship_id uuid NOT NULL REFERENCES public.user_memory_relationships(id) ON DELETE CASCADE,
			owner varchar(16) NOT NULL CHECK (owner IN ('user', 'consumer')),
			source_kind varchar(32) NOT NULL,
			actor_type varchar(32) NOT NULL,
			actor_id uuid,
			source varchar(32) NOT NULL,
			operation_id uuid,
			conversation_id uuid,
			message_id uuid,
			message text NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_memory_commits_relationship ON public.memory_commits (relationship_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_memory_commits_created ON public.memory_commits (created_at);

		CREATE TABLE IF NOT EXISTS public.memory_file_versions (
			id uuid PRIMARY KEY,
			commit_id uuid NOT NULL REFERENCES public.memory_commits(id) ON DELETE CASCADE,
			relationship_id uuid NOT NULL REFERENCES public.user_memory_relationships(id) ON DELETE CASCADE,
			definition_id uuid REFERENCES public.memory_block_definitions(id) ON DELETE SET NULL,
			memory_key varchar(64) NOT NULL,
			action varchar(16) NOT NULL CHECK (action IN ('create', 'update', 'delete', 'clear', 'undo')),
			before_revision bigint,
			after_revision bigint,
			before_payload text NOT NULL DEFAULT '',
			before_encoding varchar(24) NOT NULL DEFAULT 'plain' CHECK (before_encoding IN ('plain')),
			before_key_id varchar(128),
			after_payload text NOT NULL DEFAULT '',
			after_encoding varchar(24) NOT NULL DEFAULT 'plain' CHECK (after_encoding IN ('plain')),
			after_key_id varchar(128),
			content_purged_at timestamptz,
			created_at timestamptz NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_memory_file_versions_commit ON public.memory_file_versions (commit_id);
		CREATE INDEX IF NOT EXISTS idx_memory_file_versions_relationship_key ON public.memory_file_versions (relationship_id, memory_key, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_memory_file_versions_unpurged ON public.memory_file_versions (created_at) WHERE content_purged_at IS NULL;
	`)
}

func downMemoryBlocksAndHistory(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TABLE IF EXISTS public.memory_file_versions;
		DROP TABLE IF EXISTS public.memory_commits;
		DROP INDEX IF EXISTS public.idx_user_memory_items_definition;
		ALTER TABLE public.user_memory_items DROP COLUMN IF EXISTS definition_id;
		DROP TABLE IF EXISTS public.memory_block_definitions;
	`)
}
