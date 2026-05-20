# AGENTS.md

## Scope

These instructions apply to the Go backend under `api/`.

## Backend Commands

Run commands from `api/` unless noted otherwise:

```bash
make deps
make fmt
make test
make build
make run
```

Useful direct commands:

```bash
go test ./...
go test ./internal/...
go run cmd/server/main.go start
go run cmd/migrate/main.go up
```

## Architecture

- Keep HTTP routing in `routes/` and module-specific route registration near the module it serves.
- Keep business logic in `internal/modules/*`.
- Keep shared infrastructure in `internal/infra`, `internal/bootstrap`, `internal/config`, and `internal/container`.
- Keep reusable capabilities in `internal/capabilities/*` when they are domain-independent services.
- Keep DTOs, contracts, validators, and errors in their existing shared packages.
- Prefer dependency injection and existing container/Fx patterns instead of package-level globals.

## Go Style

- Run `go fmt` on changed Go files.
- Return wrapped errors with enough context for operators to diagnose failures.
- Keep handlers thin: parse input, call services, map responses.
- Keep repositories focused on persistence. Do not put workflow or billing decisions in repository code.
- Prefer typed request/response structs over unstructured maps.
- Use context-aware APIs for request-scoped work.

## Database and Storage

- PostgreSQL is the production database target.
- SQLite usage in tests is intentional for isolated unit and migration coverage. Do not remove SQLite test dependencies unless replacing the test strategy.
- Add migrations for schema changes and include focused tests for migration behavior when practical.
- Do not commit generated runtime files from `storage/`; keep only placeholders needed to preserve directories.

## LLM and Provider Code

- Do not hardcode provider-specific fallbacks unless the existing module already requires them.
- Preserve model selection, billing, quota, and routing behavior when touching LLM gateway code.
- Keep provider protocol code isolated under the existing `llm` module boundaries.

## Tests

- Prefer narrow package tests for the changed module before running the full suite.
- Use existing test fixtures and in-memory databases where available.
- Do not delete tests only because they are large; first verify whether they protect active functionality.
