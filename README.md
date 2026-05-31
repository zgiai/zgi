# ZGI

ZGI is a source-available AI application platform for building, running, and
operating agents, visual workflows, skills, prompts, knowledge, and model routes
from one workspace.

[![License](https://img.shields.io/badge/license-ZGI%20Community%20License-blue)](LICENSE)
[![Docker](https://img.shields.io/badge/run-Docker%20Compose-2496ED)](#quick-start)
[![Frontend](https://img.shields.io/badge/frontend-Next.js-black)](web)
[![Backend](https://img.shields.io/badge/backend-Go-00ADD8)](api)

![ZGI workflow editor](docs/assets/zgi-workflow-editor-api-enrichment.png)

## Core Features

ZGI brings the pieces of an AI application platform into one self-hostable
workspace: visual app building, model routing, knowledge retrieval, reusable
skills, and runtime services.

- **Agent applications**: configure instructions, model settings, knowledge,
  memory, file upload, skills, and web app publishing from the console.
- **Visual workflows**: compose API calls, JSON parsing, LLM steps, branching,
  loops, approvals, tools, code execution, database access, notifications, and
  knowledge retrieval on a canvas.
- **Model operations**: manage providers, channels, credentials, defaults,
  policy controls, and pricing metadata without scattering model configuration
  across applications.
- **Knowledge and skills**: connect datasets, content parsing, retrieval, and
  reusable tool skills so agents can act on real workspace context.
- **Local-first deployment**: run the Go API, Next.js console, sandbox, plugin
  runner, PostgreSQL, and Redis behind a local gateway at
  `http://localhost:2679`.
- **Starter templates**: explore 23 built-in agent and workflow templates in
  English and Simplified Chinese.

## Quick Start

Start the full local stack:

```bash
make dev-docker
```

If you do not have `make`, run the startup script directly:

```bash
./dev/start-docker
```

Open:

```text
http://localhost:2679
```

On first launch, create the first administrator account. ZGI does not ship with
a default admin account.

For a lighter product preview, start only the core services:

```bash
./dev/start-docker --core
```

Other profiles:

```bash
./dev/start-docker --runtime    # Core stack plus Sandbox and Runner
./dev/start-docker --knowledge  # Core stack plus knowledge dependencies
./dev/start-docker --full       # Full stack, same as the default
```

Stop the stack:

```bash
make docker-down
```

View logs:

```bash
make docker-logs
```

## Architecture

```text
Browser
  |
  v
Nginx gateway (:2679)
  |
  +-- Next.js console (web)
  +-- Go API service
        |
        +-- PostgreSQL / Redis
        +-- Sandbox service
        +-- Plugin runner
        +-- Optional knowledge services: Weaviate / Neo4j
```

## Repository Layout

```text
.
├── api/             Go backend service
├── web/             Next.js web console
├── sandbox/         Isolated execution service
├── runner/          Plugin execution service
├── docker/          Product-level Docker Compose assets
├── dev/             Local development scripts
├── scripts/         Maintenance scripts
├── docs/            Public documentation and assets
├── Makefile         Common local entry points
└── README.md
```

## Development

- Docker and Docker Compose
- Make
- Go, for backend source development
- Node.js and pnpm, for frontend source development

The web app uses `pnpm@10.12.1`.

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

## Documentation

- Product-level Docker notes: `docker/README.md`
- Release process: `docs/release-process.md`
- Web app notes: `web/README.md`
- Backend service docs: `api/`

## Contributing

Contributions are welcome. Please read `CONTRIBUTING.md` before opening a pull
request.

Community expectations are documented in `CODE_OF_CONDUCT.md`.

For security-sensitive reports, follow `SECURITY.md`.

## License

ZGI is source-available under the ZGI Community License, based on Apache License
2.0 with additional conditions. ZGI is free for personal, research, educational,
and internal organizational use. Hosted multi-tenant services, white-label
distribution, and removal of official ZGI branding require a commercial license.
See `LICENSE` for details.

The Apache License 2.0 text referenced by the ZGI Community License is included
in `LICENSE-APACHE`.
