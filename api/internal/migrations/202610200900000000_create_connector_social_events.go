package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const createConnectorSocialEventsSQL = `
CREATE TABLE public.connector_social_events (
 id bigserial PRIMARY KEY,
 event_id uuid NOT NULL UNIQUE,
 organization_id uuid NOT NULL,
 owner_account_id uuid NOT NULL,
 connection_id uuid NOT NULL REFERENCES public.connector_connections(id),
 external_account_id varchar(255) NOT NULL,
 source_id varchar(64) NOT NULL CHECK (source_id ~ '^[0-9a-f]{64}$'),
 encrypted_payload text NOT NULL CHECK (length(encrypted_payload) > 0),
 received_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 acknowledged_at timestamptz,
 CONSTRAINT connector_social_events_source_unique UNIQUE (connection_id, source_id)
);
CREATE INDEX idx_connector_social_events_unread
 ON public.connector_social_events (organization_id, owner_account_id, connection_id, external_account_id, id)
 WHERE acknowledged_at IS NULL;
`

func init() {
	registerSchemaMigration("202610200900000000_create_connector_social_events", up202610200900000000, down202610200900000000)
}
func up202610200900000000(schema *mschema.Builder) error {
	return schema.Raw(createConnectorSocialEventsSQL)
}
func down202610200900000000(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.connector_social_events;`)
}
