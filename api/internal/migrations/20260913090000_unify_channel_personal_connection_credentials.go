package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migration20260913090000ID = "20260913090000_unify_channel_personal_connection_credentials"

const unifyChannelPersonalConnectionCredentialsSQL = `
ALTER TABLE public.channel_installations
    ADD COLUMN IF NOT EXISTS connector_id varchar(128),
    ADD COLUMN IF NOT EXISTS connection_owner_id uuid,
    ADD COLUMN IF NOT EXISTS credential_source varchar(32) NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS connection_revision bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS connection_credential_version integer NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'channel_installations_credential_source_check'
          AND conrelid = 'public.channel_installations'::regclass
    ) THEN
        ALTER TABLE public.channel_installations
            ADD CONSTRAINT channel_installations_credential_source_check
            CHECK (credential_source IN ('embedded', 'personal_connection', 'legacy'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'channel_installations_credential_locator_check'
          AND conrelid = 'public.channel_installations'::regclass
    ) THEN
        ALTER TABLE public.channel_installations
            ADD CONSTRAINT channel_installations_credential_locator_check
            CHECK (
                (credential_source = 'embedded' AND connection_id = id AND connection_owner_id IS NOT NULL)
                OR (credential_source = 'personal_connection' AND connection_id <> id AND connector_id <> '' AND connection_owner_id IS NOT NULL)
                OR credential_source = 'legacy'
            );
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'channel_installations_connection_owner_id_fkey'
          AND conrelid = 'public.channel_installations'::regclass
    ) THEN
        ALTER TABLE public.channel_installations
            ADD CONSTRAINT channel_installations_connection_owner_id_fkey
            FOREIGN KEY (connection_owner_id) REFERENCES public.accounts(id) ON DELETE RESTRICT;
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_channel_installations_personal_connection
    ON public.channel_installations (organization_id, workspace_id, connector_id, connection_id)
    WHERE deleted_at IS NULL AND credential_source = 'personal_connection';
`

const backfillChannelPersonalConnectionCredentialSQL = `
UPDATE public.channel_installations
SET credential_source = 'embedded',
    connection_owner_id = COALESCE(connection_owner_id, created_by),
    connector_id = COALESCE(NULLIF(connector_id, ''), lower(provider))
WHERE credential_source = 'legacy' AND connection_id = id;
`

func init() {
	registerSchemaMigration(migration20260913090000ID, up20260913090000, down20260913090000)
}

func up20260913090000(schema *mschema.Builder) error {
	if err := schema.Raw(unifyChannelPersonalConnectionCredentialsSQL); err != nil {
		return err
	}
	return schema.DataFix("classify legacy DigitalHuman channel credential sources", func(db *gorm.DB) error {
		return db.Exec(backfillChannelPersonalConnectionCredentialSQL).Error
	})
}

func down20260913090000(schema *mschema.Builder) error {
	return schema.Raw(`
DROP INDEX IF EXISTS public.idx_channel_installations_personal_connection;
ALTER TABLE public.channel_installations
    DROP CONSTRAINT IF EXISTS channel_installations_connection_owner_id_fkey,
    DROP CONSTRAINT IF EXISTS channel_installations_credential_locator_check,
    DROP CONSTRAINT IF EXISTS channel_installations_credential_source_check,
    DROP COLUMN IF EXISTS connection_credential_version,
    DROP COLUMN IF EXISTS connection_revision,
    DROP COLUMN IF EXISTS credential_source,
    DROP COLUMN IF EXISTS connection_owner_id,
    DROP COLUMN IF EXISTS connector_id;
`)
}
