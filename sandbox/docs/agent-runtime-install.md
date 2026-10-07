# Self-hosted Agent Runtime installation

The self-hosted Worker is installed by scripts/zgi-runtime-server. The user
only needs a supported Linux host, an immutable Profile image, and the control
plane credentials. The script owns the low-level containerd and gVisor setup;
users do not need to edit containerd runtime handlers by hand.

## Host requirements

- Ubuntu 22.04/24.04 or another systemd Linux distribution with cgroup v2.
- x86_64 for the published Profile image.
- At least 4 GiB RAM and 30 GiB free disk for cc-codex-standard; 8 GiB RAM is
  recommended for office and media workloads.
- root (or sudo) during installation and outbound HTTPS to the control plane and
  the operator package sources.
- Docker is not required. /dev/kvm is not required when the runsc Profile uses
  the systrap platform.

The installer fails closed if runsc or containerd-shim-runsc-v1 is missing.
It never changes the Worker to runc as a fallback.

## Offline image import

The image must be digest-qualified. This also supports hosts that cannot access
Docker Hub or another registry. On a build machine export an OCI/Docker archive:

    docker save zgi-agent-runtime:cc-codex-standard | zstd -T0 -o zgi-agent-runtime.tar.zst
    scp zgi-agent-runtime.tar.zst root@worker:/tmp/

The installer imports the archive into the configured containerd namespace and
checks that the requested digest is present before enabling the Worker.

## Install

To validate only the host runtime before the control-plane credentials are
available, use the host-only command. It imports the image, registers runsc,
restarts containerd, and runs a disposable gVisor command without starting a
Worker:

    sudo env \
      ZGI_AGENT_RUNTIME_IMAGE_REF='registry.example/zgi/agent-runtime@sha256:...' \
      ZGI_AGENT_RUNTIME_IMAGE_ARCHIVE=/tmp/zgi-agent-runtime.tar.zst \
      ./scripts/zgi-runtime-server host-install

Build or obtain the Linux Worker binary, then run:

    sudo env \
      ZGI_AGENT_RUNTIME_IMAGE_REF='registry.example/zgi/agent-runtime@sha256:...' \
      ZGI_AGENT_RUNTIME_IMAGE_ARCHIVE=/tmp/zgi-agent-runtime.tar.zst \
      ZGI_AGENT_RUNTIME_WORKER_SOURCE=./bin/zgi-agent-runtime-worker \
      ZGI_AGENT_RUNTIME_API_KEY='operator-issued-key' \
      ZGI_AGENT_RUNTIME_DATABASE_URL='postgres://...' \
      ./scripts/zgi-runtime-server install

If ZGI_AGENT_RUNTIME_PROFILES_JSON is omitted, the installer creates the
operator-owned cc-codex-standard Profile with package-only egress to PyPI, npm
and Debian mirrors. Profile, policy, snapshot and network choices are not
exposed to end users.

The install is idempotent. It backs up the containerd configuration, creates a
root-owned /var/lib/zgi-agent-runtime, writes the Worker environment with mode
0600, installs a systemd unit, restarts containerd, and runs a disposable
io.containerd.runsc.v1 smoke command.

## Diagnostics and rollback

    sudo ./scripts/zgi-runtime-server preflight
    sudo ./scripts/zgi-runtime-server doctor
    sudo ./scripts/zgi-runtime-server smoke
    sudo ./scripts/zgi-runtime-server rollback

doctor checks the runtime handler, runsc config, image and Worker inputs.
rollback stops the Worker and restores the newest installer backup; it does
not remove user workspaces or checkpoint data.

## Operational contract

The runtime handler is registered as io.containerd.runsc.v1. Standard Skills
run in the approved cc-codex-standard image. Jobs remain UID/GID 65532; only
the Runtime Agent has the narrow root capability needed by the guarded APT
facade. Ordinary outbound networking remains disabled, and package installation
uses only the fixed package-source allowlist.
