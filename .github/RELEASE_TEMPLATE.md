# ZGI vX.Y.Z

## Highlights

- TBD

## Upgrade Notes

- TBD

## Breaking Changes

- None.

## Docker Images

- `zgiai/zgi-api:X.Y.Z`
- `zgiai/zgi-web:X.Y.Z`
- `zgiai/zgi-sandbox:X.Y.Z`
- `zgiai/zgi-runner:X.Y.Z`

Stable releases also publish:

- `zgiai/zgi-api:X.Y`
- `zgiai/zgi-web:X.Y`
- `zgiai/zgi-sandbox:X.Y`
- `zgiai/zgi-runner:X.Y`
- `latest`
- `sha-<short-sha>`

Pre-releases publish the exact pre-release version and `sha-<short-sha>`, but
do not update `latest`.

## Migration Checklist

- [ ] `go run ./cmd/migrate check` passed.
- [ ] `go run ./cmd/migrate check -db "$ZGI_MIGRATION_CHECK_DSN"` passed on a fresh PostgreSQL database.
- [ ] New migrations are append-only `YYYYMMDDHHMMSS_slug.go` files.
- [ ] Existing released migrations were not edited, deleted, or reordered.
- [ ] No destructive migration operation is present without explicit maintainer review.
- [ ] Rollback behavior is documented when rollback is supported.

## Validation

- [ ] `make check-open-source`
- [ ] API targeted tests
- [ ] Runner targeted tests
- [ ] Web type check
- [ ] Docker image build

## Contributors

- TBD
