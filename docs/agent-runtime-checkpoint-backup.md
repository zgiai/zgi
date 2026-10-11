# Runtime checkpoint backup and isolated recovery

This operator-only workflow exports suspended runtime Bindings and their verified
Workspace/Home checkpoint payloads. It does not back up the application database,
conversations, Skill packages, exported-file download cards, credentials, Jobs or
provider compute. Keep platform database/object backups and approved immutable
images/templates separately. This increment is not production DR certification.

## Preconditions and safety

- Use the approved Worker binary and its effective environment. Backup and restore
  enforce the existing release contract. Verification does not require the source
  binary version, but still reads the normal Worker configuration at startup.
- Suspend every retained Binding for the selected backend, finish/cancel Jobs,
  then stop all Workers and other writers sharing its database/checkpoint storage.
  Keep admission, retention and cleanup stopped for the entire backup window.
  A read-only database snapshot and object storage are not a globally atomic
  snapshot. Missing objects or non-suspended Bindings fail the export.
- Use a private operator account and a protected backup volume. The recovery set
  contains confidential ownership metadata and user files. Encrypt storage and
  transport; SHA-256 detects byte corruption, not malicious replacement of a
  manifest. This format is not signed and must come from a trusted operator.
- Restore only into a dedicated empty runtime database and separate checkpoint
  directory or S3 prefix/account. Stop target Workers during recovery. The source
  store and backup's own object directory cannot be used as the file target.
  Storage fingerprints catch ordinary misconfiguration, not aliases to the same
  S3 bucket through different endpoint hostnames. Operators must ensure isolation.
- Required Profile IDs must retain identical effective configuration, including
  the approved immutable image or E2B Template. Retention deadlines are preserved,
  not extended. Expired or partially expired sets are rejected, including by verify.

## Commands

All commands below run on a Linux operator host with the relevant Worker
environment already supplied securely. Do not put DSNs or tokens in command lines.

```sh
# Source environment: parent exists and is private; this child must not exist.
agentworker --backup-checkpoints /srv/private-backups/recovery-001
agentworker --verify-checkpoint-backup /srv/private-backups/recovery-001
```

The private set contains `checkpoints/` and `manifest.json`. Each object has a
SHA-256 and byte count. `manifest.json` is published last, after full source/copy
verification. An interrupted directory is diagnostic material, not a usable set;
choose a new child directory on retry. Verify reads without creating directories
or changing permissions. Copy the whole set to independent protected storage and
verify it again there before treating the backup as durable.

For the target, supply a new `ZGI_AGENT_RUNTIME_DATABASE_URL`, a separate existing
private `ZGI_AGENT_RUNTIME_CHECKPOINT_DIR` (or the established S3 configuration),
the matching backend/Profiles, and a valid target release manifest. The target
database may be an empty newly created database; schema initialization is handled
after object verification. Do not point at the source deployment.

```sh
ZGI_AGENT_RUNTIME_RECOVERY_TARGET=true \
  agentworker --restore-checkpoints /srv/private-backups/recovery-001
```

Restore checks for existing runtime data before uploads, verifies destination
bytes even on an idempotent object-store write, then rechecks emptiness under
transactional locks before atomically importing Bindings. Existing runtime data
is never overwritten. A repeated successful restore is refused. A failed upload
or database import may leave objects in the dedicated target prefix; inspect only
that owned prefix and retry with an empty target. Do not delete business data to
make a target empty.

Recovered Bindings are suspended, have a new generation and no source provider
references. First authorized resume creates fresh compute and imports the verified
checkpoint. It never resumes the original provider resource. Jobs, historical
output and cleanup queues are deliberately not replayed. Ownership IDs are
preserved, so the corresponding application identities and conversation mapping
must also exist in the independently recovered application database.

Each successful command prints only operation, backend, Binding/checkpoint counts,
bytes, elapsed milliseconds and `result=verified`; it does not print tenant IDs or
credentials. Failure returns a nonzero exit status. Limits are 30 minutes per
command, 10,000 Bindings, 16 MiB manifest, 1 GiB + 40 MiB per checkpoint and 8 GiB
per set. This initial implementation exports all retained Bindings for one backend;
larger deployments require a separately designed partitioning/streaming workflow.

## Read-only operational metrics

```sh
agentworker --audit-metrics
```

This performs the existing real-provider/database inventory and emits Prometheus
text gauges: audit timestamp, Binding count, active Jobs, pending cleanup, compute
resources and fixed-code finding counts. Labels contain only backend and a bounded
finding code, never tenant/resource IDs or raw diagnostics. It opens no endpoint
and schedules no background task. An operator collector can publish output to a
textfile exporter only after the command succeeds, using a temporary file and
atomic rename. Preserve the last successful sample on failure and alert on stale
`zgi_runtime_audit_timestamp_seconds`; provider failure must not become a healthy
zero-resource report.

Alert candidates include overdue cleanup, expired Job claims, resources remaining
after deletion, and active Bindings missing compute. Unattributed resources require
manual ownership checks across deployments; they are never automatically removed.
This provides a collection primitive, not installed alert rules, SLOs or billing
reconciliation.

## Repeatable acceptance

With Go 1.26.2, an isolated PostgreSQL administrator, and a dedicated real S3 test
endpoint/private pre-created bucket:

```sh
make -C sandbox test-agent-runtime-backup
```

For each real backend, supply the normal Worker environment and
`ZGI_RUNTIME_BACKUP_TEST_PROFILE` identifying an approved package-install Profile:

```sh
make -C sandbox test-agent-runtime-backup-real-backend
```

The latter creates and removes its own two PostgreSQL databases, filesystem roots
and provider environments. It installs pinned Python `ujson` and native Node
`bufferutil`, writes a marker, exports/verifies the set, removes **only its own**
source compute/checkpoint, restores to the new empty target, and verifies imports,
non-root UID, exact marker bytes and cross-tenant refusal without reinstalling.
Do not run it with accounts lacking permission to create/drop isolated test DBs.

Acceptance on 2026-10-08:

- Real Linux gVisor: independent recovery passed (16,537,088-byte checkpoint).
- Real E2B: independent recovery passed (16,536,576-byte checkpoint).
- Real PostgreSQL: empty-target transaction, generation fencing, source unchanged
  and repeated-restore rejection passed.
- Dedicated S3-compatible server: independent prefix copy, source-object loss,
  destination checksum and missing-source refusal passed.
- Race tests and `go vet` passed for the changed execution/backup/Worker packages.

Post-test read-only metrics left the dedicated Linux deployment at 34 Bindings,
zero compute, active Jobs, cleanup tasks or findings. E2B retained its pre-existing
29 Bindings, 22 account resources and nine scheduled discovery tasks, with zero
active Jobs or overdue cleanup. Eight resources not attributed to this database
were already identified in other test deployments and were deliberately retained.
Both integration tests removed only their own source/recovery compute and test DBs.

Only disposable test resources are removed. Existing sessions, paused provider
environments and production databases are not recovery targets. This evidence does
not replace cloud object-store account policy tests, node loss, production restore
time objectives, disk quotas, capacity/billing reconciliation or rollout approval.
