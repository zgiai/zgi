# ZGI

ZGI is a source-available AI application platform for building, running, and
operating agents, visual workflows, skills, prompts, knowledge, and model routes
from one workspace.

[![License](https://img.shields.io/badge/license-ZGI%20Community%20License-blue)](LICENSE)
[![Docker](https://img.shields.io/badge/run-Docker%20Compose-2496ED)](#quick-start)
[![Frontend](https://img.shields.io/badge/frontend-Next.js-black)](web)
[![Backend](https://img.shields.io/badge/backend-Go-00ADD8)](api)

Repository: https://github.com/zgiai/zgi

![ZGI workflow editor](docs/assets/zgi-workflow-editor-api-enrichment.png)

## At A Glance

| Area | What is included |
| --- | --- |
| Application builder | Agents, chatflows, workflows, prompt library, web app publishing |
| Workflow nodes | HTTP, JSON parsing, LLM, branching, loops, approval, tools, code, database, notification, knowledge retrieval |
| Model operations | Provider catalog, model channels, model policy, model defaults, pricing metadata |
| Runtime services | Go API, Next.js console, sandbox service, plugin runner |
| Built-in templates | 23 agent/workflow templates in English and Simplified Chinese |
| Local startup | One Docker Compose command, default gateway at `http://localhost:2679` |

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

For a lighter product preview, start only nginx, API, web, PostgreSQL, and
Redis:

```bash
./dev/start-docker --core
```

Other startup profiles:

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

## Why ZGI

ZGI is designed for teams that want more than a chat UI. It gives you a control
plane for model access, a visual runtime for multi-step AI applications, and a
skill system for connecting agents to real tools.

- Build visual workflows with typed inputs, HTTP calls, JSON parsing, LLM steps,
  branching, loops, approvals, and outputs.
- Configure agents with instructions, model settings, knowledge, file upload,
  memory, and selected skills.
- Route traffic across model providers and channels while keeping credentials,
  model defaults, and model policy centrally managed.
- Debug agent and workflow runs with visible runtime events, skill calls, tool
  results, and final output.
- Manage prompts, skills, datasets, files, API keys, and content parsing from
  the same workspace console.

## Core Capabilities

| Capability | Description |
| --- | --- |
| Agent runtime | Configure instructions, model settings, knowledge, skills, files, memory, and web app experience. |
| Skill calls | Select reusable skills and inspect loading, invocation, results, and final responses during debug. |
| Visual workflows | Compose multi-step applications with graph nodes for APIs, parsing, models, tools, control flow, and outputs. |
| Model gateway | Manage providers, model channels, usable models, defaults, pricing metadata, and organization policy. |
| Prompt operations | Maintain reusable prompts, versions, labels, and prompt optimization workflows. |
| Runtime isolation | Use the sandbox and runner services for code execution and plugin/tool execution. |

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

## Requirements

- Docker and Docker Compose
- Make
- Go, for backend source development
- Node.js and pnpm, for frontend source development

The web app uses `pnpm@10.12.1`.

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

Service-specific commands:

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

## Screenshots

The main workflow editor screenshot is shown above. Additional product screens
are intentionally kept out of the README so the page stays fast to scan.

## Environment Files

Environment templates are checked in as `.env.example` files. Local `.env`
files are not committed.

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

Source-development helpers are designed for Unix-like shells. Windows
contributors can use WSL for source development.

## Documentation

- Product-level Docker notes: `docker/README.md`
- Release process: `docs/release-process.md`
- Web app notes: `web/README.md`
- Backend service docs: `api/`

## Project Links

- Repository: https://github.com/zgiai/zgi
- Issues: https://github.com/zgiai/zgi/issues
- Pull requests: https://github.com/zgiai/zgi/pulls
- Security: https://github.com/zgiai/zgi/security

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
