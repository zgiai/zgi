# Workflows

This directory contains the public CI workflows for the monorepo.

Current checks:

- Repository hygiene checks from `make check-open-source`
- API targeted Go tests for migrations, SQL metadata, workflow, model gateway, and content parsing packages
- PostgreSQL migration smoke test against a fresh database
- Runner targeted Go tests
- Web TypeScript checks

Release workflows:

- `docker-release.yml` publishes `zgiai/zgi-api`, `zgiai/zgi-web`, `zgiai/zgi-sandbox`, and `zgiai/zgi-runner` to Docker Hub.
- Required repository secrets: `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`.
- Pushing a `v*` tag publishes `version`, `latest`, and `sha-xxxxxxx` tags.
- Manual dispatch can publish `sha-xxxxxxx` only, or an explicit version with optional `latest`.

Keep CI focused on checks that are stable for external contributors. Add heavier integration tests behind explicit jobs or service-specific workflows.
