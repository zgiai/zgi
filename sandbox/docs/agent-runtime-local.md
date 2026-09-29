# Local Agent Runtime

The local Agent execution backend is an opt-in Linux worker capability. It is separate from the existing one-shot Sandbox API and from Runtime Workspace credentials.

## Security boundary

Each active Binding runs in a dedicated containerd task using the `io.containerd.runsc.v1` runtime. The worker rejects any other runtime name. It also requires an approved image reference pinned by SHA-256 digest, creates a new network namespace without attaching a network, uses a read-only root filesystem, drops Linux capabilities, enables `no_new_privs`, and applies memory, PID, and CPU limits.

The workspace and Runtime Agent socket are the only writable host bind mounts. `/tmp` is a bounded tmpfs. The Runtime Agent listens on a mode `0600` Unix socket and requires a randomly generated bearer token. Tokens are stored in the worker's local secret directory, never in Agent execution database records. The worker root must therefore be owned and writable only by the worker service account.

The Runtime Agent is not an isolation boundary by itself. Do not run the local provider with runc as a substitute for gVisor, and do not expose the Runtime Agent socket or token to API clients.

## Image contract

Build the development image with:

```bash
make -C sandbox build-agent-runtime-image
```

Push it to the deployment registry, resolve the resulting immutable digest, preload that digest on the worker node, and put only that digest-qualified reference in the approved Profile map. The worker does not pull images while creating a Binding.

The image must contain `/usr/local/bin/zgi-runtime-agent` and the commands promised by its Profile. `Dockerfile.agent-runtime` provides the Runtime Agent, Python 3.13, and Node.js 22 for the initial standard Profile. Adding packages produces a new image digest; it must not mutate an existing approved Profile in place.

## Worker process

Build the independent Linux worker with:

```bash
make -C sandbox build-agent-runtime-worker
```

Run it as a root-owned Linux service account that can access containerd and the worker root. The worker hosts only the authenticated `/v1/agent-runtime/` API; it does not register those routes in the existing Sandbox server.

Required environment:

| Variable | Purpose |
|---|---|
| `ZGI_AGENT_RUNTIME_API_KEY` | Dedicated internal bearer credential. Do not reuse `ZGI_SANDBOX_API_KEY` or a Runtime Workspace credential. |
| `ZGI_AGENT_RUNTIME_DATABASE_URL` | PostgreSQL control-plane database. |
| `ZGI_AGENT_RUNTIME_PROFILES_JSON` | Approved Profile object, for example `{"standard":{"image":"registry.example/zgi/agent-runtime@sha256:..."}}`. |

Important optional settings are `ZGI_AGENT_RUNTIME_LISTEN` (default `127.0.0.1:2670`), `ZGI_AGENT_RUNTIME_WORKER_ID`, `ZGI_AGENT_RUNTIME_ROOT` (default `/var/lib/zgi-agent-runtime`), `ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS`, `ZGI_AGENT_RUNTIME_MAX_WORKERS`, `ZGI_AGENT_RUNTIME_IDLE_TTL_SECONDS` (default 1800), `ZGI_AGENT_RUNTIME_STARTUP_TIMEOUT_SECONDS` (default 30), and the `MEMORY_BYTES`, `PIDS`, `CPU_PERIOD`, and `CPU_QUOTA` variables with the `ZGI_AGENT_RUNTIME_` prefix.

Keep `ZGI_AGENT_RUNTIME_ROOT` under a service-data parent such as `/var/lib`, not under `/root` (normally mode `0700`). The Runtime Agent runs as UID/GID 65532, and the runtime uses host-side bind mounts and a control socket. A native gVisor smoke test timed out when the root was under `/root`, even with a 45-second startup limit, while the same test passed with a root under `/var/lib`. This identifies a deployment-path constraint on the tested node; the exact failing access operation was not exposed by the Worker API. Do not make `/root` world-traversable to work around this; move the Worker root instead. The startup limit is a deadline, not a fix for inaccessible paths.

The process owns bounded, cancelable loops for asynchronous Jobs, cleanup reconciliation, and idle Binding suspension. Job submission and file transfer atomically refresh the Binding idle deadline and generation; a stale reaper observation therefore cannot suspend a Binding after new activity.

## Lifecycle semantics

- Create prepares a retained workspace and starts a dedicated gVisor task.
- Suspend stops and deletes compute plus connection secrets, while retaining the workspace.
- Resume starts fresh compute around the retained workspace and issues a new connection token.
- Destroy stops compute before deleting workspace, control, and secret directories.
- Repeated lifecycle calls are safe. Partial creation returns opaque references so the control-plane cleanup queue can retry deletion.

No stage is considered to have passed the real local-backend gate until the common contract suite runs on a Linux node with containerd and runsc installed and verifies command execution, PTY, file transfer, cancellation, process-tree cleanup, disabled networking, and resource enforcement.

On that node, preload the digest-qualified image and run:

```bash
sudo ZGI_AGENT_RUNTIME_TEST_IMAGE='registry.example/zgi/agent-runtime@sha256:...' \
  make -C sandbox test-agent-runtime-containerd
```

Set `ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS` only when containerd does not use `/run/containerd/containerd.sock`. The test intentionally fails instead of falling back to runc when runsc, the image, root privileges, or the containerd socket is unavailable.

Run this gate on a native Linux worker or Linux VM. A privileged container on Docker Desktop can build and preload the image and is useful for exercising bounded failure and cleanup paths, but nested containerd/gVisor task creation is not a supported deployment topology and does not satisfy the isolation gate. In particular, a nested shim can complete `runsc create` without completing the containerd task-create handshake. The Provider bounds the complete launch path with `ZGI_AGENT_RUNTIME_STARTUP_TIMEOUT_SECONDS`, but the node operator must still reconcile any runtime process that its containerd installation cannot reap.
