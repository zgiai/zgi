# Workflows

This directory contains the public CI workflows for the monorepo.

Current checks:

- Repository hygiene checks from `./scripts/check-open-source.sh --worktree`
- Web TypeScript checks

Release workflows:

- `vercel-web-deploy.yml` deploys the `web/` Next.js app to Vercel from `deploy-dev`.
- `sync-deploy-dev.yml` updates `deploy-dev` only when `dev` changes web deployment paths, or when manually dispatched.
- Required repository secrets: `VERCEL_TOKEN`, `VERCEL_ORG_ID`, and `VERCEL_PROJECT_ID`.
- `docker-release.yml` publishes `zgiai/zgi-api`, `zgiai/zgi-web`, `zgiai/zgi-sandbox`, and `zgiai/zgi-runner` to Docker Hub.
- Required repository secrets: `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`.
- Pushing a `v*` tag publishes `version`, `latest`, and `sha-xxxxxxx` tags.
- Manual dispatch can publish `sha-xxxxxxx` only, or an explicit version with optional `latest`.

Keep CI focused on checks that are stable for external contributors. Add heavier integration tests behind explicit jobs or service-specific workflows.
