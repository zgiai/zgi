package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

// An organization's administrators keep command rules that forbid or always
// ask for server commands its DigitalHumans run (ADR 0029).
func init() {
	registerSchemaMigration("202610281200000000_connector_command_rules", upConnectorCommandRules, downConnectorCommandRules)
}

func upConnectorCommandRules(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.connector_command_rules (
			id uuid PRIMARY KEY,
			organization_id uuid NOT NULL,
			prefix varchar(200) NOT NULL,
			action varchar(16) NOT NULL CHECK (action IN ('forbid', 'ask')),
			note varchar(200) NOT NULL DEFAULT '',
			created_by uuid NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			CONSTRAINT connector_command_rules_prefix_unique UNIQUE (organization_id, prefix)
		);
		CREATE INDEX IF NOT EXISTS idx_connector_command_rules_organization ON public.connector_command_rules (organization_id, created_at);
	`)
}

// The table stays: dropping it would silently lift every organization's rules.
func downConnectorCommandRules(*mschema.Builder) error {
	return nil
}
