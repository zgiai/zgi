package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationDigitalHumanConnectorResourcesID = "20260904130000_allow_digital_human_connector_resources"

// Historical integration rows remain readable in immutable versions. New
// resource writes are governed by the DigitalHuman host, not this type check.
const allowDigitalHumanConnectorResourcesSQL = `
ALTER TABLE public.digital_human_resource_bindings
 DROP CONSTRAINT dh_resource_bindings_type_check;
ALTER TABLE public.digital_human_resource_bindings
 ADD CONSTRAINT dh_resource_bindings_type_check CHECK (binding_type IN (
  'skill', 'knowledge_dataset', 'database', 'database_table', 'workflow',
  'integration_connection', 'connector'
 ));
`

func init() {
	registerSchemaMigration(migrationDigitalHumanConnectorResourcesID, upDigitalHumanConnectorResources, nil)
}

func upDigitalHumanConnectorResources(schema *mschema.Builder) error {
	return schema.Raw(allowDigitalHumanConnectorResourcesSQL)
}
