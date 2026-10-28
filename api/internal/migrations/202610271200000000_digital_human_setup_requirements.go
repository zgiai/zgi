package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

// A DigitalHuman created from a built-in template keeps the template's
// required skills and connections, and publishes only once its draft meets
// them (ADR 0023). Existing DigitalHumans require nothing.
func init() {
	registerSchemaMigration("202610271200000000_digital_human_setup_requirements", upDigitalHumanSetupRequirements, downDigitalHumanSetupRequirements)
}

func upDigitalHumanSetupRequirements(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.digital_humans ADD COLUMN IF NOT EXISTS setup_requirements jsonb NOT NULL DEFAULT '{}'::jsonb;
	`)
}

// The column stays: dropping it would let drafts publish without what their
// template requires.
func downDigitalHumanSetupRequirements(*mschema.Builder) error {
	return nil
}
