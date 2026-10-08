# Runtime release, admission and inventory gates

This is the first bounded M8 increment following scoped M6C console acceptance.
It is not M8 production certification, M7/Kata support, backup/restore acceptance,
or authorization to expose a Worker publicly. Existing Skill and M6C acceptance
matrices are reused; only changed admission, release and inventory paths are tested.

## Release identity and content

For strict deployments set `ZGI_AGENT_RUNTIME_RELEASE_REQUIRED=true` on both API
and Worker. Set `ZGI_AGENT_RUNTIME_RELEASE_ID` on the API and
`ZGI_AGENT_RUNTIME_RELEASE_MANIFEST` to a private operator-owned JSON file on the
Worker. The manifest must be a regular file, at most 64 KiB, not writable by group
or others. Keep actual deployment manifests and credentials outside Git.

With the final binary and effective deployment environment, run:

```bash
/opt/zgi-agent-runtime/agentworker --release-fingerprint
```

This reads its executable and hashes the effective backend, approved Profiles,
package source configuration, platform-CLI flag, admission limits and resource
settings. It does not connect to PostgreSQL or create compute. Process IDs, API
keys, database URLs and storage credentials are not fingerprint inputs. Keep
package-source URLs credential-free as required by the existing source contract.
Preserve the approved Profile configuration alongside the fingerprint: its hash
is not a substitute for a human-readable provenance record.

Construct a manifest with the returned hashes and actual registry image pins:

```json
{
  "version": 1,
  "id": "runtime-release-1",
  "api_image": "registry.example/zgi/api@sha256:<64 lowercase hex digits>",
  "web_image": "registry.example/zgi/web@sha256:<64 lowercase hex digits>",
  "worker_digest": "sha256:<worker executable hash>",
  "configuration_digest": "sha256:<effective configuration hash>",
  "platform_protocol_version": 1
}
```

Placeholders above are intentionally invalid. Fill every field; unknown fields,
extra JSON values, mutable image tags, missing pins and incompatible versions fail
verification. The Worker verifies its own executable/configuration before opening
the control-plane store or starting backend/background workers.

On the API/Web Docker host, verify the actual running image content separately:

```bash
bash sandbox/scripts/verify-agent-runtime-release release.json api-container web-container
```

The verifier checks the running containers' actual image IDs and registry
`RepoDigests`, not just their configured image names. Locally built images without
registry digests cannot pass this release evidence gate. It does not restart
containers or inspect their environment variables.

An authenticated `GET /v1/agent-runtime/release` returns release ID, contract
version and platform protocol version, never tokens, template IDs or source URLs.
The API verifies it before create/resume/Job submission. The Worker also checks
`X-ZGI-Runtime-Release` on those requests, closing the race between the check and
the write. Unknown/old Workers, mismatches and missing strict configuration deny
admission. Reads, cancellation, suspension, deletion and existing-file access do
not require matching release IDs. Tenant and authentication checks still apply.

Installations without strict mode or a manifest retain the legacy behavior.
Strict releases must explicitly enable the required flag; compatibility mode is
not production release evidence. Do not reuse a release ID for different content.

Local OCI digest and E2B Template ID are covered by the approved Profile hash.
E2B build ID, source OCI digest and build results still require the operator's
existing template provenance record. This gate does **not** prove an immutable
provider build from a Template ID alone. Publish a separate approved template
for each release; do not rebuild a pinned template in place. Provider-side build
attestation remains a separate rollout requirement.

## Database-atomic admission

Worker defaults (all `ZGI_AGENT_RUNTIME_` prefixed):

| Environment suffix | Default | Scope |
|---|---:|---|
| `MAX_BINDINGS` | 2 | Resident/uncertain environments per backend/database |
| `MAX_BINDINGS_PER_ORG` | 2 | Resident/uncertain environments per organization/backend |
| `MAX_JOBS` | 2 | Queued plus running Jobs per backend/database |
| `MAX_JOBS_PER_ORG` | 2 | Queued plus running Jobs per organization/backend |
| `MAX_JOBS_PER_BINDING` | 1 | Queued plus running Jobs per Binding |
| `JOB_TIMEOUT_SECONDS` | 300 | Default and maximum timeout; range 1–3600 |

All values must be positive integers; malformed values do not fall back silently.
Organization limits cannot exceed backend limits, and the Binding Job limit cannot
exceed the organization limit. These are conservative capacity settings, not host
memory auto-sizing. Retain the existing compute memory/CPU/PID isolation limits.
`MAX_WORKERS` controls consumers in one process, not shared admission capacity.

Migration `2026100802` adds the backend policy registry and capacity indexes.
Count-and-reserve use one PostgreSQL transaction and an advisory transaction lock,
shared across Worker processes. Resume reserves capacity before calling the
provider. Queued Jobs already reserve capacity. Idempotent requests return their
original resource without consuming a second slot. CAS generation remains enforced.

Allocating, active, resuming, suspending, deleting and failed Bindings consume
capacity. Suspended/deleted Bindings do not. Failed/uncertain allocations are
deliberately conservative: confirm cleanup and use the normal explicit lifecycle
operation; never remove retained user files merely to release a slot. New admissions
return `429 capacity_exceeded`; mismatched policy returns
`503 admission_policy_mismatch`. Neither response disables authorized retrieval or
cleanup of existing resources. A zero Job timeout becomes the configured maximum;
an explicit larger timeout is rejected, not silently made unbounded.

The first successful reservation registers the backend's policy. All upgraded
processes sharing the backend/database must agree. Processes without limits cannot
bypass an already registered policy. This does not retrofit enforcement into an
old executable: drain admission and upgrade all consumers before relying on it.

To change policy, stop new admission, finish/cancel Jobs, suspend environments and
explicitly resolve failed/deleting allocations. With the new environment settings:

```bash
/opt/zgi-agent-runtime/agentworker --set-admission-policy
```

This explicit maintenance command refuses a non-drained backend, atomically
updates only the policy, and leaves suspended files intact. Stale processes then
fail new admission. Regenerate the release manifest/configuration fingerprint and
restart the dedicated workers under the new release ID. Do not delete the policy
table to bypass a limit. Registry state is retained on rollback; old binaries may
not enforce it, so keep new admission disabled during rollback to old code.

## Read-only reconciliation

Using the Worker's effective environment, run:

```bash
/opt/zgi-agent-runtime/agentworker --audit
```

This runs once, with a 30-second deadline, without starting HTTP/background workers.
It reads containerd's fixed `zgi-agent-runtime` namespace and managed labels, or
E2B running **and** paused resources through bounded pagination. It does not
connect to/resume a sandbox, install packages, create directories or mutate compute.
PostgreSQL uses a repeatable-read, read-only transaction, without migrations.

The JSON report contains resource/Binding/active-Job/pending-cleanup counts and
safe findings for missing compute, state/ref drift, expired Job claims, overdue
cleanup and conservative failed capacity. Pending cleanup excludes completed tasks.
Each DB collection is capped at 10,000 rows; E2B at ten 100-item pages per state.
Timeouts, unavailable providers, bad responses and exhausted pagination fail the
command rather than producing a falsely clean partial report.

Provider and DB snapshots are not atomic. Confirm findings with a second sample
before using the existing ownership/generation-aware lifecycle/cleanup APIs.
Resources missing from this database are `unattributed_resource`, not proven
orphans: another deployment may share the provider account/namespace. The audit
does not automatically delete or enqueue deletion of any finding. Store reports
privately; resource IDs are operational data, not public UI content.

E2B state/pagination behavior follows its [official SDK implementation](https://github.com/e2b-dev/E2B/blob/main/packages/js-sdk/src/sandbox/sandboxApi.ts).

## Incremental validation and remaining gates

Validated with Go 1.26.2 on Linux:

- Two independent PostgreSQL connections/pools: concurrent reservations, backend/
  organization/Binding limits, idempotent retry at capacity, resume rejection,
  timeout enforcement, stale/mismatched policy rejection and explicit drained update.
- Release manifest malformed/missing/mutable/mismatched cases, old-Worker refusal,
  API release handshake and header checks, real existing-file download after
  capacity rejection and release mismatch, plus cross-organization rejection.
- Bounded read-only E2B inventory tests including paused state, pagination and
  incomplete/error responses; snapshot comparison and existing storage/lifecycle
  regressions; race tests, vet and Linux Worker build.
- Installer persistence/rollback mock tests and running-image verifier tests,
  including mismatched image content and stopped containers; ShellCheck.
- Real-account read-only E2B audit: 20 DB Bindings, zero active Jobs, zero pending
  cleanup tasks, 16 ZGI-marked provider resources. Eight resources were not
  attributable to this database and were not deleted or labeled confirmed orphans.

Focused commands (from `sandbox/`, using the repository's Go 1.26.2 Linux toolchain):

```bash
go test -race ./internal/storage -run 'TestAgentAdmission|TestAgentRuntime|TestAgentExecution|TestPlatformJobGeneration'
go test -race ./internal/agentexecution ./internal/hostedruntime/e2b ./internal/localruntime ./internal/localruntime/containerd ./cmd/agentworker
go vet ./internal/storage ./internal/agentexecution ./internal/hostedruntime/e2b ./internal/localruntime/containerd ./cmd/agentworker
bash scripts/test_zgi_runtime_server
bash scripts/test_verify_agent_runtime_release
shellcheck scripts/zgi-runtime-server scripts/verify-agent-runtime-release scripts/test_verify_agent_runtime_release
```

Set `ZGI_SANDBOX_TEST_POSTGRES_ADMIN_DSN` to an isolated test administrator; the
fixtures create/drop only their unique test databases. The script tests need jq
and Python 3 and mock host/service commands, not a real deployment. From `api/`:

```bash
go test -race ./internal/modules/app/agents -run 'TestGatewayReleaseGate|TestStrictGatewayRelease|TestAgentExecutionHTTP|TestAgentExecutionGateway'
go vet ./internal/modules/app/agents
go build -o /tmp/zgi-api-release-test ./cmd/server
```

Use the existing test-only connector credential fixture/environment required by
API bootstrap; never provide application/provider secrets in test output.

The initial concurrent test encountered the shared local PostgreSQL connection
ceiling. Revalidation used a separate disposable PostgreSQL 16 container and passed;
the application database was not tuned/restarted. Its test data/container were
removed afterward. An unconfigured S3 test was skipped and is not evidence for M8
backup/restore. The subsequent dedicated deployment checks below use real running
images and providers; neither the unit tests nor that deployment certify production.

### Dedicated deployment acceptance (2026-10-08)

- Deployed the API and both Worker backends to dedicated test services. Reused the
  unchanged, previously accepted Web build. Verified actual running API/Web image
  IDs against registry digests, Worker executable/configuration fingerprints and
  authenticated release identities. Registry transfers stayed local; Worker access
  remained private through the existing SSH connection.
- Real gVisor startup, Python/Node checks and database-backed health passed. No
  containerd restart was needed. Local and real-account E2B checks exercised
  Binding capacity, retry-at-capacity idempotency, per-Binding Job capacity,
  excessive-timeout rejection, cancellation under release mismatch, stdout/stderr,
  result files, suspension, resume-capacity rejection and file recovery after resume.
- A separate internal-only Worker with stale limits was refused by the deployed
  shared PostgreSQL admission policy with `admission_policy_mismatch`; it allocated
  no provider environment. It was removed after the negative test.
- A deliberately incorrect Worker manifest prevented startup during a real Linux
  upgrade. The installer restored the previous installation automatically; health,
  executable content and 60 retained checkpoint files were unchanged afterward.
- The eight E2B resources not attributable to the selected database were matched
  to suspended Bindings in other test databases. They were not deleted. Five
  expired local test environments were suspended through the normal API, retaining
  their checkpoints. New completed test environments were also suspended.
- Fixed an actual lifecycle defect: deleting an unpublished failed allocation no
  longer sends empty resource references to the backend. Existing durable cleanup
  and allocation-discovery tasks remain intact. Generation fencing, retry deletion
  and cleanup ownership have targeted tests; race tests and vet passed on Go 1.26.2.
  Only failed/empty environments created by this acceptance were deleted.

### Follow-up checkpoint and cleanup repair (2026-10-08)

The three blockers exposed above have been repaired and verified:

- Checkpoint capture initializes the confined session HOME before writing its
  ABI/cache marker. Retained-ledger verification still runs first; a damaged
  imported inventory is not silently accepted. Both the updated OCI image on the
  real gVisor host and a newly built real E2B template passed creation, file upload,
  suspension **before any Job**, resume, byte-for-byte file retrieval, a first
  non-root Python Job and another suspension.
- Idle suspension has a ten-minute context deadline, preserving the archive
  transport budget. A failed `suspending` attempt is excluded until fifteen
  minutes after its persisted `updated_at`, before applying the expired queue's
  result limit. Retry state survives Worker restart; unit tests with virtual time
  verify cancellation, and separate real PostgreSQL pools verify oldest-failure
  fairness and the exact retry boundary. These tests do not simulate a real
  provider hanging for ten minutes.
- An empty-reference destroy task completes only when its matching Binding is
  already deleted, uses the same backend and has no published resource references.
  Missing, live, mismatched or referenced Bindings retain retry ownership. Separate
  allocation-discovery records are untouched. The previously retrying empty task
  on each deployed backend completed through the normal reconciler, without SQL
  rewriting task state or deleting unowned provider resources.
- The historical unused local environment still ran the old Runtime Agent. Only
  its confirmed absent HOME was initialized with the session UID and private
  permissions; normal suspend/resume then succeeded. It contained no business
  files. Existing result-bearing checkpoints on the old Profile were separately
  restored on both backends and their marker/result bytes verified unchanged.

New image/template versions use new internal Profile IDs; old versions remain
available for restoration. The Linux installer now accepts explicitly configured
secondary immutable image references only when preloaded in its fixed namespace.
Mutable or missing secondary images fail closed. Installer persistence/rollback
tests and real upgraded-service health passed. New test environments were left
suspended with their files; final local audit had zero compute, active Jobs,
pending cleanup or findings. E2B allocation-discovery tasks remain scheduled and
the eight previously attributed resources from other test databases were retained.

File-access boundaries are unchanged: an active workspace file remains readable
under admission rejection. Raw Worker file reads from a suspended Binding require
resume (and therefore available capacity); they currently return a state conflict.
Previously exported persistent download cards are a separate API/storage path.
This deployment did not repeat browser or the complete standard-Skill matrix.

Still outside this increment: automatic orphan repair, multi-node placement,
disk-byte/retained-storage quotas, cost/billing reconciliation, provider build
attestation, stable private connectivity, alerting/SLOs, backup/restore, failure
drills and broad production rollout. Existing exported-file retention and prior
M6C acceptance are not re-certified by these tests.
