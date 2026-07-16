package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddAgentPublishedVersionNameID = "20260715100000_add_agent_published_version_name"

func init() {
	registerSchemaMigration(
		migrationAddAgentPublishedVersionNameID,
		upAddAgentPublishedVersionName,
		func(schema *mschema.Builder) error {
			return schema.Raw(`ALTER TABLE public.agent_published_versions DROP COLUMN IF EXISTS name`)
		},
	)
}

func upAddAgentPublishedVersionName(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.agent_published_versions
			ADD COLUMN IF NOT EXISTS name character varying(80) NOT NULL DEFAULT ''
	`)
}
