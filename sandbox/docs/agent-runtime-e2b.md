# E2B Agent Runtime

The managed Agent execution backend is an opt-in mode of the independent Agent Runtime worker. It does not add routes to the legacy Sandbox service and it does not grant Shell access to Runtime Workspace credentials.

## Fixed provider contract

The adapter uses E2B's authenticated REST interfaces directly instead of a third-party Go SDK:

- `POST /v2/sandboxes` creates an environment from an approved template.
- `POST /v2/sandboxes/{id}/connect` connects or resumes it and returns fresh connection tokens.
- `POST /sandboxes/{id}/pause` retains a full-memory snapshot.
- `DELETE /sandboxes/{id}` destroys it idempotently.
- envd `POST /files` uses the create-scoped `X-Access-Token` to deliver a per-Binding Runtime Agent token through a fixed file outside the workspace.
- envd `GET /envs` uses the connection-scoped `X-Access-Token` only to recover that token after a worker restart.

The E2B API key remains in Worker memory. Database allocations contain only the opaque Sandbox ID. envd and traffic tokens are accepted only from the current create/connect response and are never written to Agent execution records.

Every create request sets `allow_internet_access=false`, denies `0.0.0.0/0`, disables public unauthenticated traffic, enables full-memory auto-pause at the Worker timeout, and disables traffic-triggered auto-resume. The ZGI Runtime Agent endpoint therefore requires both E2B's traffic token and its own independently generated bearer token.

## Approved template contract

E2B Build System 2.0 accepts a single-stage Dockerfile. Prepare the Linux
Runtime Agent binary, verify the local image contract, then create the template
with the pinned CLI version:

```bash
make -C sandbox prepare-agent-runtime-e2b
docker build \
  -f sandbox/Dockerfile.agent-runtime-e2b \
  -t zgi-agent-runtime:e2b-template-contract \
  sandbox
npx --yes @e2b/cli@2.20.0 template create <approved-template-name> \
  --path sandbox \
  --dockerfile Dockerfile.agent-runtime-e2b \
  --cmd /usr/local/bin/zgi-runtime-agent-bootstrap \
  --ready-cmd 'test -x /usr/local/bin/zgi-runtime-agent && test -x /usr/local/bin/zgi-runtime-agent-bootstrap'
```

The generated binary is placed under ignored `sandbox/e2b-template-build/` build storage and
must not be committed. The template Dockerfile downloads the fixed Node.js
22.23.3 archive and verifies its pinned SHA-256 before installation. Python
3.13 comes from the digest-resolved base image at build time; production build
automation should additionally pin that base image by digest. Keep the generated
Runtime Agent as a file-level `COPY`: current CLI upload caching can skip an
unchanged directory input without materializing its contents in a later build,
while the explicit file input remains reproducible across cached rebuilds.

Approve the resulting immutable template ID in Worker configuration. The template must preserve the image entrypoint, non-root UID/GID 65532, `/workspace`, Python 3.13, Node.js 22, and port 49984. It must not bake provider credentials or a Runtime Agent token into an image layer.

The template start process waits for `/run/zgi-agent/bootstrap-token`. After E2B creates the microVM, the Provider uploads a random per-Binding token through authenticated envd. Runtime Agent accepts only a regular token file, restricts it to mode `0600`, reads it, and removes it before opening port 49984. This handshake is necessary because an E2B template start process is restored from the template snapshot and does not inherit per-Sandbox environment variables supplied later at create time.

Runtime Agent control variables are stripped from every child Job environment, including when a Job tries to override them. Static Profile approval is the only template selection path; arbitrary template IDs are rejected before contacting E2B. Readiness and reconnect fail closed if the injected token cannot be delivered or recovered through authenticated envd.

## Worker configuration

Set the shared Worker variables documented in `agent-runtime-local.md`, then select the hosted backend:

| Variable | Purpose |
|---|---|
| `ZGI_AGENT_RUNTIME_BACKEND=e2b` | Selects only the E2B backend for this Worker instance. |
| `ZGI_AGENT_RUNTIME_E2B_API_KEY` | E2B control-plane credential. Never reuse the Worker API credential. |
| `ZGI_AGENT_RUNTIME_E2B_PROFILES_JSON` | Approved map, for example `{"standard":{"template_id":"zgi-runtime-template","dependency_preparation":false}}`. Set `dependency_preparation=true` only on a test template whose API approval also enables the same capability. |
| `ZGI_AGENT_RUNTIME_E2B_API_URL` | Optional control-plane URL; defaults to `https://api.e2b.app`. |
| `ZGI_AGENT_RUNTIME_E2B_DOMAIN` | Optional Sandbox domain; defaults to `e2b.app`. |
| `ZGI_AGENT_RUNTIME_E2B_TIMEOUT_SECONDS` | Create/connect TTL and auto-pause deadline; defaults to 1800. |
| `ZGI_AGENT_RUNTIME_E2B_AGENT_PORT` | Runtime Agent port; defaults to 49984. |
| `ZGI_AGENT_RUNTIME_E2B_ENVD_PORT` | envd port; defaults to 49983. |

Run local-gVisor and E2B workers as separate service instances with distinct Worker IDs. Both may share the control-plane database, but a Binding records one backend and cannot migrate between providers implicitly.

The optional checkpoint settings described in `agent-runtime-local.md` also apply to E2B Workers. If a paused E2B Sandbox has disappeared, the Worker can create a replacement from the same approved Profile and restore the last committed workspace archive before activating the Binding. The approved E2B template must contain the Runtime Agent archive endpoint from this version; an older template fails closed on checkpoint transfer. The local checkpoint directory is a persistent Worker volume; the alternative S3-compatible store permits cross-Worker archive access. Neither mode alone guarantees M5 seven-day retention.

The Worker records a delayed allocation-reconciliation task before creating an E2B Sandbox. An opaque Sandbox ID lost in a Worker crash can be rediscovered through the per-attempt metadata on running or paused Sandboxes; the reconciler deletes unrecorded allocations while preserving the one referenced by the current Binding. The task runs after a two-hour grace period, beyond the 90-minute maximum create/restore operation. This bounds orphan cleanup but is not a substitute for the still-required real-provider crash-injection acceptance.

An approved [Home Snapshot](agent-runtime-home-snapshot.md) may select a separate, versioned E2B template for new Bindings. The Worker still applies offline network restrictions and requires the Runtime Agent to become healthy before activation.

The archive endpoint appends a version marker and SHA-256 digest to its response body. The Worker removes and verifies that footer before saving the TAR. HTTP trailers are not used for completion because the E2B traffic proxy does not preserve them. Rebuild both the Worker and approved template together when changing this internal archive protocol; a mismatched version fails closed.

## Real-account gate

Unit tests use an HTTP contract server and do not count as managed-runtime evidence. With an approved template and disposable E2B project, run:

```bash
ZGI_AGENT_RUNTIME_E2B_API_KEY='...' \
ZGI_AGENT_RUNTIME_E2B_TEST_TEMPLATE='...' \
make -C sandbox test-agent-runtime-e2b
```

The tagged test intentionally fails when credentials or the template are absent. It creates and cleans a real environment, verifies stdout/stderr, file upload/download, pause/resume retention, outbound-network denial, cancellation, and final deletion. Do not mark M3 complete until this test has passed and the E2B dashboard confirms no orphaned environment.
