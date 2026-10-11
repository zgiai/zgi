package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

func init() {
	registerSchemaMigration("202610081000000000_bind_execution_uploads", upBindExecutionUploads, nil)
}

func upBindExecutionUploads(schema *mschema.Builder) error {
	// Additive DDL only; DataFix permits the BEFORE UPDATE immutability trigger.
	return schema.DataFix("Install immutable execution upload ownership", func(db *gorm.DB) error {
		return db.Exec(`
CREATE TABLE IF NOT EXISTS public.agent_execution_uploads (
 file_id uuid PRIMARY KEY,
 organization_id uuid NOT NULL, workspace_id uuid NOT NULL,
 application_type varchar(32) NOT NULL, application_id uuid NOT NULL,
 entry varchar(32) NOT NULL, subject_kind varchar(32) NOT NULL,
 subject_id uuid NOT NULL, created_at timestamptz NOT NULL,
 CONSTRAINT ck_agent_execution_upload_owner CHECK (
  organization_id <> '00000000-0000-0000-0000-000000000000'::uuid
  AND workspace_id <> '00000000-0000-0000-0000-000000000000'::uuid
  AND application_id <> '00000000-0000-0000-0000-000000000000'::uuid
  AND subject_id <> '00000000-0000-0000-0000-000000000000'::uuid
  AND file_id <> '00000000-0000-0000-0000-000000000000'::uuid
  AND application_type IN ('agent', 'digital_human')
  AND ((entry = 'webapp' AND subject_kind = 'account')
   OR (entry = 'external-api' AND subject_kind = 'api_end_user'))
 )
);
CREATE INDEX IF NOT EXISTS idx_agent_execution_upload_owner ON public.agent_execution_uploads
 (organization_id, workspace_id, application_type, application_id, entry, subject_kind, subject_id);
CREATE OR REPLACE FUNCTION public.preserve_execution_upload_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD IS DISTINCT FROM NEW THEN
  RAISE EXCEPTION 'execution upload ownership is immutable';
 END IF;
 RETURN NEW;
END $$;
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'tr_execution_upload_owner' AND tgrelid = 'public.agent_execution_uploads'::regclass) THEN
  CREATE TRIGGER tr_execution_upload_owner BEFORE UPDATE ON public.agent_execution_uploads
   FOR EACH ROW EXECUTE FUNCTION public.preserve_execution_upload_owner();
 END IF;
END $$;
`).Error
	})
}
