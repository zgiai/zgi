# Agent Runtime M6A Deployment Gate

This document is the deployment and acceptance checklist for the first M6A slice. It makes the network contract explicit without enabling outbound networking yet.

## What this slice enables

M6A adds an immutable network policy to each Agent Runtime Binding. The policy is copied from the approved console Agent configuration into the API request, stored in PostgreSQL, and passed to the selected Worker backend.

The only accepted policy in this slice is `offline`:

- `offline` maps to `network_mode=disabled`.
- An omitted policy is normalized to `offline`, preserving old clients.
- Any other policy is rejected as `backend_unavailable` and no backend allocation is started.
- Local gVisor and E2B both fail closed if a non-disabled mode reaches the backend.

This is a deployment gate, not an outbound-network feature. Do not publish `controlled-http`, `public-web`, or any other policy as usable until the M6A egress gateway and its deny tests are implemented.

## Required configuration

The API and Worker must continue to use separate credentials and separate endpoints. The API uses:

```text
ZGI_AGENT_RUNTIME_URL
ZGI_AGENT_RUNTIME_API_KEY
ZGI_AGENT_RUNTIME_APPROVED_PROFILES_JSON
```

The approved profile map must contain an explicit `offline` policy, for example:

```json
{
  "standard": {
    "backend": "local-gvisor",
    "organization_ids": ["<organization UUID>"],
    "policy_ids": ["offline"]
  }
}
```

The Linux Worker additionally requires the existing backend settings:

```text
ZGI_AGENT_RUNTIME_API_KEY
ZGI_AGENT_RUNTIME_DATABASE_URL
ZGI_AGENT_RUNTIME_BACKEND=local-gvisor   # or e2b
ZGI_AGENT_RUNTIME_PROFILES_JSON         # local-gvisor
ZGI_AGENT_RUNTIME_E2B_API_KEY           # e2b only
ZGI_AGENT_RUNTIME_E2B_PROFILES_JSON     # e2b only
```

Keep the Worker listener private. Put it on loopback or an internal network and allow only the API process to reach it. Do not reuse Runtime Workspace credentials.

## Pre-deployment checks

Run these from the repository root on Go 1.26.2 Linux or an equivalent CI image:

```bash
cd sandbox
go test ./internal/agentexecution ./internal/storage ./internal/runtimeagent ./internal/localruntime/... ./internal/hostedruntime/e2b ./cmd/agentworker
go vet ./internal/agentexecution ./internal/storage ./internal/runtimeagent ./internal/localruntime/... ./internal/hostedruntime/e2b ./cmd/agentworker
```

Run the API package tests with the normal CI Connector test-key environment:

```bash
cd api
go test ./internal/modules/app/agents -count=1
go vet ./internal/modules/app/agents ./internal/dto
```

The existing E2B contract test remains opt-in and must use a disposable approved template and a real account. Do not replace it with a mock when accepting a deployment:

```bash
cd sandbox
go test -tags=e2b_integration -count=1 -v ./internal/hostedruntime/e2b -run TestE2BContract
```

## HTTP acceptance

With a Worker running against an isolated PostgreSQL database, call the dedicated Agent Runtime API with the Worker API key and API-injected ownership headers.

1. Create a binding without `network_policy`. Expect HTTP 201 and response fields `network_policy=offline` and `network_mode=disabled`.
2. Create a binding with `network_policy=offline`. Expect HTTP 201 (or the idempotent existing Binding) with the same fields.
3. Create a binding with `network_policy=controlled-http` or another non-offline value. Expect HTTP 503 with error code `backend_unavailable`. Confirm that no local containerd task/container or E2B Sandbox was created.
4. Run the existing offline Shell, upload/download, suspend/resume, and cleanup contract. Confirm the Binding keeps the persisted policy and mode after resume.
5. Repeat the negative policy request after a Worker restart. It must remain rejected; restarting must not turn an unsupported policy into an allowed one.

For local gVisor, additionally verify the task has no network namespace connectivity. For E2B, verify the provider request still carries its existing forced-deny settings. These checks are evidence for the offline gate only.

## Rollback

Rollback is configuration-first:

1. Stop publishing or enabling new execution-runtime configurations.
2. Keep the Worker endpoint available long enough to download retained authorized files and run cleanup.
3. Disable new Binding creation at the API rollout layer if required. Do not delete the PostgreSQL columns, retained workspace records, checkpoint objects, or user files.
4. Existing legacy Script Skill execution remains on its original Sandbox path.

The migration is additive and has defaults, so an older Worker can be upgraded only after its binary is able to read the new columns. Do not run a mixed-version deployment where an older Worker writes an incomplete Binding contract.

## Not accepted by M6A

This gate does not yet provide:

- allowlisted outbound HTTPS;
- DNS, redirect, private-CIDR, IPv6, raw-TCP, or proxy-bypass enforcement in Agent Runtime;
- organization-level policy revocation during a running Job;
- model-directed third-party dependency installation;
- unrestricted shell, network, or external SSH access.

Those are separate M6A/M6B deliverables and require a real data-plane gateway plus local and hosted deny/allow evidence before exposure.
