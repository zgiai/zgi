package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const createWorkJournalSQL = `
CREATE TABLE public.employee_work_records (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL,
 workspace_id uuid NOT NULL,
 owner_id uuid NOT NULL,
 employee_id uuid NOT NULL,
 scope_key varchar(64) NOT NULL CHECK (length(scope_key) = 64),
 kind varchar(128) NOT NULL CHECK (length(kind) BETWEEN 1 AND 128),
 key varchar(128) NOT NULL CHECK (length(key) BETWEEN 1 AND 128),
 state varchar(128) NOT NULL CHECK (length(state) BETWEEN 1 AND 128),
 content jsonb NOT NULL,
 sources jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(sources) = 'array'),
 revision bigint NOT NULL CHECK (revision >= 1),
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 UNIQUE (scope_key, kind, key)
);
CREATE INDEX idx_work_records_scope ON public.employee_work_records
 (organization_id, workspace_id, owner_id, employee_id, id);
CREATE TABLE public.employee_work_record_versions (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL,
 workspace_id uuid NOT NULL,
 owner_id uuid NOT NULL,
 employee_id uuid NOT NULL,
 record_id uuid NOT NULL REFERENCES public.employee_work_records(id),
 revision bigint NOT NULL CHECK (revision >= 1),
 kind varchar(128) NOT NULL CHECK (length(kind) BETWEEN 1 AND 128),
 key varchar(128) NOT NULL CHECK (length(key) BETWEEN 1 AND 128),
 state varchar(128) NOT NULL CHECK (length(state) BETWEEN 1 AND 128),
 content jsonb NOT NULL,
 sources jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(sources) = 'array'),
 created_at timestamptz NOT NULL,
 UNIQUE (record_id, revision)
);
CREATE INDEX idx_work_record_versions_scope ON public.employee_work_record_versions
 (organization_id, workspace_id, owner_id, employee_id, record_id, revision);
CREATE TABLE public.employee_work_operations (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL,
 workspace_id uuid NOT NULL,
 owner_id uuid NOT NULL,
 employee_id uuid NOT NULL,
 scope_key varchar(64) NOT NULL CHECK (length(scope_key) = 64),
 work_key varchar(128) NOT NULL CHECK (length(work_key) BETWEEN 1 AND 128),
 employee_version_id uuid NOT NULL,
 connector_id varchar(128) NOT NULL CHECK (length(connector_id) BETWEEN 1 AND 128),
 connection_id uuid NOT NULL,
 operation_id varchar(128) NOT NULL CHECK (length(operation_id) BETWEEN 1 AND 128),
 input text NOT NULL CHECK (jsonb_typeof(input::jsonb) = 'object' AND octet_length(input) <= 49152),
 digest varchar(64) NOT NULL CHECK (length(digest) = 64),
 unresolved_fingerprint varchar(64),
 state varchar(16) NOT NULL CHECK (state IN ('pending', 'executing', 'succeeded', 'failed', 'unknown')),
 attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 1),
 receipt jsonb NOT NULL DEFAULT '{}'::jsonb,
 evidence_id uuid,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 UNIQUE (scope_key, work_key),
 UNIQUE (unresolved_fingerprint),
 CHECK ((state IN ('pending', 'executing', 'unknown') AND unresolved_fingerprint IS NOT NULL)
 OR (state IN ('succeeded', 'failed') AND unresolved_fingerprint IS NULL)),
 CHECK ((state = 'pending' AND attempts = 0) OR (state <> 'pending' AND attempts = 1))
);
CREATE INDEX idx_work_operations_scope ON public.employee_work_operations
 (organization_id, workspace_id, owner_id, employee_id, id);
CREATE INDEX idx_work_operations_scope_state ON public.employee_work_operations
 (organization_id, workspace_id, owner_id, employee_id, state, id);
`

func init() {
	registerSchemaMigration("202610201300000000_create_work_journal", up202610201300000000, down202610201300000000)
}
func up202610201300000000(schema *mschema.Builder) error { return schema.Raw(createWorkJournalSQL) }
func down202610201300000000(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.employee_work_operations; DROP TABLE IF EXISTS public.employee_work_record_versions; DROP TABLE IF EXISTS public.employee_work_records;`)
}
