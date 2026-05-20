# ZGI Web

This directory contains the ZGI web console. It is part of the ZGI monorepo and is normally run through the root Docker stack behind the gateway on `http://localhost:2679`.

## Stack

- Next.js 16 App Router
- React 19
- TypeScript
- Tailwind CSS
- Radix UI and shadcn/ui
- TanStack Query
- Zustand
- next-intl
- XYFlow workflow editor

## Run With The Full Stack

From the repository root:

```bash
make docker-up
```

The gateway serves the web UI and API from one origin:

- Web: `http://localhost:2679`
- API gateway paths: `/console/api/`, `/v1/`, `/files/`

## Web-Only Development

Use web-only development when the API stack is already running.

```bash
pnpm install
pnpm dev
```

Open `http://localhost:3000`.

Configure API endpoints in `.env.local`. Start from the example file when needed:

```bash
cp .env.example .env.local
```

## Checks

```bash
pnpm lint
pnpm type-check
pnpm build
```

## Contributing

Use the repository-level guide at [../CONTRIBUTING.md](../CONTRIBUTING.md). Web changes follow the same commit, review, and open-source hygiene rules as the rest of the monorepo.

## License

This component is distributed under the repository license unless a file states otherwise. See [../LICENSE](../LICENSE).
