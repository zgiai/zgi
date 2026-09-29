# E2B Agent Runtime

The managed Agent execution backend is an opt-in mode of the independent Agent Runtime worker. It does not add routes to the legacy Sandbox service and it does not grant Shell access to Runtime Workspace credentials.

## Fixed provider contract

The adapter uses E2B's authenticated REST interfaces directly instead of a third-party Go SDK:

- `POST /v2/sandboxes` creates an environment from an approved template.
- `POST /v2/sandboxes/{id}/connect` connects or resumes it and returns fresh connection tokens.
- `POST /sandboxes/{id}/pause` retains a full-memory snapshot.
- `DELETE /sandboxes/{id}` destroys it idempotently.
- envd `GET /envs` uses the connection-scoped `X-Access-Token` only to recover the Runtime Agent bootstrap token after a worker restart.

The E2B API key remains in Worker memory. Database allocations contain only the opaque Sandbox ID. envd and traffic tokens are accepted only from the current create/connect response and are never written to Agent execution records.

Every create request sets `allow_internet_access=false`, denies `0.0.0.0/0`, disables public unauthenticated traffic, enables full-memory auto-pause at the Worker timeout, and disables traffic-triggered auto-resume. The ZGI Runtime Agent endpoint therefore requires both E2B's traffic token and its own independently generated bearer token.

## Approved template contract

Build the Runtime Agent image for TCP transport with:

```bash
docker build \
  --build-arg RUNTIME_AGENT_LISTEN=tcp://0.0.0.0:49984 \
  -f sandbox/Dockerfile.agent-runtime \
  -t registry.example/zgi/agent-runtime-e2b:<immutable-version> \
  sandbox
```

Publish an immutable image, create an E2B template from that exact image, and approve the resulting template ID in Worker configuration. The template must preserve the image entrypoint, non-root UID/GID 65532, `/workspace`, Python 3.13, Node.js 22, and port 49984. It must not bake provider credentials or a Runtime Agent token into an image layer. E2B injects the per-Binding token and fixed listener/network declarations at create time.

Runtime Agent control variables are stripped from every child Job environment, including when a Job tries to override them. Static Profile approval is the only template selection path; arbitrary template IDs are rejected before contacting E2B.

## Worker configuration

Set the shared Worker variables documented in `agent-runtime-local.md`, then select the hosted backend:

| Variable | Purpose |
|---|---|
| `ZGI_AGENT_RUNTIME_BACKEND=e2b` | Selects only the E2B backend for this Worker instance. |
| `ZGI_AGENT_RUNTIME_E2B_API_KEY` | E2B control-plane credential. Never reuse the Worker API credential. |
| `ZGI_AGENT_RUNTIME_E2B_PROFILES_JSON` | Approved map, for example `{"standard":{"template_id":"zgi-runtime-template"}}`. |
| `ZGI_AGENT_RUNTIME_E2B_API_URL` | Optional control-plane URL; defaults to `https://api.e2b.app`. |
| `ZGI_AGENT_RUNTIME_E2B_DOMAIN` | Optional Sandbox domain; defaults to `e2b.app`. |
| `ZGI_AGENT_RUNTIME_E2B_TIMEOUT_SECONDS` | Create/connect TTL and auto-pause deadline; defaults to 1800. |
| `ZGI_AGENT_RUNTIME_E2B_AGENT_PORT` | Runtime Agent port; defaults to 49984. |
| `ZGI_AGENT_RUNTIME_E2B_ENVD_PORT` | envd port; defaults to 49983. |

Run local-gVisor and E2B workers as separate service instances with distinct Worker IDs. Both may share the control-plane database, but a Binding records one backend and cannot migrate between providers implicitly.

## Real-account gate

Unit tests use an HTTP contract server and do not count as managed-runtime evidence. With an approved template and disposable E2B project, run:

```bash
ZGI_AGENT_RUNTIME_E2B_API_KEY='...' \
ZGI_AGENT_RUNTIME_E2B_TEST_TEMPLATE='...' \
make -C sandbox test-agent-runtime-e2b
```

The tagged test intentionally fails when credentials or the template are absent. It creates and cleans a real environment, verifies stdout/stderr, file upload/download, pause/resume retention, outbound-network denial, cancellation, and final deletion. Do not mark M3 complete until this test has passed and the E2B dashboard confirms no orphaned environment.

