# Sandbox Artifact Manifest Config Limit

```flow
@flow id=sandbox-artifact-manifest-config-limit
@name Sandbox artifact manifest configured limit
```

```step
@id inspect-artifact-manifest-config-limit
@name Inspect artifact manifest configured limit

GET {{base_url}}/v1/policies

[Asserts]
status == 200
code == 0
data.limits.max_artifact_manifest_files == 10
data.limits.max_artifact_manifest_total_bytes == 8
data.limits.max_artifact_manifest_bytes == 8
data.limits.max_artifact_bytes_per_organization == 10
data.limits.organization_artifact_byte_limit_enforced == true
```

```step
@id create-artifact-limit-sandbox
@name Create artifact-limit sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "{{artifact_limit_organization_id}}"
}

[Captures]
artifact_limit_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.organization_id == "{{artifact_limit_organization_id}}"
data.effective_limits.max_artifact_manifest_files == 10
data.effective_limits.max_artifact_manifest_total_bytes == 8
data.effective_limits.max_artifact_manifest_bytes == 8
data.effective_limits.max_artifact_bytes_per_organization == 10
data.effective_limits.organization_artifact_byte_limit_enforced == true
```

```step
@id upload-artifact-over-config-limit
@name Upload artifact over configured manifest byte limit

POST {{base_url}}/v1/files/upload
Content-Type: application/json

{
  "sandbox_id": "{{artifact_limit_sandbox_id}}",
  "path": "artifacts/report.txt",
  "content": "hello manifest"
}

[Asserts]
status == 200
code == 0
```

```step
@id reject-configured-artifact-manifest-byte-limit
@name Reject configured artifact manifest byte limit

GET {{base_url}}/v1/files/manifest?sandbox_id={{artifact_limit_sandbox_id}}&path=artifacts

[Asserts]
status == 429
code == -429
data.error_type == "limit_exceeded"
data.code == "artifact_manifest_total_bytes_exceeded"
data.limit == "max_artifact_manifest_total_bytes"
data.maximum == 8
data.actual == 14
data.path == "artifacts"
```

```step
@id create-second-artifact-limit-sandbox
@name Create second artifact-limit sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "{{artifact_limit_organization_id}}"
}

[Captures]
second_artifact_limit_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.organization_id == "{{artifact_limit_organization_id}}"
data.effective_limits.max_artifact_bytes_per_organization == 10
data.effective_limits.organization_artifact_byte_limit_enforced == true
```

```step
@id upload-second-organization-artifact
@name Upload second organization artifact

POST {{base_url}}/v1/files/upload
Content-Type: application/json

{
  "sandbox_id": "{{second_artifact_limit_sandbox_id}}",
  "path": "artifacts/second.txt",
  "content": "123456"
}

[Asserts]
status == 200
code == 0
data.size == 6
```

```step
@id reject-organization-artifact-byte-limit
@name Reject organization artifact byte limit

GET {{base_url}}/v1/files/manifest?sandbox_id={{second_artifact_limit_sandbox_id}}&path=artifacts

[Asserts]
status == 429
code == -429
data.error_type == "limit_exceeded"
data.code == "organization_artifact_byte_limit_exceeded"
data.limit == "max_artifact_bytes_per_organization"
data.maximum == 10
data.actual == 20
data.organization_id == "{{artifact_limit_organization_id}}"
data.organization_artifact_bytes == 20
```

```step
@id create-other-artifact-limit-sandbox
@name Create other organization artifact-limit sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "organization_artifact_limit_other"
}

[Captures]
other_artifact_limit_sandbox_id = data.id

[Asserts]
status == 200
code == 0
```

```step
@id upload-other-organization-artifact
@name Upload other organization artifact

POST {{base_url}}/v1/files/upload
Content-Type: application/json

{
  "sandbox_id": "{{other_artifact_limit_sandbox_id}}",
  "path": "artifacts/other.txt",
  "content": "123456"
}

[Asserts]
status == 200
code == 0
data.size == 6
```

```step
@id allow-other-organization-artifact-manifest
@name Allow other organization artifact manifest

GET {{base_url}}/v1/files/manifest?sandbox_id={{other_artifact_limit_sandbox_id}}&path=artifacts

[Asserts]
status == 200
code == 0
data.total_size == 6
```

```step
@id delete-artifact-limit-sandbox
@name Delete artifact-limit sandbox

DELETE {{base_url}}/v1/sandboxes/{{artifact_limit_sandbox_id}}

[Asserts]
status == 200
code == 0
```

```step
@id delete-second-artifact-limit-sandbox
@name Delete second artifact-limit sandbox

DELETE {{base_url}}/v1/sandboxes/{{second_artifact_limit_sandbox_id}}

[Asserts]
status == 200
code == 0
```

```step
@id delete-other-artifact-limit-sandbox
@name Delete other artifact-limit sandbox

DELETE {{base_url}}/v1/sandboxes/{{other_artifact_limit_sandbox_id}}

[Asserts]
status == 200
code == 0
```
