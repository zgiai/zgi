# Docker Assets

This directory is the product-level docker entrypoint for ZGI.

## Current Compose Scope

The default `docker-compose.yaml` starts the lightweight core stack:

- PostgreSQL
- Redis
- API
- Web

Optional profiles add heavier services only when needed:

- `runtime` starts Sandbox and Runner.
- `knowledge` starts Weaviate and Neo4j.
- `full` starts both optional profiles through the helper script.

When started from `docker/`, `sandbox` reuses the shared root Postgres and Redis services:

- dedicated Postgres database: `${POSTGRES_SANDBOX_DB:-zgi_sandbox}`
- dedicated Redis logical DB: `${SANDBOX_REDIS_DB:-1}`

This is intentionally different from `sandbox/docker-compose.yml`, which remains self-contained for standalone development.

Run:

```bash
make docker-up
```

To start optional runtime services for code execution and plugin execution:

```bash
./dev/start-docker --runtime
```

To start optional knowledge services for vector and graph retrieval:

```bash
./dev/start-docker --knowledge
```

To start everything:

```bash
./dev/start-docker --full
```

If you are already inside `docker/` or do not have `make`, run from the repository root:

```bash
./dev/start-docker
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

PowerShell also supports `-Runtime`, `-Knowledge`, and `-Full` for optional service profiles.

Local default endpoints:

- Web and API gateway: `http://localhost:${PUBLIC_PORT:-2679}`

On first launch, open the gateway URL and create the first administrator account.
For local testing, you can use example credentials such as:

- Email: `admin@zgi.ai`
- Password: `Zgi@2679`

These credentials are examples only. ZGI does not ship with a default administrator account.
Use your own email and a strong password in production.

Internal Docker network ports:

- Web: `2680`
- API: `2670`
- Sandbox: `2660` when the `runtime` profile is enabled
- Runner: `2665` when the `runtime` profile is enabled
- PostgreSQL: `5432`
- Redis: `6379`
- Neo4j HTTP: `7474` when the `knowledge` profile is enabled
- Neo4j Bolt: `7687` when the `knowledge` profile is enabled

## Notes

- `api/`, `web/`, `sandbox/`, and `runner/` keep their own runtime env files inside each service directory.
- `dev/bootstrap` copies missing env templates into each service directory and regenerates `docker-compose.yaml`.
- `dev/bootstrap.ps1` and `dev/start-docker.ps1` provide the minimal native Windows path for Docker startup.
- `dev/check-env` compares local env files with their templates and reports missing keys, changed values, and extra local keys without modifying anything.
- `dev/check-env --sync` creates a timestamped backup next to each env file and appends only template keys that are currently missing.
- `docker/.env` is intentionally small and only carries compose-level orchestration values such as the public gateway port and shared infrastructure defaults.
- The default Docker stack intentionally uses `VECTOR_STORE=mock`, disables Neo4j, and disables plugin runner integration so first-time startup stays small.
- `./dev/start-docker --runtime` wires `CODE_EXECUTION_ENDPOINT`, `PLUGIN_RUNNER_ENABLED`, and `PLUGIN_RUNNER_URL` for the current run.
- `./dev/start-docker --knowledge` wires `VECTOR_STORE`, `WEAVIATE_ENDPOINT`, and `NEO4J_URI` for the current run.
- `sandbox` is wired differently in product mode versus standalone mode:
  product mode reuses the shared root Postgres / Redis, while standalone mode keeps its own bundled Postgres / Redis.
- `./dev/start-docker --china` currently wires China mainland build mirrors for `api`, `sandbox`, and `runner` through compose build args.
- `PUBLIC_PORT` defaults to `2679`; app and infrastructure services stay on the internal Docker network.
