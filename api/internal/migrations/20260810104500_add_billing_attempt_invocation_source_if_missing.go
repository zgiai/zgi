package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationEnsureBillingAttemptInvocationSourceID = "20260810104500_add_billing_attempt_invocation_source_if_missing"

func init() {
	registerSchemaMigration(
		migrationEnsureBillingAttemptInvocationSourceID,
		upEnsureBillingAttemptInvocationSource,
		downEnsureBillingAttemptInvocationSource,
	)
}

func upEnsureBillingAttemptInvocationSource(schema *mschema.Builder) error {
	return schema.WhenTableDoesntHaveColumn("billing_attempts", "invocation_source", func() error {
		return schema.Table("billing_attempts", func(table *mschema.Blueprint) {
			table.String("invocation_source", 20).DefaultSQL("'unknown'").NotNull()
		})
	})
}

func downEnsureBillingAttemptInvocationSource(schema *mschema.Builder) error {
	return schema.WhenTableHasColumn("billing_attempts", "invocation_source", func() error {
		return schema.Table("billing_attempts", func(table *mschema.Blueprint) {
			table.DropColumn("invocation_source")
		})
	})
}
