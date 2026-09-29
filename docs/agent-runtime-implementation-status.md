# Agent Runtime Implementation Status

This document records verified delivery evidence for the Agent execution environment roadmap. A stage is complete only when its stated evidence is available. Planned commands or capabilities are not recorded as passing results.

## Operating contract

Deliver the staged Agent execution environment on `px/runtime-workspace-integration`, verified by the stage-specific automated tests, real backend exercises, and cleanup evidence, while preserving the existing one-shot Script Skill behavior, dependency authorization, Runtime Workspace external-access boundary, and public response contracts.

Work only in the existing runtime workspace integration worktree. At each iteration, run the narrowest relevant test first, record the result here, then broaden validation. If a required real backend, credential, or Linux capability is unavailable, mark that evidence as blocked instead of substituting a mock result.

## Baseline

- Branch: `px/runtime-workspace-integration`
- Baseline commit: `b7c4734bf`
- Required Go toolchain: `1.26.2`
- Local developer shell observed before M0: Windows with Go `1.25.0`
- Existing runtime execution: one-shot Sandbox plus synchronous command and Skill execution
- Existing reusable behavior: Python and Node Script Skills, file inputs, Artifact persistence, bundled local package recognition, dependency Profiles, dependency authorization, and dependency-scan configuration
- Existing `runtimeworkspace` domain: external Agent exposure and authentication; it is not a filesystem execution environment and must not authorize Agent Runtime APIs

## Stage status

| Stage | Status | Verified evidence |
|---|---|---|
| M0 baseline | Complete | Architecture and business plans are present on the implementation branch. Go 1.26.2 Linux API and Sandbox baselines, Kest black-box flow, secure-runtime checks, API-to-Sandbox Script Skill E2E, and local template smoke tests are recorded below. The two pre-existing Sandbox runner process-tree failures remain explicitly listed rather than hidden. |
| M1 control plane | Complete | Versioned resource tables, idempotent Binding state transitions, tenant ownership, generation fencing, admission leases, leased cleanup reconciliation, fake-backend tests, real PostgreSQL migration tests, and Linux race tests pass. HTTP exposure remains intentionally deferred until the M2 data plane exists. |
| M2 local backend | In progress | Persistent asynchronous Job records, leased worker claims, idempotent sequenced stdout/stderr, cancellation, PTY and file-transfer contracts, tenant-scoped data-plane services, a standalone token-authenticated Runtime Agent, its Unix-socket remote adapter, and the containerd/gVisor local Provider implementation pass unit and image-contract checks. A real runsc node contract run is still required for M2 exit. |
| M3 managed cloud backend | Not started | Requires a real E2B integration run and cleanup evidence. |
| M4 first user release | Not started | Requires the authenticated console CSV acceptance flow and legacy regression. |
| M5 retention and templates | Not started | Requires compute release, restart, restore, and template-isolation evidence. |
| M6 controlled expansion | Not started | Requires network-bypass, dependency preparation, authorization, and revocation evidence. |
| M7 self-hosted VM | Not started | Requires a real KVM/Kata integration run. |
| M8 production readiness | Not started | Requires recovery, cost reconciliation, rollback, and complete legacy regression evidence. |

## M0 verification record

The following checks define the baseline. Record exact results when executed with the required toolchain and services.

| Check | Command | Result |
|---|---|---|
| Sandbox Go tests | `go test ./...` from `sandbox/` | Go 1.26.2 Linux container: all packages passed except existing `internal/runner` process-tree tests `TestCommandCancellationKillsShellChildren` and `TestCommandTimeoutKillsShellChildren`. Both observed child processes surviving the shell process-group termination. |
| Sandbox black-box flow | `make test-sandbox-kest` | Passed against temporary PostgreSQL 16 with the CI-pinned Kest `v0.7.7` amd64 binary after verifying SHA-256 `3814d8662a1cc01410302fcea43910f8437d01f969140c406f64b10249134a5a`. Because the Windows checkout used CRLF, only the disposable Linux test copy of Kest scripts was normalized before execution. The temporary database container and Docker network were removed after the run. |
| Linux secure runtime | `make -C sandbox test-linux-secure` equivalent in privileged Docker Desktop Linux | Passed all existing secure-backend tests: Python execution, network control, non-root workspace bind, namespaces/read-only root, signal reporting, per-execution process limits, cancellation, timeout, and child reaping. This is the existing bubblewrap/cgroup baseline, not gVisor evidence. |
| API Script Skill tests | `go test ./internal/modules/skills/...` from `api/` | Passed in a Go 1.26.2 Linux container (`ok`, 3.609s). A Windows Go run exposed two model-channel timing failures, so Linux remains the required baseline environment. |
| API Script Skill E2E | `make test-api-skill-script-e2e` equivalent with Sandbox and API tests in Go 1.26.2 Linux containers | Passed `TestSandboxScriptRunnerRealSandboxE2E` and all backtest scenarios: JSON and HTML Artifacts, oversized Artifact skipping, stderr, non-zero exit, and Node manifest execution. The first disposable Go-only Sandbox container exposed a missing Node binary; installing Debian's Node runtime in that same disposable environment made the complete suite pass. Temporary containers, database, network, and Go cache volumes were removed after the run. |
| Python template smoke test | documented example request and Artifact hash | Passed in `python:3.13-bookworm`: compile, valid JSON-object stdout, declared `artifacts/result.json`, invalid-input non-zero exit, SHA-256 `e4fbef670e11e57e1eeaa55de2746bf94ddfbe7aef863159e58fd04cfc074d59`. |
| Node template smoke test | documented example request and Artifact hash | Passed in `node:22-bookworm`: syntax checks, valid JSON-object stdout, declared `artifacts/result.json`, invalid-input non-zero exit, SHA-256 `6a6c3f1aa59955ffb67312093a1c80c2079c821c0a98934b5f0d70e021774e83`. |

## M1 verification record

| Check | Command | Result |
|---|---|---|
| State-machine unit tests | `go test ./internal/agentexecution` from `sandbox/` | Passed. Covers idempotent creation, idempotency-key conflicts, cross-tenant hiding, generation fencing, pause recovery, repeated deletion, concurrent creation, compensating cleanup, expired-claim recovery, cleanup retry, and lease-owner fencing. |
| State-machine race test | `CGO_ENABLED=1 go test -race ./internal/agentexecution` in `golang:1.26.2-bookworm` | Passed. |
| PostgreSQL migration/repository | `go test -count=1 -v ./internal/storage` against temporary `postgres:16-alpine` | Passed, including migration ledger through `2026092904`, idempotent reservation, generation CAS, lease acquisition/renewal/release, exclusive cleanup claims carrying both compute and workspace handles, expired-claim recovery, and all pre-existing storage tests. The temporary database used no persistent volume and was removed after the run. |
| Static analysis | `go vet ./internal/agentexecution ./internal/storage` from `sandbox/` | Passed. |

## M2 verification record

| Check | Command | Result |
|---|---|---|
| Data-plane contract tests | `go test ./internal/agentexecution` from `sandbox/` | Passed. Covers idempotent Job submission, tenant isolation, chunked output, worker completion, running-job cancellation, path traversal rejection, and file round trips with a Fake Backend. |
| Data-plane race test | `CGO_ENABLED=1 go test -race ./internal/agentexecution` in `golang:1.26.2-bookworm` | Passed. |
| PostgreSQL Job repository | `go test -count=1 -v ./internal/storage` against temporary `postgres:16-alpine` | Passed. Covers Job idempotency, exclusive and expired claims, output sequencing and limits, cancellation, terminal state, and all existing storage tests. |
| Internal HTTP contract | `go test ./internal/agentexecution` from `sandbox/` | Passed. A dedicated Bearer credential and API-injected organization/subject headers are mandatory; the ordinary Sandbox API key is rejected. Binding, Job, output, capability, upload, download, strict JSON, and traversal checks are covered. Registration in the production server remains gated on a real backend. |
| In-environment Runtime Agent | `CGO_ENABLED=1 go test -race ./internal/runtimeagent ./cmd/runtimeagent` in `golang:1.26.2-bookworm` | Passed. Covers direct argv execution, stable output sequencing, token-authenticated remote execution, reconnect-safe output replay, environment/workdir, idempotent Job IDs, Unix-socket lifecycle, PTY and resize, timeout, process-tree cancellation with child reaping, atomic file round trip, and symlink escape rejection. This component assumes an outer isolation boundary and is not safe to advertise as one by itself. |
| Output replay persistence | `go test -count=1 -v ./internal/storage` against temporary `postgres:16-alpine` | Passed. Runtime Agent sequence is the durable `(job_id, sequence)` key; identical replay is a no-op, conflicting replay is rejected, and byte limits are charged once. |
| Local environment lifecycle | `CGO_ENABLED=1 go test -race ./internal/localruntime/... ./internal/runtimeagent ./internal/agentexecution` in `golang:1.26.2-bookworm` | Passed. Covers opaque allocation validation, create/suspend/resume/destroy delegation, retained-workspace resume semantics, non-database connection handling, gVisor-only configuration, digest-pinned Profiles, OCI privilege/resource hardening, symlink rejection, and Unix-socket Runtime Agent access. `go vet` passed for the same packages. |
| Runtime Agent image contract | `docker build -f sandbox/Dockerfile.agent-runtime` plus a disposable Docker smoke run | Passed. The image started as UID/GID 65532 with a read-only root, no network, 256 MiB memory, 64 PID and 1 CPU runtime limits; Python `3.13.15`, Node `v22.23.3`, the mode-0600 Unix socket, bearer authentication, and disabled-network capability response were verified. This validates the image and protocol contract under runc, not gVisor isolation. The disposable container, volumes, image, and build/test cache volumes were removed after validation. |
| Real local runtime backend | containerd + gVisor contract suite | Pending. No gVisor result is inferred from the existing bubblewrap baseline. |

## Compatibility gates

Every later stage must continue to verify:

- Existing Agents do not receive Shell or Workspace tools when the new configuration is absent or disabled.
- Runtime Workspace credentials cannot call Agent Runtime endpoints.
- Existing Script Skills retain input, dependency, output, Artifact, stderr, non-zero exit, and package-size behavior.
- Frontend disconnect is not treated as a user stop request.
- Model-visible tool output, persisted transcript content, audit summaries, and raw Job logs obey separate size and redaction rules.
- Disabling new Binding creation does not prevent authorized users from retrieving retained files or operators from cleaning resources.

