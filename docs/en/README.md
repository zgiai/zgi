# ZGI

ZGI is the top-level product repository for the open-source ZGI stack.

At this stage, the repository is organized as a product shell:

- `api/` is the backend codebase and is tracked as a git submodule.
- `web/` is the frontend codebase and is tracked as a git submodule.
- `sandbox/` is the code execution sandbox and is tracked as a git submodule.
- `plugin-runner/` is the plugin execution service and is tracked as a git submodule.
- `docker/` contains shared local middleware and future self-hosting assets.
- `dev/` contains local development helper scripts.
- `docs/` stores localized versions of the repository README.

## Repository Strategy

This repository intentionally does not rewrite the history of the existing backend and frontend repositories.

Instead:

- `api/` points to `git@github.com:zgiai/zgi-api.git`
- `web/` points to `git@github.com:zgiai/zgi-web.git`
- `sandbox/` points to `git@github.com:zgiai/zgi-sandbox.git`
- `plugin-runner/` points to `git@github.com:zgiai/zgi-plugin-runner.git`

This keeps the original repositories independent while giving the open-source product a single top-level home.

## Directory Layout

```text
.
├── api/                  backend submodule
├── web/                  frontend submodule
├── sandbox/              sandbox submodule
├── plugin-runner/        plugin runner submodule
├── docker/               shared middleware and deployment assets
├── dev/                  local development entrypoints
├── docs/                 localized README files
├── scripts/              release and maintenance helpers
├── .github/              templates and CI planning
├── CONTRIBUTING.md
├── Makefile
└── README.md
```

## Quick Start

### macOS / Linux

#### Full Docker Stack

This is the recommended local boot path when you want the whole product stack.

```bash
make dev-docker
```

`make dev-docker` bootstraps the repo automatically on first run: it initializes submodules, copies missing env templates into each submodule, regenerates the root compose file, and then starts Docker.

If you are building from China mainland networks and Docker image builds are slow or flaky, use:

```bash
./dev/start-docker --china
```

If you want to review how your local env files differ from the checked-in templates without overwriting anything, run:

```bash
make env-check
```

If a template adds new keys later and you want to append only the missing ones while keeping your existing values untouched, run:

```bash
make env-sync
```

#### Source Development (macOS / Linux only)

Use this mode if you want to run `api/` and `web/` from source but keep shared middleware in Docker.

1. Initialize submodules and install local dependencies.

```bash
make setup
```

If templates change later, `make env-check` will show missing keys, changed values, and extra local keys across the root docker env plus every submodule env file. `make env-sync` will first create a timestamped backup beside each target file and then append only the missing keys.

2. Start the docker stack or only shared middleware, depending on your workflow.

```bash
make dev-docker
```

3. Start backend and frontend in separate terminals.

```bash
make dev-api
make dev-web
```

### Windows

The minimal supported path is Docker Desktop plus PowerShell. Source-development helpers such as `dev/check-env`, `dev/start-api`, and `dev/start-web` assume a Unix-like shell and are not available on Windows.

```powershell
# PowerShell
.\dev\start-docker.ps1

# CMD
.\dev\start-docker.cmd

# PowerShell (China mirror)
.\dev\start-docker.ps1 -china

# CMD (China mirror)
.\dev\start-docker.cmd -china
```

Default local endpoints:

- Web: `http://localhost:13000`
- API: `http://localhost:2678`
- PostgreSQL: `localhost:${HOST_POSTGRES_PORT:-15432}`
- Redis: `localhost:${HOST_REDIS_PORT:-16379}`
- Weaviate: `http://localhost:${HOST_WEAVIATE_PORT:-18080}`
- Neo4j HTTP: `http://localhost:${HOST_NEO4J_HTTP_PORT:-17474}`
- Neo4j Bolt: `bolt://localhost:${HOST_NEO4J_BOLT_PORT:-17687}`
- Sandbox: `http://localhost:${HOST_SANDBOX_PORT:-18194}`
- Plugin Runner: `http://localhost:${HOST_PLUGIN_RUNNER_PORT:-15000}`

The default web host port is `13000` instead of `3000` to reduce collisions with existing local frontend projects. If you need a different host port, update `docker/.env`.
`docker/.env.example` also includes optional build mirror variables if you prefer to keep custom defaults locally.

In the full `zgi-pre` stack, `sandbox` reuses the shared root Postgres and Redis instead of starting its own copies:

- Postgres instance: shared `postgres` service
- Sandbox database: `${POSTGRES_SANDBOX_DB:-zgi_sandbox}`
- Redis instance: shared `redis` service
- Sandbox Redis logical DB: `${SANDBOX_REDIS_DB:-1}`

That means:

- running `sandbox` alone still uses its own `docker-compose.yml` and its own bundled Postgres / Redis
- running `make dev-docker` uses one shared Postgres container and one shared Redis container for the whole product stack

## Current Scope

The repository currently focuses on product-level organization only.

- `api/`, `web/`, `sandbox/`, and `plugin-runner/` are kept as-is.
- `zgi-console-api` is not part of this repository yet.
- Full-stack release and license consolidation will be handled later.
- The root `docker/` directory now provides a unified local stack for `api`, `web`, and core dependencies.

## README Translations

- `README.md` is the external source English version
- `docs/en/README.md` is the in-docs English mirror
- `docs/zh-CN/README.md` is the Simplified Chinese version
- `docs/ja-JP/README.md` is the Japanese version

Whenever `README.md` is updated, every existing translated README under `docs/<locale>/README.md` should be updated in the same change window.
