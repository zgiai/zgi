package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260902150000ID = "20260902150000_create_connector_reversal_baselines"

	createConnectorReversalBaselinesSQL = `
		CREATE TABLE public.connector_reversal_baselines (
			snapshot_ref varchar(255) PRIMARY KEY,
			connection_id uuid NOT NULL UNIQUE,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			owner_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			connector_id varchar(128) NOT NULL,
			provider_id varchar(128) NOT NULL,
			driver_id varchar(128) NOT NULL,
			auth_method_id varchar(128) NOT NULL,
			credential_source varchar(32) NOT NULL,
			name varchar(128) NOT NULL,
			external_account_id varchar(255),
			external_display_name varchar(255),
			encrypted_credentials text NOT NULL,
			credential_version integer NOT NULL,
			readiness varchar(32) NOT NULL,
			diagnostic_code varchar(64),
			last_verified_at timestamptz NOT NULL,
			legacy_created_at timestamptz NOT NULL,
			legacy_updated_at timestamptz NOT NULL,
			restriction_digest varchar(64) NOT NULL,
			baseline_bindings jsonb NOT NULL DEFAULT '[]'::jsonb,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT connector_reversal_baselines_connection_fk
				FOREIGN KEY (connection_id, organization_id, owner_account_id, connector_id)
				REFERENCES public.connector_connections(id, organization_id, owner_account_id, connector_id) ON DELETE RESTRICT,
			CONSTRAINT connector_reversal_baselines_legacy_fk
				FOREIGN KEY (connection_id) REFERENCES public.integration_connections(id) ON DELETE RESTRICT,
			CONSTRAINT connector_reversal_baselines_identity_check
				CHECK (
					connector_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'
					AND provider_id = connector_id
					AND driver_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'
					AND auth_method_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'
					AND credential_source = 'account'
					AND btrim(snapshot_ref) <> ''
					AND btrim(name) <> ''
				),
			CONSTRAINT connector_reversal_baselines_credential_check
				CHECK (credential_version >= 1 AND length(btrim(encrypted_credentials)) > 3),
			CONSTRAINT connector_reversal_baselines_readiness_check
				CHECK (readiness IN ('ready', 'reconnect_required', 'unavailable')),
			CONSTRAINT connector_reversal_baselines_restriction_check
				CHECK (restriction_digest ~ '^[a-f0-9]{64}$' AND jsonb_typeof(baseline_bindings) = 'array')
		);

		CREATE OR REPLACE FUNCTION public.enforce_connector_reversal_baseline_identity()
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
				AND native.provider_id = NEW.provider_id
				AND native.driver_id = NEW.driver_id
				AND native.auth_method_id = NEW.auth_method_id
			WHERE legacy.id = NEW.connection_id
				AND legacy.organization_id = NEW.organization_id
				AND legacy.credential_source = NEW.credential_source
				AND legacy.owner_account_id = NEW.owner_account_id
				AND lower(legacy.integration_id) = NEW.provider_id
				AND lower(legacy.driver_id) = NEW.driver_id
				AND lower(legacy.auth_method_id) = NEW.auth_method_id
			FOR SHARE;
			IF NOT FOUND THEN
				RAISE EXCEPTION 'Connector reversal baseline requires an exact legacy Personal Connection'
					USING ERRCODE = '23514';
			END IF;
			RETURN NEW;
		END
		$$;
		CREATE TRIGGER connector_reversal_baselines_identity_guard
			BEFORE INSERT ON public.connector_reversal_baselines
			FOR EACH ROW EXECUTE FUNCTION public.enforce_connector_reversal_baseline_identity();

		CREATE TRIGGER connector_reversal_baselines_immutable
			BEFORE UPDATE OR DELETE ON public.connector_reversal_baselines
			FOR EACH ROW EXECUTE FUNCTION public.reject_connector_immutable_mutation();
	`

	dropConnectorReversalBaselinesSQL = `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM public.connector_reversal_baselines LIMIT 1) THEN
				RAISE EXCEPTION 'cannot remove Connector reversal baselines while snapshots exist';
			END IF;
		END $$;
		DROP TABLE IF EXISTS public.connector_reversal_baselines;
		DROP FUNCTION IF EXISTS public.enforce_connector_reversal_baseline_identity();
	`
)

func init() {
	registerSchemaMigration(migration20260902150000ID, up20260902150000, down20260902150000)
}

func up20260902150000(schema *mschema.Builder) error {
	return schema.Raw(createConnectorReversalBaselinesSQL)
}

func down20260902150000(schema *mschema.Builder) error {
	return schema.Raw(dropConnectorReversalBaselinesSQL)
}
