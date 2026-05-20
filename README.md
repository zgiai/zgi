# ZGI

ZGI is a source-available platform for building and operating AI applications.

It includes a Go backend, a Next.js web console, a sandbox service for code execution, and a runner service for extensible plugin execution. The repository is organized as a monorepo so contributors can run and inspect the full stack from one place.

Repository: https://github.com/zgiai/zgi

## What Is Included

- Multi-provider LLM gateway with routing, model management, billing, and quota support.
- Workflow and agent application runtime.
- Dataset, knowledge, file, and content parsing capabilities.
- Web console for workspace, model, workflow, dataset, and application management.
- Sandbox service for isolated execution workloads.
- Plugin runner for installing and invoking external tools.
- Docker-based local development stack.

## Repository Layout

```text
.
├── api/             Go backend service
├── web/             Next.js web application
├── sandbox/         Isolated execution service
├── runner/          Plugin execution service
├── docker/          Product-level Docker Compose assets
├── dev/             Local development scripts
├── scripts/         Maintenance scripts
├── Makefile         Common local entry points
└── README.md
```

## Requirements

- Docker and Docker Compose
- Make
- Go, for source development of backend services
- Node.js and pnpm, for source development of the web app

The web app uses `pnpm@10.12.1`.

## Quick Start

Start the full local stack with Docker:

```bash
make docker-up
```

The startup script copies missing environment files from examples, prepares Docker Compose configuration, and starts the product stack.

If you do not have `make`, run the startup script directly:

```bash
./dev/start-docker
```

Default local endpoints:

- Web and API gateway: `http://localhost:2679`

On first launch, open `http://localhost:2679` and create the first administrator account.
For local testing, you can use example credentials such as:

- Email: `admin@zgi.ai`
- Password: `Zgi@2679`

These credentials are examples only. ZGI does not ship with a default administrator account.
Use your own email and a strong password in production.

The application and infrastructure services use internal container ports by default:

- Web: `2680`
- API: `2670`
- Sandbox: `2660`
- Runner: `2665`
- PostgreSQL: `5432`
- Redis: `6379`
- Neo4j HTTP: `7474`
- Neo4j Bolt: `7687`

Stop the stack:

```bash
make docker-down
```

View logs:

```bash
make docker-logs
```

## Source Development

Install and prepare local dependencies:

```bash
make setup
```

Start shared infrastructure:

```bash
make dev-docker
```

Run backend and frontend from source in separate terminals:

```bash
make dev-api
make dev-web
```

Service-specific commands are available inside each service directory:

```bash
cd api
make test
make build
make run
```

```bash
cd web
pnpm lint
pnpm type-check
pnpm build
```

## Environment Files

Environment templates are checked in as `.env.example` files. Local `.env` files are not committed.

Check local environment drift:

```bash
make env-check
```

Append newly added template keys while keeping existing local values:

```bash
make env-sync
```

## Windows

The recommended Windows path is Docker Desktop plus PowerShell:

```powershell
.\dev\start-docker.ps1
```

CMD is also supported:

```bat
dev\start-docker.cmd
```

Source-development helpers are designed for Unix-like shells. Windows contributors can use WSL for source development.

## Documentation

- Product-level Docker notes: `docker/README.md`
- Web app notes: `web/README.md`
- Backend service docs: `api/`

## Project Links

- Repository: https://github.com/zgiai/zgi
- Issues: https://github.com/zgiai/zgi/issues
- Pull requests: https://github.com/zgiai/zgi/pulls
- Security: https://github.com/zgiai/zgi/security

## Contributing

Contributions are welcome. Please read `CONTRIBUTING.md` before opening a pull request.

Community expectations are documented in `CODE_OF_CONDUCT.md`.

For security-sensitive reports, follow `SECURITY.md`.

## License

ZGI is source-available under the ZGI Community License, based on Apache
License 2.0 with additional conditions. ZGI is free for personal, research,
educational, and internal organizational use. Hosted multi-tenant services,
white-label distribution, and removal of official ZGI branding require a
commercial license. See `LICENSE` for details.

The Apache License 2.0 text referenced by the ZGI Community License is
included in `LICENSE-APACHE`.
