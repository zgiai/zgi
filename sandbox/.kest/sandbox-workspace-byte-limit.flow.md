# Sandbox Workspace Byte Limit

```flow
@flow id=sandbox-workspace-byte-limit
@name Sandbox workspace byte limit
```

```step
@id create-workspace-limited-sandbox
@name Create workspace-limited sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "{{workspace_organization_id}}"
}

[Captures]
workspace_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.effective_limits.max_workspace_bytes == 16
data.effective_limits.workspace_byte_limit_enforced == true
```

```step
@id upload-file-below-workspace-limit
@name Upload file below workspace limit

POST {{base_url}}/v1/files/upload
Content-Type: application/json

{
  "sandbox_id": "{{workspace_sandbox_id}}",
  "path": "notes/one.txt",
  "content": "1234567890"
}

[Asserts]
status == 200
code == 0
data.size == 10
```

```step
@id reject-file-above-workspace-limit
@name Reject file above workspace limit

POST {{base_url}}/v1/files/upload
Content-Type: application/json

{
  "sandbox_id": "{{workspace_sandbox_id}}",
  "path": "notes/two.txt",
  "content": "1234567890"
}

[Asserts]
status == 429
code == -429
data.error_type == "limit_exceeded"
data.code == "workspace_byte_limit_exceeded"
data.limit == "max_workspace_bytes"
data.maximum == 16
data.actual == 20
data.workspace_bytes == 20
```

```step
@id rejected-file-was-not-written
@name Rejected file was not written

GET {{base_url}}/v1/files/info?sandbox_id={{workspace_sandbox_id}}&path=notes/two.txt

[Asserts]
status == 400
code == -400
```

```step
@id reject-command-generated-workspace-growth
@name Reject command-generated workspace growth

POST {{base_url}}/v1/exec/command
Content-Type: application/json
X-Request-ID: req_kest_workspace_command

{
  "sandbox_id": "{{workspace_sandbox_id}}",
  "command": "python3",
  "args": ["-c", "open('generated.txt', 'w').write('1234567890')"],
  "profile": "code-short"
}

[Asserts]
status == 429
code == -429
data.error_type == "limit_exceeded"
data.code == "workspace_byte_limit_exceeded"
data.limit == "max_workspace_bytes"
data.maximum == 16
data.actual == 20
data.workspace_bytes == 20
```

```step
@id observer-workspace-limit-event
@name Observer workspace limit event

GET {{base_url}}/v1/observer/events?sandbox_id={{workspace_sandbox_id}}&type=exec.command.failed&request_id=req_kest_workspace_command&limit=1

[Asserts]
status == 200
code == 0
data.events.0.metadata.status == "failure"
data.events.0.metadata.error_type == "limit_exceeded"
data.events.0.metadata.code == "workspace_byte_limit_exceeded"
data.events.0.metadata.limit == "max_workspace_bytes"
data.events.0.metadata.organization_id == "{{workspace_organization_id}}"
data.events.0.metadata.request_id == "req_kest_workspace_command"
```

```step
@id delete-workspace-limited-sandbox
@name Delete workspace-limited sandbox

DELETE {{base_url}}/v1/sandboxes/{{workspace_sandbox_id}}

[Asserts]
status == 200
code == 0
```
