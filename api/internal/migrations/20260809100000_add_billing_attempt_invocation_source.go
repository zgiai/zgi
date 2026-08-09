package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddBillingAttemptInvocationSourceID = "20260809100000_add_billing_attempt_invocation_source"

func init() {
	registerSchemaMigration(
		migrationAddBillingAttemptInvocationSourceID,
		upAddBillingAttemptInvocationSource,
		downAddBillingAttemptInvocationSource,
	)
}

func upAddBillingAttemptInvocationSource(schema *mschema.Builder) error {
	return schema.WhenTableDoesntHaveColumn("billing_attempts", "invocation_source", func() error {
		return schema.Table("billing_attempts", func(table *mschema.Blueprint) {
			table.String("invocation_source", 20).DefaultSQL("'unknown'").NotNull()
		})
	})
}

func downAddBillingAttemptInvocationSource(schema *mschema.Builder) error {
	return schema.WhenTableHasColumn("billing_attempts", "invocation_source", func() error {
		return schema.Table("billing_attempts", func(table *mschema.Blueprint) {
			table.DropColumn("invocation_source")
		})
	})
}
