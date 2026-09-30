# Approved Home Snapshots (M5 operator path)

A Home Snapshot is an immutable, prebuilt base environment for new Agent Runtime Bindings. It is **not** a copy of a live conversation. A local gVisor snapshot points to a digest-pinned container image; an E2B snapshot points to an approved, versioned E2B template. Build these artifacts in an independent, operator-controlled environment and keep the image/template available while any published Agent or Binding references it.

## Prepare the artifact

1. Start from a reviewed Runtime Agent base image/template. Prepare only approved tools and offline dependencies during the separate build. Do not copy `/workspace`, customer files, `.env` files, provider credentials, sockets, runtime tokens, caches containing user data, or command histories into the artifact.
2. For local gVisor, publish and preload an immutable `repository@sha256:<64 lowercase hex>` image in the Worker's containerd namespace. For E2B, publish a distinct template ID for each immutable build. The Runtime Agent protocol and offline network policy must be compatible with the Worker version.
3. Launch a disposable validation environment from the artifact. Verify that `/workspace` is empty, a Python or Node tool expected from the template runs without runtime installation, no user data or control credentials are readable by the Job user, and the network remains disabled. Destroy the disposable environment after verification.

The local runtime binds a fresh workspace over `/workspace` and uses a read-only image root filesystem. E2B templates need explicit validation that their built filesystem contains no previous session data. Neither backend copies a live Binding into a Home Snapshot.

## Register and approve

Insert one record into the Worker's PostgreSQL `agent_runtime_home_snapshots` table using an operator account. Use an organization-wide record (`subject_id = ''`) and a new ID for every artifact version. Required values are `id`, `organization_id`, `profile_id`, `backend` (`local-gvisor` or `e2b`), `backend_ref`, `status = 'approved'`, `generation = 1`, and timestamps. Once approved, database triggers prohibit changing its organization, profile, backend, or artifact reference. Do not reuse an ID for a rebuild.

The API also needs an operator-owned publication allowlist, for example:

```json
ZGI_AGENT_RUNTIME_APPROVED_HOME_SNAPSHOTS_JSON={"prepared-v1":{"backend":"local-gvisor","profile_id":"standard","organization_ids":["organization-id"]}}
```

The allowlist must match the database row. The API rejects publication without a matching allowlist entry; the Worker independently checks the database row, organization, profile, backend, approved status, and nonempty artifact reference on every new Binding. A snapshot ID is selected in the console's optional Home Snapshot field. Model Tool arguments cannot override it.

An existing Binding persists the resolved artifact reference, so resuming it does not silently switch to a newer build. The local backend rejects a floating image tag and requires the image to be preloaded. If an approved artifact must change, register a new ID and publish a new Agent configuration; keep the old artifact until its references are retired.

## Current boundary

Checkpoint and seven-day retention currently cover `/workspace`. The Home Snapshot is the immutable prepared base, not a mutable per-session `/home/zgi` archive. Applying a different Home Snapshot to an existing conversation requires a separate, explicit rebuild workflow; changing the Agent configuration alone does not alter existing Bindings. Do not advertise full M5 Home persistence or template-switch recovery before those gates pass.
