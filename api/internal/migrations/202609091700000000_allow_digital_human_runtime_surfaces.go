package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAllowDigitalHumanRuntimeSurfacesID = "202609091700000000_allow_digital_human_runtime_surfaces"

const allowDigitalHumanRuntimeSurfacesSQL = `
	ALTER TABLE public.published_runtime_surfaces
		DROP CONSTRAINT IF EXISTS published_runtime_surfaces_resource_type_check;
	ALTER TABLE public.published_runtime_surfaces
		ADD CONSTRAINT published_runtime_surfaces_resource_type_check CHECK (
			resource_type IN ('agent', 'builtin_workflow', 'digital_human')
		);
`

func init() {
	registerSchemaMigration(
		migrationAllowDigitalHumanRuntimeSurfacesID,
		upAllowDigitalHumanRuntimeSurfaces,
		downAllowDigitalHumanRuntimeSurfaces,
	)
}

func upAllowDigitalHumanRuntimeSurfaces(schema *mschema.Builder) error {
	return schema.Raw(allowDigitalHumanRuntimeSurfacesSQL)
}

func downAllowDigitalHumanRuntimeSurfaces(_ *mschema.Builder) error {
	// Keep the additive resource type on rollback. Existing DigitalHuman
	// surface rows would make narrowing the constraint unsafe.
	return nil
}
