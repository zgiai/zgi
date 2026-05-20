# Release Process

This document defines the release rules for ZGI.

## Version Tags

Use SemVer tags with a leading `v`:

- Stable release: `v1.2.3`
- Pre-release: `v1.2.3-rc.1`, `v1.2.3-beta.1`, `v1.2.3-alpha.1`

Do not use ad hoc release tags such as `release-1.2`, `v1`, `latest`, or date
tags.

## Docker Tags

Docker Hub namespace: `zgiai`.

Images:

- `zgiai/zgi-api`
- `zgiai/zgi-web`
- `zgiai/zgi-sandbox`
- `zgiai/zgi-runner`

Every release publishes:

- `sha-<short-sha>`
- The exact version without the leading `v`, for example `1.2.3`

Stable releases also publish:

- Minor tag, for example `1.2`
- `latest`

Pre-releases do not update `latest` or the minor tag.

Manual Docker releases may publish `latest` only when the release owner
explicitly enables it in the workflow input.

## Release Notes

Use `.github/RELEASE_TEMPLATE.md` for GitHub release notes.

Release notes must include:

- Highlights
- Upgrade notes
- Breaking changes, or `None`
- Docker image tags
- Migration checklist
- Validation checklist
- Contributors

After publishing, summarize the release in `CHANGELOG.md`.

## Migration Release Checklist

Before cutting a tag that includes database changes:

1. Run `go run ./cmd/migrate check`.
2. Run `go run ./cmd/migrate check -db "$ZGI_MIGRATION_CHECK_DSN"` against a fresh PostgreSQL database.
3. Confirm every new migration is a new append-only `YYYYMMDDHHMMSS_slug.go` file.
4. Confirm no released migration was edited, deleted, or reordered.
5. Confirm destructive operations were not added to normal `up` migrations.
6. Confirm data backfills, if any, are separated from table-shape changes.
7. Document required operator action in the release notes.

## Standard Release Flow

1. Ensure `main` is green in CI.
2. Update `CHANGELOG.md` from `Unreleased` into the target version section.
3. Prepare GitHub release notes from `.github/RELEASE_TEMPLATE.md`.
4. Create and push a SemVer tag:

   ```bash
   git tag v1.2.3
   git push origin v1.2.3
   ```

5. Wait for the Docker Release workflow to publish all images.
6. Publish the GitHub release.
7. Verify Docker Hub tags and the quick-start Docker path.
