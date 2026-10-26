package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// ADR 0015 gives a DigitalHuman one memory of its own and keeps its notes on
// an external end user in that person's relationship memory, so the memory
// classes and scopes go away:
//
//   - Each entry gains a visibility (everyone, or internal: never shown to
//     external end users) and an origin (operator, member, channel and which
//     one, or end user).
//   - Workspace, project, customer, campaign and channel memory fold into
//     the DigitalHuman's own memory as internal, one row at a time and oldest
//     first, each with an audit event. A key the DigitalHuman's own memory
//     already uses gets a suffix naming the old scope.
//   - Instance memory moves to the person's relationship memory: active rows
//     become notes under notes/, archived rows go to the person's memory
//     archive, and invalidated rows are dropped. The moved rows leave with
//     their events, undo records and ledger rows, under the end-user erasure
//     guard.
//   - Rows of a deleted DigitalHuman, or of an instance that no longer
//     exists, stay in its memory as internal entries.
//
// Then the class and scope checks, key and indexes are dropped and the
// memory key is unique per DigitalHuman. Up migrations never drop columns, so
// memory_class, scope_type and scope_id stay behind as retired columns with
// defaults; no code reads or writes them.
func init() {
	registerSchemaMigration("202610261200000000_digital_human_memory_without_scopes", upDigitalHumanMemoryWithoutScopes, downDigitalHumanMemoryWithoutScopes)
}

func upDigitalHumanMemoryWithoutScopes(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.digital_human_memories ADD COLUMN IF NOT EXISTS origin varchar(16) NOT NULL DEFAULT 'operator';
		ALTER TABLE public.digital_human_memories ADD COLUMN IF NOT EXISTS origin_channel_id varchar(255) NOT NULL DEFAULT '';
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_visibility_check;
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_origin_check;
		ALTER TABLE public.digital_human_memories ALTER COLUMN visibility SET DEFAULT 'everyone';
	`); err != nil {
		return err
	}
	if err := schema.DataFix("fold scoped memory into the DigitalHuman's own memory", func(db *gorm.DB) error {
		return db.Exec(foldDigitalHumanMemoryScopesSQL).Error
	}); err != nil {
		return err
	}
	if err := schema.DataFix("move instance memory into relationship memory as notes", func(db *gorm.DB) error {
		return db.Exec(moveInstanceMemoryToNotesSQL).Error
	}); err != nil {
		return err
	}
	if err := schema.Raw(digitalHumanMemoryFunctionsWithoutScopesSQL); err != nil {
		return err
	}
	if err := schema.DataFix("settle the memory rows that could not move", func(db *gorm.DB) error {
		return db.Exec(strandedDigitalHumanMemorySQL).Error
	}); err != nil {
		return err
	}
	return schema.Raw(`
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_scope_check;
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_class_check;
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_identity_check;
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_subject_key_unique;
		DROP INDEX IF EXISTS public.idx_digital_human_memories_retrieval;
		DROP INDEX IF EXISTS public.idx_digital_human_memories_instance;
		ALTER TABLE public.digital_human_memories ALTER COLUMN memory_class SET DEFAULT 'self';
		ALTER TABLE public.digital_human_memories ALTER COLUMN scope_type SET DEFAULT '';
		ALTER TABLE public.digital_human_memories ALTER COLUMN scope_id SET DEFAULT '';
		ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_subject_key_unique
			UNIQUE (organization_id, workspace_id, digital_human_id, memory_key);
		CREATE INDEX IF NOT EXISTS idx_digital_human_memories_retrieval
			ON public.digital_human_memories (organization_id, workspace_id, digital_human_id, status, updated_at DESC);
		ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_visibility_check CHECK (visibility IN ('everyone', 'internal'));
		ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_origin_check
			CHECK (origin IN ('operator', 'member', 'channel', 'end_user'));
	`)
}

const foldDigitalHumanMemoryScopesSQL = `
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
				WHERE m.memory_class = 'business' AND m.scope_type <> 'instance'
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

const moveInstanceMemoryToNotesSQL = `
		DO $$
		DECLARE
			note record;
			target_relationship uuid;
			note_key varchar(64);
		BEGIN
			FOR note IN
				SELECT m.*, i.subject_id, i.organization_id AS instance_organization_id, i.workspace_id AS instance_workspace_id, d.name AS digital_human_name
				FROM public.digital_human_memories m
				JOIN public.digital_human_end_user_instances i ON i.id::text = m.scope_id AND i.digital_human_id = m.digital_human_id
				JOIN public.digital_humans d ON d.id = m.digital_human_id AND d.deleted_at IS NULL
				WHERE m.memory_class = 'business' AND m.scope_type = 'instance'
				ORDER BY m.created_at, m.id
			LOOP
				IF note.status IN ('active', 'archived') THEN
					INSERT INTO public.user_memory_relationships (organization_id, workspace_id, subject_id, consumer_type, consumer_id, consumer_name_snapshot)
					VALUES (note.instance_organization_id, note.instance_workspace_id, note.subject_id, 'digital_human', note.digital_human_id, note.digital_human_name)
					ON CONFLICT (workspace_id, subject_id, consumer_type, consumer_id) DO NOTHING;
					SELECT r.id INTO target_relationship FROM public.user_memory_relationships r
					WHERE r.workspace_id = note.instance_workspace_id AND r.subject_id = note.subject_id
					  AND r.consumer_type = 'digital_human' AND r.consumer_id = note.digital_human_id;

					note_key := 'notes/' || left(replace(note.memory_key, '/', '_'), 58);
					IF note.status = 'active' THEN
						IF EXISTS (SELECT 1 FROM public.user_memory_items WHERE user_memory_items.relationship_id = target_relationship AND memory_key = note_key) THEN
							note_key := 'notes/' || left(replace(note.memory_key, '/', '_'), 49) || '-' || left(note.id::text, 8);
						END IF;
						INSERT INTO public.user_memory_items (id, relationship_id, memory_key, display_name, category, content_payload, content_encoding,
							revision, source_kind, created_at, updated_at)
						VALUES (uuid_generate_v4(), target_relationship, note_key, left(note.memory_key, 80), note.memory_type, note.content, 'plain',
							1, 'automatic', note.created_at, note.updated_at);
					ELSE
						INSERT INTO public.user_memory_archive_entries (id, relationship_id, tags, source_key, source_kind, content_payload, content_encoding, created_at, updated_at)
						VALUES (uuid_generate_v4(), target_relationship, note.memory_type, note_key, 'automatic', note.content, 'plain', note.updated_at, note.updated_at);
					END IF;
				END IF;

				-- The projection segment loses its text now; the next projection
				-- sync removes it and its vector.
				UPDATE public.document_segments SET content = '', word_count = 0, tokens = 0, index_node_hash = NULL WHERE id::text = note.id::text;
				-- The memory events of the moved note may be deleted only under
				-- the end-user erasure guard.
				PERFORM set_config('zgi.digital_human_end_user_erase', note.subject_id::text, true);
				DELETE FROM public.digital_human_memory_undo_records WHERE memory_id = note.id;
				DELETE FROM public.digital_human_action_ledger WHERE memory_id = note.id;
				DELETE FROM public.digital_human_memory_events WHERE memory_id = note.id;
				UPDATE public.digital_human_memories SET supersedes_id = NULL WHERE supersedes_id = note.id;
				DELETE FROM public.digital_human_memories WHERE id = note.id;
			END LOOP;
			PERFORM set_config('zgi.digital_human_end_user_erase', '', true);
		END $$;
`

// strandedDigitalHumanMemorySQL settles the rows that could not move: those
// of a deleted DigitalHuman, whose rows the scope trigger otherwise freezes,
// and instance memory whose instance no longer exists. A key another row of
// the DigitalHuman uses gets a suffix naming the old scope, then each becomes
// an internal entry of the DigitalHuman's own memory, so the new checks and
// key hold every row.
const strandedDigitalHumanMemorySQL = `
		ALTER TABLE public.digital_human_memories DISABLE TRIGGER trg_digital_human_memories_scope;
		UPDATE public.digital_human_memories m
		SET memory_key = left(m.memory_key, 130) || '-' || m.scope_type || '-' || left(m.id::text, 8)
		WHERE m.memory_class = 'business' AND EXISTS (
			SELECT 1 FROM public.digital_human_memories other
			WHERE other.organization_id = m.organization_id AND other.workspace_id = m.workspace_id
			  AND other.digital_human_id = m.digital_human_id AND other.memory_key = m.memory_key AND other.id <> m.id
		);
		UPDATE public.digital_human_memories m
		SET visibility = CASE WHEN m.memory_class = 'business' OR m.visibility IN ('restricted', 'internal') THEN 'internal' ELSE 'everyone' END,
		    origin = CASE WHEN m.scope_type = 'instance' THEN 'end_user' WHEN m.scope_type = 'channel' THEN 'channel' ELSE m.origin END,
		    origin_channel_id = CASE WHEN m.scope_type = 'channel' THEN m.scope_id ELSE m.origin_channel_id END,
		    status = CASE WHEN m.scope_type = 'instance' THEN 'invalidated' ELSE m.status END,
		    memory_class = 'self', scope_type = '', scope_id = ''
		WHERE m.memory_class = 'business' OR m.visibility NOT IN ('everyone', 'internal');
		ALTER TABLE public.digital_human_memories ENABLE TRIGGER trg_digital_human_memories_scope;
`

// Without scopes a superseded entry need only belong to the same
// DigitalHuman, and the end-user erasure guard no longer deletes memory
// events: an end user's notes live in relationship memory.
const digitalHumanMemoryFunctionsWithoutScopesSQL = `
CREATE OR REPLACE FUNCTION public.enforce_digital_human_memory_supersedes_scope() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		-- A move rewrites every memory of the DigitalHuman, so a superseded
		-- row may not have moved yet when its successor is checked.
		IF TG_OP = 'UPDATE' AND public.digital_human_workspace_move_allows(OLD.digital_human_id, to_jsonb(OLD), to_jsonb(NEW)) THEN
			RETURN NEW;
		END IF;
		IF NEW.supersedes_id IS NOT NULL AND NOT EXISTS (
			SELECT 1
			FROM public.digital_human_memories prior
			WHERE prior.id = NEW.supersedes_id
			  AND prior.id <> NEW.id
			  AND prior.organization_id = NEW.organization_id
			  AND prior.workspace_id = NEW.workspace_id
			  AND prior.digital_human_id = NEW.digital_human_id
		) THEN
			RAISE EXCEPTION 'digital human superseded memory scope mismatch' USING ERRCODE = '23514';
		END IF;
		RETURN NEW;
	END;
$$;

CREATE OR REPLACE FUNCTION public.digital_human_end_user_erase_allows(old_row public.digital_human_memory_events, new_row public.digital_human_memory_events, operation text)
RETURNS boolean LANGUAGE sql STABLE AS $$
	SELECT COALESCE(current_setting('zgi.digital_human_end_user_erase', true), '') <> ''
		AND operation = 'UPDATE' AND old_row.run_id IS NOT NULL AND new_row.run_id IS NULL
		AND (to_jsonb(old_row) - 'run_id') = (to_jsonb(new_row) - 'run_id')
		AND EXISTS (
			SELECT 1 FROM public.digital_human_runs run
			WHERE run.id = old_row.run_id
				AND run.user_subject_id::text = current_setting('zgi.digital_human_end_user_erase', true));
$$;
`

// The down migration restores the class and scope checks, key, indexes and
// functions, and the earlier visibility names. Every row stays the
// DigitalHuman's own memory; the moves themselves are not reversed, as their
// audit events record.
func downDigitalHumanMemoryWithoutScopes(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_origin_check;
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_visibility_check;
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_subject_key_unique;
		DROP INDEX IF EXISTS public.idx_digital_human_memories_retrieval;
		ALTER TABLE public.digital_human_memories ALTER COLUMN memory_class DROP DEFAULT;
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
		ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_class_check CHECK (memory_class IN ('self', 'business'));
		ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_identity_check CHECK (memory_type <> 'identity' OR memory_class = 'self');
		ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_visibility_check CHECK (visibility IN ('workspace', 'restricted')) NOT VALID;
		ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_scope_check CHECK (
			(memory_class = 'self' AND scope_type = '' AND scope_id = '') OR
			(memory_class = 'business' AND scope_type IN ('workspace', 'project', 'customer', 'campaign', 'channel', 'instance') AND scope_id <> '')
		);
		ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_subject_key_unique UNIQUE (
			organization_id, workspace_id, digital_human_id, memory_class, scope_type, scope_id, memory_key
		);
		CREATE INDEX IF NOT EXISTS idx_digital_human_memories_retrieval
			ON public.digital_human_memories (organization_id, workspace_id, digital_human_id, memory_class, scope_type, scope_id, status, updated_at DESC);
		CREATE INDEX IF NOT EXISTS idx_digital_human_memories_instance
			ON public.digital_human_memories (digital_human_id, scope_id, updated_at) WHERE scope_type = 'instance' AND status = 'active';
		ALTER TABLE public.digital_human_memories DROP COLUMN IF EXISTS origin_channel_id;
		ALTER TABLE public.digital_human_memories DROP COLUMN IF EXISTS origin;

CREATE OR REPLACE FUNCTION public.enforce_digital_human_memory_supersedes_scope() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		-- A move rewrites every memory of the DigitalHuman, so a superseded
		-- row may not have moved yet when its successor is checked.
		IF TG_OP = 'UPDATE' AND public.digital_human_workspace_move_allows(OLD.digital_human_id, to_jsonb(OLD), to_jsonb(NEW)) THEN
			RETURN NEW;
		END IF;
		IF NEW.supersedes_id IS NOT NULL AND NOT EXISTS (
			SELECT 1
			FROM public.digital_human_memories prior
			WHERE prior.id = NEW.supersedes_id
			  AND prior.id <> NEW.id
			  AND prior.organization_id = NEW.organization_id
			  AND prior.workspace_id = NEW.workspace_id
			  AND prior.digital_human_id = NEW.digital_human_id
			  AND prior.memory_class = NEW.memory_class
			  AND prior.scope_type = NEW.scope_type
			  AND prior.scope_id = NEW.scope_id
		) THEN
			RAISE EXCEPTION 'digital human superseded memory scope mismatch' USING ERRCODE = '23514';
		END IF;
		RETURN NEW;
	END;
$$;

CREATE OR REPLACE FUNCTION public.digital_human_end_user_erase_allows(old_row public.digital_human_memory_events, new_row public.digital_human_memory_events, operation text)
RETURNS boolean LANGUAGE sql STABLE AS $$
	SELECT COALESCE(current_setting('zgi.digital_human_end_user_erase', true), '') <> ''
		AND CASE operation
			WHEN 'DELETE' THEN EXISTS (
				SELECT 1 FROM public.digital_human_memories memory
				JOIN public.digital_human_end_user_instances instance
					ON memory.scope_type = 'instance' AND memory.scope_id = instance.id::text
				WHERE memory.id = old_row.memory_id
					AND instance.subject_id::text = current_setting('zgi.digital_human_end_user_erase', true))
			WHEN 'UPDATE' THEN old_row.run_id IS NOT NULL AND new_row.run_id IS NULL
				AND (to_jsonb(old_row) - 'run_id') = (to_jsonb(new_row) - 'run_id')
				AND EXISTS (
					SELECT 1 FROM public.digital_human_runs run
					WHERE run.id = old_row.run_id
						AND run.user_subject_id::text = current_setting('zgi.digital_human_end_user_erase', true))
			ELSE false
		END;
$$;
	`)
}
