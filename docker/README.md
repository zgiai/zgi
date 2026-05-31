# Docker Assets

This directory is the product-level docker entrypoint for ZGI.

## Current Compose Scope

The default `docker-compose.yaml` starts the full local stack so first-time users can use the main product capabilities immediately:

- PostgreSQL
- Redis
- Weaviate
- Neo4j
- Sandbox
- Runner
- API
- Web

The helper also supports a lightweight core mode for contributors who only need the API and web shell:

- `./dev/start-docker --core` starts only nginx, API, web, PostgreSQL, and Redis.
- `./dev/start-docker --runtime` starts the core stack plus Sandbox and Runner.
- `./dev/start-docker --knowledge` starts the core stack plus Weaviate and Neo4j.
- `./dev/start-docker --full` starts the same full stack as the default.

When started from `docker/`, `sandbox` reuses the shared root Postgres and Redis services:

- dedicated Postgres database: `${POSTGRES_SANDBOX_DB:-zgi_sandbox}`
- dedicated Redis logical DB: `${SANDBOX_REDIS_DB:-1}`

This is intentionally different from `sandbox/docker-compose.yml`, which remains self-contained for standalone development.

Run:

```bash
make docker-up
```

To start only the lightweight core stack:

```bash
./dev/start-docker --core
```

To start only the core stack plus runtime services for code execution and plugin execution:

```bash
./dev/start-docker --runtime
```

To start only the core stack plus knowledge services for vector and graph retrieval:

```bash
./dev/start-docker --knowledge
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

PowerShell also supports `-Core`, `-Runtime`, `-Knowledge`, and `-Full` for service profiles.

Local default endpoints:

- Web and API gateway: `http://localhost:${PUBLIC_PORT:-2679}`
- API direct port: `http://localhost:${API_PORT:-2670}`
- Web direct port: `http://localhost:${WEB_PORT:-2680}`

On first launch, open the gateway URL and create the first administrator account.
ZGI does not ship with a default administrator account. Use your own email and a strong password.

## AIChat / Agent Knowledge Smoke Checks

When smoke testing local AIChat or Agent knowledge retrieval changes, rebuild the
application images first because the default Docker stack runs images rather
than bind-mounted source:

```bash
cd docker
docker compose --env-file .env up -d --build api web
docker compose --env-file .env ps api web
```

Wait until `zgi-api-1` and `zgi-web-1` are healthy before browser testing.

Use a dedicated smoke Agent that is already bound to a small knowledge base,
for example a "story outline" knowledge base. Do not temporarily modify a
general-purpose demo Agent during smoke tests. The smoke pass should cover:

- AIChat knowledge-base listing with fallback candidates.
- AIChat retrieve success and no-results cases.
- Agent retrieve success from the bound knowledge base.
- Original-wording questions such as "what is the one-sentence synopsis",
  verifying that the answer quotes or closely excerpts the source and cites it.

Host ports can be changed in `docker/.env`:

- Gateway: `${PUBLIC_PORT:-2679}`
- API: `${API_PORT:-2670}`
- Web: `${WEB_PORT:-2680}`
- PostgreSQL: `${POSTGRES_PORT:-5434}`
- Redis: `${REDIS_HOST_PORT:-6381}`
- Weaviate: `${WEAVIATE_PORT:-18080}`
- Neo4j HTTP: `${NEO4J_HTTP_PORT:-7474}`
- Neo4j Bolt: `${NEO4J_BOLT_PORT:-7687}`

Application images can also be changed in `docker/.env` with `API_IMAGE_NAME`,
`WEB_IMAGE_NAME`, `SANDBOX_IMAGE_NAME`, `RUNNER_IMAGE_NAME`, and `IMAGE_TAG`.
This lets sibling worktrees reuse the same images while keeping compose project
names, containers, volumes, and ports isolated.

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
- The default Docker stack intentionally starts Weaviate, Neo4j, Sandbox, and Runner so knowledge base, code execution, and plugin features work during first-time evaluation.
- `./dev/start-docker --core` wires `VECTOR_STORE=mock`, disables Neo4j, clears the code execution endpoint, and disables plugin runner integration for the current run.
- `./dev/start-docker --runtime` wires `CODE_EXECUTION_ENDPOINT`, `PLUGIN_RUNNER_ENABLED`, and `PLUGIN_RUNNER_URL` for the current run.
- `./dev/start-docker --knowledge` wires `VECTOR_STORE`, `WEAVIATE_ENDPOINT`, and `NEO4J_URI` for the current run.
- `sandbox` is wired differently in product mode versus standalone mode:
  product mode reuses the shared root Postgres / Redis, while standalone mode keeps its own bundled Postgres / Redis.
- `./dev/start-docker --china` currently wires China mainland build mirrors for `api`, `sandbox`, and `runner` through compose build args.
- `PUBLIC_PORT` defaults to `2679`; app and infrastructure service host ports are configurable in `docker/.env`.
