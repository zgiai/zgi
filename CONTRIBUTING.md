# Contributing

Thanks for your interest in contributing to ZGI.

ZGI is organized as a monorepo. The main services live in:

- `api/` - Go backend service
- `web/` - Next.js web application
- `sandbox/` - isolated execution service
- `runner/` - plugin execution service
- `docker/` and `dev/` - local development and product-level orchestration

## Before You Start

Please review:

- `README.md` for setup and project layout
- `SECURITY.md` for security reporting
- `CODE_OF_CONDUCT.md` for community expectations
- `AGENTS.md` for repository-level coding guidance

For larger changes, open an issue first so maintainers and contributors can align on scope.

Project issue tracker: https://github.com/zgiai/zgi/issues

## Ways To Contribute

- Report reproducible bugs.
- Propose improvements to workflows, APIs, or documentation.
- Fix issues in the backend, frontend, sandbox, runner, or local development stack.
- Improve tests, examples, and setup documentation.

## Reporting Bugs

Please include:

- what happened
- what you expected to happen
- steps to reproduce
- logs or screenshots when useful
- operating system, Docker version, browser, and relevant service versions

Do not include secrets, API keys, tokens, private URLs, or customer data in public issues.

## Submitting Pull Requests

1. Fork the repository.
2. Create a branch from `main`.
3. Make the smallest reasonable change that solves the problem.
4. Add or update tests when behavior changes.
5. Install the repository hooks with `make install-hooks`.
6. Run the relevant local checks.
7. Open a pull request with a clear description and link any related issue.

Pull requests are opened at https://github.com/zgiai/zgi/pulls.

Small documentation fixes do not need a prior issue. Larger behavior changes usually should start with one.

## Local Checks

For backend changes:

```bash
cd api
make fmt
make test
make build
```

For frontend changes:

```bash
cd web
pnpm lint
pnpm type-check
pnpm build
```

For full-stack startup changes:

```bash
make dev-docker
make docker-logs
make docker-down
```

Run the narrowest useful checks first, then broaden validation when the change affects shared behavior.

## Commit Messages

Use English Conventional Commits:

```text
feat(api): add workspace quota endpoint
fix(web): handle empty workflow list
docs: update Docker quick start
```

Do not use non-English commit messages in public history. The repository hook rejects non-ASCII commit messages to keep project history searchable and consistent for global contributors.

## Open Source Hygiene

Install hooks before committing:

```bash
make install-hooks
```

Run the same checks manually:

```bash
make check-open-source
```

The checks reject unapproved binary files, large files, local absolute paths, private tooling references, and high-confidence secret patterns. If a binary fixture is truly required, add a short justification by listing it in `.github/allowed-binaries.txt`.

## Code Guidelines

- Follow existing patterns in the area you change.
- Keep unrelated refactors out of focused fixes.
- Do not commit local `.env` files, generated runtime files, uploaded files, logs, caches, or editor state.
- Update documentation when commands, setup, APIs, or public behavior change.
- Keep public examples free of real secrets or production credentials.

## Need Help?

Open an issue with a short description of what you are trying to do and where you are stuck.
