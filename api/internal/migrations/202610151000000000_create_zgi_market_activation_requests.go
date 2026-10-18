package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateZGIMarketActivationRequestsID = "202610151000000000_create_zgi_market_activation_requests"

func init() {
	registerSchemaMigration(migrationCreateZGIMarketActivationRequestsID, upCreateZGIMarketActivationRequests, downCreateZGIMarketActivationRequests)
}

// upCreateZGIMarketActivationRequests keeps the requests of members who may not activate a ZGI
// Market item themselves: what they asked for and why, and how an administrator answered. It
// belongs to the zgimarket module only.
func upCreateZGIMarketActivationRequests(schema *mschema.Builder) error {
	return schema.Create("zgi_market_activation_requests", func(table *mschema.Blueprint) {
		table.UUID("id").Primary()
		table.UUID("organization_id").NotNull()
		table.UUID("requester_id").NotNull()
		table.BigInteger("listing_id").NotNull()
		table.String("listing_kind", 16).NotNull()
		table.String("listing_slug", 128).NotNull()
		table.String("listing_title", 200).Default("").NotNull()
		table.BigInteger("offering_id").Default(0).NotNull()
		table.String("offering_title", 200).Default("").NotNull()
		table.String("reason", 500).Default("").NotNull()
		table.String("status", 16).NotNull()
		table.UUID("reviewer_id").Nullable()
		table.String("review_note", 500).Default("").NotNull()
		table.TimestampTz("reviewed_at").Nullable()
		table.TimestampTz("created_at").NotNull()
		table.TimestampTz("updated_at").NotNull()
		table.Index("idx_zgi_market_activation_requests_organization", "organization_id", "status", "created_at")
		table.Index("idx_zgi_market_activation_requests_requester", "organization_id", "requester_id", "listing_id")
	})
}

func downCreateZGIMarketActivationRequests(schema *mschema.Builder) error {
	return schema.DropIfExists("zgi_market_activation_requests")
}
