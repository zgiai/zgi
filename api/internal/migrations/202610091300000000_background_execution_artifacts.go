package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

func init() {
	registerSchemaMigration("202610091300000000_background_execution_artifacts", upBackgroundExecutionArtifacts, nil)
}

// Retain the existing immutable owner trigger and legacy/v1 contract. Install
// the replacement constraint before removing its predecessor; no rows change.
func upBackgroundExecutionArtifacts(schema *mschema.Builder) error {
	return schema.DataFix("Allow versioned DigitalHuman background artifact ownership", func(db *gorm.DB) error {
		return db.Exec(`
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ck_tool_files_execution_owner_v2' AND conrelid = 'public.tool_files'::regclass) THEN
  ALTER TABLE public.tool_files ADD CONSTRAINT ck_tool_files_execution_owner_v2 CHECK (
   (execution_identity IS NULL AND execution_message_id IS NULL) OR
   COALESCE((execution_identity IS NOT NULL AND jsonb_typeof(execution_identity) = 'object'
    AND (
     (execution_identity ->> 'version' = '1'
      AND execution_identity ->> 'application_type' IN ('agent', 'digital_human')
      AND execution_identity ->> 'entry' IN ('console', 'webapp', 'external-api'))
     OR
     (execution_identity ->> 'version' = '2'
      AND execution_identity ->> 'application_type' = 'digital_human'
      AND ((execution_identity ->> 'entry' = 'channel' AND execution_identity ->> 'subject_kind' = 'channel_conversation')
       OR (execution_identity ->> 'entry' = 'task' AND execution_identity ->> 'subject_kind' = 'scheduled_task')))
    )
    AND execution_identity ?& ARRAY['workspace_id','application_id','subject_kind','conversation_id','publication_ref']
    AND execution_message_id IS NOT NULL AND conversation_id IS NOT NULL
    AND execution_identity ->> 'conversation_id' = conversation_id::text), false)
  );
 END IF;
 ALTER TABLE public.tool_files DROP CONSTRAINT IF EXISTS ck_tool_files_execution_owner;
END $$;
`).Error
	})
}
