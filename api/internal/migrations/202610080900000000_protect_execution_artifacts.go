package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

func init() {
	// Protection is additive. A rollback of compute admission must not erase
	// ownership or make retained artifacts accessible through legacy signatures.
	registerSchemaMigration("202610080900000000_protect_execution_artifacts", upProtectExecutionArtifacts, nil)
}

func upProtectExecutionArtifacts(schema *mschema.Builder) error {
	// The migration safety checker requires the explicit DataFix gate for SQL
	// containing UPDATE, including an immutable BEFORE UPDATE trigger. This
	// operation only installs additive DDL; no existing row is rewritten.
	return schema.DataFix("Install immutable execution artifact ownership", func(db *gorm.DB) error {
		return db.Exec(`
ALTER TABLE public.tool_files
 ADD COLUMN IF NOT EXISTS execution_identity jsonb,
 ADD COLUMN IF NOT EXISTS execution_message_id uuid;
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ck_tool_files_execution_owner' AND conrelid = 'public.tool_files'::regclass) THEN
  ALTER TABLE public.tool_files ADD CONSTRAINT ck_tool_files_execution_owner CHECK (
   (execution_identity IS NULL AND execution_message_id IS NULL) OR
   COALESCE((execution_identity IS NOT NULL AND jsonb_typeof(execution_identity) = 'object'
    AND execution_identity ->> 'version' = '1'
    AND execution_identity ->> 'application_type' IN ('agent', 'digital_human')
    AND execution_identity ->> 'entry' IN ('console', 'webapp', 'external-api')
    AND execution_identity ?& ARRAY['workspace_id','application_id','subject_kind','conversation_id','publication_ref']
    AND execution_message_id IS NOT NULL AND conversation_id IS NOT NULL
    AND execution_identity ->> 'conversation_id' = conversation_id::text), false)
  );
 END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_tool_files_execution_owner ON public.tool_files
 (tenant_id, user_id, conversation_id) WHERE execution_identity IS NOT NULL;
CREATE OR REPLACE FUNCTION public.preserve_tool_file_execution_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.execution_identity IS DISTINCT FROM NEW.execution_identity
  OR OLD.execution_message_id IS DISTINCT FROM NEW.execution_message_id
  OR (OLD.execution_identity IS NOT NULL AND (
   OLD.tenant_id IS DISTINCT FROM NEW.tenant_id OR OLD.user_id IS DISTINCT FROM NEW.user_id
   OR OLD.conversation_id IS DISTINCT FROM NEW.conversation_id OR OLD.file_key IS DISTINCT FROM NEW.file_key)) THEN
  RAISE EXCEPTION 'tool file execution ownership is immutable';
 END IF;
 RETURN NEW;
END $$;
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'tr_tool_file_execution_owner' AND tgrelid = 'public.tool_files'::regclass) THEN
  CREATE TRIGGER tr_tool_file_execution_owner BEFORE UPDATE ON public.tool_files
   FOR EACH ROW EXECUTE FUNCTION public.preserve_tool_file_execution_owner();
 END IF;
END $$;
`).Error
	})
}
