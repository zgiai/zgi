# Sandbox Security and Limit Rejections

```flow
@flow id=sandbox-security-limits
@name Sandbox security and limit rejections
```

```step
@id policy-surface
@name Inspect preview network enforcement surface

GET {{base_url}}/v1/policies

[Asserts]
status == 200
code == 0
data.limits.runtime_backend == "preview-process"
data.limits.network_policy_enforced == false
```

```step
@id reject-preview-network-sandbox
@name Reject network-enabled sandbox on preview backend

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 120,
  "dependency_profile": "stdlib",
  "network_enabled": true,
  "network_policy": "workflow-safe"
}

[Asserts]
status == 400
code == -400
```

```step
@id reject-preview-network-run
@name Reject stateless network run on preview backend

POST {{base_url}}/v1/sandbox/run
Content-Type: application/json

{
  "language": "python3",
  "code": "print('blocked')",
  "enable_network": true
}

[Asserts]
status == 400
code == -400
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
@id seed-existing-file
@name Seed existing file for rollback check

POST {{base_url}}/v1/files/upload
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "path": "pkg/ok.txt",
  "content": "cHJldmlvdXM=",
  "encoding": "base64"
}

[Asserts]
status == 200
code == 0
data.path == "pkg/ok.txt"
```

```step
@id reject-zip-slip
@name Reject zip slip archive

POST {{base_url}}/v1/files/upload-archive
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "path": ".",
  "archive_base64": "{{zip_slip_archive_base64}}",
  "format": "zip",
  "strip_root": false
}

[Asserts]
status == 400
code == -400
```

```step
@id reject-symlink-archive
@name Reject symlink archive and rollback

POST {{base_url}}/v1/files/upload-archive
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "path": "pkg",
  "archive_base64": "{{symlink_archive_base64}}",
  "format": "zip",
  "strip_root": false
}

[Asserts]
status == 400
code == -400
```

```step
@id verify-rollback
@name Verify existing file survived failed archive upload

GET {{base_url}}/v1/files/download?sandbox_id={{sandbox_id}}&path=pkg/ok.txt&encoding=base64

[Asserts]
status == 200
code == 0
data.content == "cHJldmlvdXM="
```

```step
@id reject-dangerous-env
@name Reject dangerous command environment

POST {{base_url}}/v1/exec/command
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "command": "python3",
  "args": ["-c", "print('nope')"],
  "env": {
    "LD_PRELOAD": "x"
  },
  "profile": "code-short"
}

[Asserts]
status == 400
code == -400
```

```step
@id reject-unknown-command-profile
@name Reject unknown command profile

POST {{base_url}}/v1/exec/command
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "command": "python3",
  "args": ["-c", "print('nope')"],
  "profile": "unknown-profile"
}

[Asserts]
status == 400
code == -400
```

```step
@id reject-network-run
@name Reject network-enabled code in deny-by-default sandbox

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "language": "python3",
  "code": "print('blocked')",
  "enable_network": true
}

[Asserts]
status == 400
code == -400
```

```step
@id reject-root-delete
@name Reject sandbox root deletion

DELETE {{base_url}}/v1/files?sandbox_id={{sandbox_id}}&path=.

[Asserts]
status == 400
code == -400
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
