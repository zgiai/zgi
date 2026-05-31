# Sandbox Organization Concurrent Execution Limit

```flow
@flow id=sandbox-organization-concurrent-execution-limit
@name Sandbox organization concurrent execution limit
```

```step
@id inspect-organization-concurrent-execution-limit
@name Inspect organization concurrent execution limit

GET {{base_url}}/v1/policies

[Asserts]
status == 200
code == 0
data.limits.max_concurrent_executions_per_organization == 1
```

```step
@id create-concurrent-limited-sandbox
@name Create concurrent-limited sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "{{concurrent_organization_id}}"
}

[Captures]
concurrent_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.organization_id == "{{concurrent_organization_id}}"
data.effective_limits.max_concurrent_executions_per_organization == 1
```

```step
@id execute-command-under-concurrent-limit
@name Execute command under concurrent limit

POST {{base_url}}/v1/exec/command
Content-Type: application/json
X-Request-ID: req_kest_concurrent_execution_command

{
  "sandbox_id": "{{concurrent_sandbox_id}}",
  "command": "python3",
  "args": ["-c", "print('ok')"],
  "profile": "code-short"
}

[Asserts]
status == 200
code == 0
data.exit_code == 0
data.execution_id != ""
```

```step
@id observer-concurrent-command-event
@name Observer concurrent command event

GET {{base_url}}/v1/observer/events?sandbox_id={{concurrent_sandbox_id}}&type=exec.command&request_id=req_kest_concurrent_execution_command&limit=1

[Asserts]
status == 200
code == 0
data.events.0.metadata.status == "success"
data.events.0.metadata.organization_id == "{{concurrent_organization_id}}"
data.events.0.metadata.request_id == "req_kest_concurrent_execution_command"
```

```step
@id delete-concurrent-limited-sandbox
@name Delete concurrent-limited sandbox

DELETE {{base_url}}/v1/sandboxes/{{concurrent_sandbox_id}}

[Asserts]
status == 200
code == 0
```
