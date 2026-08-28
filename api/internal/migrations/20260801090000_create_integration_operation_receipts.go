package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260801090000ID             = "20260801090000_create_integration_operation_receipts"
	createIntegrationOperationReceiptsSQL = `
		CREATE TABLE public.integration_operation_receipts (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL,
			workspace_id uuid,
			conversation_id uuid NOT NULL,
			message_id uuid NOT NULL,
			connection_id uuid NOT NULL,
			batch_id varchar(128) NOT NULL,
			operation_item_id varchar(128) NOT NULL,
			item_index integer NOT NULL,
			item_count integer NOT NULL,
			integration_id varchar(64) NOT NULL,
			action_id varchar(128) NOT NULL,
			operation_key varchar(64) NOT NULL,
			target_hmac varchar(64) NOT NULL,
			frozen_input_hmac varchar(64) NOT NULL,
			status varchar(32) NOT NULL,
			claim_token uuid NOT NULL,
			execution_id uuid,
			provider_request_id varchar(128),
			result_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
			result_count integer NOT NULL DEFAULT 0,
			provider_started_at timestamptz,
			lease_expires_at timestamptz NOT NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT integration_operation_receipts_status_check
				CHECK (status IN ('executing', 'succeeded', 'outcome_unknown')),
			CONSTRAINT integration_operation_receipts_operation_key_length
				CHECK (char_length(operation_key) = 64),
			CONSTRAINT integration_operation_receipts_target_hmac_length
				CHECK (char_length(target_hmac) = 64),
			CONSTRAINT integration_operation_receipts_frozen_input_hmac_length
				CHECK (char_length(frozen_input_hmac) = 64),
			CONSTRAINT integration_operation_receipts_item_bounds
				CHECK (item_index >= 1 AND item_count >= 1 AND item_index <= item_count),
			CONSTRAINT integration_operation_receipts_result_count_nonnegative
				CHECK (result_count >= 0)
		);
		CREATE UNIQUE INDEX uidx_integration_operation_receipts_org_key
			ON public.integration_operation_receipts (organization_id, operation_key);
		CREATE INDEX idx_integration_operation_receipts_message
			ON public.integration_operation_receipts (organization_id, conversation_id, message_id);
		CREATE INDEX idx_integration_operation_receipts_connection
			ON public.integration_operation_receipts (connection_id, created_at);
		CREATE INDEX idx_integration_operation_receipts_status_lease
			ON public.integration_operation_receipts (status, lease_expires_at);
	`
	guardDropIntegrationOperationReceiptsSQL = `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM public.integration_operation_receipts LIMIT 1) THEN
				RAISE EXCEPTION 'cannot remove integration operation receipts while replay-protection history exists';
			END IF;
		END
		$$;
	`
)

func init() {
	registerSchemaMigration(migration20260801090000ID, up20260801090000, down20260801090000)
}

func up20260801090000(schema *mschema.Builder) error {
	return schema.Raw(createIntegrationOperationReceiptsSQL)
}

func down20260801090000(schema *mschema.Builder) error {
	if err := schema.Raw(guardDropIntegrationOperationReceiptsSQL); err != nil {
		return err
	}
	return schema.DropIfExists("integration_operation_receipts")
}
