# Docker Assets

This directory is the product-level docker entrypoint for ZGI.

## Current Compose Scope

The current `docker-compose.yaml` starts the full local stack:

- PostgreSQL
- Redis
- Weaviate
- Neo4j
- Sandbox
- Runner
- API
- Web

When started from `docker/`, `sandbox` reuses the shared root Postgres and Redis services:

- dedicated Postgres database: `${POSTGRES_SANDBOX_DB:-zgi_sandbox}`
- dedicated Redis logical DB: `${SANDBOX_REDIS_DB:-1}`

This is intentionally different from `sandbox/docker-compose.yml`, which remains self-contained for standalone development.

Run:

```bash
../dev/bootstrap
docker compose --env-file .env up -d --build
```

For Windows users who want the quickest Docker-only path, run from the repository root:

```powershell
powershell -ExecutionPolicy Bypass -File .\dev\start-docker.ps1
```

Or from `cmd.exe`:

```bat
dev\start-docker.cmd
```

Those wrappers copy missing env templates, regenerate `docker-compose.yaml`, and then start the stack with `docker compose`.

If Docker builds run from China mainland networks, prefer:

```bash
../dev/start-docker --china
```

PowerShell equivalent:

```powershell
powershell -ExecutionPolicy Bypass -File .\dev\start-docker.ps1 -China
```

That applies recommended build mirrors for the current run while keeping service runtime env files in their own service directories.

Local default endpoints:

- Web: `http://localhost:13000`
- API: `http://localhost:2678`
- PostgreSQL: `localhost:${HOST_POSTGRES_PORT:-15432}`
- Redis: `localhost:${HOST_REDIS_PORT:-16379}`
- Weaviate: `http://localhost:${HOST_WEAVIATE_PORT:-18081}`
- Neo4j HTTP: `http://localhost:${HOST_NEO4J_HTTP_PORT:-17474}`
- Neo4j Bolt: `localhost:${HOST_NEO4J_BOLT_PORT:-17687}`
- Sandbox: `http://localhost:${HOST_SANDBOX_PORT:-18194}`
- Runner: `http://localhost:${HOST_PLUGIN_RUNNER_PORT:-15000}`

## Notes

- `api/`, `web/`, `sandbox/`, and `runner/` keep their own runtime env files inside each service directory.
- `dev/bootstrap` copies missing env templates into each service directory and regenerates `docker-compose.yaml`.
- `dev/bootstrap.ps1` and `dev/start-docker.ps1` provide the minimal native Windows path for Docker startup.
- `dev/check-env` compares local env files with their templates and reports missing keys, changed values, and extra local keys without modifying anything.
- `dev/check-env --sync` creates a timestamped backup next to each env file and appends only template keys that are currently missing.
- `docker/.env` is intentionally small and only carries compose-level orchestration values such as host ports and shared infrastructure defaults.
- `sandbox` is wired differently in product mode versus standalone mode:
  product mode reuses the shared root Postgres / Redis, while standalone mode keeps its own bundled Postgres / Redis.
- `./dev/start-docker --china` currently wires China mainland build mirrors for `api`, `sandbox`, and `runner` through compose build args.
- `WEB_PORT` defaults to `13000` to avoid common collisions on `3000`.
