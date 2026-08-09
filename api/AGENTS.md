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

## Application Errors

- Treat `pkg/apperror` as the transport-neutral error contract. Domain and service code may create or wrap an AppError; HTTP, SSE, OpenAI-compatible, persistence, and observability layers must project it at their boundary.
- Declare each stable code once at package scope with `apperror.MustCode("domain.subject.reason")`. Reuse cataloged codes; do not create numeric ranges, translate meaning from digits, or use a user-facing sentence as an identity.
- Use `apperror.Wrap` when a cause exists so `errors.Is` and `errors.As` keep working. Add a stable `WithOperation` value for diagnosis, and only scalar, non-sensitive `WithParams` values needed to render or classify the error.
- Never return `err.Error()` to a client. It is diagnostic text and may contain an upstream cause. Public messages and locale selection belong to the error catalog and protocol adapter.
- Adding a product error is incomplete until its catalog definition, safe parameter schema, supported locale messages, legacy mapping (when required), and focused tests are present. `pkg/apperror` validates the code grammar but is not itself the complete product code list.
- Preserve existing response contracts while migrating. Do not replace a legacy handler response until the relevant adapter has explicit HTTP/status/code/message compatibility tests.

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
