# Local Agent Runtime server deployment

This guide installs one Linux gVisor Worker against an existing PostgreSQL database and a prebuilt, digest-pinned runtime image. It does not install the full ZGI API/web stack, expose a public management endpoint, configure Kubernetes CRI, or restart containerd. E2B continues to use its separate hosted Worker configuration.

## Prerequisites

- Linux x86_64, systemd, cgroup v2; recommended minimum 4 GiB RAM and enough disk for the runtime image, writable workspaces and retained checkpoints.
- A running containerd daemon, `ctr`, `runsc` and `containerd-shim-runsc-v1` in PATH. Use the project's validated versions, not an implicit fallback to runc.
- `bash`, `jq`, `curl`, `flock`, `ss`, `realpath`, and standard GNU file tools.
- A PostgreSQL database reachable by the server independently of the operator's SSH session. Use a dedicated runtime database and its own credentials.
- An approved runtime image matching the intended dependency profile and a Linux Worker binary built from the same release. Build with the Go version in `sandbox/go.mod`, or use a verified release binary.

The Worker uses containerd namespace `zgi-agent-runtime`. Preload the pinned image into that namespace **on the configured socket**, not a different daemon's default namespace. An offline image archive may be supplied through `ZGI_AGENT_RUNTIME_IMAGE_ARCHIVE` (`.tar` or `.zst`). The installer does not silently pull mutable tags.

## Private settings

Create an administrator-owned JSON file, for example `/etc/zgi-agent-runtime/settings.json`, with mode `0600`. All values are strings. JSON is parsed as data, not sourced as Shell. Do not commit credentials, use real secrets in examples, print this file, or copy it into an Agent workspace.

```json
{
  "ZGI_AGENT_RUNTIME_API_KEY": "<dedicated Worker credential>",
  "ZGI_AGENT_RUNTIME_DATABASE_URL": "<private PostgreSQL connection URL>",
  "ZGI_AGENT_RUNTIME_IMAGE_REF": "registry.example/zgi/runtime@sha256:<64 lowercase hex characters>",
  "ZGI_AGENT_RUNTIME_WORKER_SOURCE": "/opt/zgi-release/zgi-agent-runtime-worker",
  "ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS": "/run/containerd/containerd.sock",
  "ZGI_AGENT_RUNTIME_LISTEN": "127.0.0.1:2670",
  "ZGI_AGENT_RUNTIME_STATE_DIR": "/var/lib/zgi-ar",
  "ZGI_AGENT_RUNTIME_MAX_WORKERS": "1",
  "ZGI_AGENT_RUNTIME_MEMORY_BYTES": "805306368",
  "ZGI_AGENT_RUNTIME_PIDS": "256"
}
```

Defaults:

| Setting | Default / restriction |
| --- | --- |
| `SERVICE_NAME` | `zgi-agent-runtime.service`; dedicated instances may use `zgi-agent-runtime-<suffix>.service`. |
| `ENV_FILE` | `/etc/zgi-agent-runtime/runtime.env`; generated with mode 0600 and systemd-compatible quoting. It is not a user-editable Shell configuration. |
| `UNIT_FILE` | `/etc/systemd/system/<SERVICE_NAME>`; unmanaged existing units are never overwritten. |
| `WORKER_BINARY` | `/usr/local/bin/zgi-agent-runtime-worker`; custom targets must have a dedicated `zgi-agent-runtime-worker`, `agentworker` or `worker` filename, not a host utility name. |
| `STATE_DIR` / `ROOT` | `/var/lib/zgi-ar`; if both are set they must agree. A dedicated directory, not a shared system root; canonical absolute paths only, no symlinks, at most 28 bytes to leave room for runtime Unix sockets. |
| `CHECKPOINT_DIR` | `<STATE_DIR>/checkpoints`; local checkpoints must remain inside state. This installer does not configure S3 retention. |
| `LISTEN` | `127.0.0.1:2670`; this installer accepts loopback only. |
| `CONTAINERD_NAMESPACE` | Fixed to `zgi-agent-runtime`, matching the Worker. |
| `HEALTH_TIMEOUT_SECONDS` / `SMOKE_TIMEOUT_SECONDS` | 45; positive, at most 600. |

All setting names in the table have the `ZGI_AGENT_RUNTIME_` prefix. Optional Worker resource, idle/startup/shutdown timeout, worker-ID and package-source settings are carried into the service environment. Unknown JSON keys are rejected. Environment variables remain supported for existing operator workflows; `--config` values take precedence.

The default internal Profile is `cc-codex-standard`, using the pinned image and controlled package-install capability. Optional mirror URLs are `PYPI_INDEX_URL`, `NPM_REGISTRY_URL`, `APT_MIRROR_URL`, and `APT_SECURITY_MIRROR_URL`; their HTTPS hostnames are added to the default package allowlist. URLs must not contain credentials or custom ports. For a custom `PROFILES_JSON`, use a JSON-encoded string and provide its approved source hosts explicitly; every Profile must reference the selected image digest. Missing/broken source connectivity is a diagnostic error, not permission to use arbitrary external network access.

## Install and verify

Run from the repository root, or use an installed copy of the script:

```bash
sudo bash sandbox/scripts/zgi-runtime-server preflight --config /etc/zgi-agent-runtime/settings.json
sudo bash sandbox/scripts/zgi-runtime-server install --config /etc/zgi-agent-runtime/settings.json --dry-run
sudo bash sandbox/scripts/zgi-runtime-server install --config /etc/zgi-agent-runtime/settings.json
sudo bash sandbox/scripts/zgi-runtime-server doctor --config /etc/zgi-agent-runtime/settings.json
```

`preflight` is read-only. `host-install` imports/verifies the approved image and runs a disposable gVisor command; it never changes containerd's configuration or restarts it. `install` also verifies Python/Node as UID 65532, snapshots existing Worker inputs, writes private configuration atomically, and checks that `/health` belongs to the installed service's PID and can reach PostgreSQL. Identical, healthy installations do not restart the Worker or advance the rollback point. A port owned by another service fails closed.

The installed unit runs the control-plane Worker as root because it manages containerd; sandbox Jobs remain non-root. The unit uses `ProtectSystem=strict`, `ProtectHome=true`, `NoNewPrivileges=true` and a writable path restricted to runtime state. Client FIFO files are under `<STATE_DIR>/fifo`, not the shared containerd runtime directory. The Worker must not be reachable using public HTTP or existing external Agent credentials.

`doctor` verifies the pinned image, private environment permissions, service PID/listener ownership and database-backed health. This is **not** proof of successful environment creation or dependency installation. Before enabling the API's Linux feature gate, run an authenticated real contract through the Worker: create a binding, upload a fixed input, execute a non-root script, download its result, verify its contents, suspend/resume and recover after compute loss. Also run the public [portable Skill regression](portable-agent-skills.md#regression-checks).

API and Worker need stable connectivity. For same-host deployments, point the API at loopback; for containerized API deployments, use an approved private gateway/service network. Separate hosts need an existing private network or authenticated HTTPS gateway designed for this purpose. Do not expose the Worker publicly or treat a temporary SSH port forward as a production dependency. Keep API organization approval and the Linux feature gate disabled until acceptance succeeds.

## Upgrade and rollback

Before an upgrade, disable creation of new runtime environments and let active Jobs finish or cancel them through their normal API. Install a new verified Worker source using the same settings. The installed listener and workspace root are immutable within one service deployment; changing them requires a separately planned service migration.

```bash
sudo bash sandbox/scripts/zgi-runtime-server install --config /etc/zgi-agent-runtime/settings.json
sudo bash sandbox/scripts/zgi-runtime-server rollback --config /etc/zgi-agent-runtime/settings.json
```

Installation startup/health failure automatically restores the previous binary, environment, unit and active/enabled state. Explicit rollback performs the same restoration. Repeating a completed rollback is a no-op. On a first installation with no previous Worker, rollback leaves the generated inputs retained but disables the service. It does not claim a transaction rollback of package installation or database changes.

Backups are private directories under `<STATE_DIR>/deployments/release.*`; only the recorded service and exact file targets may be restored. Workspaces, checkpoints, image layers, PostgreSQL data and shared containerd are not deleted or rolled back. Keep backups until the release and recovery are reviewed; they contain credentials and must remain administrator-only. Do not use database downgrade or deletion of user files as a feature-disable mechanism.

## Regression entry points

```bash
make -C sandbox test-agent-runtime-server
shellcheck sandbox/scripts/zgi-runtime-server sandbox/scripts/test_zgi_runtime_server
```

The first test uses isolated mock host commands and checks quoting, config safety, idempotency, controlled smoke cleanup and rollback. Real Linux/systemd/gVisor installation, checkpoint recovery, and E2B account tests are separate mandatory gates. Follow [M6A deployment gates](agent-runtime-m6a-deployment.md) and [dependency installation limits](agent-runtime-m6b-dependency-preparation.md).

Configuration rationale: containerd's [CRI configuration](https://github.com/containerd/containerd/blob/main/docs/cri/config.md) is not used by non-CRI clients; generated environment values follow [systemd EnvironmentFile quoting](https://github.com/systemd/systemd/blob/main/man/systemd.exec.xml).
