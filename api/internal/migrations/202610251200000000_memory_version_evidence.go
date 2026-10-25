package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Memory history shows each change with the user's words it was based on and
// why it was made (ADR 0013). The quote is user content: it is purged with
// the version's content.
func init() {
	registerSchemaMigration("202610251200000000_memory_version_evidence", upMemoryVersionEvidence, downMemoryVersionEvidence)
}

func upMemoryVersionEvidence(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.memory_file_versions ADD COLUMN IF NOT EXISTS evidence text NOT NULL DEFAULT '';
		ALTER TABLE public.memory_file_versions ADD COLUMN IF NOT EXISTS reason text NOT NULL DEFAULT '';
	`)
}

func downMemoryVersionEvidence(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.memory_file_versions DROP COLUMN IF EXISTS reason;
		ALTER TABLE public.memory_file_versions DROP COLUMN IF EXISTS evidence;
	`)
}
