# Sandbox Dependency Profile Artifact Autoload

```flow
@flow id=sandbox-dependency-profile-artifact-autoload
@name Sandbox dependency profile artifact autoload
```

```step
@id inspect-artifact-profile-catalog
@name Inspect artifact dependency profile in catalog

GET /v1/sandbox/dependencies?language=python3
X-API-Key: {{admin_api_key}}

[Asserts]
status == 200
code == 0
data.profiles.3.name == "skill-office"
data.profiles.3.version == "2026.05.31-artifact"
data.profiles.3.status == "ready"
data.profiles.3.enabled == true
data.profiles.3.base_runtime == "linux-secure"
data.profiles.3.checksum == "{{skill_office_checksum}}"
```

```step
@id create-artifact-profile-sandbox
@name Create sandbox with artifact dependency profile

POST /v1/sandboxes
Content-Type: application/json
X-API-Key: {{admin_api_key}}

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "dependency_profile": "skill-office"
}

[Captures]
artifact_profile_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.dependency_profile == "skill-office"
data.dependency_profile_version == "2026.05.31-artifact"
```

```step
@id delete-artifact-profile-sandbox
@name Delete artifact dependency profile sandbox

DELETE /v1/sandboxes/{{artifact_profile_sandbox_id}}
X-API-Key: {{admin_api_key}}

[Asserts]
status == 200
code == 0
```
