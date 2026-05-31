# Sandbox Organization Queued Execution Limit

```flow
@flow id=sandbox-organization-queued-execution-limit
@name Sandbox organization queued execution limit
```

```step
@id inspect-organization-queued-execution-limit
@name Inspect organization queued execution limit

GET {{base_url}}/v1/policies

[Asserts]
status == 200
code == 0
data.limits.max_concurrent_executions_per_organization == 1
data.limits.max_queued_executions_per_organization == 1
```

```step
@id create-queued-limited-sandbox
@name Create queued-limited sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "{{queued_organization_id}}"
}

[Captures]
queued_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.organization_id == "{{queued_organization_id}}"
data.effective_limits.max_concurrent_executions_per_organization == 1
data.effective_limits.max_queued_executions_per_organization == 1
```

```step
@id execute-command-under-queued-limit-config
@name Execute command under queued limit config

POST {{base_url}}/v1/exec/command
Content-Type: application/json
X-Request-ID: req_kest_queued_execution_command

{
  "sandbox_id": "{{queued_sandbox_id}}",
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
@id observer-queued-command-event
@name Observer queued command event

GET {{base_url}}/v1/observer/events?sandbox_id={{queued_sandbox_id}}&type=exec.command&request_id=req_kest_queued_execution_command&limit=1

[Asserts]
status == 200
code == 0
data.events.0.metadata.status == "success"
data.events.0.metadata.organization_id == "{{queued_organization_id}}"
data.events.0.metadata.request_id == "req_kest_queued_execution_command"
```

```step
@id delete-queued-limited-sandbox
@name Delete queued-limited sandbox

DELETE {{base_url}}/v1/sandboxes/{{queued_sandbox_id}}

[Asserts]
status == 200
code == 0
```
