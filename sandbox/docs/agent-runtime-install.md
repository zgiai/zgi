# Self-hosted Agent Runtime installation

The self-hosted Worker is installed by scripts/zgi-runtime-server. The user
only needs a supported Linux host, an immutable Profile image, and the control
plane credentials. Install containerd, ctr, runsc and containerd-shim-runsc-v1
before invoking the script. The Worker selects the gVisor shim directly; users
do not need to edit CRI runtime handlers by hand. The installer does not download
host runtimes, change shared containerd configuration or restart containerd.

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
available, use the host-only command. It imports/verifies the image and runs a
disposable gVisor command without starting a Worker or changing containerd:

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

The install is idempotent. It preserves the previous Worker inputs, creates a
root-owned /var/lib/zgi-ar, writes the Worker environment with mode 0600,
installs a systemd unit and verifies database-backed Worker health. It does not
back up or modify shared containerd configuration. See
[setup and use](../../docs/agent-runtime-quickstart.md) for private settings-file
examples that avoid putting credentials in command arguments.

## Diagnostics and rollback

    sudo ./scripts/zgi-runtime-server preflight
    sudo ./scripts/zgi-runtime-server doctor
    sudo ./scripts/zgi-runtime-server smoke
    sudo ./scripts/zgi-runtime-server rollback

doctor checks the pinned image, private environment, service PID/listener and
database-backed health; it is not a full script-execution test.
rollback stops the Worker and restores the newest installer backup; it does
not remove user workspaces or checkpoint data.

## Operational contract

The runtime handler is registered as io.containerd.runsc.v1. Standard Skills
run in the approved cc-codex-standard image. Jobs remain UID/GID 65532; only
the Runtime Agent has the narrow root capability needed by the guarded APT
facade. Ordinary outbound networking remains disabled, and package installation
uses only the fixed package-source allowlist.
