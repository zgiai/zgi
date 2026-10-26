package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// ADR 0015 keeps a DigitalHuman's notes on an external end user in that
// person's relationship memory, under the notes/ key namespace. Active
// instance-scope memories become notes, creating the relationship where it is
// missing; archived ones go to the person's memory archive. The moved rows
// leave the DigitalHuman's own memory with their events, undo records, and
// ledger rows, under the same erasure guard end-user erasure uses.
//
// Rows of a deleted DigitalHuman or of an instance that no longer exists stay
// where they are; the scope cleanup of ADR 0015 handles them.
func init() {
	registerSchemaMigration("202610261100000000_person_notes_in_relationship_memory", upPersonNotesInRelationshipMemory, downPersonNotesInRelationshipMemory)
}

func upPersonNotesInRelationshipMemory(schema *mschema.Builder) error {
	return schema.DataFix("move instance memory into relationship memory as notes", func(db *gorm.DB) error {
		return db.Exec(personNotesSQL).Error
	})
}

const personNotesSQL = `
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
				WHERE m.memory_class = 'business' AND m.scope_type = 'instance' AND m.status IN ('active', 'archived')
				ORDER BY m.created_at, m.id
			LOOP
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

// The move is not reversed: notes stay in relationship memory, which is
// where every newer write puts them.
func downPersonNotesInRelationshipMemory(*mschema.Builder) error { return nil }
