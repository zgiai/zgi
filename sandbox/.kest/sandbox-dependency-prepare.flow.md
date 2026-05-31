# Sandbox Dependency Prepare

```flow
@flow id=sandbox-dependency-prepare
@name Sandbox dependency prepare
```

```step
@id prepare-skill-dependencies
@name Prepare dependency request from skill archive

POST {{base_url}}/v1/sandbox/dependencies/prepare
Content-Type: application/json

{
  "archive_base64": "{{dependency_prepare_archive_base64}}",
  "format": "zip",
  "strip_root": false,
  "base_runtime": "linux-secure"
}

[Asserts]
status == 200
code == 0
data.status == "build_required"
data.next_action == "queue_dependency_build"
data.package_count == 5
data.dependency_request.schema_version == 1
data.dependency_request.language == "python3"
data.dependency_request.base_runtime == "linux-secure"
data.packages.0.ecosystem == "nodejs"
data.packages.0.name == "@org/tool"
data.packages.1.ecosystem == "nodejs"
data.packages.1.name == "pdf-lib"
data.packages.1.version == "1.17.1"
data.packages.2.ecosystem == "python3"
data.packages.2.name == "pandas"
data.packages.2.version == "==2.2.3"
data.packages.3.ecosystem == "python3"
data.packages.3.name == "pillow"
data.packages.4.ecosystem == "python3"
data.packages.4.name == "pydantic"
data.packages.4.version == "=={{dependency_prepare_pydantic_version}}"
```

```step
@id queue-dependency-build
@name Queue dependency build from skill archive

POST {{base_url}}/v1/sandbox/dependencies/builds
Content-Type: application/json
X-Request-ID: req_kest_dependency_build_queue

{
  "organization_id": "organization_dependency_prepare_kest",
  "archive_base64": "{{dependency_prepare_archive_base64}}",
  "format": "zip",
  "strip_root": false,
  "base_runtime": "linux-secure"
}

[Captures]
dependency_build_id = data.build_id
dependency_build_fingerprint = data.fingerprint

[Asserts]
status == 200
code == 0
data.status == "queued"
data.next_action == "wait_for_dependency_build"
data.organization_id == "organization_dependency_prepare_kest"
data.package_count == 5
data.profile_name != ""
data.dependency_request.schema_version == 1
data.dependency_request.base_runtime == "linux-secure"
```

```step
@id lookup-dependency-build
@name Lookup queued dependency build

GET {{base_url}}/v1/sandbox/dependencies/builds?fingerprint={{dependency_build_fingerprint}}

[Asserts]
status == 200
code == 0
data.build_id == "{{dependency_build_id}}"
data.fingerprint == "{{dependency_build_fingerprint}}"
data.status == "queued"
data.next_action == "wait_for_dependency_build"
data.package_count == 5
```

```step
@id observer-dependency-build-queued
@name Observer dependency build queued event

GET {{base_url}}/v1/observer/events?type=dependency_build.queued&request_id=req_kest_dependency_build_queue&limit=1

[Asserts]
status == 200
code == 0
data.events.0.type == "dependency_build.queued"
data.events.0.metadata.status == "queued"
data.events.0.metadata.organization_id == "organization_dependency_prepare_kest"
data.events.0.metadata.build_id == "{{dependency_build_id}}"
data.events.0.metadata.fingerprint == "{{dependency_build_fingerprint}}"
```
