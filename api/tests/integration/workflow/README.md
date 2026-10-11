# Workflow integration tests

Use Go 1.26.2 with CGO enabled and a dedicated PostgreSQL database. Do not point
these tests at an application database. Enable `uuid-ossp` in the test database
before running the tests:

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
```

Set `TEST_POSTGRES_DSN` using your environment or CI secret store, then run from
`api/`:

```sh
go test -count=1 ./tests/integration/workflow
go test -count=1 ./internal/modules/app/workflow/... ./internal/testutil/testdb
```

Both URL and PostgreSQL key/value DSNs are supported. Workflow integration tests
create a unique schema per test, prepare the required models, and remove that
schema on completion. They temporarily bind the workflow services to that scoped
database and restore the previous binding during cleanup. They require the DSN
explicitly; the application database configuration is not used as a fallback.

For the full API suite, the SQL metadata tests additionally use
`SQLMETA_TEST_DB_HOST`, `SQLMETA_TEST_DB_PORT`, `SQLMETA_TEST_DB_USER`,
`SQLMETA_TEST_DB_PASS`, and `SQLMETA_TEST_DB_NAME`. Set those to the same dedicated
test database. Optional provider tests may require their own explicitly configured
test credentials; keep all credentials out of Git.

```sh
go test -p=2 -count=1 -timeout=2m ./...
go vet -p=2 ./...
```

SQLite unit fixtures use the test-only `internal/testutil/testdb` migration helper
to translate PostgreSQL UUID and timestamp defaults. This does not change runtime
migrations, production model tags, or database constraints.
