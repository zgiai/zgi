package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Erasing a Runtime Workspace end user (ADR 0010) removes their DigitalHuman
// instance memory, whose events are otherwise immutable. The erase sets
// zgi.digital_human_end_user_erase to the end user's subject for its
// transaction; the guard then accepts deleting the events of that end user's
// instance memories, and clearing the run of other events when it was the
// end user's run, so their runs can be deleted. Everything else stays
// rejected, and workspace moves keep their own exception.
func init() {
	registerSchemaMigration("202610231600000000_digital_human_end_user_erasure", upDigitalHumanEndUserErasure, downDigitalHumanEndUserErasure)
}

func upDigitalHumanEndUserErasure(schema *mschema.Builder) error {
	return schema.Raw(`
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

CREATE OR REPLACE FUNCTION public.digital_human_memory_event_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		IF TG_OP = 'UPDATE' AND public.digital_human_workspace_move_allows(OLD.digital_human_id, to_jsonb(OLD), to_jsonb(NEW)) THEN
			RETURN NEW;
		END IF;
		IF TG_OP = 'UPDATE' AND public.digital_human_end_user_erase_allows(OLD, NEW, TG_OP) THEN
			RETURN NEW;
		END IF;
		IF TG_OP = 'DELETE' AND public.digital_human_end_user_erase_allows(OLD, NULL, TG_OP) THEN
			RETURN OLD;
		END IF;
		RAISE EXCEPTION 'digital human memory events are immutable' USING ERRCODE = '23514';
	END;
$$;
`)
}

func downDigitalHumanEndUserErasure(schema *mschema.Builder) error {
	return schema.Raw(`
CREATE OR REPLACE FUNCTION public.digital_human_memory_event_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		IF TG_OP = 'UPDATE' AND public.digital_human_workspace_move_allows(OLD.digital_human_id, to_jsonb(OLD), to_jsonb(NEW)) THEN
			RETURN NEW;
		END IF;
		RAISE EXCEPTION 'digital human memory events are immutable' USING ERRCODE = '23514';
	END;
$$;
DROP FUNCTION IF EXISTS public.digital_human_end_user_erase_allows(public.digital_human_memory_events, public.digital_human_memory_events, text);
`)
}
