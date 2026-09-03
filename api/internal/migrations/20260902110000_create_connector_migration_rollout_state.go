package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260902110000ID               = "20260902110000_create_connector_migration_rollout_state"
	createConnectorMigrationRolloutStateSQL = `
		CREATE TABLE public.connector_migration_inventory (
			connection_id uuid PRIMARY KEY,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			owner_account_id uuid REFERENCES public.accounts(id) ON DELETE RESTRICT,
			connector_id varchar(128) NOT NULL REFERENCES public.connector_definitions(connector_id) ON DELETE RESTRICT,
			provider_id varchar(128) NOT NULL,
			driver_id varchar(128) NOT NULL,
			auth_method_id varchar(128) NOT NULL,
			eligibility varchar(32) NOT NULL,
			restriction_digest varchar(64),
			verification_digest varchar(64) NOT NULL,
			restriction_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
			block_reasons jsonb NOT NULL DEFAULT '[]'::jsonb,
			captured_at timestamptz NOT NULL,
			backfilled_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT connector_migration_inventory_connector_id_check
				CHECK (connector_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_migration_inventory_provider_id_check
				CHECK (provider_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_migration_inventory_driver_id_check
				CHECK (driver_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_migration_inventory_auth_method_id_check
				CHECK (auth_method_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_migration_inventory_eligibility_check
				CHECK (eligibility IN ('eligible', 'blocked', 'excluded', 'backfilled')),
			CONSTRAINT connector_migration_inventory_digests_check
				CHECK (
					verification_digest ~ '^[a-f0-9]{64}$'
					AND (restriction_digest IS NULL OR restriction_digest ~ '^[a-f0-9]{64}$')
				),
			CONSTRAINT connector_migration_inventory_payload_check
				CHECK (jsonb_typeof(restriction_snapshot) = 'object' AND jsonb_typeof(block_reasons) = 'array'),
			CONSTRAINT connector_migration_inventory_personal_check
				CHECK (
					(eligibility IN ('eligible', 'blocked', 'backfilled') AND owner_account_id IS NOT NULL AND restriction_digest IS NOT NULL)
					OR (eligibility = 'excluded' AND backfilled_at IS NULL)
				),
			CONSTRAINT connector_migration_inventory_backfill_check
				CHECK ((eligibility = 'backfilled') = (backfilled_at IS NOT NULL))
		);
		CREATE INDEX idx_connector_migration_inventory_provider
			ON public.connector_migration_inventory (connector_id, eligibility, captured_at, connection_id);

		CREATE TABLE public.connector_provider_rollouts (
			connector_id varchar(128) PRIMARY KEY REFERENCES public.connector_definitions(connector_id) ON DELETE RESTRICT,
			phase varchar(32) NOT NULL DEFAULT 'legacy',
			revision bigint NOT NULL DEFAULT 1,
			canary_basis_points integer NOT NULL DEFAULT 0,
			canary_salt varchar(128),
			observation_window_started_at timestamptz,
			reversal_materialized boolean NOT NULL DEFAULT false,
			in_flight_connector_claims integer NOT NULL DEFAULT 0,
			in_flight_reconnects integer NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT connector_provider_rollouts_phase_check
				CHECK (phase IN ('legacy', 'shadow', 'canary', 'connector', 'reversing')),
			CONSTRAINT connector_provider_rollouts_revision_check CHECK (revision >= 1),
			CONSTRAINT connector_provider_rollouts_canary_check
				CHECK (
					(phase = 'canary' AND canary_basis_points > 0 AND canary_basis_points < 10000 AND btrim(canary_salt) <> '')
					OR (phase <> 'canary' AND canary_basis_points = 0 AND canary_salt IS NULL)
				),
			CONSTRAINT connector_provider_rollouts_observation_check
				CHECK (
					(phase = 'legacy' AND observation_window_started_at IS NULL)
					OR (phase <> 'legacy' AND observation_window_started_at IS NOT NULL)
				),
			CONSTRAINT connector_provider_rollouts_reversal_check
				CHECK (
					(phase = 'reversing')
					OR (reversal_materialized = false AND in_flight_connector_claims = 0 AND in_flight_reconnects = 0)
				),
			CONSTRAINT connector_provider_rollouts_in_flight_check
				CHECK (in_flight_connector_claims >= 0 AND in_flight_reconnects >= 0)
		);

		CREATE TABLE public.connector_shadow_observations (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			owner_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			connector_id varchar(128) NOT NULL REFERENCES public.connector_provider_rollouts(connector_id) ON DELETE RESTRICT,
			connection_id uuid NOT NULL,
			target_type varchar(16) NOT NULL,
			target_id varchar(255) NOT NULL,
			binding_instance_id varchar(255),
			operation_id varchar(128) NOT NULL,
			logical_operation_id varchar(128) NOT NULL,
			rollout_revision bigint NOT NULL,
			live_path varchar(32) NOT NULL,
			legacy_decision_digest varchar(64),
			connector_decision_digest varchar(64),
			matched boolean NOT NULL,
			difference_code varchar(64),
			observed_at timestamptz NOT NULL,
			CONSTRAINT connector_shadow_observations_target_type_check CHECK (target_type IN ('chat', 'agent', 'workflow')),
			CONSTRAINT connector_shadow_observations_target_check
				CHECK (
					(target_type IN ('chat', 'agent') AND btrim(target_id) <> '' AND binding_instance_id IS NULL)
					OR (target_type = 'workflow' AND btrim(target_id) <> '' AND btrim(binding_instance_id) <> '')
				),
			CONSTRAINT connector_shadow_observations_identity_check
				CHECK (
					operation_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'
					AND btrim(logical_operation_id) <> ''
					AND rollout_revision >= 1
					AND live_path = 'legacy'
				),
			CONSTRAINT connector_shadow_observations_result_check
				CHECK (
					(matched = true AND difference_code IS NULL AND legacy_decision_digest ~ '^[a-f0-9]{64}$' AND connector_decision_digest = legacy_decision_digest)
					OR (matched = false AND btrim(difference_code) <> '')
				),
			CONSTRAINT connector_shadow_observations_logical_unique
				UNIQUE (connector_id, rollout_revision, organization_id, logical_operation_id)
		);
		CREATE INDEX idx_connector_shadow_observations_provider
			ON public.connector_shadow_observations (connector_id, observed_at, matched);

		CREATE TABLE public.connector_rollout_evidence (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			connector_id varchar(128) NOT NULL REFERENCES public.connector_provider_rollouts(connector_id) ON DELETE RESTRICT,
			rollout_revision bigint NOT NULL,
			evidence_type varchar(64) NOT NULL,
			correlation_id varchar(128) NOT NULL,
			target_type varchar(16),
			path varchar(32),
			observed_at timestamptz NOT NULL,
			CONSTRAINT connector_rollout_evidence_revision_check CHECK (rollout_revision >= 1),
			CONSTRAINT connector_rollout_evidence_type_check CHECK (evidence_type IN (
				'verification_set_complete', 'terminal_outcome', 'forced_fallback', 'duplicate_submission',
				'outcome_gap', 'unknown_outcome_opened', 'unknown_outcome_resolved', 'legacy_dependency',
				'reversal_exercised', 'reversal_failed', 'operation_started', 'operation_completed',
				'reconnect_started', 'reconnect_completed'
			)),
			CONSTRAINT connector_rollout_evidence_target_check CHECK (target_type IS NULL OR target_type IN ('chat', 'agent', 'workflow')),
			CONSTRAINT connector_rollout_evidence_path_check CHECK (path IS NULL OR path IN ('legacy', 'connector_native')),
			CONSTRAINT connector_rollout_evidence_identity_check CHECK (btrim(correlation_id) <> ''),
			CONSTRAINT connector_rollout_evidence_unique UNIQUE (connector_id, rollout_revision, evidence_type, correlation_id)
		);
		CREATE INDEX idx_connector_rollout_evidence_provider
			ON public.connector_rollout_evidence (connector_id, observed_at, evidence_type);

		CREATE TRIGGER connector_shadow_observations_immutable
			BEFORE UPDATE OR DELETE ON public.connector_shadow_observations
			FOR EACH ROW EXECUTE FUNCTION public.reject_connector_immutable_mutation();
		CREATE TRIGGER connector_rollout_evidence_immutable
			BEFORE UPDATE OR DELETE ON public.connector_rollout_evidence
			FOR EACH ROW EXECUTE FUNCTION public.reject_connector_immutable_mutation();
	`
	dropConnectorMigrationRolloutStateSQL = `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM public.connector_migration_inventory LIMIT 1)
				OR EXISTS (SELECT 1 FROM public.connector_provider_rollouts LIMIT 1)
				OR EXISTS (SELECT 1 FROM public.connector_shadow_observations LIMIT 1)
				OR EXISTS (SELECT 1 FROM public.connector_rollout_evidence LIMIT 1) THEN
				RAISE EXCEPTION 'cannot remove Connector migration rollout state while evidence exists';
			END IF;
		END $$;
		DROP TABLE IF EXISTS public.connector_rollout_evidence;
		DROP TABLE IF EXISTS public.connector_shadow_observations;
		DROP TABLE IF EXISTS public.connector_provider_rollouts;
		DROP TABLE IF EXISTS public.connector_migration_inventory;
	`
)

func init() {
	registerSchemaMigration(migration20260902110000ID, up20260902110000, down20260902110000)
}

func up20260902110000(schema *mschema.Builder) error {
	return schema.Raw(createConnectorMigrationRolloutStateSQL)
}

func down20260902110000(schema *mschema.Builder) error {
	return schema.Raw(dropConnectorMigrationRolloutStateSQL)
}
