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

- Web and API gateway: `http://localhost:${PUBLIC_PORT:-2679}`

Internal Docker network ports:

- Web: `2680`
- API: `2670`
- Sandbox: `2660`
- Runner: `2665`
- PostgreSQL: `5432`
- Redis: `6379`
- Neo4j HTTP: `7474`
- Neo4j Bolt: `7687`

## Notes

- `api/`, `web/`, `sandbox/`, and `runner/` keep their own runtime env files inside each service directory.
- `dev/bootstrap` copies missing env templates into each service directory and regenerates `docker-compose.yaml`.
- `dev/bootstrap.ps1` and `dev/start-docker.ps1` provide the minimal native Windows path for Docker startup.
- `dev/check-env` compares local env files with their templates and reports missing keys, changed values, and extra local keys without modifying anything.
- `dev/check-env --sync` creates a timestamped backup next to each env file and appends only template keys that are currently missing.
- `docker/.env` is intentionally small and only carries compose-level orchestration values such as the public gateway port and shared infrastructure defaults.
- `sandbox` is wired differently in product mode versus standalone mode:
  product mode reuses the shared root Postgres / Redis, while standalone mode keeps its own bundled Postgres / Redis.
- `./dev/start-docker --china` currently wires China mainland build mirrors for `api`, `sandbox`, and `runner` through compose build args.
- `PUBLIC_PORT` defaults to `2679`; app and infrastructure services stay on the internal Docker network.
