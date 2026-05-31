# Sandbox Dependency Profile Build

```flow
@flow id=sandbox-dependency-profile-build
@name Sandbox administrator dependency profile build path
```

```step
@id reject-unauthorized-profile-build
@name Reject dependency profile build without admin key

POST /v1/sandbox/dependencies/update
Content-Type: application/json

{
  "name": "{{dependency_profile_name}}",
  "version": "2026.05.31",
  "languages": ["python3"],
  "base_runtime": "preview-process",
  "checksum": "sha256:{{dependency_profile_name}}",
  "size_bytes": 1024
}

[Asserts]
status == 401
code == -401
```

```step
@id build-ready-profile
@name Build ready dependency profile with admin key

POST /v1/sandbox/dependencies/update
Content-Type: application/json
X-API-Key: {{admin_api_key}}
X-Request-ID: req_kest_profile_build

{
  "name": "{{dependency_profile_name}}",
  "version": "2026.05.31",
  "languages": ["python3"],
  "packages": [
    {
      "name": "data-tools",
      "version": "managed"
    }
  ],
  "base_runtime": "preview-process",
  "checksum": "sha256:{{dependency_profile_name}}",
  "size_bytes": 1024,
  "description": "Managed document automation profile."
}

[Asserts]
status == 200
code == 0
data.accepted == true
data.status == "ready"
data.profile.name == "{{dependency_profile_name}}"
data.profile.version == "2026.05.31"
data.profile.status == "ready"
data.profile.enabled == true
data.profile.owner_scope == "global"
data.profile.packages.0.ecosystem == "python3"
```

```step
@id inspect-built-profile-catalog
@name Inspect built dependency profile in catalog

GET /v1/sandbox/dependencies?language=python3

[Asserts]
status == 200
code == 0
```

```step
@id create-built-profile-sandbox
@name Create sandbox with built dependency profile

POST /v1/sandboxes
Content-Type: application/json
X-API-Key: {{admin_api_key}}

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "dependency_profile": "{{dependency_profile_name}}"
}

[Captures]
built_profile_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.dependency_profile == "{{dependency_profile_name}}"
data.dependency_profile_version == "2026.05.31"
```

```step
@id build-organization-profile-reference
@name Build organization dependency profile reference

POST /v1/sandbox/dependencies/update
Content-Type: application/json
X-API-Key: {{admin_api_key}}
X-Request-ID: req_kest_organization_profile_build

{
  "name": "team-data-{{dependency_profile_name}}",
  "version": "2026.06.01",
  "scope": "organization",
  "organization_id": "organization_profile_kest",
  "languages": ["python3"],
  "packages": [
    {
      "ecosystem": "python3",
      "name": "data-tools",
      "version": "managed"
    }
  ],
  "base_runtime": "preview-process",
  "checksum": "sha256:team-data-{{dependency_profile_name}}",
  "artifact_checksum": "sha256:shared-team-data-artifact",
  "size_bytes": 1024,
  "description": "Organization managed data runtime."
}

[Asserts]
status == 200
code == 0
data.profile.name == "team-data-{{dependency_profile_name}}"
data.profile.scope == "organization"
data.profile.organization_id == "organization_profile_kest"
data.profile.artifact_checksum == "sha256:shared-team-data-artifact"
```

```step
@id create-organization-profile-sandbox
@name Create sandbox with organization dependency profile

POST /v1/sandboxes
Content-Type: application/json
X-API-Key: {{admin_api_key}}

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "organization_profile_kest",
  "dependency_profile": "team-data-{{dependency_profile_name}}"
}

[Captures]
organization_profile_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.organization_id == "organization_profile_kest"
data.dependency_profile == "team-data-{{dependency_profile_name}}"
data.dependency_profile_version == "2026.06.01"
```

```step
@id reject-other-organization-profile-sandbox
@name Reject organization dependency profile from another organization

POST /v1/sandboxes
Content-Type: application/json
X-API-Key: {{admin_api_key}}

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "organization_profile_kest_other",
  "dependency_profile": "team-data-{{dependency_profile_name}}"
}

[Asserts]
status == 400
code == -400
```

```step
@id reject-unpinned-profile-build
@name Reject unpinned dependency profile build

POST /v1/sandbox/dependencies/update
Content-Type: application/json
X-API-Key: {{admin_api_key}}
X-Request-ID: req_kest_profile_build_failed

{
  "name": "bad-profile",
  "version": "latest",
  "languages": ["python3"],
  "base_runtime": "preview-process",
  "checksum": "sha256:bad",
  "size_bytes": 1024
}

[Asserts]
status == 400
code == -400
data.accepted == true
data.status == "failed"
data.error == "dependency profile version must be pinned"
```

```step
@id observer-build-failure-event
@name Observer records dependency profile build failure

GET /v1/observer/events?type=dependency_profile.build.failed&request_id=req_kest_profile_build_failed&limit=1
X-API-Key: {{admin_api_key}}

[Asserts]
status == 200
code == 0
data.events.0.type == "dependency_profile.build.failed"
data.events.0.metadata.status == "failed"
data.events.0.metadata.error == "dependency profile version must be pinned"
```

```step
@id delete-organization-profile-sandbox
@name Delete organization dependency profile sandbox

DELETE /v1/sandboxes/{{organization_profile_sandbox_id}}
X-API-Key: {{admin_api_key}}

[Asserts]
status == 200
code == 0
```

```step
@id delete-built-profile-sandbox
@name Delete built dependency profile sandbox

DELETE /v1/sandboxes/{{built_profile_sandbox_id}}
X-API-Key: {{admin_api_key}}

[Asserts]
status == 200
code == 0
```
