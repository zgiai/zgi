# Standard script Skills: setup and use

The Linux execution environment runs standard Python/Node script Skills without
adding a ZGI manifest. Agent users only enable **Linux execution environment**;
backend, default dependencies, package sources and organization approval belong
to the deployment administrator. Existing one-shot `run_script` Skills remain
available through their original execution path.

## Choose a backend

- **Local gVisor:** native Linux x86_64 with systemd, containerd and runsc. Use
  the native Worker executable and the default runtime OCI image. KVM is not
  required. See [host setup](../sandbox/docs/agent-runtime-install.md).
- **E2B:** run the Worker container alongside the existing API on a private
  Docker network. Provide an E2B key and an approved package template. No local
  containerd/KVM or host socket mount is needed. The standard OCI image is not
  itself an E2B template; use the dedicated template build instructions below.

Both require a reachable dedicated PostgreSQL runtime database and the same
dedicated Worker/API credential. They do not deploy or replace the existing
ZGI API/Web, and are not new public Agent entry points.

## Build or obtain matching artifacts

The Docker release workflow includes `zgi-agentworker` (E2B control plane) and
`zgi-agent-runtime` (local execution image), initially **linux/amd64 only**.
Successful runs attach immutable image identities and a native Linux Worker
archive containing its SHA-256, source revision, operator scripts and examples.
These workflow additions do not mean a release has already been published.
Always use artifacts from the same source revision; do not deploy `latest`.
After extracting the native archive, run `sha256sum --check SHA256SUMS` before
installation. It contains no provider keys or filled operator settings.

Without Go on the host, build locally from the repository root:

```sh
docker buildx build --platform linux/amd64 --target binary \
  -f sandbox/Dockerfile.agentworker --output type=local,dest=outputs/runtime-worker sandbox
docker build --platform linux/amd64 -f sandbox/Dockerfile.agentworker \
  -t zgi-agentworker:local sandbox
docker build --platform linux/amd64 -f sandbox/Dockerfile.agent-runtime \
  -t zgi-agent-runtime:local sandbox
```

The runtime image already installs and verifies the standard Profile's language,
office/PDF/media dependencies. No separate dependency-profile build is needed
for this Dockerfile. Push to your approved registry, resolve the immutable digest
and preload the local execution image into the Worker's configured containerd
namespace. Building an ordinary Docker container is not gVisor isolation.

For E2B, follow [the package-template build](../sandbox/docs/agent-runtime-e2b.md#approved-template-contract)
using `Dockerfile.agent-runtime-e2b-package`. Record the resulting template ID and
source revision with the Worker release. Template creation needs the deployment's
E2B account; it is not automatically performed by the Docker release workflow.

## Local setup

Copy [local settings](../sandbox/deploy/local.settings.example.json) to a private
administrator-owned settings file, mode `0600`. Fill the Worker source path,
approved runtime image digest, database URL and dedicated Worker credential.
The installer supplies the default standard Profile and package-source allowlist.

```sh
sudo bash sandbox/scripts/zgi-runtime-server preflight --config /etc/zgi-agent-runtime/settings.json
sudo bash sandbox/scripts/zgi-runtime-server host-install --config /etc/zgi-agent-runtime/settings.json
sudo bash sandbox/scripts/zgi-runtime-server install --config /etc/zgi-agent-runtime/settings.json
sudo bash sandbox/scripts/zgi-runtime-server doctor --config /etc/zgi-agent-runtime/settings.json
```

If using the downloaded archive, invoke its `zgi-runtime-server` at the archive
root instead. `host-install` verifies/imports the approved image and shim; it does
not install missing containerd/runsc or modify the host's containerd configuration.
Complete the linked host setup first. Image archives may be imported offline.

Apply [the local API example](../sandbox/deploy/api.local.env.example) to the
existing API environment and replace the organization ID. The sample loopback
URL is for a native same-host API. A Docker API or separate host needs an approved
private connection; do not publish the Worker publicly. Temporary SSH forwarding
remains a development option, not a permanent deployment solution.

## E2B setup

Copy [the compose file](../sandbox/deploy/compose.e2b.yaml) and
[Worker environment example](../sandbox/deploy/e2b.env.example) into a private
deployment directory. Rename the environment example to `e2b.env`, mode `0600`,
then fill its dedicated credential, runtime database URL, E2B key and package
template ID. The database hostname must resolve on the existing API network.

```sh
export ZGI_AGENT_RUNTIME_WORKER_IMAGE='your-registry/zgi-agentworker@sha256:<digest>'
export ZGI_AGENT_RUNTIME_DOCKER_NETWORK='your-existing-api-network'
docker compose -f compose.e2b.yaml up -d
```

The Worker runs as UID 65532 with a retained state volume and no published port.
Add [the E2B API example](../sandbox/deploy/api.e2b.env.example) to the existing
API environment, replace the organization ID and use the same Worker credential.
Recreate only your intended API service to apply its environment changes. Keep
the state volume when recreating the Worker; `down -v` deletes retained files.
The API and Worker must be on the specified private network.

## Agent user workflow

1. Import the original Skill ZIP through preview and confirm. Keep its `SKILL.md`
   and referenced files; do not add a ZGI manifest or change its script protocol.
2. Bind the Skill to an approved console Agent and enable Linux execution.
3. Upload input files and describe the result you need. The Agent uses the active
   Skill's session path and its original helpers, not a host filesystem path.
4. Missing Python/Node or supported system dependencies may be installed by the
   model through controlled package sources. No separate dependency-preparation
   tool or Skill declaration is required on the standard package-install Profile.
5. Obtain the generated file through its download card. A printed sandbox path
   or a model statement alone is not a delivered file.

The administrator's `ZGI_AGENT_RUNTIME_ENABLED` flag and organization approval
control availability; users cannot choose Profile, Policy, Snapshot or networking.
Existing conversations retain their approved runtime version. When upgrading
runtime images/templates, preserve old Profile versions for retained sessions.

Ordinary capabilities include uploaded-file processing, Python/Node and native
extension use, common office/PDF/media tools, explicit dependency installation,
multi-turn file edits and same-session dependency retention. This is not arbitrary
internet access, unrestricted root/Shell services, browser automation or general
compatibility with every host-specific Skill extension. See
[portable Skill rules](portable-agent-skills.md) for scripts, paths and limits.
