# AGENTS.md

## Repository Scope

This repository is an open-source monorepo for ZGI.

- `api/` contains the Go backend. Read `api/AGENTS.md` before changing backend code.
- `web/` contains the Next.js frontend. Read `web/AGENTS.md` before changing frontend code.
- `runner/` and `sandbox/` contain Go services used by the platform runtime.
- `docs/` contains public documentation.

## Open Source Hygiene

- Do not commit local editor settings, agent/tool state, generated runtime data, secrets, API keys, logs, caches, or uploaded files.
- Keep `api/storage/` as a runtime directory, but do not commit generated files under it.
- Keep generated documentation out of Git unless it is explicitly maintained as public documentation.
- Avoid references to private workflows, personal tools, or vendor-specific AI tools in project files.
- Prefer clear, durable project instructions over large narrative documents.

## Common Commands

From the repository root:

```bash
make setup
make dev-api
make dev-web
make dev-docker
make docker-down
```

## Change Guidelines

- Keep changes scoped to the requested area.
- Prefer existing project patterns over new abstractions.
- Do not remove tests or dependencies just to reduce repository size unless the feature they support is removed too.
- Update documentation when public behavior, setup, or commands change.
- Run targeted validation for the files you changed before handing off.

## Git Guidelines

- Work from `main` unless a maintainer asks for another branch.
- Do not rewrite unrelated user changes.
- Before committing, check `git status --short` and make sure only intended files are staged.
