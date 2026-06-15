package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202606071030000000ID = "202606071030000000_add_short_link_expires_at"

const addShortLinkExpiresAtColumnSQL = `
ALTER TABLE public.system_short_links
ADD COLUMN IF NOT EXISTS expires_at timestamp with time zone
`

const addShortLinkExpiresAtIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_system_short_links_expires_at
ON public.system_short_links (expires_at)
WHERE expires_at IS NOT NULL
`

const dropShortLinkExpiresAtIndexSQL = `
DROP INDEX IF EXISTS public.idx_system_short_links_expires_at
`

func init() {
	registerSchemaMigration(migration202606071030000000ID, up202606071030000000, down202606071030000000)
}

func up202606071030000000(schema *mschema.Builder) error {
	if err := schema.Raw(addShortLinkExpiresAtColumnSQL); err != nil {
		return err
	}
	if err := schema.Raw(addShortLinkExpiresAtIndexSQL); err != nil {
		return err
	}
	return nil
}

func down202606071030000000(schema *mschema.Builder) error {
	return schema.Raw(dropShortLinkExpiresAtIndexSQL)
}
