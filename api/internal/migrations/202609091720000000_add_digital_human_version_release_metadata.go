package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddDigitalHumanVersionReleaseMetadataID = "202609091720000000_add_digital_human_version_release_metadata"

func init() {
	registerSchemaMigration(
		migrationAddDigitalHumanVersionReleaseMetadataID,
		upAddDigitalHumanVersionReleaseMetadata,
		downAddDigitalHumanVersionReleaseMetadata,
	)
}

func upAddDigitalHumanVersionReleaseMetadata(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.digital_human_versions
			ADD COLUMN IF NOT EXISTS name character varying(80) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT ''
	`)
}

func downAddDigitalHumanVersionReleaseMetadata(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.digital_human_versions
			DROP COLUMN IF EXISTS description,
			DROP COLUMN IF EXISTS name
	`)
}
