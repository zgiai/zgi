# ZGI Seed Data

`internal/seeders` contains seed data executed after schema migrations.

Runtime entry points:

- `server seed`
- `go run ./cmd/migrate seed`

The seed runner is idempotent through the `seed_executions` marker table. Built-in workflows are embedded from `seeds/00_base/workflows` and refreshed even when the initial seed marker already exists.

Add production-safe bootstrap data here. Do not add local test data, customer data, secrets, or environment-specific credentials.
