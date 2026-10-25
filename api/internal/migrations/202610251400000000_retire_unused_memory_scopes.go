package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// ADR 0014 retires the project, customer, and campaign business scopes: the
// runtime never resolved them. Any rows in them move to their workspace's
// business scope, each with an audit event, so nothing is lost. Rows move one
// at a time, oldest first, so a superseded row always moves before the row
// that supersedes it and the scope triggers hold. A key already used in the
// workspace scope gets a suffix naming the old scope.
//
// Rows of a deleted DigitalHuman cannot be updated, so they stay where they
// are; when any remain, the narrower check is added NOT VALID, which still
// holds every new and changed row.
func init() {
	registerSchemaMigration("202610251400000000_retire_unused_memory_scopes", upRetireUnusedMemoryScopes, downRetireUnusedMemoryScopes)
}

func upRetireUnusedMemoryScopes(schema *mschema.Builder) error {
	if err := schema.DataFix("move memory in retired scopes to the workspace scope", func(db *gorm.DB) error {
		return db.Exec(retireUnusedMemoryScopesSQL).Error
	}); err != nil {
		return err
	}
	return schema.Raw(`
		DO $$
		BEGIN
			ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_scope_check;
			IF EXISTS (SELECT 1 FROM public.digital_human_memories WHERE scope_type IN ('project', 'customer', 'campaign')) THEN
				ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_scope_check CHECK (
					(memory_class = 'self' AND scope_type = '' AND scope_id = '') OR
					(memory_class = 'business' AND scope_type IN ('workspace', 'channel', 'instance') AND scope_id <> '')
				) NOT VALID;
			ELSE
				ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_scope_check CHECK (
					(memory_class = 'self' AND scope_type = '' AND scope_id = '') OR
					(memory_class = 'business' AND scope_type IN ('workspace', 'channel', 'instance') AND scope_id <> '')
				);
			END IF;
		END $$;
	`)
}

const retireUnusedMemoryScopesSQL = `
		DO $$
		DECLARE
			moving record;
			target_key varchar(160);
			moved jsonb;
		BEGIN
			FOR moving IN
				SELECT m.*
				FROM public.digital_human_memories m
				JOIN public.digital_humans d ON d.id = m.digital_human_id AND d.deleted_at IS NULL
				WHERE m.memory_class = 'business' AND m.scope_type IN ('project', 'customer', 'campaign')
				ORDER BY m.created_at, m.id
			LOOP
				target_key := moving.memory_key;
				IF EXISTS (
					SELECT 1 FROM public.digital_human_memories w
					WHERE w.organization_id = moving.organization_id AND w.workspace_id = moving.workspace_id
					  AND w.digital_human_id = moving.digital_human_id AND w.memory_class = 'business'
					  AND w.scope_type = 'workspace' AND w.scope_id = moving.workspace_id::text AND w.memory_key = target_key
				) THEN
					target_key := left(moving.memory_key, 130) || '-' || moving.scope_type || '-' || left(md5(moving.scope_id), 8);
					IF EXISTS (
						SELECT 1 FROM public.digital_human_memories w
						WHERE w.organization_id = moving.organization_id AND w.workspace_id = moving.workspace_id
						  AND w.digital_human_id = moving.digital_human_id AND w.memory_class = 'business'
						  AND w.scope_type = 'workspace' AND w.scope_id = moving.workspace_id::text AND w.memory_key = target_key
					) THEN
						target_key := left(moving.memory_key, 130) || '-' || left(moving.id::text, 8);
					END IF;
				END IF;
				UPDATE public.digital_human_memories
				SET scope_type = 'workspace', scope_id = moving.workspace_id::text, memory_key = target_key,
				    revision = moving.revision + 1, updated_at = CURRENT_TIMESTAMP
				WHERE id = moving.id
				RETURNING to_jsonb(public.digital_human_memories.*) INTO moved;
				INSERT INTO public.digital_human_memory_events (
					operation_id, organization_id, workspace_id, digital_human_id, memory_id, action, result,
					before_revision, after_revision, before_snapshot, after_snapshot, evidence, source_principal_id, reason_code
				) VALUES (
					uuid_generate_v4(), moving.organization_id, moving.workspace_id, moving.digital_human_id, moving.id, 'upsert', 'committed',
					moving.revision, moving.revision + 1, to_jsonb(moving), moved, '[]'::jsonb, 'system:migration', 'scope_retired_to_workspace'
				);
			END LOOP;
		END $$;
`

// The down migration widens the check again; moved rows stay in their
// workspace scope, as their audit events record.
func downRetireUnusedMemoryScopes(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_scope_check;
		ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_scope_check CHECK (
			(memory_class = 'self' AND scope_type = '' AND scope_id = '') OR
			(memory_class = 'business' AND scope_type IN ('workspace', 'project', 'customer', 'campaign', 'channel', 'instance') AND scope_id <> '')
		);
	`)
}
