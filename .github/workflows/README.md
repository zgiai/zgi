# Workflows

This directory contains the public CI workflows for the monorepo.

Current checks:

- Repository hygiene checks from `make check-open-source`
- API targeted Go tests for migrations, SQL metadata, workflow, model gateway, and content parsing packages
- PostgreSQL migration smoke test against a fresh database
- Runner targeted Go tests
- Web lint and TypeScript checks

Keep CI focused on checks that are stable for external contributors. Add heavier integration tests behind explicit jobs or service-specific workflows.
