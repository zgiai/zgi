# zgi-sandbox Engineering Standards

## 1. Purpose

This document defines the engineering bar for `zgi-sandbox` as an open-source ZGI project.

It exists for two reasons:

- Keep the public codebase clearly ZGI-native in structure, naming, and product voice
- Hold every implementation to a production-quality standard instead of a demo-quality standard

## 2. Open-Source Identity

The project must read like an original ZGI service.

That means:

- Public documentation should describe ZGI decisions, not vendor comparisons
- Module names, runtime names, config prefixes, and API language should stay ZGI-native
- Comments should explain local intent and constraints, not reference another project as the source of truth
- UI copy should sound like one product, not a stitched blend of external patterns

Avoid:

- Vendor-prefixed runtime names
- Public comments such as "same as X" or "copied from Y"
- File layouts that mirror another repository without adaptation
- Compatibility layers leaking into long-term module naming

## 3. Code Quality Bar

Every module should meet the following standards before it is treated as complete:

- Clear ownership of responsibilities
- Explicit error handling
- Test coverage for successful and failing paths
- Configuration-driven behavior for limits and security-sensitive settings
- No hidden global mutation unless there is a strong runtime reason
- Predictable request and response contracts
- Platform guards for operating-system-specific behavior

For sandbox code specifically:

- Security posture must be enforced by runtime boundaries, not only by request flags
- Unsafe preview behavior must be isolated behind explicit configuration
- Resource limits must be enforced in code paths, not only documented in policy snapshots
- Observability must be queryable and correlation-friendly

## 4. Architecture Rules

The service should keep a clean separation between:

- API-facing modules
- Runtime execution backends
- Policy decisions
- Lifecycle state management
- Observability and audit records

Required direction:

- `compat` exposes the narrow compatibility contract
- `lifecycle` owns sandbox resources and TTL
- `exec` owns execution and filesystem operations
- `policy` owns validation and runtime envelopes
- `observer` owns event capture and audit access

Avoid:

- Business logic embedded directly in HTTP handlers
- Runtime decisions scattered across handlers and UI code
- One package owning lifecycle, execution, policy, and formatting at the same time

## 5. Security Rules

`zgi-sandbox` is not allowed to claim production readiness unless these conditions are true:

- Untrusted code does not execute as a normal host process without isolation
- Network controls are enforced below the HTTP layer
- File access is scoped to the sandbox root and validated on every operation
- Authentication is mandatory outside explicit local-development mode
- Sensitive endpoints return structured errors without leaking host details

Until those conditions are met:

- Preview code must be labeled as preview
- Unsafe runtime paths must stay clearly separated from hardened runtime paths
- Public docs must not imply full production isolation

## 6. Testing Rules

The minimum quality gates are:

- `gofmt` clean
- `go test ./...` clean
- `go build ./...` clean

Production-ready sandbox paths also require:

- Linux integration tests for the real isolated runtime
- API-level tests for auth, quota, lifecycle, and error paths
- Regression tests for file path escaping and network policy failures
- End-to-end operator-console verification for key user flows

## 7. Review Rules

Every substantial change should be reviewed against these questions:

1. Does this change strengthen the ZGI product identity?
2. Does this change improve security, clarity, or maintainability?
3. Is the module boundary cleaner after the change?
4. Is the behavior tested in both success and failure paths?
5. Would an open-source contributor understand why this code exists?

If the answer to any of those is "no", the change is not finished.

## 8. Delivery Standard

The project should ship features in this order:

- Correct
- Safe
- Observable
- Extensible

Never in this order:

- Impressive demo first
- Security later
- Refactor later
- Tests later

That tradeoff is what separates a public production-grade sandbox from a preview implementation.
