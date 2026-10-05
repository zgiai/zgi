# Agent Runtime M6A Deployment Gate

This document is the deployment and acceptance checklist for the first M6A slice. The local gVisor backend now supports a narrowly scoped HTTPS egress bridge; hosted E2B remains fail-closed until it exposes an equivalent allowlist gateway.

## What this slice enables

M6A adds an immutable network policy to each Agent Runtime Binding. The policy is copied from the approved console Agent configuration into the API request, stored in PostgreSQL, and passed to the selected Worker backend.

The accepted policies are:

- `offline` maps to `network_mode=disabled`.
- An omitted policy is normalized to `offline`, preserving old clients.
- `controlled-http` maps to `network_mode=controlled-http` and is available only to a local gVisor Profile with a non-empty `egress_allowed_hosts` list.
- Hosted E2B still rejects `controlled-http` because its current adapter cannot enforce the same domain allowlist.
- Unknown policies are rejected as `backend_unavailable` and no backend allocation is started.

The local bridge keeps the gVisor network namespace disconnected. Jobs receive only a loopback HTTP proxy; the Worker dials HTTPS port 443 after exact or single-label-wildcard host, DNS, private-CIDR and special-address checks. HTTP, raw TCP, non-443 ports and proxy bypass are not supported.

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
  },
  "controlled-standard": {
    "backend": "local-gvisor",
    "organization_ids": ["<organization UUID>"],
    "policy_ids": ["controlled-http"],
    "dependency_preparation": true,
    "dependency_preparation_mode": "controlled-http"
  }
}
```

The corresponding local Worker profile for `controlled-standard` must carry the allowlist:

```json
{
  "controlled-standard": {
    "image": "registry.example/zgi-agent-runtime@sha256:<digest>",
    "dependency_preparation": true,
    "dependency_preparation_mode": "controlled-http",
    "egress_allowed_hosts": ["pypi.org", "*.pythonhosted.org", "registry.npmjs.org"]
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
3. Create a local binding with `network_policy=controlled-http` and an approved allowlist. Expect HTTP 201 with `network_mode=controlled-http`; verify that a job can reach an allowlisted HTTPS endpoint through the loopback proxy and that a non-allowlisted host, private destination, non-443 port and HTTP URL are denied. Create the same binding against E2B and expect `backend_unavailable` until an equivalent hosted gateway is available.
4. Run the existing offline Shell, upload/download, suspend/resume, and cleanup contract. Confirm the Binding keeps the persisted policy and mode after resume.
5. Repeat the negative policy request after a Worker restart. It must remain rejected; restarting must not turn an unsupported policy into an allowed one.

For local gVisor, additionally verify the task has no network namespace connectivity and that only the host-visible bridge socket exists. For E2B, verify the provider request still carries its existing forced-deny settings.

## Rollback

Rollback is configuration-first:

1. Stop publishing or enabling new execution-runtime configurations.
2. Keep the Worker endpoint available long enough to download retained authorized files and run cleanup.
3. Disable new Binding creation at the API rollout layer if required. Do not delete the PostgreSQL columns, retained workspace records, checkpoint objects, or user files.
4. Existing legacy Script Skill execution remains on its original Sandbox path.

The migration is additive and has defaults, so an older Worker can be upgraded only after its binary is able to read the new columns. Do not run a mixed-version deployment where an older Worker writes an incomplete Binding contract.

## Not accepted by M6A

This slice does not yet provide:

- hosted/E2B allowlisted outbound HTTPS;
- redirect policy beyond the package-manager client (the proxy only permits HTTPS CONNECT and re-checks every target);
- organization-level policy revocation during a running Job;
- arbitrary online installation or system dependency builds;
- unrestricted shell, network, or external SSH access.

Those are separate M6A/M6B deliverables and require a real data-plane gateway plus local and hosted deny/allow evidence before exposure.
