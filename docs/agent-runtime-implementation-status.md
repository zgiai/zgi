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
| M2 local backend | Complete | The real containerd/gVisor node contract passes with the standalone Runtime Agent: file transfer, Python Job, PTY, denied outbound traffic, suspend/resume with retained files, and process-tree cancellation. A dedicated real-node test confirms CPU, memory, and PID cgroup enforcement; see the resource-exhaustion behavior and cleanup caveat below. |
| M3 managed cloud backend | Complete | A direct REST E2B adapter, approved-template mapping, authenticated one-time bootstrap, network-disabled secured creation, pause/resume/destroy lifecycle, restart-safe token recovery, Worker backend selection, and the real-account tagged contract test pass. The provider list showed no matching orphan after cleanup. |
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
| Docker Desktop nested-gVisor probe | Privileged Docker harness with containerd `v2.2.9`, gVisor `release-20260622.0`, and image `sha256:8665d122c3ac72c853c57a7ca18c9ece3555374428b2482d732897ae0f2cc932` | The image was built and imported successfully, but the shim blocked during task creation after `runsc create` completed and the sandbox began waiting for its start signal. The same failure occurred with containerd `v2.4.1` and gVisor `release-20260921.0`. The Provider now applies its startup deadline to stale cleanup, image lookup, container creation, task creation, task start, and readiness; the same probe returned after 45.01 seconds instead of hanging until the test-wide timeout. Native Linux later reproduced this without the required sandbox annotation, so this was not solely a nested-virtualization limitation. Existing `zgi-c-*` containers were not restarted or modified. |
| Independent Linux Worker | `go test` and `go vet` for `cmd/agentworker`, `internal/agentexecution`, and `internal/storage` in `golang:1.26.2-bookworm`; PostgreSQL tests against temporary `postgres:16-alpine` | Passed. The process wires only the dedicated Agent Runtime API to the local Provider, owns cancelable Job/cleanup/idle-reaper loops, keeps the legacy Sandbox server separate, refreshes the 30-minute idle deadline on real data-plane activity, and fences stale idle reaps through generation changes. Temporary database, network, and cache volumes were removed after validation. |
| Native Linux containerd/gVisor contract | `TestContainerdGVisorContract` on Ubuntu 24.04.5, kernel 6.8.0-139, x86_64, 2 vCPU, 1.7 GiB RAM, no `/dev/kvm`; containerd `v1.7.29` and then `v2.2.9`, gVisor `release-20260921.0`, pinned image `sha256:3d4a486e1a58c75ac73380fbff594c7204c88001d813728f6fa0985465ef1eb4` | Passed on both containerd versions (2.10s and 2.05s); then passed three consecutive runs on `v2.2.9` (2.01s, 1.71s, 1.70s). Verified authenticated Runtime Agent capability query, file upload/download, Python transform, PTY, denied outbound connection, suspend/resume with retained file, process-tree cancellation, and task cleanup. The final node runs containerd `v2.2.9`; no container task remained after the contract. A temporary 4 GiB swap file used on the small node was disabled and removed after validation. |
| Native Linux failure diagnosis | Minimal `ctr run` and Runtime Agent probes against the same node | Without `io.kubernetes.cri.container-type=sandbox`, `containerd-shim-runsc-v1` held `runsc create` output open and task creation hit the startup deadline. With that OCI annotation, the standalone container started. Its sandbox-local Unix socket was then visible only inside gVisor, not at the host bind-mount path. Adding only `dev.gvisor.flag.host-uds=create` made the socket host-visible and an authenticated HTTP capability request returned 200. The Provider now sets both annotations; it does not enable opening existing host Unix sockets. |
| Resource-limit enforcement | `TestContainerdGVisorContract` and `TestContainerdGVisorResourceLimits` with `-test.count 3` on the native Linux node, containerd `v2.2.9`, gVisor `release-20260921.0`, and the pinned image above | Passed three consecutive combined runs. The resource test read the task's actual cgroup v2 path and asserted `cpu.max=50000 100000`, `memory.max=536870912`, and `pids.max=64` for the CPU/PID binding; CPU work increased `cpu.stat nr_throttled`, and a fork burst increased `pids.events max` with a kernel `fork rejected by pids controller` message. A separate 256 MiB binding's 384 MiB allocation increased `memory.events oom_kill`. After each run, the containerd namespace had no tasks or containers, and test temporary directories were gone; the host had no active swap. Hard PID exhaustion also severed the Runtime Agent's Unix-socket Job stream (`connection reset by peer`), so a graceful Job result is **not** guaranteed under this condition. The test checks enforcement and cleanup, not continued Binding availability after exhaustion. One empty `zgi-ar-contract-binding` cgroup directory remained with `pids.current=0`; it was removed with a targeted `rmdir`, after which the test cgroup subtree was empty. Automatic removal of empty cgroup directories remains a cleanup-hardening follow-up. |

## M3 verification record

| Check | Command | Result |
|---|---|---|
| Hosted adapter unit contract | `CGO_ENABLED=1 go test -race ./internal/hostedruntime/e2b ./internal/runtimeagent ./cmd/runtimeagent ./cmd/agentworker` in `golang:1.26.2-bookworm` | Passed. Covers current E2B REST request shapes, API/envd/traffic authentication separation, approved templates, authenticated file bootstrap, forced network denial, auto-pause without auto-resume, opaque allocations, pause/delete idempotency, fresh-token reconnect, provider rate-limit classification, Worker backend selection, one-time token-file removal, and removal of Runtime Agent control secrets from child Job environments. `go vet` passed for the same packages. |
| Hosted template image contract | `docker build -f sandbox/Dockerfile.agent-runtime-e2b` plus a disposable container bootstrap smoke run | Passed. E2B's single-stage build contract, UID/GID 65532, Python 3.13, Node.js 22, file-gated Runtime Agent entrypoint, TCP listener, bearer authentication, token-file removal, and disabled-network capability response were verified. The committed file-level build input also passed two consecutive E2B template builds, including a fully cached rebuild. The disposable container was removed. This local check is only template-input evidence. |
| Real managed runtime backend | `go test -tags=e2b_integration -count=1 -v ./internal/hostedruntime/e2b -run TestE2BContract` in `golang:1.26.2-bookworm` with a real E2B account and approved template | Passed twice in 34.83 and 29.09 seconds, including after the final cached template rebuild. Verified secured create and bootstrap, stdout/stderr, file upload/download, full-memory pause/resume with retained file, denied outbound HTTPS, process-tree cancellation, and final deletion. A subsequent provider list found zero matching test or manual-probe Sandboxes. No mock result was substituted for this gate. |

## Compatibility gates

Every later stage must continue to verify:

- Existing Agents do not receive Shell or Workspace tools when the new configuration is absent or disabled.
- Runtime Workspace credentials cannot call Agent Runtime endpoints.
- Existing Script Skills retain input, dependency, output, Artifact, stderr, non-zero exit, and package-size behavior.
- Frontend disconnect is not treated as a user stop request.
- Model-visible tool output, persisted transcript content, audit summaries, and raw Job logs obey separate size and redaction rules.
- Disabling new Binding creation does not prevent authorized users from retrieving retained files or operators from cleaning resources.

