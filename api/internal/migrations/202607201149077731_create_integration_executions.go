package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration202607201149077731ID  = "202607201149077731_create_integration_executions"
	createIntegrationExecutionsSQL = `
		CREATE TABLE public.integration_executions (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL,
			workspace_id uuid,
			account_id uuid,
			app_id uuid,
			conversation_id uuid,
			message_id uuid,
			connection_id uuid,
			integration_id varchar(64) NOT NULL,
			driver_id varchar(64) NOT NULL,
			action_id varchar(128) NOT NULL,
			invoke_from varchar(32) NOT NULL,
			status varchar(32) NOT NULL,
			provider_request_id varchar(128),
			duration_ms bigint NOT NULL DEFAULT 0,
			cost_usd numeric(20,8),
			input_hmac varchar(64),
			result_count integer NOT NULL DEFAULT 0,
			attempt_count integer NOT NULL DEFAULT 0,
			error_code varchar(64),
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT integration_executions_duration_nonnegative CHECK (duration_ms >= 0),
			CONSTRAINT integration_executions_cost_nonnegative CHECK (cost_usd IS NULL OR cost_usd >= 0),
			CONSTRAINT integration_executions_input_hmac_length CHECK (input_hmac IS NULL OR char_length(input_hmac) = 64),
			CONSTRAINT integration_executions_result_count_nonnegative CHECK (result_count >= 0),
			CONSTRAINT integration_executions_attempt_count_nonnegative CHECK (attempt_count >= 0)
		);
		CREATE INDEX idx_integration_executions_org_created
			ON public.integration_executions (organization_id, created_at);
		CREATE INDEX idx_integration_executions_conversation_created
			ON public.integration_executions (conversation_id, created_at);
		CREATE INDEX idx_integration_executions_provider_request
			ON public.integration_executions (provider_request_id)
	`
)

func init() {
	registerSchemaMigration(migration202607201149077731ID, up202607201149077731, down202607201149077731)
}

func up202607201149077731(schema *mschema.Builder) error {
	return schema.Raw(createIntegrationExecutionsSQL)
}

func down202607201149077731(schema *mschema.Builder) error {
	return schema.DropIfExists("integration_executions")
}
