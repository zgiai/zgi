package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// A DigitalHuman per-user Connector slot (ADR 0007) runs with a Connection
// of its requester. The selection records which Connection a requester, an
// end user or a workspace member, supplies for one slot of one
// DigitalHuman; slots keep their key across published versions.
func init() {
	registerSchemaMigration("202610231800000000_digital_human_connector_selections", upDigitalHumanConnectorSelections, downDigitalHumanConnectorSelections)
}

func upDigitalHumanConnectorSelections(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.digital_human_connector_selections (
			id uuid PRIMARY KEY,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE CASCADE,
			owner_kind varchar(16) NOT NULL,
			owner_id uuid NOT NULL,
			slot_key varchar(64) NOT NULL,
			connector_id varchar(128) NOT NULL,
			connection_id uuid NOT NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT ck_dh_connector_selections_owner_kind CHECK (owner_kind IN ('account', 'end_user')),
			CONSTRAINT uq_dh_connector_selections_slot UNIQUE (digital_human_id, owner_id, slot_key)
		);
		CREATE INDEX IF NOT EXISTS idx_dh_connector_selections_owner ON public.digital_human_connector_selections (owner_id);
		CREATE INDEX IF NOT EXISTS idx_dh_connector_selections_connection ON public.digital_human_connector_selections (connection_id);
	`)
}

func downDigitalHumanConnectorSelections(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.digital_human_connector_selections;`)
}
