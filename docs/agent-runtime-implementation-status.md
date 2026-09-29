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
| M0 baseline | In progress | Architecture and business plans are present on the implementation branch. Go 1.26.2 Linux API and Sandbox baselines plus local template smoke tests are recorded below; black-box and secure-runtime checks remain. |
| M1 control plane | Complete | Versioned resource tables, idempotent Binding state transitions, tenant ownership, generation fencing, admission leases, leased cleanup reconciliation, fake-backend tests, real PostgreSQL migration tests, and Linux race tests pass. HTTP exposure remains intentionally deferred until the M2 data plane exists. |
| M2 local backend | Not started | Requires a real Linux gVisor integration run. |
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
| Sandbox black-box flow | `make test-sandbox-kest` | Pending |
| Linux secure runtime | `make -C sandbox test-linux-secure` | Pending; requires Linux isolation support |
| API Script Skill tests | `go test ./internal/modules/skills/...` from `api/` | Passed in a Go 1.26.2 Linux container (`ok`, 3.609s). A Windows Go run exposed two model-channel timing failures, so Linux remains the required baseline environment. |
| API Script Skill E2E | `make test-api-skill-script-e2e` | Pending; requires a running Sandbox |
| Python template smoke test | documented example request and Artifact hash | Passed in `python:3.13-bookworm`: compile, valid JSON-object stdout, declared `artifacts/result.json`, invalid-input non-zero exit, SHA-256 `e4fbef670e11e57e1eeaa55de2746bf94ddfbe7aef863159e58fd04cfc074d59`. |
| Node template smoke test | documented example request and Artifact hash | Passed in `node:22-bookworm`: syntax checks, valid JSON-object stdout, declared `artifacts/result.json`, invalid-input non-zero exit, SHA-256 `6a6c3f1aa59955ffb67312093a1c80c2079c821c0a98934b5f0d70e021774e83`. |

## M1 verification record

| Check | Command | Result |
|---|---|---|
| State-machine unit tests | `go test ./internal/agentruntime` from `sandbox/` | Passed. Covers idempotent creation, idempotency-key conflicts, cross-tenant hiding, generation fencing, pause recovery, repeated deletion, concurrent creation, compensating cleanup, expired-claim recovery, cleanup retry, and lease-owner fencing. |
| State-machine race test | `CGO_ENABLED=1 go test -race ./internal/agentruntime` in `golang:1.26.2-bookworm` | Passed. |
| PostgreSQL migration/repository | `go test -count=1 -v ./internal/storage` against temporary `postgres:16-alpine` | Passed, including migration ledger, idempotent reservation, generation CAS, lease acquisition/renewal/release, exclusive cleanup claims, expired-claim recovery, and all pre-existing storage tests. The temporary database used no persistent volume and was removed after the run. |
| Static analysis | `go vet ./internal/agentruntime ./internal/storage` from `sandbox/` | Passed. |

## Compatibility gates

Every later stage must continue to verify:

- Existing Agents do not receive Shell or Workspace tools when the new configuration is absent or disabled.
- Runtime Workspace credentials cannot call Agent Runtime endpoints.
- Existing Script Skills retain input, dependency, output, Artifact, stderr, non-zero exit, and package-size behavior.
- Frontend disconnect is not treated as a user stop request.
- Model-visible tool output, persisted transcript content, audit summaries, and raw Job logs obey separate size and redaction rules.
- Disabling new Binding creation does not prevent authorized users from retrieving retained files or operators from cleaning resources.

