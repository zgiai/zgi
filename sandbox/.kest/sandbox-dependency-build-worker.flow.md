# Sandbox Dependency Build Worker

```flow
@flow id=sandbox-dependency-build-worker
@name Sandbox dependency build worker
```

```step
@id queue-worker-dependency-build
@name Queue dependency build for worker

POST /v1/sandbox/dependencies/builds
Content-Type: application/json
X-API-Key: {{admin_api_key}}
X-Request-ID: req_kest_dependency_worker_queue

{
  "organization_id": "organization_dependency_worker_kest",
  "archive_base64": "{{dependency_prepare_archive_base64}}",
  "format": "zip",
  "strip_root": false,
  "base_runtime": "linux-secure"
}

[Captures]
worker_dependency_build_fingerprint = data.fingerprint
worker_dependency_build_profile = data.profile_name

[Asserts]
status == 200
code == 0
data.status == "queued"
data.next_action == "wait_for_dependency_build"
data.profile_name != ""
```

```step
@id run-worker-dependency-build
@name Run dependency build worker

POST /v1/sandbox/dependencies/builds/{{worker_dependency_build_fingerprint}}/run
X-API-Key: {{admin_api_key}}
X-Request-ID: req_kest_dependency_worker_run

[Captures]
worker_dependency_artifact_checksum = data.artifact_checksum

[Asserts]
status == 200
code == 0
data.status == "ready"
data.next_action == "use_dependency_profile"
data.profile_name == "{{worker_dependency_build_profile}}"
data.artifact_checksum != ""
data.size_bytes > 0
```

```step
@id lookup-worker-ready-build
@name Lookup ready dependency build

GET /v1/sandbox/dependencies/builds?fingerprint={{worker_dependency_build_fingerprint}}
X-API-Key: {{admin_api_key}}

[Asserts]
status == 200
code == 0
data.status == "ready"
data.next_action == "use_dependency_profile"
data.profile_name == "{{worker_dependency_build_profile}}"
data.artifact_checksum == "{{worker_dependency_artifact_checksum}}"
```

```step
@id create-worker-built-profile-sandbox
@name Create sandbox with worker-built dependency profile

POST /v1/sandboxes
Content-Type: application/json
X-API-Key: {{admin_api_key}}

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "dependency_profile": "{{worker_dependency_build_profile}}"
}

[Captures]
worker_dependency_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.dependency_profile == "{{worker_dependency_build_profile}}"
data.dependency_artifact_checksum == "{{worker_dependency_artifact_checksum}}"
```

```step
@id observer-worker-build-ready
@name Observer dependency build ready event

GET /v1/observer/events?type=dependency_build.ready&request_id=req_kest_dependency_worker_run&limit=1
X-API-Key: {{admin_api_key}}

[Asserts]
status == 200
code == 0
data.events.0.type == "dependency_build.ready"
data.events.0.metadata.status == "ready"
data.events.0.metadata.profile_name == "{{worker_dependency_build_profile}}"
data.events.0.metadata.artifact_checksum == "{{worker_dependency_artifact_checksum}}"
```

```step
@id delete-worker-built-profile-sandbox
@name Delete worker-built profile sandbox

DELETE /v1/sandboxes/{{worker_dependency_sandbox_id}}
X-API-Key: {{admin_api_key}}

[Asserts]
status == 200
code == 0
```
