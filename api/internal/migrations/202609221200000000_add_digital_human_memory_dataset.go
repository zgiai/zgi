package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202609221200000000ID = "202609221200000000_add_digital_human_memory_dataset"

func init() {
	registerSchemaMigration(migration202609221200000000ID, up202609221200000000, down202609221200000000)
}

func up202609221200000000(b *mschema.Builder) error {
	return b.Raw(`
		ALTER TABLE public.digital_human_memory_states
			ADD COLUMN IF NOT EXISTS memory_dataset_id uuid
			REFERENCES public.datasets(id) ON DELETE SET NULL;
		CREATE INDEX IF NOT EXISTS idx_digital_human_memory_states_dataset
			ON public.digital_human_memory_states (memory_dataset_id)
			WHERE memory_dataset_id IS NOT NULL;
	`)
}

func down202609221200000000(b *mschema.Builder) error {
	return b.Raw(`
		DROP INDEX IF EXISTS public.idx_digital_human_memory_states_dataset;
		ALTER TABLE public.digital_human_memory_states
			DROP COLUMN IF EXISTS memory_dataset_id;
	`)
}
