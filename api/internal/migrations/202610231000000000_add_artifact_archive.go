package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Artifacts can be archived out of the default listing and keep the
// conversation that produced them, so a file exported again under the same
// name in that conversation becomes a new version (ADR 0009).
func init() {
	registerSchemaMigration("202610231000000000_add_artifact_archive", upAddArtifactArchive, downAddArtifactArchive)
}

func upAddArtifactArchive(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.artifacts ADD COLUMN IF NOT EXISTS archived_at timestamptz;
		ALTER TABLE public.artifacts ADD COLUMN IF NOT EXISTS source_conversation_id uuid;
		CREATE INDEX IF NOT EXISTS idx_artifacts_conversation
			ON public.artifacts (application_id, source_conversation_id) WHERE source_conversation_id IS NOT NULL AND deleted_at IS NULL;
	`)
}

func downAddArtifactArchive(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_artifacts_conversation;
		ALTER TABLE public.artifacts DROP COLUMN IF EXISTS source_conversation_id;
		ALTER TABLE public.artifacts DROP COLUMN IF EXISTS archived_at;
	`)
}
