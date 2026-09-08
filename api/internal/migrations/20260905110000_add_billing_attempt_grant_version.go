package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddBillingAttemptGrantVersionID = "20260905110000_add_billing_attempt_grant_version"

const addBillingAttemptGrantVersionSQL = `
	ALTER TABLE public.billing_attempts
		ADD COLUMN IF NOT EXISTS grant_authorization_version bigint;
`

const rollbackBillingAttemptGrantVersionSQL = `
	ALTER TABLE public.billing_attempts
		DROP COLUMN IF EXISTS grant_authorization_version;
`

func init() {
	registerSchemaMigration(
		migrationAddBillingAttemptGrantVersionID,
		upAddBillingAttemptGrantVersion,
		downAddBillingAttemptGrantVersion,
	)
}

func upAddBillingAttemptGrantVersion(schema *mschema.Builder) error {
	return schema.Raw(addBillingAttemptGrantVersionSQL)
}

func downAddBillingAttemptGrantVersion(schema *mschema.Builder) error {
	return schema.Raw(rollbackBillingAttemptGrantVersionSQL)
}
