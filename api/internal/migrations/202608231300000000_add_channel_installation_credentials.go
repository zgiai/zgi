package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202608231300000000ID = "202608231300000000_add_channel_installation_credentials"

const addChannelInstallationCredentialsSQL = `
	ALTER TABLE public.channel_installations
		DROP CONSTRAINT IF EXISTS channel_installations_connection_id_fkey;
	ALTER TABLE public.channel_delivery_outbox
		DROP CONSTRAINT IF EXISTS channel_delivery_outbox_connection_id_fkey;

	DROP INDEX IF EXISTS public.idx_channel_installations_connection;
	CREATE INDEX IF NOT EXISTS idx_channel_installations_connection
		ON public.channel_installations (organization_id, workspace_id, provider, connection_id)
		WHERE deleted_at IS NULL;

	CREATE TABLE IF NOT EXISTS public.channel_installation_credentials (
		installation_id uuid PRIMARY KEY REFERENCES public.channel_installations(id) ON DELETE CASCADE,
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		provider varchar(32) NOT NULL,
		encrypted_credentials text NOT NULL,
		credential_version integer NOT NULL DEFAULT 1,
		created_by uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
		updated_by uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT channel_installation_credentials_provider_check CHECK (provider <> ''),
		CONSTRAINT channel_installation_credentials_envelope_check CHECK (encrypted_credentials <> ''),
		CONSTRAINT channel_installation_credentials_version_check CHECK (credential_version > 0)
	);
	CREATE INDEX IF NOT EXISTS idx_channel_installation_credentials_scope
		ON public.channel_installation_credentials (organization_id, workspace_id, provider);

	-- A directly configured installation uses its own UUID as the credential
	-- locator. Connection-center references continue to use the connection UUID.
	DROP TRIGGER IF EXISTS trg_channel_installations_scope ON public.channel_installations;

	CREATE OR REPLACE FUNCTION public.enforce_channel_installation_credential_scope()
	RETURNS trigger AS $$
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM public.channel_installations installation
			WHERE installation.id = NEW.installation_id
			  AND installation.organization_id = NEW.organization_id
			  AND installation.workspace_id = NEW.workspace_id
			  AND installation.provider = NEW.provider
			  AND installation.connection_id = NEW.installation_id
			  AND installation.deleted_at IS NULL
		) THEN
			RAISE EXCEPTION 'channel installation credential scope mismatch' USING ERRCODE = '23514';
		END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;

	DROP TRIGGER IF EXISTS trg_channel_installation_credentials_scope ON public.channel_installation_credentials;
	CREATE CONSTRAINT TRIGGER trg_channel_installation_credentials_scope
		AFTER INSERT OR UPDATE ON public.channel_installation_credentials
		DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW EXECUTE FUNCTION public.enforce_channel_installation_credential_scope();
`

func init() {
	registerSchemaMigration(migration202608231300000000ID, up202608231300000000, down202608231300000000)
}

func up202608231300000000(schema *mschema.Builder) error {
	return schema.Raw(addChannelInstallationCredentialsSQL)
}

func down202608231300000000(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TRIGGER IF EXISTS trg_channel_installation_credentials_scope ON public.channel_installation_credentials;
		DROP FUNCTION IF EXISTS public.enforce_channel_installation_credential_scope();
		DROP TABLE IF EXISTS public.channel_installation_credentials;
		DROP INDEX IF EXISTS public.idx_channel_installations_connection;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_installations_connection
			ON public.channel_installations (organization_id, workspace_id, provider, connection_id)
			WHERE deleted_at IS NULL;
		DO $$
		BEGIN
			-- The legacy link only exists while integration_connections does.
			IF to_regclass('public.integration_connections') IS NOT NULL THEN
				ALTER TABLE public.channel_installations
					ADD CONSTRAINT channel_installations_connection_id_fkey
					FOREIGN KEY (connection_id) REFERENCES public.integration_connections(id) ON DELETE RESTRICT;
				ALTER TABLE public.channel_delivery_outbox
					ADD CONSTRAINT channel_delivery_outbox_connection_id_fkey
					FOREIGN KEY (connection_id) REFERENCES public.integration_connections(id) ON DELETE RESTRICT;
			END IF;
		END $$;
		CREATE TRIGGER trg_channel_installations_scope
			BEFORE INSERT OR UPDATE ON public.channel_installations
			FOR EACH ROW EXECUTE FUNCTION public.enforce_channel_scope_consistency();
	`)
}
