package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202608191200000000ID = "202608191200000000_add_digital_human_memory_supersede_undo"

const addDigitalHumanMemorySupersedeUndoSQL = `
	ALTER TABLE public.digital_human_memory_undo_records
		ADD COLUMN IF NOT EXISTS superseded_snapshot jsonb;

	CREATE OR REPLACE FUNCTION public.enforce_digital_human_memory_supersedes_scope()
	RETURNS trigger AS $$
	BEGIN
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
	$$ LANGUAGE plpgsql;

	DROP TRIGGER IF EXISTS trg_digital_human_memories_supersedes_scope ON public.digital_human_memories;
	CREATE TRIGGER trg_digital_human_memories_supersedes_scope
		BEFORE INSERT OR UPDATE ON public.digital_human_memories
		FOR EACH ROW EXECUTE FUNCTION public.enforce_digital_human_memory_supersedes_scope();
`

func init() {
	registerSchemaMigration(migration202608191200000000ID, up202608191200000000, down202608191200000000)
}

func up202608191200000000(schema *mschema.Builder) error {
	return schema.Raw(addDigitalHumanMemorySupersedeUndoSQL)
}

func down202608191200000000(schema *mschema.Builder) error {
	// Keep the nullable snapshot column during rollback. Older binaries ignore it,
	// and retaining it avoids destroying the only recovery record for a committed
	// supersede operation. Reapplying this migration is safe through IF NOT EXISTS.
	return schema.Raw(`
		DROP TRIGGER IF EXISTS trg_digital_human_memories_supersedes_scope ON public.digital_human_memories;
		DROP FUNCTION IF EXISTS public.enforce_digital_human_memory_supersedes_scope();
	`)
}
