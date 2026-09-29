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
| M0 baseline | In progress | Architecture and business plans are present on the implementation branch. Test evidence is recorded below. |
| M1 control plane | Not started | Requires resource-state, idempotency, ownership, generation, cleanup, and fake-backend tests. |
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
| Sandbox Go tests | `go test ./...` from `sandbox/` | Pending |
| Sandbox black-box flow | `make test-sandbox-kest` | Pending |
| Linux secure runtime | `make -C sandbox test-linux-secure` | Pending; requires Linux isolation support |
| API Script Skill tests | `go test ./internal/modules/skills/...` from `api/` | Pending |
| API Script Skill E2E | `make test-api-skill-script-e2e` | Pending; requires a running Sandbox |
| Python template smoke test | documented example request and Artifact hash | Pending |
| Node template smoke test | documented example request and Artifact hash | Pending |

## Compatibility gates

Every later stage must continue to verify:

- Existing Agents do not receive Shell or Workspace tools when the new configuration is absent or disabled.
- Runtime Workspace credentials cannot call Agent Runtime endpoints.
- Existing Script Skills retain input, dependency, output, Artifact, stderr, non-zero exit, and package-size behavior.
- Frontend disconnect is not treated as a user stop request.
- Model-visible tool output, persisted transcript content, audit summaries, and raw Job logs obey separate size and redaction rules.
- Disabling new Binding creation does not prevent authorized users from retrieving retained files or operators from cleaning resources.

