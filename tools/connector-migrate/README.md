# Offline Connector conversion

This independent Go module converts retained database records to native Connector
identities and `c1` secrets. It imports neither the API module nor old providers,
performs no provider requests, and never modifies original Skill files.

## Maintenance procedure

Fresh installations use normal API migrations and reach schema2 directly. A
current schema1 database already marked `ready` skips the historical converter:
advance to `20260903120000_messaging_connector_protocol` if needed, then follow
step 7. The independent converter accepts only the older checkpoint below unless
the operator explicitly opts in to an advanced checkpoint (see below).

1. Stop API writers, workers and scheduled jobs. Drain running work and confirmations.
2. Take a consistent database backup and a backup of Skill storage and the old/new keyrings.
3. Prepare the independent native keyring through protected configuration, including
   `CONNECTOR_CREDENTIAL_KEYS_JSON` and `CONNECTOR_CREDENTIAL_ACTIVE_KEY_ID` for the API
   migration command. From `api/`, run:

   ```text
   go run ./cmd/migrate connector-checkpoint --id 20260902190000_detach_connector_runtime
   ```
   Existing data sets readiness to `conversion_required`; the new runtime will refuse it.
4. Set `CONNECTOR_MIGRATE_DSN` through protected deployment configuration. Use a private
   JSON keyring file with `active`, `native` and `previous` fields. Each keyring maps
   key IDs to exactly 32-byte secret strings. Include every historical key ID; `active`
   must identify a key in `native`. Do not put secrets or the DSN in command arguments.
5. Run the following from this directory, substituting private file paths:

   ```text
   go run . inventory --skills-root <skill-storage-root>
   go run . dry-run --keys-file <private-keyrings.json> --skills-root <skill-storage-root>
   go run . apply --keys-file <private-keyrings.json> --skills-root <skill-storage-root> --expected-report <reviewed-source-digest>
   go run . verify --keys-file <private-keyrings.json> --skills-root <skill-storage-root>
   ```

   `--skills-root` is required when stored Skills have file paths. It bounds file access,
   including resolved symlinks. Inventory/dry-run/verify use read-only transactions.
6. Review `issues`, per-source `results`, unavailable reference counts and the source digest.
   Resolve every issue before apply. Apply checks that the reviewed source is unchanged,
   locks the conversion tables, performs all database writes in one serializable
   transaction, verifies the result, validates the continuation ownership FK, and marks
   readiness `ready` only before commit. A failure rolls back all database changes and
   never updates readiness. In advanced mode apply locks `public.migrations` before it
   locks the conversion tables (see the advanced section).
7. After the historical converter verifies schema1, use the API-owned finalizer from
   `api/` while all writers remain stopped. Keep all paths outside the repository:

   ```text
   go run ./cmd/migrate connector-checkpoint --id 20260903120000_messaging_connector_protocol
   go run ./cmd/migrate connector-finalize plan --report <plan.json> --archive <archive.enc> --archive-key-file <archive.key>
   go run ./cmd/migrate connector-finalize apply --expected-report <digest> --archive <archive.enc> --archive-key-file <archive.key> --report <apply.json>
   go run ./cmd/migrate connector-finalize verify --report <verify.json>
   ```

   The plan digest is the top-level `digest` in the reviewed report. Confirm the API
   uses the same native keyring, start only the schema2 runtime, and verify it before
   restoring traffic. The database backup, encrypted archive and its separately
   protected key are the recovery materials; the retired database tables are removed.

The JSON report contains identifiers, counts, digests and bounded result codes. It
does not contain Keys, decrypted payloads, SQL driver messages or Skill text. Treat
reports as operational data and keep them outside source control.

## Advanced checkpoint (explicit opt-in)

A schema1 database that advanced past `20260902190000_detach_connector_runtime`
but has not applied `202609031410353961_finalize_connector_detachment` sits
outside the default checkpoint. The converter refuses it unless the operator
explicitly names the exact applied migration head:

```text
go run . inventory --skills-root <skill-storage-root> --advanced-checkpoint <applied-migration-id>
go run . dry-run --keys-file <private-keyrings.json> --skills-root <skill-storage-root> --advanced-checkpoint <applied-migration-id>
go run . apply --keys-file <private-keyrings.json> --skills-root <skill-storage-root> --advanced-checkpoint <applied-migration-id> --expected-report <reviewed-source-digest>
go run . verify --keys-file <private-keyrings.json> --skills-root <skill-storage-root> --advanced-checkpoint <applied-migration-id>
```

The flag is an explicit opt-in, never a silent skip. It is rejected for an empty
value, for an identifier at or before the historical checkpoint, and unless the
named identifier is exactly the applied migration head. The converter also
requires `20260902190000_detach_connector_runtime` to be applied and
`202609031410353961_finalize_connector_detachment` to be absent. In addition, it
requires all 22 retired Connector tables, the `connector_target_bindings`
`compatibility`/`compatibility_state` columns and their check constraints, the
`agent_resource_bindings_type_check` check constraint, and the
`connector_confirmation_continuations_native_claim_fk` foreign key to still
exist, proving the API-owned retirement finalizer has not run. The native claim
FK only has to exist: it may still be `NOT VALID`, because converter apply
validates it after conversion.

Every transaction reads the complete ordered migration ledger alongside the
conversion source. The dry-run source digest therefore includes the migration
history, and apply refuses a reviewed digest whose ledger changed, even when the
applied head still matches. Advanced apply also locks `public.migrations` in
`SHARE ROW EXCLUSIVE` mode as the first statement of its transaction, before any
read and before the conversion advisory lock. That blocks a concurrent migration
marker insert while the ledger is read, so the reviewed ledger cannot change
under apply. `inventory`, `dry-run` and `verify` stay read-only and take no lock,
and the default historical apply keeps its existing lock order.

Review an advanced dry-run as carefully as a normal one. The opt-in covers a
specific, manually reviewed ahead-of-checkpoint database, not an open-ended
migration skip, and it does not bypass the retirement finalizer or justify
skipping migrations. In that database, 7 stored Skill files are missing or
unreadable; the converter may mark those Skills `invalid`, so confirm those
`issues`, the per-source `results` and the unavailable reference counts in the
dry-run before apply.

## Conversion behavior

- Existing native UUIDs, credential versions, binding restrictions and tombstones remain.
  Supported old personal GitHub/Exa Key connections retain their connection UUIDs.
- Connection and Binding restrictions preserve operation intersection, owner, target
  scope, expiry and denials. An unsupported restriction stays denied. Copying a
  snapshot never expands a restriction to permit its new target.
- Agent draft and published snapshots receive canonical binding references. Workflow
  graphs, runs and pauses receive exact version/node references. A historical run whose
  graph differs from its current draft receives a retained immutable workflow snapshot.
- Pending native confirmations retain accepted operation contracts, input, receipt and
  logical-operation identity only when the complete source and destination identities
  can be proven. Unsupported pending work is retired. Submitted work without a known
  outcome remains `unknown`; successful, failed and unknown operations never replay.
- Organization/shared connections, OAuth/QR, unsupported providers and unprovable
  restrictions become non-executable references requiring explicit replacement.
- User-authored Skills containing old integration requirements or executable metatool
  instructions are marked invalid and reported for re-import with `connector_requirements`,
  `connector_id` and `operation_ids`. Rewriting prose cannot prove an equivalent operation
  contract, so the converter retains those source files. Ordinary/native Skills remain
  unchanged. Re-import uses the ordinary Skill validation flow.

Repeated apply on an already verified database writes no conversion records. A partial
ledger, changed archive, identity conflict, missing key or incomplete file inventory
blocks conversion; no guessed mapping or permission is substituted.

## Recovery and validation

Keep traffic stopped after any apply/commit error. Run `verify` to establish whether a
commit succeeded before retrying. Restore the complete maintenance-window backup
(database, storage and matching keyrings) or deploy a forward fix. There is no online
reversal or second provider execution path. Do not mix old and new runtime writers.

`go test ./...` covers independent synthetic secret vectors, restrictions, tombstones,
source digest stability, host/paused snapshots, terminal barriers, file boundaries and
sanitized database errors. Command tests also cover read-only transaction modes, stale
reports, immediate rollback on write/verification failures and ambiguous commit errors.
Checkpoint tests cover the strict default head, the explicit advanced opt-in, a missing
historical checkpoint, an already-applied finalizer marker, a mismatched head, a stale
digest caused by a changed migration ledger, the advanced migration-ledger lock ordering
and its absence for read-only runs, and a missing retained-schema fingerprint (including
`agent_resource_bindings_type_check` and the unvalidated native claim FK).
These use transaction fixtures and do not prove PostgreSQL durability.
API tests named `TestConnectorFinalizationPostgres` and
`TestConnectorHistoricalUpgradePostgres` use the isolated
`ZGI_CONNECTOR_FINALIZE_TEST_DSN`. Without it, PostgreSQL behavior and a full historical
upgrade remain release gates; in-memory and SQL mock tests are not a substitute.
