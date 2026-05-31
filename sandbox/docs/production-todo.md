# zgi-sandbox Production TODO

## 1. Purpose

This document tracks the remaining work required to move `zgi-sandbox` from a
functional execution service into a production-grade runtime foundation for ZGI
workflows, skills, agents, and interactive workspaces.

The plan uses only ZGI-native terminology. It should guide implementation,
testing, review, and release readiness with a consistent ZGI product voice.

## 2. Current Baseline

Already available:

- Independent `zgi-sandbox` HTTP service
- Sandbox lifecycle APIs
- Code execution API
- Command execution API
- Bounded template rendering API
- File upload, download, info, tree, and delete APIs
- Archive upload API for zip packages
- Skill script package execution through `zgi-api`
- Manifest-driven skill execution API
- Artifact collection for script-generated files
- Command profiles: `code-short`, `skill-python`, `skill-node`
- Template profile: `template-short`
- Dependency profile catalog
- PostgreSQL-backed sandbox metadata, endpoint metadata, and observer events
- API key support
- Kest black-box sandbox flows
- API skill-script E2E runner
- Stateless `code-short` profile execution by default, with explicit workspace binding when needed
- Path escape, zip slip, symlink, dangerous env, stdin, timeout, and output guardrails
- Process group cleanup for preview command timeouts
- Structured cancellation errors for request-canceled execution paths
- Configurable graceful shutdown for service drain on SIGTERM/SIGINT
- Request correlation IDs for HTTP responses and execution observer events
- Execution IDs for code, command, template, and skill execution responses and observer events
- Sandbox-scoped execution history backed by observer execution events
- Structured observer events for successful and failed execution paths
- Structured observer events for API boundary policy denials
- Observer event pagination with bounded default and maximum page sizes
- Observer event filters for organization, workspace, app, workflow run, user, and request scope
- Observer event retention by age and maximum row count
- Metrics endpoint for worker active sandbox count, runner pressure, and execution observer counters
- Readiness probe for dependency checks
- Startup log emits effective non-secret configuration
- Worker-scoped active sandbox limit accounting
- Optional service-scoped concurrent execution limit accounting
- Optional profile-scoped concurrent execution limit accounting
- Optional organization-scoped active sandbox limit accounting
- Optional organization-scoped concurrent execution limit accounting
- Optional organization-scoped queued execution limit accounting
- Optional organization-scoped execution rate limit accounting
- Optional sandbox workspace file count limit accounting
- Optional sandbox workspace byte limit accounting
- Optional organization-scoped active dependency profile limit accounting
- Sandbox ownership fields for organization, workspace, app, workflow run, and user context
- Ownership context persisted with sandbox records and propagated to lifecycle, endpoint, expiration, execution, file, archive, and artifact manifest observer events
- Artifact manifests include content type, reference encoding, SHA-256 hashes, timestamps, and enforce operator-configurable file count and total byte limits
- Optional Linux secure backend with namespace-based isolation

The service is useful for validation and controlled internal environments. It
should not be described as fully production-ready until the runtime isolation,
resource governance, network enforcement, and organization quota items below are
implemented and verified.

## 3. Delivery Principles

- Short code paths should be stateless, tightly limited, and fast.
- Session code paths may preserve workspace files, but must still be bounded.
- Interactive code paths need stronger isolation than short workflow execution.
- Network access must be denied by default and explicitly granted by policy.
- Runtime behavior must be enforced below request validation, not only documented.
- Every production claim needs automated tests and an operator-facing validation path.
- Public API names, config keys, comments, docs, and release notes must stay ZGI-native.

## 4. Milestone A: Short Code Runtime

Goal: make `code-short` a reliable workflow building block for small transforms,
simple calculations, and deterministic data shaping.

### A1. Contract

- Define a structured short-code response:
  - `stdout`
  - `stderr`
  - `exit_code`
  - `duration_ms`
  - `truncated`
  - `result_json`
  - `warnings`
- Add optional request fields:
  - `input_json`
  - [x] `expected_output_schema`
  - `strict_result_json`
- Keep `POST /v1/exec/code` backward compatible.
- [x] Add a new profile-level behavior flag for stateless execution.

### A2. Limits

- Default timeout: 3-5 seconds.
- Default stdin/input limit: 64 KiB.
- Default stdout limit: 64 KiB.
- Default stderr limit: 64 KiB.
- Default generated file limit: disabled or temporary-only.
- [x] Reject outputs that exceed configured JSON result limits.
- [x] Reject request bodies over a profile-specific maximum before decoding large payloads.

### A3. Filesystem Behavior

- [x] Add an explicitly stateless execution mode.
- [x] Run each short-code request in a temporary workspace.
- [x] Remove the workspace after execution.
- [x] Prevent short-code paths from writing into session workspaces unless explicitly bound.
- [x] Add tests that prove no files survive a stateless execution.

### A4. Tests

- [x] Unit tests for profile normalization.
- [x] API tests for size, timeout, output, and schema failures.
- [x] Kest flow for short-code success.
- [x] Kest flow for short-code timeout.
- [x] Kest flow for short-code output truncation.

## 5. Milestone B: Template Runtime

Goal: support bounded template rendering as a first-class runtime path without
forcing template use cases through general command execution.

### B1. API

- Added `POST /v1/exec/template`.
- Added request fields:
  - `engine`
  - `template`
  - `variables`
  - `profile`
  - `timeout_ms`
  - `output_limit_kb`
- Added response fields:
  - `content`
  - `duration_ms`
  - `truncated`
  - `warnings`

### B2. Policy

- Added default profile: `template-short`.
- Filesystem access is not exposed to template rendering.
- Network access is not exposed to template rendering.
- Added a small helper allowlist.
- Added variable count, nesting depth, string length, and rendered output size limits.
- Added rejection for template engines not registered in policy.

### B3. Tests

- Successful render.
- Missing variable behavior.
- Oversized variable rejection.
- Function allowlist enforcement.
- Timeout behavior.
- Output truncation behavior.

## 6. Milestone C: Skill and Interpreter Runtime

Goal: make script skills and interpreter-style tools safe, repeatable, and
ergonomic for workflow and agent use cases.

### C1. Skill Package Execution

- Added `POST /v1/exec/skill` for manifest-driven package execution.
- Added archive upload plus manifest execution as a runtime contract.
- Added a skill execution manifest:
  - `entrypoint`
  - `language`
  - `timeout_ms`
  - `allowed_artifact_paths`
  - `max_artifact_count`
  - `max_artifact_bytes`
  - `result_mode`
- Validate the manifest before upload and execution.
- Keep `SKILL.md`, `references/`, `scripts/`, and `artifacts/` as the default package shape.

### C2. Artifact Manifest

- Added artifact manifest generation for sandbox artifact paths.
- Added manifest fields:
  - path
  - size
  - content type
  - reference encoding
  - SHA-256 hash
  - modified time
  - truncated flag
- Added operator-configurable artifact count and total byte enforcement before returning manifest results.
- Return large artifacts by reference rather than embedding content.

### C3. Interpreter Sessions

- Add a session-bound execution mode for multi-step tools.
- Preserve workspace state within the session TTL.
- Added sandbox-scoped execution history backed by observer execution events.
- Support explicit cleanup of generated files.
- Add a clear boundary between workflow session sandboxes and interactive workspaces.

### C4. Tests

- Real sandbox E2E for skill package execution.
- Kest flow for archive upload, command execution, and artifact download.
- Artifact manifest unit tests.
- Session persistence tests.
- Session cleanup tests.

## 7. Milestone D: Dependency Profiles

Goal: make dependencies repeatable, controlled, and fast without allowing
arbitrary runtime installs from untrusted code.

### D1. Profile Model

- [x] Store dependency profiles as versioned records in the policy catalog.
- [x] Track:
  - profile name
  - language
  - package list
  - package versions
  - base runtime image or rootfs version
  - checksum
  - enabled flag
  - owner scope
- [x] Add profile status:
  - `draft`
  - `building`
  - `ready`
  - `failed`
  - `disabled`

### D2. Build Path

- Build dependency profiles outside request execution.
- Cache completed profiles.
- [x] Make profile selection explicit in sandbox creation.
- [x] Reject unknown or disabled profiles.
- [x] Record dependency profile version on each sandbox.

### D3. Runtime Policy

- Disable arbitrary dependency installation inside normal execution.
- Add an administrator-only profile build path.
- Add package allowlist and denylist controls.
- Add maximum profile size and build timeout.

### D4. Tests

- [x] Profile selection tests.
- [x] Disabled profile rejection.
- Version pinning tests.
- Build failure reporting tests.
- [x] Execution uses expected profile version.

## 8. Milestone E: Network Governance

Goal: make outbound network access explicitly governed, auditable, and isolated
from internal infrastructure by default.

### E1. Default Behavior

- [x] Deny outbound network by default for all profiles.
- [x] Require both sandbox-level and profile-level permission for network access.
- Reject network-enabled requests when the selected runtime cannot enforce policy.
- [x] Expose this rejection clearly in `/v1/policies`.

### E2. Egress Policy

- Add egress policy records:
  - policy name
  - allowed hosts
  - allowed ports
  - allowed protocols
  - denied CIDR ranges
  - DNS behavior
  - max request duration
- Block local metadata addresses, loopback, private networks, service networks,
  and link-local ranges unless an operator explicitly permits them.
- Add DNS resolution checks to prevent host allowlist bypass.

### E3. Egress Proxy

- Route approved outbound traffic through a policy-aware proxy.
- Log destination, policy decision, sandbox ID, and request correlation ID.
- Enforce connect, read, and write timeouts.
- Add response body caps for proxied requests when applicable.

### E4. Tests

- Network disabled blocks outbound requests.
- Allowed host succeeds.
- Private address is blocked.
- DNS rebinding attempt is blocked.
- Policy decision is recorded in observer events.

## 9. Milestone F: Resource Governance

Goal: enforce hard resource boundaries for every execution path.

### F1. Runtime Limits

- CPU time limit.
- Memory limit.
- Disk quota.
- Process count limit.
- Open file limit.
- Added optional max file count per sandbox with `ZGI_SANDBOX_MAX_WORKSPACE_FILES`.
- Added optional max workspace bytes per sandbox with `ZGI_SANDBOX_MAX_WORKSPACE_BYTES`.
- Added operator-configurable max artifact manifest files per run with `ZGI_SANDBOX_MAX_ARTIFACT_MANIFEST_FILES`.
- Added operator-configurable max artifact manifest bytes per run with `ZGI_SANDBOX_MAX_ARTIFACT_MANIFEST_BYTES`.
- Max sandbox lifetime.

### F2. Queue Limits

- Added optional max concurrent executions per service with `ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS`.
- Added optional max concurrent executions per profile with `ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS_PER_PROFILE`.
- Added optional max concurrent executions per organization with `ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS_PER_ORGANIZATION`.
- Added optional max queued executions per organization with `ZGI_SANDBOX_MAX_QUEUED_EXECUTIONS_PER_ORGANIZATION`.
- Queue wait timeout.
- Cancellation propagation.
- Graceful shutdown drain behavior.

### F3. Policy Surface

- Expose effective limits in `/v1/policies`.
- Include effective limits in sandbox create responses.
- Include limit decisions in observer events.
- Add structured errors for every limit failure.

### F4. Tests

- CPU-bound timeout.
- Memory pressure rejection or termination.
- Disk quota enforcement.
- Process count enforcement.
- Queue timeout.
- Cancellation cleanup.

## 10. Milestone G: Strong Runtime Isolation

Goal: make a hardened runtime the production default while keeping preview mode
clearly separated for local development.

### G1. Backend Selection

- Add explicit backend modes:
  - `preview-process`
  - `linux-secure`
  - future remote worker mode
- Require production deployments to choose a non-preview backend.
- Fail startup when production mode uses preview execution.
- Surface backend mode in `/health` and observer events.

### G2. Linux Secure Runtime

- Validate rootfs at startup.
- Run as non-root.
- Use isolated namespaces.
- Enforce network policy below the HTTP layer.
- Bind only the sandbox workspace.
- Keep host filesystem read-only and minimal.
- Add platform guards for unsupported operating systems.

### G3. Future Worker Runtime

- Define a worker protocol before adding distributed execution.
- Keep lifecycle, policy, and observer contracts stable.
- Support worker registration, heartbeat, drain, and capacity reporting.
- Avoid leaking worker implementation details into public execution APIs.

### G4. Tests

- Linux integration tests for isolated execution.
- Network isolation tests.
- Filesystem isolation tests.
- Backend startup validation tests.
- Unsupported platform tests.

## 11. Milestone H: Observability and Audit

Goal: make execution behavior visible enough for debugging, operations, billing,
and security review.

### H1. Event Model

- Added cursor-style observer pagination on `GET /v1/observer/events` with a default page size, maximum page size, `has_more`, and `next_cursor`.
- Added observer event filters for organization, workspace, app, workflow run, user, and request scope.
- Added observer event retention with `ZGI_SANDBOX_OBSERVER_RETENTION_DAYS` and `ZGI_SANDBOX_OBSERVER_MAX_EVENTS`.
- Standardize observer event fields:
  - event ID
  - sandbox ID
  - organization ID
  - workspace ID
  - workflow run ID
  - skill ID
  - Added execution ID for code, command, template, and skill execution events
  - request ID
  - event type
  - status
  - duration
  - limit decisions
  - backend
- Add event retention policy.
- Add event pagination.

### H2. Metrics

- Added `GET /v1/metrics` with:
  - current worker active sandbox count
  - runner max, active, and queued execution gauges
  - runner backend and configured execution limits
  - observer-derived execution success, failure, timeout, cancellation, output truncation, backend error, and duration counters
- Export metrics for:
  - active sandboxes
  - queued executions
  - execution duration
  - timeout count
  - cancellation count
  - output truncation count
  - artifact bytes
  - egress decisions
  - backend errors
- Add labels carefully to avoid high-cardinality data.

### H3. Logs and Traces

- Add request correlation ID support.
- Propagate correlation IDs from `zgi-api`.
- Include sandbox ID and execution ID in logs.
- Do not log raw code, secrets, or large input payloads by default.

### H4. Tests

- Observer events are emitted for success and failure paths.
- Correlation ID appears in events.
- Sensitive data is not recorded.
- Metrics endpoint reports expected counters.

## 12. Milestone I: Multi-Organization Controls

Goal: bind sandbox usage to ZGI organizations, workspaces, apps, workflows, and users.

### I1. Ownership Model

- Add ownership fields to sandbox create requests:
  - organization ID
  - workspace ID
  - app ID
  - workflow run ID
  - user ID
- Validate ownership at API boundaries.
- Store ownership fields in sandbox records.
- Include ownership fields in observer events.
- Keep ownership fields optional for existing callers.
- Use short identifier-safe values only.
- Propagate ownership context to lifecycle, endpoint, expiration, and execution events.

### I2. Quotas

- Added optional max active sandboxes per organization with `ZGI_SANDBOX_MAX_ACTIVE_PER_ORGANIZATION`.
- Added optional max executions per minute per organization with `ZGI_SANDBOX_MAX_EXECUTIONS_PER_MINUTE_PER_ORGANIZATION`.
- [x] Max artifact bytes per organization.
- [x] Max workspace bytes per organization.
- Max network requests per organization.
- [x] Max active dependency profiles per organization.

### I3. Audit

- Record create, renew, execute, upload, download, delete, and policy-deny events.
- Keep raw file contents out of audit logs.
- File, archive, and artifact manifest observer events include ownership context.
- Include hashes for uploaded archives and artifacts.
- Added operator search by sandbox ID, workflow run ID, and request ID.

### I4. Tests

- Organization quota success and failure.
- [x] Cross-organization sandbox access rejection.
- [x] Audit event completeness.
- [x] Ownership metadata propagation.

## 13. Milestone J: API and Workflow Integration

Goal: make sandbox usage first-class inside ZGI runtime flows without coupling
business logic to sandbox internals.

### J1. API Adapter

- Keep sandbox calls behind a typed adapter in `zgi-api`.
- Add retries only for safe idempotent operations.
- Add clear timeout settings:
  - connect timeout
  - upload timeout
  - execution timeout
  - artifact download timeout
- Add structured sandbox errors mapped to API-level errors.

### J2. Workflow Runtime

- Allocate session sandboxes at workflow-run start when needed.
- Reuse a session sandbox across compatible workflow nodes.
- Cancel sandbox executions when the workflow run is canceled.
- Cleanup or archive sandbox output when the workflow run finishes.
- Correlate workflow logs with sandbox observer events.

### J3. Skill Runtime

- Validate skill package manifests before execution.
- Apply skill-specific artifact and timeout policies.
- Store skill execution traces.
- Return structured tool messages with artifacts.
- Add deterministic test fixtures for skill runs.

### J4. Tests

- API adapter unit tests.
- API skill-script E2E.
- Workflow run cancellation E2E.
- Artifact return E2E.
- Authenticated black-box flows when the test harness supports setup and mocks.

## 14. Milestone K: Operator Experience

Goal: make the service easy to deploy, diagnose, and operate in self-hosted and
managed environments.

### K1. Configuration

- Document all `ZGI_SANDBOX_` environment variables.
- Provide safe defaults for local development.
- Provide strict defaults for production examples.
- Add config validation at startup.
- Added startup logging for effective non-secret config.

### K2. Deployment

- Keep Docker Compose path working.
- Add hardened Linux deployment notes.
- Add Kubernetes deployment notes after worker/runtime model stabilizes.
- Add health and readiness probes.
- Add graceful shutdown behavior.

### K3. Diagnostics

- Add `/health` for liveness.
- Added `/ready` for dependency readiness.
- Add `/v1/policies` for effective policy.
- Add operator runbook for common failures:
  - dependency profile missing
  - sandbox startup failure
  - execution timeout
  - archive rejected
  - artifact too large
  - egress denied

### K4. Tests

- Startup config validation tests.
- Readiness tests.
- Shutdown drain tests.
- Docker Compose smoke test.

## 15. Milestone L: Test and Release Gates

Goal: make production readiness measurable.

### L1. Required Local Gates

- `cd sandbox && go test ./...`
- `make test-sandbox-kest`
- `make test-api-skill-script-e2e`
- `./scripts/check-open-source.sh --worktree`
- `git diff --check`

### L2. Required CI Gates

- Open-source hygiene.
- Web type check.
- Sandbox Go tests.
- API skill package tests.
- Sandbox Kest flows.
- Linux secure runtime tests where supported.

### L3. Release Checklist

- No public docs mention preview behavior as production isolation.
- Runtime backend mode is visible in `/health`.
- Network policy is enforced by the selected backend.
- Resource limits are enforced and tested.
- Organization quota is enforced and tested.
- Audit events exist for execution and file operations.
- Operator docs list all required environment variables.

## 16. Suggested PR Order

1. Short-code profile hardening and structured result contract.
2. Stateless short-code workspace cleanup and Kest coverage.
3. Artifact manifest and artifact limit enforcement.
4. Skill package manifest validation.
5. Network policy enforcement contract and preview backend rejection behavior.
6. Linux secure backend production startup guard.
7. Resource limit policy surface and structured errors.
8. Queue and cancellation model.
9. Observer event normalization and correlation IDs.
10. Organization ownership fields and quota checks.
11. Dependency profile records and version pinning.
12. Template runtime API.
13. Workflow session sandbox integration.
14. Operator readiness endpoint and deployment runbook.

Each PR should include:

- Narrow implementation scope.
- Focused unit tests.
- At least one failure-path test for security or limit behavior.
- Updated docs when public behavior changes.
- Local validation commands in the PR description.

## 17. Readiness Definition

`zgi-sandbox` can be called production-ready only when all of these are true:

- The default production backend is strongly isolated.
- Preview process execution is rejected in production mode.
- Network access is denied by default and enforced below the HTTP layer.
- CPU, memory, disk, process, file, timeout, and output limits are enforced.
- Organization ownership and quotas are enforced.
- Execution events are auditable and correlation-friendly.
- Skill package execution has manifest validation and artifact limits.
- Short-code execution is stateless by default.
- Operator deployment docs are complete.
- CI runs unit, E2E, hygiene, and black-box sandbox flows.
