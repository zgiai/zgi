package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Artifacts keep what agents deliver across conversations (ADR 0009). An
// artifact belongs to an application and an owner; its immutable versions
// hold a document's Markdown or a file the version owns.
const artifactsSQL = `
CREATE TABLE IF NOT EXISTS public.artifacts (
	id uuid PRIMARY KEY,
	organization_id uuid NOT NULL,
	workspace_id uuid NOT NULL,
	application_type varchar(32) NOT NULL,
	application_id uuid NOT NULL,
	owner_kind varchar(32) NOT NULL,
	owner_id uuid NOT NULL,
	visibility varchar(16) NOT NULL,
	kind varchar(16) NOT NULL,
	title varchar(255) NOT NULL,
	source_task_id uuid,
	current_version integer NOT NULL,
	created_by_kind varchar(32) NOT NULL,
	created_by_id varchar(64) NOT NULL,
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL,
	deleted_at timestamptz,
	CONSTRAINT artifacts_owner_check CHECK (owner_kind IN ('account', 'api_end_user', 'workspace', 'application')),
	CONSTRAINT artifacts_visibility_check CHECK (visibility IN ('owner', 'team', 'application')),
	CONSTRAINT artifacts_kind_check CHECK (kind IN ('document', 'file', 'image', 'table'))
);
CREATE INDEX IF NOT EXISTS idx_artifacts_application
	ON public.artifacts (organization_id, workspace_id, application_type, application_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_artifacts_owner
	ON public.artifacts (application_id, owner_kind, owner_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_artifacts_task
	ON public.artifacts (application_id, source_task_id) WHERE source_task_id IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS public.artifact_versions (
	id uuid PRIMARY KEY,
	artifact_id uuid NOT NULL REFERENCES public.artifacts(id) ON DELETE CASCADE,
	number integer NOT NULL,
	content text,
	file_id varchar(64),
	file_name varchar(255) NOT NULL DEFAULT '',
	mime_type varchar(128) NOT NULL,
	size bigint NOT NULL,
	checksum varchar(64) NOT NULL,
	note text NOT NULL DEFAULT '',
	source_kind varchar(32) NOT NULL,
	source_task_id uuid,
	source_run_id uuid,
	source_report_id uuid,
	created_by_kind varchar(32) NOT NULL,
	created_by_id varchar(64) NOT NULL,
	created_at timestamptz NOT NULL,
	CONSTRAINT artifact_versions_content_check CHECK ((content IS NULL) <> (file_id IS NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_versions_number
	ON public.artifact_versions (artifact_id, number);`

func init() {
	registerSchemaMigration("202610211500000000_add_artifacts", upAddArtifacts, downAddArtifacts)
}

func upAddArtifacts(schema *mschema.Builder) error {
	return schema.Raw(artifactsSQL)
}

func downAddArtifacts(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TABLE IF EXISTS public.artifact_versions;
		DROP TABLE IF EXISTS public.artifacts;
	`)
}
