package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateConnectorSocialOAuthID = "202610191000000000_create_connector_social_oauth"

func init() {
	registerSchemaMigration(migrationCreateConnectorSocialOAuthID, upCreateConnectorSocialOAuth, downCreateConnectorSocialOAuth)
}
func upCreateConnectorSocialOAuth(s *mschema.Builder) error {
	if e := s.Create("connector_social_oauth_flows", func(t *mschema.Blueprint) {
		t.UUID("id").Primary()
		t.UUID("organization_id").NotNull()
		t.UUID("owner_account_id").NotNull()
		t.String("connector_id", 128).NotNull()
		t.String("state_hash", 64).NotNull()
		t.String("status", 32).NotNull()
		t.Text("envelope").NotNull()
		t.UUID("connection_id").NotNull()
		t.String("error_code", 64).NotNull()
		t.TimestampTz("expires_at").NotNull()
		t.TimestampTz("created_at").NotNull()
		t.Unique("idx_social_oauth_state", "state_hash")
		t.Index("idx_social_oauth_flow_owner", "organization_id", "owner_account_id", "id")
		t.Index("idx_social_oauth_flow_expiry", "expires_at")
	}); e != nil {
		return e
	}
	return s.Create("connector_social_oauth_grants", func(t *mschema.Blueprint) {
		t.UUID("id").Primary()
		t.UUID("organization_id").NotNull()
		t.UUID("connection_id").NotNull()
		t.String("connector_id", 128).NotNull()
		t.Integer("credential_version").NotNull()
		t.String("credential_digest", 64).NotNull()
		t.Text("envelope").NotNull()
		t.TimestampTz("updated_at").NotNull()
		t.Unique("idx_social_oauth_grant_connection", "connection_id")
	})
}
func downCreateConnectorSocialOAuth(s *mschema.Builder) error {
	if e := s.DropIfExists("connector_social_oauth_grants"); e != nil {
		return e
	}
	return s.DropIfExists("connector_social_oauth_flows")
}
