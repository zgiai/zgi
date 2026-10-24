package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Links that open one version of an artifact without signing in, until they
// expire or are revoked. Only a hash of each link's token is stored.
func init() {
	registerSchemaMigration("202610241700000000_add_artifact_shares", upAddArtifactShares, downAddArtifactShares)
}

func upAddArtifactShares(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.artifact_shares (
			id uuid PRIMARY KEY,
			artifact_id uuid NOT NULL REFERENCES public.artifacts(id) ON DELETE CASCADE,
			organization_id uuid NOT NULL,
			version_number integer NOT NULL,
			token_hash varchar(64) NOT NULL,
			created_by_kind varchar(32) NOT NULL,
			created_by_id varchar(64) NOT NULL,
			expires_at timestamptz,
			revoked_at timestamptz,
			access_count bigint NOT NULL DEFAULT 0,
			last_accessed_at timestamptz,
			created_at timestamptz NOT NULL,
			CONSTRAINT artifact_shares_version_check CHECK (version_number > 0)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_shares_token ON public.artifact_shares (token_hash);
		CREATE INDEX IF NOT EXISTS idx_artifact_shares_artifact ON public.artifact_shares (artifact_id, created_at DESC);
	`)
}

func downAddArtifactShares(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.artifact_shares;`)
}
