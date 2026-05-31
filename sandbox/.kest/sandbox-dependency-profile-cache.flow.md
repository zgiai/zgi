# Sandbox Dependency Profile Cache

```flow
@flow id=sandbox-dependency-profile-cache
@name Sandbox dependency profile cache after restart
```

```step
@id inspect-cached-built-profile
@name Inspect cached dependency profile after restart

GET /v1/sandbox/dependencies?language=python3

[Asserts]
status == 200
code == 0
```

```step
@id create-cached-profile-sandbox
@name Create sandbox with cached dependency profile

POST /v1/sandboxes
Content-Type: application/json
X-API-Key: {{admin_api_key}}

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "dependency_profile": "{{dependency_profile_name}}"
}

[Captures]
cached_profile_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.dependency_profile == "{{dependency_profile_name}}"
data.dependency_profile_version == "2026.05.31"
```

```step
@id create-cached-skill-office-sandbox
@name Create sandbox with cached promoted skill office profile

POST /v1/sandboxes
Content-Type: application/json
X-API-Key: {{admin_api_key}}

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "dependency_profile": "skill-office"
}

[Captures]
cached_skill_office_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.dependency_profile == "skill-office"
data.dependency_profile_version == "2026.05.31"
```

```step
@id delete-cached-profile-sandbox
@name Delete cached dependency profile sandbox

DELETE /v1/sandboxes/{{cached_profile_sandbox_id}}
X-API-Key: {{admin_api_key}}

[Asserts]
status == 200
code == 0
```

```step
@id delete-cached-skill-office-sandbox
@name Delete cached skill office sandbox

DELETE /v1/sandboxes/{{cached_skill_office_sandbox_id}}
X-API-Key: {{admin_api_key}}

[Asserts]
status == 200
code == 0
```
