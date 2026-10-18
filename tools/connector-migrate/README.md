# Offline Connector conversion

This independent Go module converts retained database records to native Connector
identities and `c1` secrets. It imports neither the API module nor old providers,
performs no provider requests, and never modifies original Skill files.

## Maintenance procedure

Fresh installations use normal API migrations and reach schema2 directly. A
current schema1 database already marked `ready` skips the historical converter:
advance to `20260903120000_messaging_connector_protocol` if needed, then follow
step 7. The independent converter accepts the detach checkpoint and the explicitly
supported, empty additive stages described under recovery below.

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
   includes migration history in the source digest, acquires the shared migration
   maintenance lock, locks the conversion tables and history, performs all database writes in one serializable
   transaction, verifies the result, validates the continuation ownership FK, and marks
   readiness `ready` only before commit. A failure rolls back all database changes.
7. After the historical converter verifies schema1, advance to the messaging checkpoint,
   preserve every non-deleted DigitalHuman channel installation, and review the safe
   report before applying. Configure `DIGITAL_HUMAN_CREDENTIAL_KEYS_JSON` and
   `DIGITAL_HUMAN_CREDENTIAL_ACTIVE_KEY_ID` with every historical v1/v2 key. From `api/`:

   ```text
   go run ./cmd/migrate connector-checkpoint --id 20260903120000_messaging_connector_protocol
   go run ./cmd/migrate digital-human-preserve plan --report <dh-plan.json>
   go run ./cmd/migrate digital-human-preserve apply --expected-report <digest> --report <dh-apply.json>
   go run ./cmd/migrate digital-human-preserve verify --report <dh-verify.json>
   ```

   This step rebinds organization channel credentials to the stable installation UUID in
   one serializable transaction. It retains the old Connection row for the final archive
   and writes paired source/target commitments to `connector_conversion_records`.
   Pending deliveries and processing markers carry a copy of the installation's
   connection locator; the same transaction moves them too. When an installation already
   owns its credential but older rows still carry the legacy locator, the report lists it
   under `repairs` and apply moves those rows; verify requires both `items` and `repairs`
   to be empty.
8. Use the API-owned finalizer while all writers remain stopped. Keep all paths outside
   the repository:

   ```text
   go run ./cmd/migrate connector-finalize plan --report <plan.json> --archive <archive.enc> --archive-key-file <archive.key>
   go run ./cmd/migrate connector-finalize apply --expected-report <digest> --archive <archive.enc> --archive-key-file <archive.key> --report <apply.json>
   go run ./cmd/migrate connector-finalize verify --report <verify.json>
   ```

   The finalizer inventory and encrypted archive include DigitalHuman definitions,
   published versions, runs, resource bindings, memories, channel rows, outbox rows and
   installation credential envelopes. It refuses any non-deleted installation that still
   resolves through a retired Connection. The plan digest is the top-level `digest` in
   the reviewed report. Confirm the API
   uses the same native keyring, start only the schema2 runtime, and verify it before
   restoring traffic. The database backup, encrypted archive and its separately
   protected key are the recovery materials; the retired database tables are removed.

The JSON report contains identifiers, counts, digests and bounded result codes. It
does not contain Keys, decrypted payloads, SQL driver messages or Skill text. Treat
reports as operational data and keep them outside source control.

## Databases running with retirement deferred

A database that ran with Connector retirement deferred (explicitly, or chosen by
the default `auto` mode because legacy storage held data) has applied the
migrations after the detach checkpoint without retiring legacy storage, so the
default checkpoint check refuses it. Convert it with an explicit opt-in naming
the exact applied migration head:

1. Upgrade the API first (`go run ./cmd/migrate up` from `api/`); retirement
   stays deferred and the database reaches the current schema.
2. Stop API writers, workers and scheduled jobs, and take the backups above.
3. Run the converter with `--advanced-checkpoint <applied-migration-head>` on
   every command (`inventory`, `dry-run`, `apply`, `verify`).
4. Preserve DigitalHuman channels that still use a legacy Connection, then run
   `connector-finalize plan|apply|verify` from `api/` with the same
   `--advanced-checkpoint` and the encrypted archive.

The opt-in requires schema1, the detach checkpoint, an absent retirement
migration, the complete retained Connector structure, and the same empty
additive Connector stages as the default path; the named head must be exactly
the latest applied migration. The ledger is part of the source digest, so apply
refuses a reviewed dry-run whose history changed.

## Conversion behavior

- Existing native UUIDs, credential versions, binding restrictions and tombstones remain.
  Supported old personal GitHub/Exa Key connections retain their connection UUIDs.
- Connection and Binding restrictions preserve operation intersection, owner, target
  scope, expiry and denials. An unsupported restriction stays denied. Copying a
  snapshot never expands a restriction to permit its new target.
- Agent draft and published snapshots receive canonical binding references. Workflow
  graphs, runs and pauses receive exact version/node references. A historical run whose
  graph differs from its current draft receives a retained immutable workflow snapshot.
- Chat preferences retain their exact organization, account and workspace scope.
  An unscoped preference applies only to unscoped conversations, never as a workspace
  fallback. Conflicting connections within the same target still block conversion;
  existing canonical binding restrictions and disabled state remain intact.
- Source commitments use transaction-local UTC timestamp rendering, matching the
  finalizer without changing stored timestamps or database-wide settings. Regenerate
  dry-run reports with the updated tool before apply; a report from a non-UTC session
  of an older tool is not interchangeable with a canonical UTC report.
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

If normal `up` committed the chat-preferences, onboarding and messaging migrations
before failing with `ready schema1 checkpoint required`, do not delete migration
markers, force readiness, or request the older detach checkpoint again. The converter
can resume steps 4–6 at these known pre-retirement stages, provided their new tables
are empty and their markers are complete and consistent with the tables. Populated
additive tables require a separately reviewed native-state recovery; this historical
tool does not rewrite their data.

The already-published `20260903183000_add_video_runtime_task_poster_url` migration
is an explicit compatible exception, not a Connector checkpoint. It may coexist
with either maintenance boundary. Unknown later migrations, retirement markers,
schema2 and incomplete history are still rejected. No ID-prefix heuristic or
global `max(id)` is used to authorize conversion or retirement. Retirement report
version 3 also binds the complete migration history, so old reports/archives must
be regenerated and reviewed before applying the repaired tool.

Keep traffic stopped after any apply/commit error. Run `verify` to establish whether a
commit succeeded before retrying. Restore the complete maintenance-window backup
(database, storage and matching keyrings) or deploy a forward fix. There is no online
reversal or second provider execution path. Do not mix old and new runtime writers.

`go test ./...` covers independent synthetic secret vectors, restrictions, tombstones,
source digest stability, host/paused snapshots, terminal barriers, file boundaries and
sanitized database errors. Command tests also cover read-only transaction modes, stale
reports, immediate rollback on write/verification failures and ambiguous commit errors.
These use transaction fixtures and do not prove PostgreSQL durability.
API tests named `TestConnectorFinalizationPostgres` and
`TestConnectorHistoricalUpgradePostgres` use the isolated
`ZGI_CONNECTOR_FINALIZE_TEST_DSN`. Without it, PostgreSQL behavior and a full historical
upgrade remain release gates; in-memory and SQL mock tests are not a substitute.
