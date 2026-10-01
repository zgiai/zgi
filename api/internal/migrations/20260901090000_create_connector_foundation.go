package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260901090000ID    = "20260901090000_create_connector_foundation"
	createConnectorFoundationSQL = `
		CREATE TABLE public.connector_definitions (
			connector_id varchar(128) PRIMARY KEY,
			plugin_id varchar(128) NOT NULL,
			plugin_version varchar(64) NOT NULL,
			plugin_digest varchar(128) NOT NULL,
			source_dialect varchar(32) NOT NULL DEFAULT 'agentplugins-v1',
			schema_version varchar(32) NOT NULL DEFAULT '1.0.0',
			provider_id varchar(128) NOT NULL,
			driver_id varchar(128) NOT NULL,
			classification varchar(32) NOT NULL,
			display jsonb NOT NULL DEFAULT '{}'::jsonb,
			key_schema jsonb NOT NULL DEFAULT '{}'::jsonb,
			capability_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
			contract_digest varchar(128) NOT NULL,
			status varchar(32) NOT NULL DEFAULT 'active',
			accepted_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT connector_definitions_runtime_identity_unique
				UNIQUE (connector_id, provider_id, driver_id),
			CONSTRAINT connector_definitions_connector_id_check
				CHECK (connector_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_definitions_plugin_id_check
				CHECK (plugin_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_definitions_provider_id_check
				CHECK (provider_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_definitions_driver_id_check
				CHECK (driver_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_definitions_classification_check
				CHECK (classification IN ('connector', 'data_source')),
			CONSTRAINT connector_definitions_status_check
				CHECK (status IN ('active', 'disabled', 'unavailable')),
			CONSTRAINT connector_definitions_display_object_check
				CHECK (jsonb_typeof(display) = 'object'),
			CONSTRAINT connector_definitions_key_schema_object_check
				CHECK (jsonb_typeof(key_schema) = 'object'),
			CONSTRAINT connector_definitions_capabilities_object_check
				CHECK (jsonb_typeof(capability_summary) = 'object'),
			CONSTRAINT connector_definitions_digests_check
				CHECK (btrim(plugin_digest) <> '' AND btrim(contract_digest) <> '')
		);
		CREATE INDEX idx_connector_definitions_catalog
			ON public.connector_definitions (classification, status, connector_id);

		CREATE TABLE public.connector_connections (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			owner_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			connector_id varchar(128) NOT NULL,
			provider_id varchar(128) NOT NULL,
			driver_id varchar(128) NOT NULL,
			auth_method_id varchar(128) NOT NULL,
			name varchar(255) NOT NULL,
			external_account_id varchar(255),
			external_display_name varchar(255),
			encrypted_credentials text NOT NULL,
			credential_version integer NOT NULL DEFAULT 1,
			state varchar(32) NOT NULL DEFAULT 'ready',
			usable_operation_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
			unavailable_operations jsonb NOT NULL DEFAULT '{}'::jsonb,
			diagnostic_code varchar(64),
			revision integer NOT NULL DEFAULT 1,
			last_verified_at timestamptz NOT NULL,
			last_provider_success_at timestamptz,
			last_provider_failure_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at timestamptz,
			CONSTRAINT connector_connections_definition_fk
				FOREIGN KEY (connector_id, provider_id, driver_id)
				REFERENCES public.connector_definitions(connector_id, provider_id, driver_id) ON DELETE RESTRICT,
			CONSTRAINT connector_connections_owner_identity_unique
				UNIQUE (id, organization_id, owner_account_id, connector_id),
			CONSTRAINT connector_connections_connector_id_check
				CHECK (connector_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_connections_provider_id_check
				CHECK (provider_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_connections_driver_id_check
				CHECK (driver_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_connections_auth_method_id_check
				CHECK (auth_method_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_connections_name_check
				CHECK (btrim(name) <> ''),
			CONSTRAINT connector_connections_envelope_check
				CHECK (char_length(encrypted_credentials) > 3),
			CONSTRAINT connector_connections_credential_version_positive
				CHECK (credential_version >= 1),
			CONSTRAINT connector_connections_revision_positive
				CHECK (revision >= 1),
			CONSTRAINT connector_connections_state_check
				CHECK (state IN ('ready', 'reconnect_required', 'unavailable')),
			CONSTRAINT connector_connections_usable_operations_array_check
				CHECK (
					jsonb_typeof(usable_operation_ids) = 'array'
					AND (state <> 'ready' OR jsonb_array_length(usable_operation_ids) > 0)
				),
			CONSTRAINT connector_connections_unavailable_operations_object_check
				CHECK (jsonb_typeof(unavailable_operations) = 'object')
		);
		CREATE INDEX idx_connector_connections_owner_catalog
			ON public.connector_connections (organization_id, owner_account_id, connector_id, state)
			WHERE deleted_at IS NULL;
		CREATE INDEX idx_connector_connections_readiness
			ON public.connector_connections (connector_id, state, updated_at)
			WHERE deleted_at IS NULL;

		CREATE OR REPLACE FUNCTION public.enforce_connector_connection_legacy_collision()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			IF EXISTS (
				SELECT 1
				FROM public.integration_connections AS legacy
				WHERE legacy.id = NEW.id
					AND (
						legacy.credential_source <> 'account'
						OR legacy.organization_id <> NEW.organization_id
						OR legacy.owner_account_id IS DISTINCT FROM NEW.owner_account_id
						OR lower(legacy.integration_id) <> NEW.provider_id
						OR lower(legacy.driver_id) <> NEW.driver_id
					)
			) THEN
				RAISE EXCEPTION 'connector connection identity collides with a non-personal legacy connection'
					USING ERRCODE = '23514';
			END IF;
			RETURN NEW;
		END
		$$;
		CREATE TRIGGER connector_connections_legacy_collision_guard
			BEFORE INSERT OR UPDATE OF id, organization_id, owner_account_id, provider_id, driver_id
			ON public.connector_connections
			FOR EACH ROW
			EXECUTE FUNCTION public.enforce_connector_connection_legacy_collision();

		CREATE TABLE public.connector_key_setup_attempts (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			owner_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			connector_id varchar(128) NOT NULL REFERENCES public.connector_definitions(connector_id) ON DELETE RESTRICT,
			connection_id uuid,
			target_type varchar(16),
			target_id varchar(255),
			binding_instance_id varchar(255),
			-- Server-keyed HMAC-SHA256 hex digests only. The request
			-- fingerprint covers the normalized command, Connector, target,
			-- submitted field names, and candidate values; no raw input or
			-- reversible value hash is retained.
			idempotency_key_hash varchar(128) NOT NULL,
			request_fingerprint varchar(128) NOT NULL,
			state varchar(32) NOT NULL DEFAULT 'verifying',
			error_code varchar(64),
			result_connection_id uuid,
			expires_at timestamptz NOT NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT connector_key_setup_attempts_connection_fk
				FOREIGN KEY (connection_id, organization_id, owner_account_id, connector_id)
				REFERENCES public.connector_connections(id, organization_id, owner_account_id, connector_id) ON DELETE RESTRICT,
			CONSTRAINT connector_key_setup_attempts_result_connection_fk
				FOREIGN KEY (result_connection_id, organization_id, owner_account_id, connector_id)
				REFERENCES public.connector_connections(id, organization_id, owner_account_id, connector_id) ON DELETE RESTRICT,
			CONSTRAINT connector_key_setup_attempts_state_check
				CHECK (state IN ('verifying', 'ready', 'failed', 'cancelled')),
			CONSTRAINT connector_key_setup_attempts_target_type_check
				CHECK (target_type IS NULL OR target_type IN ('chat', 'agent', 'workflow')),
			CONSTRAINT connector_key_setup_attempts_target_context_check
				CHECK (
					(target_type IS NULL AND target_id IS NULL AND binding_instance_id IS NULL)
					OR (
						target_type IN ('chat', 'agent')
						AND target_id IS NOT NULL
						AND btrim(target_id) <> ''
						AND binding_instance_id IS NULL
					)
					OR (
						target_type = 'workflow'
						AND target_id IS NOT NULL
						AND btrim(target_id) <> ''
						AND binding_instance_id IS NOT NULL
						AND btrim(binding_instance_id) <> ''
					)
				),
			CONSTRAINT connector_key_setup_attempts_result_check
				CHECK ((state = 'ready' AND result_connection_id IS NOT NULL) OR (state <> 'ready' AND result_connection_id IS NULL)),
			CONSTRAINT connector_key_setup_attempts_error_check
				CHECK ((state = 'failed' AND error_code IS NOT NULL) OR (state <> 'failed' AND error_code IS NULL)),
			CONSTRAINT connector_key_setup_attempts_idempotency_hash_check
				CHECK (idempotency_key_hash ~ '^[0-9a-f]{64}$'),
			CONSTRAINT connector_key_setup_attempts_request_fingerprint_check
				CHECK (request_fingerprint ~ '^[0-9a-f]{64}$')
		);
		CREATE UNIQUE INDEX idx_connector_key_setups_owner_idempotency
			ON public.connector_key_setup_attempts (organization_id, owner_account_id, idempotency_key_hash);
		CREATE INDEX idx_connector_key_setups_owner_state
			ON public.connector_key_setup_attempts (organization_id, owner_account_id, state, expires_at);

		CREATE TABLE public.connector_target_bindings (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			owner_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			connection_id uuid NOT NULL,
			connector_id varchar(128) NOT NULL,
			target_type varchar(16) NOT NULL,
			target_id varchar(255) NOT NULL,
			binding_instance_id varchar(255),
			status varchar(32) NOT NULL DEFAULT 'active',
			compatibility varchar(32) NOT NULL DEFAULT 'native',
			compatibility_state jsonb NOT NULL DEFAULT '{}'::jsonb,
			revision integer NOT NULL DEFAULT 1,
			disabled_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT connector_target_bindings_connection_fk
				FOREIGN KEY (connection_id, organization_id, owner_account_id, connector_id)
				REFERENCES public.connector_connections(id, organization_id, owner_account_id, connector_id) ON DELETE RESTRICT,
			CONSTRAINT connector_target_bindings_target_type_check
				CHECK (target_type IN ('chat', 'agent', 'workflow')),
			CONSTRAINT connector_target_bindings_status_check
				CHECK (status IN ('active', 'disabled', 'needs_replacement')),
			CONSTRAINT connector_target_bindings_compatibility_check
				CHECK (compatibility IN ('native', 'legacy_read_only')),
			CONSTRAINT connector_target_bindings_target_check
				CHECK (btrim(target_id) <> ''),
			CONSTRAINT connector_target_bindings_instance_check
				CHECK (
					(target_type IN ('chat', 'agent') AND binding_instance_id IS NULL)
					OR (target_type = 'workflow' AND binding_instance_id IS NOT NULL AND btrim(binding_instance_id) <> '')
				),
			CONSTRAINT connector_target_bindings_compatibility_object_check
				CHECK (jsonb_typeof(compatibility_state) = 'object'),
			CONSTRAINT connector_target_bindings_revision_positive
				CHECK (revision >= 1)
		);
		CREATE UNIQUE INDEX idx_connector_target_bindings_logical_key
			ON public.connector_target_bindings (
				organization_id,
				owner_account_id,
				target_type,
				target_id,
				connector_id,
				COALESCE(binding_instance_id, '')
			);
		CREATE INDEX idx_connector_target_bindings_connection
			ON public.connector_target_bindings (organization_id, owner_account_id, connection_id, status);
		CREATE INDEX idx_connector_target_bindings_target
			ON public.connector_target_bindings (organization_id, target_type, target_id, status);

		CREATE TABLE public.connector_execution_route_claims (
			receipt_id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			owner_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			acting_principal_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			binding_id uuid NOT NULL,
			connection_id uuid NOT NULL,
			connector_id varchar(128) NOT NULL,
			target_type varchar(16) NOT NULL,
			target_id varchar(255) NOT NULL,
			binding_instance_id varchar(255),
			operation_id varchar(128) NOT NULL,
			logical_operation_id varchar(128) NOT NULL,
			correlation_id varchar(128) NOT NULL,
			path varchar(32) NOT NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT connector_execution_route_claims_connector_id_check
				CHECK (connector_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_execution_route_claims_target_type_check
				CHECK (target_type IN ('chat', 'agent', 'workflow')),
			CONSTRAINT connector_execution_route_claims_target_check
				CHECK (
					(
						target_type IN ('chat', 'agent')
						AND btrim(target_id) <> ''
						AND binding_instance_id IS NULL
					)
					OR (
						target_type = 'workflow'
						AND btrim(target_id) <> ''
						AND binding_instance_id IS NOT NULL AND btrim(binding_instance_id) <> ''
					)
				),
			CONSTRAINT connector_execution_route_claims_operation_id_check
				CHECK (operation_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_execution_route_claims_logical_operation_check
				CHECK (btrim(logical_operation_id) <> ''),
			CONSTRAINT connector_execution_route_claims_correlation_check
				CHECK (btrim(correlation_id) <> ''),
			CONSTRAINT connector_execution_route_claims_path_check
				CHECK (path IN ('connector_native', 'legacy')),
			CONSTRAINT connector_execution_route_claims_logical_unique
				UNIQUE (organization_id, logical_operation_id)
		);
		CREATE INDEX idx_connector_execution_route_claims_connection
			ON public.connector_execution_route_claims (connection_id, created_at);

		CREATE TABLE public.connector_lifecycle_decisions (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			owner_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			acting_principal_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			connector_id varchar(128) NOT NULL REFERENCES public.connector_definitions(connector_id) ON DELETE RESTRICT,
			connection_id uuid,
			target_type varchar(16),
			target_id varchar(255),
			binding_instance_id varchar(255),
			operation_id varchar(128),
			logical_operation_id varchar(128),
			decision varchar(64) NOT NULL,
			status varchar(32) NOT NULL,
			correlation_id varchar(128) NOT NULL,
			receipt_id varchar(128),
			details jsonb NOT NULL DEFAULT '{}'::jsonb,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT connector_lifecycle_decisions_connection_fk
				FOREIGN KEY (connection_id, organization_id, owner_account_id, connector_id)
				REFERENCES public.connector_connections(id, organization_id, owner_account_id, connector_id) ON DELETE RESTRICT,
			CONSTRAINT connector_lifecycle_decisions_owner_check
				CHECK (acting_principal_id = owner_account_id),
			CONSTRAINT connector_lifecycle_decisions_target_type_check
				CHECK (target_type IS NULL OR target_type IN ('chat', 'agent', 'workflow')),
			CONSTRAINT connector_lifecycle_decisions_target_context_check
				CHECK (
					(target_type IS NULL AND target_id IS NULL AND binding_instance_id IS NULL)
					OR (
						target_type IN ('chat', 'agent')
						AND target_id IS NOT NULL AND btrim(target_id) <> ''
						AND binding_instance_id IS NULL
					)
					OR (
						target_type = 'workflow'
						AND target_id IS NOT NULL AND btrim(target_id) <> ''
						AND binding_instance_id IS NOT NULL AND btrim(binding_instance_id) <> ''
					)
				),
			CONSTRAINT connector_lifecycle_decisions_operation_id_check
				CHECK (operation_id IS NULL OR operation_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_lifecycle_decisions_status_check
				CHECK (status IN ('submitted', 'denied', 'blocked', 'cancelled', 'failed', 'successful', 'unknown')),
			CONSTRAINT connector_lifecycle_decisions_identity_check
				CHECK (btrim(decision) <> '' AND btrim(correlation_id) <> ''),
			CONSTRAINT connector_lifecycle_decisions_submission_identity_check
				CHECK (
					status <> 'submitted'
					OR (
						connection_id IS NOT NULL
						AND target_type IS NOT NULL
						AND operation_id IS NOT NULL
						AND logical_operation_id IS NOT NULL
						AND btrim(logical_operation_id) <> ''
					)
				),
			CONSTRAINT connector_lifecycle_decisions_details_object_check
				CHECK (jsonb_typeof(details) = 'object')
		);
		CREATE UNIQUE INDEX idx_connector_lifecycle_decisions_correlation_status
			ON public.connector_lifecycle_decisions (organization_id, correlation_id, decision, status);
		CREATE INDEX idx_connector_lifecycle_decisions_owner_created
			ON public.connector_lifecycle_decisions (organization_id, owner_account_id, created_at);
		CREATE INDEX idx_connector_lifecycle_decisions_connection_created
			ON public.connector_lifecycle_decisions (connection_id, created_at)
			WHERE connection_id IS NOT NULL;

		CREATE TABLE public.connector_operation_outcomes (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			decision_id uuid NOT NULL UNIQUE REFERENCES public.connector_lifecycle_decisions(id) ON DELETE RESTRICT,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			owner_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			acting_principal_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			connector_id varchar(128) NOT NULL REFERENCES public.connector_definitions(connector_id) ON DELETE RESTRICT,
			connection_id uuid NOT NULL,
			target_type varchar(16) NOT NULL,
			target_id varchar(255) NOT NULL,
			binding_instance_id varchar(255),
			operation_id varchar(128) NOT NULL,
			logical_operation_id varchar(128) NOT NULL,
			status varchar(32) NOT NULL,
			provider_request_id varchar(128),
			diagnostic_code varchar(64),
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT connector_operation_outcomes_connection_fk
				FOREIGN KEY (connection_id, organization_id, owner_account_id, connector_id)
				REFERENCES public.connector_connections(id, organization_id, owner_account_id, connector_id) ON DELETE RESTRICT,
			CONSTRAINT connector_operation_outcomes_owner_check
				CHECK (acting_principal_id = owner_account_id),
			CONSTRAINT connector_operation_outcomes_target_type_check
				CHECK (target_type IN ('chat', 'agent', 'workflow')),
			CONSTRAINT connector_operation_outcomes_target_check
				CHECK (
					(
						target_type IN ('chat', 'agent')
						AND btrim(target_id) <> ''
						AND binding_instance_id IS NULL
					)
					OR (
						target_type = 'workflow'
						AND btrim(target_id) <> ''
						AND binding_instance_id IS NOT NULL AND btrim(binding_instance_id) <> ''
					)
				),
			CONSTRAINT connector_operation_outcomes_operation_id_check
				CHECK (operation_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_operation_outcomes_logical_operation_check
				CHECK (btrim(logical_operation_id) <> ''),
			CONSTRAINT connector_operation_outcomes_status_check
				CHECK (status IN ('successful', 'failed', 'unknown'))
		);
		CREATE UNIQUE INDEX idx_connector_operation_outcomes_logical_operation
			ON public.connector_operation_outcomes (organization_id, logical_operation_id);
		CREATE INDEX idx_connector_operation_outcomes_connection_created
			ON public.connector_operation_outcomes (connection_id, created_at);

		CREATE OR REPLACE FUNCTION public.enforce_connector_operation_outcome()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			PERFORM 1
			FROM public.connector_lifecycle_decisions AS decision
			WHERE decision.id = NEW.decision_id
				AND decision.status = 'submitted'
				AND decision.organization_id = NEW.organization_id
				AND decision.owner_account_id = NEW.owner_account_id
				AND decision.acting_principal_id = NEW.acting_principal_id
				AND decision.connector_id = NEW.connector_id
				AND decision.connection_id = NEW.connection_id
				AND decision.target_type = NEW.target_type
				AND decision.target_id = NEW.target_id
				AND decision.binding_instance_id IS NOT DISTINCT FROM NEW.binding_instance_id
				AND decision.operation_id = NEW.operation_id
				AND decision.logical_operation_id = NEW.logical_operation_id
			FOR SHARE;
			IF NOT FOUND THEN
				RAISE EXCEPTION 'connector outcome requires its matching submitted decision'
					USING ERRCODE = '23514';
			END IF;
			RETURN NEW;
		END
		$$;
		CREATE TRIGGER connector_operation_outcomes_submission_guard
			BEFORE INSERT OR UPDATE
			ON public.connector_operation_outcomes
			FOR EACH ROW
			EXECUTE FUNCTION public.enforce_connector_operation_outcome();

		CREATE TABLE public.connector_legacy_identity_mappings (
			connection_id uuid PRIMARY KEY,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			owner_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			connector_id varchar(128) NOT NULL,
			legacy_provider_id varchar(128) NOT NULL,
			legacy_driver_id varchar(128) NOT NULL,
			cutover_state varchar(32) NOT NULL DEFAULT 'candidate',
			restriction_digest varchar(128) NOT NULL,
			verification_digest varchar(128),
			reversal_snapshot_ref varchar(255),
			staged_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			cutover_at timestamptz,
			reversed_at timestamptz,
			last_observed_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT connector_legacy_identity_mappings_connection_fk
				FOREIGN KEY (connection_id, organization_id, owner_account_id, connector_id)
				REFERENCES public.connector_connections(id, organization_id, owner_account_id, connector_id) ON DELETE RESTRICT,
			CONSTRAINT connector_legacy_identity_mappings_provider_id_check
				CHECK (legacy_provider_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_legacy_identity_mappings_driver_id_check
				CHECK (legacy_driver_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_legacy_identity_mappings_cutover_state_check
				CHECK (cutover_state IN ('candidate', 'shadow', 'connector', 'reversed', 'blocked')),
			CONSTRAINT connector_legacy_identity_mappings_restriction_digest_check
				CHECK (btrim(restriction_digest) <> ''),
			CONSTRAINT connector_legacy_identity_mappings_verification_digest_check
				CHECK (verification_digest IS NULL OR btrim(verification_digest) <> ''),
			CONSTRAINT connector_legacy_identity_mappings_snapshot_ref_check
				CHECK (reversal_snapshot_ref IS NULL OR btrim(reversal_snapshot_ref) <> '')
		);
		CREATE INDEX idx_connector_legacy_identity_mappings_cutover
			ON public.connector_legacy_identity_mappings (connector_id, cutover_state, updated_at);

		CREATE OR REPLACE FUNCTION public.enforce_connector_legacy_personal_mapping()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			PERFORM 1
			FROM public.integration_connections AS legacy
			JOIN public.connector_connections AS native
				ON native.id = NEW.connection_id
				AND native.organization_id = NEW.organization_id
				AND native.owner_account_id = NEW.owner_account_id
				AND native.connector_id = NEW.connector_id
				AND native.provider_id = NEW.legacy_provider_id
				AND native.driver_id = NEW.legacy_driver_id
			WHERE legacy.id = NEW.connection_id
				AND legacy.organization_id = NEW.organization_id
				AND legacy.credential_source = 'account'
				AND legacy.owner_account_id = NEW.owner_account_id
				AND lower(legacy.integration_id) = NEW.legacy_provider_id
				AND lower(legacy.driver_id) = NEW.legacy_driver_id
			FOR SHARE;
			IF NOT FOUND THEN
				RAISE EXCEPTION 'connector identity mapping requires an exact legacy personal connection'
					USING ERRCODE = '23514';
			END IF;
			RETURN NEW;
		END
		$$;
		CREATE TRIGGER connector_legacy_identity_mappings_personal_guard
			BEFORE INSERT OR UPDATE OF connection_id, organization_id, owner_account_id, legacy_provider_id, legacy_driver_id
			ON public.connector_legacy_identity_mappings
			FOR EACH ROW
			EXECUTE FUNCTION public.enforce_connector_legacy_personal_mapping();

		CREATE TABLE public.connector_mutation_journal (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			owner_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			connection_id uuid NOT NULL,
			connector_id varchar(128) NOT NULL,
			sequence_number bigint NOT NULL,
			mutation_type varchar(64) NOT NULL,
			credential_version integer,
			protected_credential_ref varchar(255),
			tombstone boolean NOT NULL DEFAULT false,
			correlation_id varchar(128) NOT NULL,
			details jsonb NOT NULL DEFAULT '{}'::jsonb,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT connector_mutation_journal_connection_fk
				FOREIGN KEY (connection_id, organization_id, owner_account_id, connector_id)
				REFERENCES public.connector_connections(id, organization_id, owner_account_id, connector_id) ON DELETE RESTRICT,
			CONSTRAINT connector_mutation_journal_sequence_positive
				CHECK (sequence_number >= 1),
			CONSTRAINT connector_mutation_journal_credential_version_check
				CHECK (credential_version IS NULL OR credential_version >= 1),
			CONSTRAINT connector_mutation_journal_protected_ref_check
				CHECK (protected_credential_ref IS NULL OR (credential_version IS NOT NULL AND btrim(protected_credential_ref) <> '')),
			CONSTRAINT connector_mutation_journal_identity_check
				CHECK (btrim(mutation_type) <> '' AND btrim(correlation_id) <> ''),
			CONSTRAINT connector_mutation_journal_details_object_check
				CHECK (jsonb_typeof(details) = 'object'),
			CONSTRAINT connector_mutation_journal_connection_sequence_unique
				UNIQUE (connection_id, sequence_number),
			CONSTRAINT connector_mutation_journal_correlation_unique
				UNIQUE (connection_id, correlation_id, mutation_type)
		);
		CREATE INDEX idx_connector_mutation_journal_owner_created
			ON public.connector_mutation_journal (organization_id, owner_account_id, created_at);

		CREATE OR REPLACE FUNCTION public.reject_connector_immutable_mutation()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE EXCEPTION 'connector audit and mutation records are immutable'
				USING ERRCODE = '55000';
		END
		$$;
		CREATE TRIGGER connector_lifecycle_decisions_immutable
			BEFORE UPDATE OR DELETE ON public.connector_lifecycle_decisions
			FOR EACH ROW EXECUTE FUNCTION public.reject_connector_immutable_mutation();
		CREATE TRIGGER connector_operation_outcomes_immutable
			BEFORE UPDATE OR DELETE ON public.connector_operation_outcomes
			FOR EACH ROW EXECUTE FUNCTION public.reject_connector_immutable_mutation();
		CREATE TRIGGER connector_mutation_journal_immutable
			BEFORE UPDATE OR DELETE ON public.connector_mutation_journal
			FOR EACH ROW EXECUTE FUNCTION public.reject_connector_immutable_mutation();
		CREATE TRIGGER connector_execution_route_claims_immutable
			BEFORE UPDATE OR DELETE ON public.connector_execution_route_claims
			FOR EACH ROW EXECUTE FUNCTION public.reject_connector_immutable_mutation();

		CREATE OR REPLACE FUNCTION public.enforce_agent_integration_connection_binding()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			IF NEW.binding_type = 'integration_connection' THEN
				PERFORM 1
				FROM public.integration_connections AS connection
				WHERE connection.id::text = lower(trim(NEW.resource_id))
					AND connection.organization_id = NEW.organization_id
					AND lower(connection.integration_id) = lower(trim(NEW.parent_resource_id))
					AND connection.status = 'active'
					AND connection.deleted_at IS NULL
					AND (connection.expires_at IS NULL OR connection.expires_at > CURRENT_TIMESTAMP)
				FOR SHARE;
				IF FOUND THEN
					RETURN NEW;
				END IF;

				PERFORM 1
				FROM public.connector_connections AS connection
				JOIN public.connector_definitions AS definition
					ON definition.connector_id = connection.connector_id
				WHERE connection.id::text = lower(trim(NEW.resource_id))
					AND connection.organization_id = NEW.organization_id
					AND connection.owner_account_id = NEW.authorized_by
					AND (
						connection.connector_id = lower(trim(NEW.parent_resource_id))
						OR connection.provider_id = lower(trim(NEW.parent_resource_id))
					)
					AND connection.state = 'ready'
					AND connection.deleted_at IS NULL
					AND definition.status = 'active'
				FOR SHARE OF connection;
				IF NOT FOUND THEN
					RAISE EXCEPTION 'integration connection binding is unavailable'
						USING ERRCODE = '23503';
				END IF;
			END IF;
			RETURN NEW;
		END
		$$;
	`
	downConnectorFoundationSQL = `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM public.connector_connections LIMIT 1)
				OR EXISTS (SELECT 1 FROM public.connector_key_setup_attempts LIMIT 1)
				OR EXISTS (SELECT 1 FROM public.connector_target_bindings LIMIT 1)
				OR EXISTS (SELECT 1 FROM public.connector_execution_route_claims LIMIT 1)
				OR EXISTS (SELECT 1 FROM public.connector_lifecycle_decisions LIMIT 1)
				OR EXISTS (SELECT 1 FROM public.connector_operation_outcomes LIMIT 1)
				OR EXISTS (SELECT 1 FROM public.connector_legacy_identity_mappings LIMIT 1)
				OR EXISTS (SELECT 1 FROM public.connector_mutation_journal LIMIT 1)
				OR EXISTS (SELECT 1 FROM public.connector_definitions LIMIT 1) THEN
				RAISE EXCEPTION 'cannot remove connector foundation while authoritative records exist';
			END IF;
		END
		$$;

		CREATE OR REPLACE FUNCTION public.enforce_agent_integration_connection_binding()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			IF NEW.binding_type = 'integration_connection' THEN
				PERFORM 1
				FROM public.integration_connections AS connection
				WHERE connection.id::text = lower(trim(NEW.resource_id))
					AND connection.organization_id = NEW.organization_id
					AND lower(connection.integration_id) = lower(trim(NEW.parent_resource_id))
					AND connection.status = 'active'
					AND connection.deleted_at IS NULL
					AND (connection.expires_at IS NULL OR connection.expires_at > CURRENT_TIMESTAMP)
				FOR SHARE;
				IF NOT FOUND THEN
					RAISE EXCEPTION 'integration connection binding is unavailable'
						USING ERRCODE = '23503';
				END IF;
			END IF;
			RETURN NEW;
		END
		$$;

		DROP TABLE IF EXISTS public.connector_operation_outcomes;
		DROP TABLE IF EXISTS public.connector_mutation_journal;
		DROP TABLE IF EXISTS public.connector_legacy_identity_mappings;
		DROP TABLE IF EXISTS public.connector_lifecycle_decisions;
		DROP TABLE IF EXISTS public.connector_execution_route_claims;
		DROP TABLE IF EXISTS public.connector_target_bindings;
		DROP TABLE IF EXISTS public.connector_key_setup_attempts;
		DROP TABLE IF EXISTS public.connector_connections;
		DROP TABLE IF EXISTS public.connector_definitions;
		DROP FUNCTION IF EXISTS public.reject_connector_immutable_mutation();
		DROP FUNCTION IF EXISTS public.enforce_connector_operation_outcome();
		DROP FUNCTION IF EXISTS public.enforce_connector_legacy_personal_mapping();
		DROP FUNCTION IF EXISTS public.enforce_connector_connection_legacy_collision();
	`
)

func init() {
	registerSchemaMigration(migration20260901090000ID, up20260901090000, down20260901090000)
}

func up20260901090000(schema *mschema.Builder) error {
	return schema.Raw(createConnectorFoundationSQL)
}

func down20260901090000(schema *mschema.Builder) error {
	return schema.Raw(downConnectorFoundationSQL)
}
