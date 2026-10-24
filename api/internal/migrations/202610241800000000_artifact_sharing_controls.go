package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// An artifact's sharing can be stopped, and the run files that became an
// artifact are recorded, so links to them sent elsewhere stop with it.
func init() {
	registerSchemaMigration("202610241800000000_artifact_sharing_controls", upArtifactSharingControls, downArtifactSharingControls)
}

func upArtifactSharingControls(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.artifacts ADD COLUMN IF NOT EXISTS sharing_disabled_at timestamptz;
		CREATE TABLE IF NOT EXISTS public.artifact_source_files (
			artifact_id uuid NOT NULL REFERENCES public.artifacts(id) ON DELETE CASCADE,
			file_id varchar(64) NOT NULL,
			created_at timestamptz NOT NULL,
			PRIMARY KEY (file_id, artifact_id)
		);
	`)
}

func downArtifactSharingControls(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TABLE IF EXISTS public.artifact_source_files;
		ALTER TABLE public.artifacts DROP COLUMN IF EXISTS sharing_disabled_at;
	`)
}
