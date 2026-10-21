package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

func init() {
	registerSchemaMigration("202610081100000000_digital_human_execution_runtime", upDigitalHumanExecutionRuntime, nil)
}

func upDigitalHumanExecutionRuntime(schema *mschema.Builder) error {
	// Existing publications and snapshots stay disabled without a data rewrite.
	return schema.Raw(`ALTER TABLE public.digital_human_versions
 ADD COLUMN IF NOT EXISTS execution_runtime jsonb;
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ck_digital_human_execution_runtime' AND conrelid = 'public.digital_human_versions'::regclass) THEN
  ALTER TABLE public.digital_human_versions ADD CONSTRAINT ck_digital_human_execution_runtime CHECK (
   execution_runtime IS NULL OR execution_runtime IN ('{"enabled":true}'::jsonb, '{"enabled":false}'::jsonb)
  );
 END IF;
END $$;`)
}
