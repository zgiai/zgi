# ZGI Database Migrations

`internal/migrations` is the public database migration chain used by the API server and migration CLI.

Runtime entry points:

- `server migrate`
- `server migrate --pretend`
- `server migrate:status`
- `server migrate:rollback --confirm <latest_migration_id>`
- `go run ./cmd/migrate up`
- `go run ./cmd/migrate up -pretend`
- `go run ./cmd/migrate status`
- `go run ./cmd/migrate check`
- `go run ./cmd/migrate check -db "host=localhost user=postgres password=postgres dbname=zgi_migration_check port=5432 sslmode=disable"`
- `go run ./cmd/migrate make create_example_table`
- `go run ./cmd/migrate rollback -confirm <latest_migration_id>`

The Docker image runs `server migrate` during startup when migration execution is enabled.

## Design

ZGI's public migration history starts from one initial schema migration:

- `20260520000000_initial_schema.go` applies the full current PostgreSQL schema.
- `baseline/*.go` is split by domain and execution layer:
  - `00_extensions.go`
  - `10_identity_access.go`
  - `20_apps_workflows.go`
  - `30_model_gateway.go`
  - `40_data_knowledge.go`
  - `50_billing_commerce.go`
  - `60_runtime_system.go`
  - `70_compat_views.go`
  - `80_constraints.go`
  - `90_indexes.go`
  - `95_foreign_keys.go`
- The baseline statements are schema-only and contain no runtime data.
- `internal/seeders` owns bootstrap data.
- Future schema changes must be append-only migrations with timestamp IDs.

Closed-source pre-public migration history is intentionally not included in the open repository.
The initial schema migration refuses to run on a non-empty public schema, so existing deployments are never silently overwritten or deleted.

The migration schema builder lives under `internal/migrations/schema` on purpose. It is a contributor-facing migration DSL, not a public Go package for runtime application code.

## Rules

1. Add new migrations only under `internal/migrations`.
2. Use `go run ./cmd/migrate make <slug>` for new migrations. New generated IDs use `YYYYMMDDHHMMSSRRRR_slug`, where `RRRR` is a four-digit random suffix. Existing `YYYYMMDDHHMMSS_slug` IDs remain valid.
3. Register each future migration with `registerSchemaMigration` unless raw `gormigrate` behavior is required.
4. Do not edit, delete, or reorder migrations after release.
5. Use PostgreSQL-compatible SQL. SQLite-backed migration tests are not allowed.
6. Keep schema changes in migrations and startup data in `internal/seeders`.
7. Large backfills should be separate from table-shape changes.
8. Never add destructive baseline statements such as `DROP`, `TRUNCATE`, or data rewrites.
9. Prefer one focused Go migration per feature after the public baseline. Use explicit PostgreSQL DDL through `tx.Exec` when GORM cannot represent the operation exactly.
10. Destructive schema operations require an explicit `AllowDestructive` builder. Normal `up` migrations should not use it.

## Safety

ZGI follows Laravel's production-safety ideas but adapts them to PostgreSQL:

- Migration runs acquire a PostgreSQL advisory lock by default, preventing two API instances from migrating at the same time.
- Disabling the migration lock requires `ZGI_UNSAFE_NO_MIGRATION_LOCK=1`.
- `migrate --pretend` / `up -pretend` prints migration status without applying changes.
- `migrate:status` / `status` shows which migrations are already applied.
- `go run ./cmd/migrate check` validates migration IDs, filenames, duplicate IDs, source safety, baseline safety, and, when `-db` or `ZGI_MIGRATION_CHECK_DSN` is provided, executes the full chain against a fresh PostgreSQL database.
- `internal/migrations/schema` blocks destructive statements such as `DROP TABLE`, `DROP COLUMN`, `TRUNCATE`, `DELETE`, and `UPDATE` unless `AllowDestructive` is explicitly enabled.
- Rollbacks created through `registerSchemaMigration` run with `AllowDestructive`, because rollback is the only normal path where dropping a newly created table or column is expected.
- Rollback commands require the exact latest migration ID via `--confirm`, so AI agents and operators cannot accidentally roll back with a short command.
- The initial public baseline refuses to run on a non-empty public schema, so existing deployments are not silently overwritten.
- `make` generates a migration whose `up` and `down` both fail with `not implemented` until the author fills in the migration body.

## Writing Migrations

Future migrations should follow the Laravel-style structure: one timestamped file, one `up` function, one `down` function, and a schema blueprint for common table work. The migration generator creates the ID and failing `up` / `down` stubs; do not hand-write migration IDs.

```go
package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202606010900000827ID = "202606010900000827_create_audit_events"

func init() {
	registerSchemaMigration(migration202606010900000827ID, upCreateAuditEvents, downCreateAuditEvents)
}

func upCreateAuditEvents(schema *mschema.Builder) error {
	return schema.Create("audit_events", func(table *mschema.Blueprint) {
		table.ID()
		table.UUID("organization_id").NotNull()
		table.UUID("account_id").Nullable()
		table.String("event_type", 64).NotNull()
		table.JSONB("payload").DefaultSQL("'{}'::jsonb").NotNull()
		table.TimestampsTz()

		table.Index("idx_audit_events_org_created", "organization_id", "created_at")
		table.Foreign("fk_audit_events_account", []string{"account_id"}, "accounts", []string{"id"}).NullOnDelete()
	})
}

func downCreateAuditEvents(schema *mschema.Builder) error {
	return schema.DropIfExists("audit_events")
}
```

Use `schema.Table` for additive changes:

```go
func upAddAuditRequestID(schema *mschema.Builder) error {
	return schema.Table("audit_events", func(table *mschema.Blueprint) {
		table.String("request_id", 128).Nullable()
		table.Index("idx_audit_events_request", "request_id")
	})
}
```

Use `schema.Raw` only for PostgreSQL features that the blueprint does not cover, such as partial indexes, generated columns, views, or advanced constraints.

For compatibility migrations, check the existing database shape before altering it:

```go
func upAddAuditRequestID(schema *mschema.Builder) error {
	return schema.WhenTableDoesntHaveColumn("audit_events", "request_id", func() error {
		return schema.Table("audit_events", func(table *mschema.Blueprint) {
			table.String("request_id", 128).Nullable()
			table.Index("idx_audit_events_request", "request_id")
		})
	})
}
```
