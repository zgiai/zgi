# Contributing

Thanks for your interest in contributing to ZGI.

This repository is the top-level product shell for the open-source ZGI stack. It coordinates:

- product-level Docker and local startup workflows
- release integration across submodules
- root repository metadata, templates, and automation
- top-level documentation such as the project README

The application code itself lives in the submodules:

- `api/`
- `web/`
- `sandbox/`
- `plugin-runner/`

## Before You Start

Please take a moment to review the project documentation before opening a pull request:

- [README.md](./README.md)
- [SECURITY.md](./SECURITY.md)

If you are planning a larger change, open an issue first so we can align on scope and approach before implementation work begins.

## Ways To Contribute

You can help by contributing in a few different ways:

- report bugs
- propose features or workflow improvements
- improve documentation
- fix issues in the root product shell
- contribute application changes in the relevant submodule repository

## Reporting Bugs

Please include:

- a clear title
- what happened
- what you expected to happen
- steps to reproduce
- screenshots or logs when available
- environment details if the issue is related to Docker, startup, or local development

For security-sensitive reports, please follow [SECURITY.md](./SECURITY.md) instead of opening a public issue.

## Suggesting Features

Feature requests are most helpful when they include:

- the problem you are trying to solve
- the proposed behavior
- why the change matters
- any relevant examples, screenshots, or references

## Submitting a Pull Request

Please follow this flow:

1. Fork the repository.
2. Create a branch for your change.
3. Make the smallest reasonable change that solves the problem.
4. Add or update tests when the change affects behavior.
5. Make sure the relevant local checks pass.
6. Open a pull request and link the related issue when applicable.

Small documentation fixes and low-risk maintenance changes do not always need a prior issue, but larger behavior or workflow changes usually should start with one.

## Local Setup

Clone the repository with submodules:

```bash
git clone --recurse-submodules <repo-url>
```

If you already cloned the repository:

```bash
git submodule update --init --recursive
```

Useful local entry points:

- Full product stack:

```bash
make dev-docker
```

- China mainland build mirrors:

```bash
./dev/start-docker --china
```

- Mixed source development:

```bash
make setup
make dev-docker
make dev-api
make dev-web
```

For service-specific setup and development details, please refer to each submodule repository.

## Working With Submodules

If your change touches code inside `api/`, `web/`, `sandbox/`, or `plugin-runner/`, there are usually two parts to the contribution:

1. Commit and push the change in the submodule repository.
2. Commit the updated submodule pointer in this root repository.

Example:

```bash
cd api
git checkout -b feature/example
git commit -am "Update backend behavior"
git push

cd ..
git add api
git commit -m "Bump api submodule"
git push
```

If you only change root-level Docker, scripts, workflows, or documentation, you only need a pull request in this repository.

## What Belongs In This Repository

Good fits for this repository include:

- `dev/` scripts
- `docker/` assets
- GitHub workflows and templates
- top-level docs and README files
- release coordination across submodules

Application business logic should stay in the relevant submodule unless the change is specifically about top-level integration.

## Need Help?

If you are unsure where a change belongs, open an issue first. That is often the fastest way for us to help you land a contribution cleanly.
