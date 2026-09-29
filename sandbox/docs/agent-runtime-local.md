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
