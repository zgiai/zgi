package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migrationAddAppliedCommandIdempotencyKeyID = "20260925130000_add_applied_command_idempotency_key"

func init() {
	registerSchemaMigration(
		migrationAddAppliedCommandIdempotencyKeyID,
		upAddAppliedCommandIdempotencyKey,
		downAddAppliedCommandIdempotencyKey,
	)
}

// upAddAppliedCommandIdempotencyKey adds the console envelope idempotency key to the
// applied-command ledger.
//
// The column is nullable so commands delivered without a key keep working exactly as
// before. It mirrors the existing command_id primary key: the ledger holds one row per
// command, and this deployment serves one console instance, so the key is unique per
// instance (i.e. within this table). All three supported dialects allow repeated NULLs in
// a unique index, so keyless commands never block each other.
func upAddAppliedCommandIdempotencyKey(schema *mschema.Builder) error {
	return schema.Table("pre_applied_commands", func(table *mschema.Blueprint) {
		table.String("idempotency_key", 128).Nullable()
		table.Unique("idx_pre_applied_commands_idempotency_key", "idempotency_key")
	})
}

func downAddAppliedCommandIdempotencyKey(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_pre_applied_commands_idempotency_key;
		ALTER TABLE public.pre_applied_commands DROP COLUMN IF EXISTS idempotency_key;
	`)
}
