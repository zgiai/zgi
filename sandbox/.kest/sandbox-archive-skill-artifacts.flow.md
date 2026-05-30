# Sandbox Archive Upload and Skill Script Artifacts

```flow
@flow id=sandbox-archive-skill-artifacts
@name Sandbox archive upload and skill-script artifacts
```

```step
@id create-sandbox
@name Create sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 120,
  "dependency_profile": "stdlib",
  "network_enabled": false,
  "network_policy": "deny-by-default"
}

[Captures]
sandbox_id = data.id

[Asserts]
status == 200
code == 0
```

```step
@id upload-archive
@name Upload skill archive

POST {{base_url}}/v1/files/upload-archive
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "path": ".",
  "archive_base64": "{{skill_archive_base64}}",
  "format": "zip",
  "strip_root": false
}

[Asserts]
status == 200
code == 0
data.file_count == 3
```

```step
@id run-script
@name Run uploaded script

POST {{base_url}}/v1/exec/command
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "command": "python3",
  "args": ["scripts/run.py"],
  "stdin": "{\"input\":\"hello from kest\"}",
  "profile": "skill-python",
  "timeout_ms": 30000,
  "stdout_limit_kb": 1024,
  "stderr_limit_kb": 1024,
  "working_subpath": "."
}

[Captures]
script_stdout = data.stdout

[Asserts]
status == 200
code == 0
data.exit_code == 0
```

```step
@id list-artifacts
@name List generated artifacts

GET {{base_url}}/v1/files/tree?sandbox_id={{sandbox_id}}

[Asserts]
status == 200
code == 0
```

```step
@id download-artifact
@name Download generated artifact

GET {{base_url}}/v1/files/download?sandbox_id={{sandbox_id}}&path=artifacts/report.txt&encoding=base64

[Asserts]
status == 200
code == 0
data.content == "a2VzdCBhcnRpZmFjdAo="
```

```step
@id artifact-manifest
@name Generate artifact manifest

GET {{base_url}}/v1/files/manifest?sandbox_id={{sandbox_id}}&path=artifacts

[Asserts]
status == 200
code == 0
data.path == "artifacts"
data.file_count == 1
data.total_size == 14
data.truncated == false
```

```step
@id delete-sandbox
@name Delete sandbox

DELETE {{base_url}}/v1/sandboxes/{{sandbox_id}}

[Asserts]
status == 200
code == 0
data.deleted == true
```
