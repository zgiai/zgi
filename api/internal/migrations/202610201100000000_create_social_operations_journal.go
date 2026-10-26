package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const createSocialOperationsJournalSQL = `
CREATE TABLE public.social_operation_artifacts (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL,
 workspace_id uuid NOT NULL,
 owner_id uuid NOT NULL,
 employee_id uuid NOT NULL,
 digest varchar(64) NOT NULL CHECK (digest ~ '^[0-9a-f]{64}$'),
 mime varchar(128) NOT NULL,
 size bigint NOT NULL CHECK (size BETWEEN 0 AND 8388608),
 created_at timestamptz NOT NULL
);
CREATE INDEX idx_social_artifacts_owner ON public.social_operation_artifacts(organization_id,workspace_id,owner_id,employee_id);
CREATE TABLE public.social_operation_image_inspections (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL,
 workspace_id uuid NOT NULL,
 owner_id uuid NOT NULL,
 employee_id uuid NOT NULL,
 scope_key varchar(64) NOT NULL,
 artifact_id uuid NOT NULL REFERENCES public.social_operation_artifacts(id),
 artifact_digest varchar(64) NOT NULL CHECK (artifact_digest ~ '^[0-9a-f]{64}$'),
 configuration_revision bigint NOT NULL CHECK(configuration_revision > 0),
 review_artifact_id uuid NOT NULL REFERENCES public.social_operation_artifacts(id),
 approved boolean NOT NULL,
 reviewed_at timestamptz NOT NULL,
 UNIQUE(scope_key,artifact_id,configuration_revision)
);
CREATE INDEX idx_social_image_inspections_owner ON public.social_operation_image_inspections(organization_id,workspace_id,owner_id,employee_id,artifact_id);
CREATE TABLE public.social_operation_records (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL,
 workspace_id uuid NOT NULL,
 owner_id uuid NOT NULL,
 employee_id uuid NOT NULL,
 scope_key varchar(64) NOT NULL,
 kind varchar(32) NOT NULL,
 record_key varchar(128) NOT NULL,
 revision bigint NOT NULL CHECK (revision > 0),
 artifact_id uuid NOT NULL REFERENCES public.social_operation_artifacts(id),
 updated_at timestamptz NOT NULL,
 UNIQUE(scope_key,kind,record_key)
);
CREATE INDEX idx_social_records_owner ON public.social_operation_records(organization_id,workspace_id,owner_id,employee_id,kind,id);
CREATE TABLE public.social_operation_publications (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL,
 workspace_id uuid NOT NULL,
 owner_id uuid NOT NULL,
 employee_id uuid NOT NULL,
 scope_key varchar(64) NOT NULL,
 connection_id uuid NOT NULL,
 account_id varchar(255) NOT NULL,
 content_key varchar(128) NOT NULL,

 configuration_revision bigint NOT NULL DEFAULT 0 CHECK(configuration_revision >= 0),
 archive_timezone varchar(128) NOT NULL DEFAULT '',
 reserved_at timestamptz,
 content_digest varchar(64) NOT NULL DEFAULT '',
 publication_action varchar(16) NOT NULL DEFAULT '',
 platform varchar(32) NOT NULL CHECK(platform IN ('x','linkedin','facebook')),
 draft_artifact_id uuid NOT NULL REFERENCES public.social_operation_artifacts(id),
 reviewed_digest varchar(64) NOT NULL,
 actual_artifact_id uuid REFERENCES public.social_operation_artifacts(id),
 state varchar(32) NOT NULL CHECK(state IN ('draft','reviewed','reserved','dispatching','outcome_unknown','remote_created','verified','archived')),
 revision bigint NOT NULL CHECK(revision > 0),
 media_json text NOT NULL DEFAULT '[]',
 remote_id varchar(255) NOT NULL,
 remote_url text NOT NULL,
 quoted_id varchar(255) NOT NULL,
 reposted_id varchar(255) NOT NULL,
 parent_remote_id varchar(255) NOT NULL,
 parent_publication_id uuid REFERENCES public.social_operation_publications(id),
 published_at timestamptz,
 verified_at timestamptz,
 archive_date varchar(10) NOT NULL,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 UNIQUE(scope_key,connection_id,account_id,content_key),
 CHECK(state NOT IN ('remote_created','verified','archived') OR length(remote_id)>0),
 CHECK(state NOT IN ('verified','archived') OR (length(remote_url)>0 AND published_at IS NOT NULL)),
 CHECK(state NOT IN ('verified','archived') OR (actual_artifact_id IS NOT NULL AND verified_at IS NOT NULL))
);
CREATE INDEX idx_social_publications_due ON public.social_operation_publications(organization_id,workspace_id,owner_id,employee_id,published_at,id);
CREATE TABLE public.social_operation_metrics (
 id uuid PRIMARY KEY,
 publication_id uuid NOT NULL UNIQUE REFERENCES public.social_operation_publications(id),
 observed_at timestamptz NOT NULL,
 age_seconds bigint NOT NULL CHECK(age_seconds >= 604800),
 late boolean NOT NULL,
 values_json text NOT NULL
);
CREATE TABLE public.social_operation_outbound (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL,
 workspace_id uuid NOT NULL,
 owner_id uuid NOT NULL,
 employee_id uuid NOT NULL,
 scope_key varchar(64) NOT NULL,
 work_key varchar(128) NOT NULL,
 configuration_revision bigint NOT NULL CHECK(configuration_revision > 0),
 kind varchar(16) NOT NULL CHECK(kind IN ('dm','handoff')),
 connector_id varchar(128) NOT NULL,
 connection_id uuid NOT NULL,
 operation_id varchar(128) NOT NULL,
 account_id varchar(255) NOT NULL,
 conversation_id varchar(255) NOT NULL,
 destination varchar(512) NOT NULL,
 body_artifact_id uuid NOT NULL REFERENCES public.social_operation_artifacts(id),
 reviewed_digest varchar(64) NOT NULL,
 state varchar(32) NOT NULL CHECK(state IN ('draft','reviewed','dispatching','completed','outcome_unknown')),
 revision bigint NOT NULL CHECK(revision > 0),
 receipt_id varchar(512) NOT NULL,
 receipt_status varchar(64) NOT NULL,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 UNIQUE(scope_key,work_key),
 CHECK(state <> 'completed' OR (length(receipt_id)>0 AND length(receipt_status)>0))
);
`

func init() {
	registerSchemaMigration("202610201100000000_create_social_operations_journal", up202610201100000000, down202610201100000000)
}
func up202610201100000000(schema *mschema.Builder) error {
	return schema.Raw(createSocialOperationsJournalSQL)
}
func down202610201100000000(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.social_operation_outbound; DROP TABLE IF EXISTS public.social_operation_metrics; DROP TABLE IF EXISTS public.social_operation_publications; DROP TABLE IF EXISTS public.social_operation_records; DROP TABLE IF EXISTS public.social_operation_image_inspections; DROP TABLE IF EXISTS public.social_operation_artifacts;`)
}
