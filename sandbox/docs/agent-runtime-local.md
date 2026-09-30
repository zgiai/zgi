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

`ZGI_AGENT_RUNTIME_CHECKPOINT_DIR` is an optional M5A development setting. When set, the Worker stores an immutable, SHA-256-verified workspace archive on that directory before suspending compute. The directory must be a private, durable Worker volume, separate from the execution workspace, and must remain mounted across Worker restarts. A lost local compute allocation or missing retained workspace is rebuilt from the last committed checkpoint. A queued or running Job prevents suspension until it becomes terminal; new Jobs cannot be admitted after the Binding enters `suspending`. The archive accepts only regular files and directories, with a 1 GiB file-content and 10,000-entry limit. An archive transfer or restore failure does not mark the Binding active or suspended as though it succeeded.

For a shared checkpoint store across Workers, configure `ZGI_AGENT_RUNTIME_CHECKPOINT_S3_BUCKET` and `ZGI_AGENT_RUNTIME_CHECKPOINT_S3_REGION`, and optionally `ZGI_AGENT_RUNTIME_CHECKPOINT_S3_ENDPOINT`, `ZGI_AGENT_RUNTIME_CHECKPOINT_S3_PREFIX`, and `ZGI_AGENT_RUNTIME_CHECKPOINT_S3_PATH_STYLE=true`. This uses the standard AWS credential chain (for example, a Worker role or environment-provided credentials). The bucket must be private, durable, and dedicated to runtime checkpoints; give the Worker least-privilege object read/write/delete access. A remote custom endpoint must use HTTPS; loopback HTTP is accepted only for local tests. The S3 bucket and local checkpoint directory are mutually exclusive. Checkpoint writes are immutable and verified by SHA-256 when read. This provides cross-Worker archive access but does **not** by itself implement seven-day retention or a Home Snapshot.

The setting is **off by default**. A single Worker volume is not an object store or a cross-node backup: losing that volume loses its checkpoints. Do not route an existing checkpointed Binding to a Worker lacking the same volume, and do not remove the configured directory before deleting those Bindings. Seven-day retention, node migration, background checkpoint garbage collection, and Home Snapshots are not provided by M5A. Do not advertise those guarantees until the later M5 gates pass.

Keep `ZGI_AGENT_RUNTIME_ROOT` under a service-data parent such as `/var/lib`, not under `/root` (normally mode `0700`). The Runtime Agent runs as UID/GID 65532, and the runtime uses host-side bind mounts and a control socket. A native gVisor smoke test timed out when the root was under `/root`, even with a 45-second startup limit, while the same test passed with a root under `/var/lib`. This identifies a deployment-path constraint on the tested node; the exact failing access operation was not exposed by the Worker API. Do not make `/root` world-traversable to work around this; move the Worker root instead. The startup limit is a deadline, not a fix for inaccessible paths.

The process owns bounded, cancelable loops for asynchronous Jobs, cleanup reconciliation, and idle Binding suspension. Job submission and file transfer atomically refresh the Binding idle deadline and generation; a stale reaper observation therefore cannot suspend a Binding after new activity.

## Lifecycle semantics

- Create prepares a retained workspace and starts a dedicated gVisor task.
- Suspend stops and deletes compute plus connection secrets, while retaining the workspace.
- Resume starts fresh compute around the retained workspace and issues a new connection token.
- With `ZGI_AGENT_RUNTIME_CHECKPOINT_DIR` enabled, suspend first publishes a durable archive. If retained compute/workspace is missing on resume, the Worker creates a new environment and restores the committed archive before marking the Binding active.
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
