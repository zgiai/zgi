package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260902120000ID                   = "20260902120000_create_connector_confirmation_continuations"
	createConnectorConfirmationContinuationsSQL = `
		CREATE UNIQUE INDEX uq_connector_execution_route_claims_owner_receipt
			ON public.connector_execution_route_claims (receipt_id, organization_id, owner_account_id);

		CREATE TABLE public.connector_confirmation_continuations (
			route_receipt uuid PRIMARY KEY,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			owner_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			origin_surface varchar(16) NOT NULL,
			conversation_id varchar(255),
			message_id varchar(255),
			agent_id varchar(255),
			workflow_id varchar(255),
			run_id varchar(255),
			node_id varchar(255),
			protected_payload_envelope text NOT NULL,
			state varchar(16) NOT NULL DEFAULT 'pending',
			decision_id uuid,
			outcome_id uuid,
			diagnostic_code varchar(64),
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			terminal_at timestamptz,
			CONSTRAINT connector_confirmation_continuations_route_owner_fk
				FOREIGN KEY (route_receipt, organization_id, owner_account_id)
				REFERENCES public.connector_execution_route_claims(receipt_id, organization_id, owner_account_id)
				ON DELETE RESTRICT,
			CONSTRAINT connector_confirmation_continuations_surface_check
				CHECK (origin_surface IN ('chat', 'agent', 'workflow')),
			CONSTRAINT connector_confirmation_continuations_origin_refs_check
				CHECK (
					(
						origin_surface = 'chat'
						AND conversation_id IS NOT NULL AND message_id IS NOT NULL
						AND btrim(conversation_id) <> '' AND btrim(message_id) <> ''
						AND agent_id IS NULL AND workflow_id IS NULL AND run_id IS NULL AND node_id IS NULL
					)
					OR (
						origin_surface = 'agent'
						AND conversation_id IS NOT NULL AND message_id IS NOT NULL AND agent_id IS NOT NULL
						AND btrim(conversation_id) <> '' AND btrim(message_id) <> '' AND btrim(agent_id) <> ''
						AND workflow_id IS NULL AND node_id IS NULL
						AND (run_id IS NULL OR btrim(run_id) <> '')
					)
					OR (
						origin_surface = 'workflow'
						AND workflow_id IS NOT NULL AND run_id IS NOT NULL AND node_id IS NOT NULL
						AND btrim(workflow_id) <> '' AND btrim(run_id) <> '' AND btrim(node_id) <> ''
						AND conversation_id IS NULL AND message_id IS NULL AND agent_id IS NULL
					)
				),
			CONSTRAINT connector_confirmation_continuations_envelope_check
				CHECK (
					btrim(protected_payload_envelope) <> ''
					AND octet_length(protected_payload_envelope) <= 1048576
				),
			CONSTRAINT connector_confirmation_continuations_state_check
				CHECK (state IN ('pending', 'cancelled', 'completed', 'failed', 'unknown')),
			CONSTRAINT connector_confirmation_continuations_diagnostic_check
				CHECK (
					diagnostic_code IS NULL
					OR diagnostic_code ~ '^[a-z0-9][a-z0-9._-]{0,63}$'
				),
			CONSTRAINT connector_confirmation_continuations_terminal_check
				CHECK (
					(
						state = 'pending'
						AND decision_id IS NULL AND outcome_id IS NULL
						AND diagnostic_code IS NULL AND terminal_at IS NULL
					)
					OR (
						state = 'cancelled'
						AND decision_id IS NOT NULL AND outcome_id IS NULL
						AND diagnostic_code IS NULL AND terminal_at IS NOT NULL
					)
					OR (
						state = 'completed'
						AND decision_id IS NOT NULL AND outcome_id IS NOT NULL
						AND terminal_at IS NOT NULL
					)
					OR (
						state IN ('failed', 'unknown')
						AND diagnostic_code IS NOT NULL AND terminal_at IS NOT NULL
						AND (outcome_id IS NULL OR decision_id IS NOT NULL)
					)
				)
		);
		CREATE INDEX idx_connector_confirmation_continuations_owner_state
			ON public.connector_confirmation_continuations
			(organization_id, owner_account_id, state, updated_at);
		CREATE INDEX idx_connector_confirmation_continuations_conversation
			ON public.connector_confirmation_continuations
			(organization_id, owner_account_id, conversation_id, message_id)
			WHERE conversation_id IS NOT NULL;
		CREATE INDEX idx_connector_confirmation_continuations_agent_run
			ON public.connector_confirmation_continuations
			(organization_id, owner_account_id, agent_id, run_id)
			WHERE agent_id IS NOT NULL;
		CREATE INDEX idx_connector_confirmation_continuations_workflow_run
			ON public.connector_confirmation_continuations
			(organization_id, owner_account_id, workflow_id, run_id, node_id)
			WHERE workflow_id IS NOT NULL;
	`
	dropConnectorConfirmationContinuationsSQL = `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM public.connector_confirmation_continuations LIMIT 1) THEN
				RAISE EXCEPTION 'cannot remove Connector confirmation continuations while records exist';
			END IF;
		END $$;
		DROP TABLE IF EXISTS public.connector_confirmation_continuations;
		DROP INDEX IF EXISTS public.uq_connector_execution_route_claims_owner_receipt;
	`
)

func init() {
	registerSchemaMigration(migration20260902120000ID, up20260902120000, down20260902120000)
}

func up20260902120000(schema *mschema.Builder) error {
	return schema.Raw(createConnectorConfirmationContinuationsSQL)
}

func down20260902120000(schema *mschema.Builder) error {
	return schema.Raw(dropConnectorConfirmationContinuationsSQL)
}
