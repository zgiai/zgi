package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAllowDigitalHumanRuntimeSourceID = "202609091710000000_allow_digital_human_runtime_source"

const allowDigitalHumanRuntimeSourceSQL = `
	ALTER TABLE public.published_runtime_surfaces
		DROP CONSTRAINT IF EXISTS published_runtime_surfaces_source_check;
	ALTER TABLE public.published_runtime_surfaces
		ADD CONSTRAINT published_runtime_surfaces_source_check CHECK (
			compatibility_source IN (
				'legacy_agent_fields',
				'grant',
				'system_default',
				'digital_human_access'
			)
		);
`

func init() {
	registerSchemaMigration(
		migrationAllowDigitalHumanRuntimeSourceID,
		upAllowDigitalHumanRuntimeSource,
		downAllowDigitalHumanRuntimeSource,
	)
}

func upAllowDigitalHumanRuntimeSource(schema *mschema.Builder) error {
	return schema.Raw(allowDigitalHumanRuntimeSourceSQL)
}

func downAllowDigitalHumanRuntimeSource(_ *mschema.Builder) error {
	// Keep the additive source on rollback. Existing DigitalHuman surface rows
	// would make narrowing the constraint unsafe.
	return nil
}
