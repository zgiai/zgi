# Changelog

All notable ZGI changes are documented in this file.

This project follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html) for public
release tags.

## [Unreleased]

### Added

- Docker release workflow for API, web, sandbox, and runner images.
- Public migration check command for validating migration IDs, filenames,
  dangerous statements, and fresh PostgreSQL execution.

### Changed

- Docker Compose starts the full local experience by default. A core-only mode
  is available for lighter development.
- Public database migrations now use a Go migration baseline plus append-only
  timestamped migration files.

### Security

- Open source hygiene checks block local agent state, secrets, generated
  runtime data, and unexpected binary files from release candidates.

## Release Notes

Release notes are written from `.github/RELEASE_TEMPLATE.md` and summarized in
this changelog after each release.
