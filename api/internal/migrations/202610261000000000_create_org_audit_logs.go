package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// createOrgAuditLogsSQL stores one row per organization or permission change:
// who did it, to whom, what changed and whether it took effect. before and
// after hold only the non-sensitive fields the recorder lets through.
const createOrgAuditLogsSQL = `
CREATE TABLE IF NOT EXISTS public.org_audit_logs (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL,
 workspace_id uuid,
 actor_account_id uuid,
 target_account_id uuid,
 target_type varchar(32) NOT NULL DEFAULT '',
 target_id varchar(64) NOT NULL DEFAULT '',
 action varchar(64) NOT NULL CHECK (length(action) BETWEEN 1 AND 64),
 result varchar(16) NOT NULL CHECK (result IN ('success', 'denied', 'failed')),
 before jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(before) = 'object'),
 after jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(after) = 'object'),
 request_id varchar(128) NOT NULL DEFAULT '',
 ip varchar(64) NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_org_audit_logs_org_created ON public.org_audit_logs
 (organization_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_org_audit_logs_org_actor ON public.org_audit_logs
 (organization_id, actor_account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_org_audit_logs_org_target ON public.org_audit_logs
 (organization_id, target_account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_org_audit_logs_org_action ON public.org_audit_logs
 (organization_id, action, created_at DESC);
`

func init() {
	registerSchemaMigration("202610261000000000_create_org_audit_logs", up202610261000000000, down202610261000000000)
}

func up202610261000000000(schema *mschema.Builder) error { return schema.Raw(createOrgAuditLogsSQL) }

func down202610261000000000(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.org_audit_logs;`)
}
