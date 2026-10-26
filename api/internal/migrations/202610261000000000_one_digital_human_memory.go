package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// ADR 0015 gives a DigitalHuman one memory of its own. Each entry gains a
// visibility (everyone, or internal: never shown to external end users) and
// an origin (operator, member, channel and which one, or end user).
// Workspace and channel memory fold into it as internal, one row at a time
// and oldest first so supersede chains keep passing the scope triggers, each
// with an audit event. A key the DigitalHuman's own memory already uses gets
// a suffix naming the old scope.
//
// Rows of a deleted DigitalHuman cannot be updated, so they stay as they
// are; when any remain, the new checks are added NOT VALID, which still holds
// every new and changed row.
func init() {
	registerSchemaMigration("202610261000000000_one_digital_human_memory", upOneDigitalHumanMemory, downOneDigitalHumanMemory)
}

func upOneDigitalHumanMemory(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.digital_human_memories ADD COLUMN IF NOT EXISTS origin varchar(16) NOT NULL DEFAULT 'operator';
		ALTER TABLE public.digital_human_memories ADD COLUMN IF NOT EXISTS origin_channel_id varchar(255) NOT NULL DEFAULT '';
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_visibility_check;
		ALTER TABLE public.digital_human_memories ALTER COLUMN visibility SET DEFAULT 'everyone';
	`); err != nil {
		return err
	}
	if err := schema.DataFix("fold workspace and channel memory into the DigitalHuman's own memory", func(db *gorm.DB) error {
		return db.Exec(oneDigitalHumanMemorySQL).Error
	}); err != nil {
		return err
	}
	return schema.Raw(`
		DO $$
		DECLARE
			stranded boolean;
		BEGIN
			ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_scope_check;
			stranded := EXISTS (
				SELECT 1 FROM public.digital_human_memories
				WHERE visibility NOT IN ('everyone', 'internal')
				   OR (memory_class = 'business' AND scope_type <> 'instance')
			);
			IF stranded THEN
				ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_visibility_check CHECK (visibility IN ('everyone', 'internal')) NOT VALID;
				ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_scope_check CHECK (
					(memory_class = 'self' AND scope_type = '' AND scope_id = '') OR
					(memory_class = 'business' AND scope_type = 'instance' AND scope_id <> '')
				) NOT VALID;
			ELSE
				ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_visibility_check CHECK (visibility IN ('everyone', 'internal'));
				ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_scope_check CHECK (
					(memory_class = 'self' AND scope_type = '' AND scope_id = '') OR
					(memory_class = 'business' AND scope_type = 'instance' AND scope_id <> '')
				);
			END IF;
			ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_origin_check;
			ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_origin_check
				CHECK (origin IN ('operator', 'member', 'channel', 'end_user'));
		END $$;
	`)
}

const oneDigitalHumanMemorySQL = `
		DO $$
		DECLARE
			moving record;
			target_key varchar(160);
			moved jsonb;
		BEGIN
			UPDATE public.digital_human_memories m
			SET visibility = CASE WHEN m.memory_class = 'business' OR m.visibility = 'restricted' THEN 'internal' ELSE 'everyone' END,
			    origin = CASE WHEN m.scope_type = 'instance' THEN 'end_user' WHEN m.scope_type = 'channel' THEN 'channel' ELSE 'operator' END
			FROM public.digital_humans d
			WHERE d.id = m.digital_human_id AND d.deleted_at IS NULL AND m.visibility IN ('workspace', 'restricted');

			FOR moving IN
				SELECT m.*
				FROM public.digital_human_memories m
				JOIN public.digital_humans d ON d.id = m.digital_human_id AND d.deleted_at IS NULL
				WHERE m.memory_class = 'business' AND m.scope_type IN ('workspace', 'channel')
				ORDER BY m.created_at, m.id
			LOOP
				target_key := moving.memory_key;
				IF EXISTS (
					SELECT 1 FROM public.digital_human_memories s
					WHERE s.organization_id = moving.organization_id AND s.workspace_id = moving.workspace_id
					  AND s.digital_human_id = moving.digital_human_id AND s.memory_class = 'self' AND s.memory_key = target_key
				) THEN
					target_key := left(moving.memory_key, 130) || '-' || moving.scope_type || '-' || left(md5(moving.scope_id), 8);
					IF EXISTS (
						SELECT 1 FROM public.digital_human_memories s
						WHERE s.organization_id = moving.organization_id AND s.workspace_id = moving.workspace_id
						  AND s.digital_human_id = moving.digital_human_id AND s.memory_class = 'self' AND s.memory_key = target_key
					) THEN
						target_key := left(moving.memory_key, 130) || '-' || left(moving.id::text, 8);
					END IF;
				END IF;
				UPDATE public.digital_human_memories
				SET memory_class = 'self', scope_type = '', scope_id = '', memory_key = target_key, visibility = 'internal',
				    origin = CASE WHEN moving.scope_type = 'channel' THEN 'channel' ELSE 'operator' END,
				    origin_channel_id = CASE WHEN moving.scope_type = 'channel' THEN moving.scope_id ELSE '' END,
				    revision = moving.revision + 1, updated_at = CURRENT_TIMESTAMP
				WHERE id = moving.id
				RETURNING to_jsonb(public.digital_human_memories.*) INTO moved;
				INSERT INTO public.digital_human_memory_events (
					operation_id, organization_id, workspace_id, digital_human_id, memory_id, action, result,
					before_revision, after_revision, before_snapshot, after_snapshot, evidence, source_principal_id, reason_code
				) VALUES (
					uuid_generate_v4(), moving.organization_id, moving.workspace_id, moving.digital_human_id, moving.id, 'upsert', 'committed',
					moving.revision, moving.revision + 1, to_jsonb(moving), moved, '[]'::jsonb, 'system:migration', 'merged_into_digital_human_memory'
				);
			END LOOP;
		END $$;
`

// The down migration restores the earlier checks and visibility names.
// Folded rows stay in the DigitalHuman's own memory, as their audit events
// record.
func downOneDigitalHumanMemory(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_origin_check;
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_visibility_check;
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_scope_check;
		ALTER TABLE public.digital_human_memories ALTER COLUMN visibility SET DEFAULT 'workspace';
	`); err != nil {
		return err
	}
	if err := schema.DataFix("restore the earlier memory visibility names", func(db *gorm.DB) error {
		return db.Exec(`
			UPDATE public.digital_human_memories m
			SET visibility = CASE WHEN m.visibility = 'internal' THEN 'restricted' ELSE 'workspace' END
			FROM public.digital_humans d
			WHERE d.id = m.digital_human_id AND d.deleted_at IS NULL AND m.visibility IN ('everyone', 'internal')`).Error
	}); err != nil {
		return err
	}
	return schema.Raw(`
		ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_visibility_check CHECK (visibility IN ('workspace', 'restricted')) NOT VALID;
		ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_scope_check CHECK (
			(memory_class = 'self' AND scope_type = '' AND scope_id = '') OR
			(memory_class = 'business' AND scope_type IN ('workspace', 'channel', 'instance') AND scope_id <> '')
		);
		ALTER TABLE public.digital_human_memories DROP COLUMN IF EXISTS origin_channel_id;
		ALTER TABLE public.digital_human_memories DROP COLUMN IF EXISTS origin;
	`)
}
