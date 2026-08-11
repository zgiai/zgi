package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateLLMInvocationContentsID = "20260808220000_create_llm_invocation_contents"

func init() {
	registerSchemaMigration(
		migrationCreateLLMInvocationContentsID,
		upCreateLLMInvocationContents,
		downCreateLLMInvocationContents,
	)
}

func upCreateLLMInvocationContents(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.organizations
			ADD COLUMN IF NOT EXISTS llm_content_capture_enabled boolean NOT NULL DEFAULT false;

		CREATE TABLE IF NOT EXISTS public.llm_invocation_contents (
			request_id varchar(100) PRIMARY KEY,
			organization_id uuid NOT NULL,
			input_text text NOT NULL DEFAULT '',
			output_text text NOT NULL DEFAULT '',
			input_json text NOT NULL DEFAULT '',
			output_json text NOT NULL DEFAULT '',
			content_status varchar(24) NOT NULL DEFAULT 'available',
			input_truncated boolean NOT NULL DEFAULT false,
			output_truncated boolean NOT NULL DEFAULT false,
			redaction_version varchar(16) NOT NULL DEFAULT 'v1',
			expires_at timestamptz NOT NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_llm_invocation_contents_org_expires
			ON public.llm_invocation_contents (organization_id, expires_at);

		CREATE TABLE IF NOT EXISTS public.llm_invocation_content_views (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL,
			request_id varchar(100) NOT NULL,
			account_id uuid NOT NULL,
			viewed_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_llm_invocation_content_views_org_time
			ON public.llm_invocation_content_views (organization_id, viewed_at DESC);
		CREATE INDEX IF NOT EXISTS idx_llm_invocation_content_views_request
			ON public.llm_invocation_content_views (request_id, viewed_at DESC)
	`)
}

func downCreateLLMInvocationContents(schema *mschema.Builder) error {
	if err := schema.DropIfExists("llm_invocation_content_views"); err != nil {
		return err
	}
	if err := schema.DropIfExists("llm_invocation_contents"); err != nil {
		return err
	}
	return schema.WhenTableHasColumn("organizations", "llm_content_capture_enabled", func() error {
		return schema.Table("organizations", func(table *mschema.Blueprint) {
			table.DropColumn("llm_content_capture_enabled")
		})
	})
}
