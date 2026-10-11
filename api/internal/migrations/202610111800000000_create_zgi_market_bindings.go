package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateZGIMarketBindingsID = "202610111800000000_create_zgi_market_bindings"

func init() {
	registerSchemaMigration(migrationCreateZGIMarketBindingsID, upCreateZGIMarketBindings, downCreateZGIMarketBindings)
}

// upCreateZGIMarketBindings keeps the ZGI Market binding of each agent and digital human: the
// market MCP listings it may use, and the credential the market issued for exactly those, sealed
// with the connector credential keyring. It belongs to the zgimarket module only.
func upCreateZGIMarketBindings(schema *mschema.Builder) error {
	return schema.Create("zgi_market_bindings", func(table *mschema.Blueprint) {
		table.UUID("id").Primary()
		table.UUID("organization_id").NotNull()
		table.String("subject_type", 32).NotNull()
		table.UUID("subject_id").NotNull()
		table.BigInteger("market_binding_id").NotNull()
		table.Integer("market_version").NotNull()
		table.Text("secret_ciphertext").NotNull()
		table.JSONB("items").DefaultSQL("'[]'::jsonb").NotNull()
		table.UUID("created_by").NotNull()
		table.UUID("updated_by").NotNull()
		table.TimestampsTz()
		table.Unique("uk_zgi_market_bindings_subject", "organization_id", "subject_type", "subject_id")
	})
}

func downCreateZGIMarketBindings(schema *mschema.Builder) error {
	return schema.DropIfExists("zgi_market_bindings")
}
