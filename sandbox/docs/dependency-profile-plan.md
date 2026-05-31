# Dependency Profile Plan

## 1. Decision

`zgi-sandbox` should use prebuilt dependency profiles with lockfiles. User code
must not install packages during normal execution.

The dependency lifecycle is:

1. Operators define a managed profile.
2. The profile is built outside user execution.
3. The build produces a pinned, verified runtime artifact.
4. The profile is registered as `ready`.
5. Sandbox creation selects one ready profile.
6. Command execution only sees the selected profile environment.

This keeps sandbox startup fast, reproducible, auditable, and compatible with
deny-by-default network policy.

## 2. Current Support

The current codebase already has the policy and API foundation:

- `GET /v1/sandbox/dependencies` exposes a managed dependency catalog.
- `POST /v1/sandbox/dependencies/update` is administrator-only.
- Sandbox creation accepts `dependency_profile`.
- The policy layer rejects unknown or disabled profiles.
- Sandbox records persist the selected profile and profile version.
- Skill manifests can declare `dependency_profile`.
- Archive upload rejects skill manifests that do not match the sandbox profile.
- The secure Linux backend can select a profile-specific rootfs directory through
  `ZGI_SANDBOX_DEPENDENCY_ROOTFS_DIR`.
- Service startup can load verified profile artifacts from
  `ZGI_SANDBOX_DEPENDENCY_ROOTFS_DIR` into the dependency catalog as ready,
  enabled profiles.
- Runtime dependency installation commands are rejected by the executor policy.

The remaining gap is operational packaging: production deployments still need a
release job that builds complete Python and Node profile environments, publishes
the profile-specific rootfs output, and rolls it to sandbox workers.

## 3. Target Model

### 3.1 Profile Identity

Each profile must have:

- `name`
- `version`
- `status`
- `enabled`
- `owner_scope`
- `languages`
- `base_runtime`
- `checksum`
- `size_bytes`
- package manifests
- build metadata
- verification metadata

Profile names stay small and operator-defined. Initial profiles:

- `stdlib`
- `skill-office`
- `skill-web`

`skill-office` should cover document and office workloads. `skill-web` should
be added only after the browser/runtime boundary is ready.

### 3.2 Source Layout

Managed profile source files should live under `sandbox/profiles/`:

```text
sandbox/profiles/
  stdlib/
    manifest.json
    verify.py
  skill-office/
    apt-packages.txt
    requirements.in
    requirements.lock
    package.json
    pnpm-lock.yaml
    manifest.json
    verify.py
    verify-node.mjs
  skill-web/
    package.json
    pnpm-lock.yaml
    manifest.json
    verify-node.mjs
```

Profile files are maintained by operators and contributors. User-provided skill
packages must not add dependency files that trigger runtime installation.

### 3.3 Built Runtime Layout

The build output should be materialized as profile-specific runtime directories:

```text
/opt/zgi/profiles/
  stdlib/
    manifest.json
  skill-office/
    venv/
    node_modules/
    bin/
    manifest.json
  skill-web/
    node_modules/
    bin/
    manifest.json
```

For the secure Linux backend, these directories can either be included in
profile-specific rootfs directories or mounted read-only into a shared rootfs.
The first production-safe version should prefer profile-specific rootfs
directories because the existing selector already supports that boundary.

### 3.4 Runtime Environment

When a sandbox selects `skill-office`, execution should receive only that
profile's environment:

```text
PATH=/opt/zgi/profiles/skill-office/venv/bin:/opt/zgi/profiles/skill-office/node_modules/.bin:/usr/local/bin:/usr/bin:/bin
PYTHONNOUSERSITE=1
NODE_PATH=/opt/zgi/profiles/skill-office/node_modules
```

The runtime must not expose global package caches, user site packages, writable
profile paths, or package manager write locations.

## 4. Install Timing

User dependencies are installed during profile build, not during sandbox
execution.

Allowed install stages:

- Docker image build for globally managed base packages.
- Profile build job for Python and Node lockfile sync.
- Release or deployment pipeline that publishes verified profile rootfs output.

Disallowed install stages:

- `POST /v1/exec/code`
- `POST /v1/exec/command`
- `POST /v1/exec/skill`
- skill script startup
- sandbox creation request handling

Sandbox creation should only validate that the requested profile exists, is
enabled, is ready, matches the runtime backend, and is below configured limits.

## 5. API Contract

### 5.1 Skill Manifest

Skill packages should use:

```json
{
  "entrypoint": "scripts/run.py",
  "language": "python3",
  "dependency_profile": "skill-office",
  "timeout_ms": 30000,
  "allowed_artifact_paths": ["artifacts"],
  "result_mode": "mixed"
}
```

The API side should:

- default missing `dependency_profile` to `stdlib`;
- reject unsupported names before creating a sandbox when the catalog is
  reachable;
- pass `dependency_profile` to sandbox creation;
- upload the package archive after sandbox creation;
- never run package installation commands for a skill.

The sandbox side should:

- persist the selected profile name and version;
- validate uploaded skill manifests against the selected profile;
- execute commands with profile-specific runtime configuration;
- include profile name and version in observer events.

### 5.2 Dependency Catalog

The catalog response should be the source of truth for the API:

- available profiles;
- supported languages;
- status;
- version;
- enabled flag;
- runtime backend compatibility;
- profile size;
- checksum;
- user update support set to `false`.

The API should cache catalog reads briefly but must fail closed for unknown
profiles.

### 5.3 Runtime Artifact Reuse

Dependency profile artifacts should be stored by checksum and referenced by
profiles. A single verified artifact may back multiple profile records when the
artifact is public, reproducible, and free of organization-private content.

Profile visibility is scoped:

- `global` profiles are visible to every organization.
- `organization` profiles are visible only to the matching `organization_id`.

This allows common runtimes to be reused without duplicating storage while still
keeping private packages, private registry output, license files, and internal
SDKs isolated to the owning organization. Cleanup should remove profile
references first and delete artifact files only when no profiles reference the
artifact.

## 6. Build Pipeline

### 6.1 Python

Python profile builds should:

- compile `requirements.in` into a locked file;
- require hashes or another deterministic integrity mechanism;
- sync into a profile-local virtual environment;
- disable user site packages at runtime;
- verify imports with `verify.py`.

### 6.2 Node

Node profile builds should:

- use the committed lockfile;
- install with a frozen lockfile;
- keep packages profile-local;
- avoid global installs;
- verify imports with `verify-node.mjs`.

### 6.3 OS Packages

OS packages should be installed only in image or rootfs build stages. They
should not be installed by the sandbox service at request time.

The office profile should start with:

- document conversion tools;
- PDF inspection tools;
- OCR runtime;
- common fonts;
- image processing libraries needed by Python packages.

## 7. Enforcement

The executor already rejects common runtime install commands. Production should
make this stronger in these layers:

- request validation for command profiles;
- secure runtime environment with package manager cache paths unavailable;
- read-only profile directories;
- no outbound network by default;
- observer events for rejected install attempts;
- profile verification before activation.

Any future administrator-only runtime install capability must be a separate
profile build operation, not part of user execution.

## 8. Implementation PRs

### PR 1: Profile Source Catalog

Goal: add maintained profile source files and strict metadata validation.

Scope:

- add `sandbox/profiles/stdlib`;
- add `sandbox/profiles/skill-office`;
- define `manifest.json` schema;
- validate lockfile presence;
- validate pinned versions;
- expose profile source metadata in tests.

Validation:

- unit tests for manifest parsing;
- Kest catalog flow asserts `skill-office` appears as disabled or ready
  according to fixture mode;
- open-source hygiene check.

### PR 2: Profile Build Script

Goal: add a deterministic local build command that creates profile artifacts.

Scope:

- add a build script under `sandbox/scripts/`;
- build Python virtual environments from lockfiles;
- build Node dependencies from lockfiles;
- run verification scripts;
- emit `manifest.json` with checksum and size.
- keep the build command operator-side and out of request execution paths.

Validation:

- script unit tests where possible;
- smoke build for `stdlib`;
- optional local build for `skill-office` when system tools are present;
- no runtime execution path calls the build script.

### PR 3: Profile Rootfs Activation

Goal: make the secure runtime activate profile-specific environments.

Scope:

- extend rootfs/profile selection to verify built profile metadata;
- inject profile environment variables in one place;
- bind profile directories read-only;
- reject missing or mismatched profile artifacts;
- include profile checksum in observer metadata.

Validation:

- secure runtime unit tests;
- rootfs selector tests;
- command execution tests for `PATH`, `PYTHONNOUSERSITE`, and `NODE_PATH`;
- Kest flow for profile selection.

The HTTP Kest suite runs against the default preview backend. Secure rootfs
artifact activation is covered by Go tests because it depends on host rootfs and
Bubblewrap setup.

### PR 4: API Catalog Preflight

Goal: make the API validate skill `dependency_profile` before sandbox creation.

Scope:

- add sandbox catalog client method;
- cache catalog responses with short TTL;
- fail closed for unknown or unavailable profiles;
- map profile errors into skill trace output;
- keep sandbox-side validation as the final authority.

Validation:

- API unit tests for known, unknown, disabled, and catalog-unavailable profiles;
- skill script E2E flow using a declared profile;
- Kest flow where API invokes a skill script against sandbox.

### PR 5: Office Profile Release Gate

Goal: make `skill-office` production-usable.

Scope:

- release verified profile artifacts from profile-specific rootfs directories;
- load verified artifacts into the dependency catalog on sandbox startup;
- keep source profiles disabled until a verified artifact is present;
- reject corrupted, mismatched, symlinked, or unverified artifacts;
- cover artifact autoload through unit tests and Kest.

Scope:

- add the initial approved package set;
- add import and command verification;
- document required OS packages;
- define size and startup budgets;
- mark profile `ready` only when verification passes.

Validation:

- office document smoke test;
- PDF smoke test;
- spreadsheet smoke test;
- artifact return smoke test;
- CI or release job that fails when the lockfile and manifest drift.

## 9. Test Matrix

Required coverage before calling the feature production-ready:

- catalog lists profiles with pinned versions;
- unknown profile is rejected at sandbox creation;
- disabled profile is rejected at sandbox creation;
- skill manifest mismatch is rejected during archive upload;
- runtime install command is rejected;
- selected profile version appears in sandbox records;
- selected profile version appears in execution observer events;
- secure runtime selects the expected rootfs;
- profile environment variables are present;
- user site packages are disabled;
- profile directories are read-only;
- output artifacts still respect count and byte limits;
- Kest flow covers catalog, create, upload archive, execute skill, artifacts,
  and cleanup.

## 10. Production Readiness Gate

The dependency profile feature is production-ready only when all of these are
true:

- users cannot install packages during normal execution;
- every executable profile is built from committed lockfiles;
- every profile has a checksum and verification result;
- sandbox execution never mutates profile directories;
- API and sandbox agree on profile names and versions;
- failure modes are visible in observer events;
- Kest and Go tests cover the full skill execution path;
- release docs explain how operators add, build, verify, and roll back profiles.
