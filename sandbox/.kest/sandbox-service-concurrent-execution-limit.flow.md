# Sandbox Service Concurrent Execution Limit

```flow
@flow id=sandbox-service-concurrent-execution-limit
@name Sandbox service concurrent execution limit
```

```step
@id inspect-service-concurrent-execution-limit
@name Inspect service concurrent execution limit

GET {{base_url}}/v1/policies

[Asserts]
status == 200
code == 0
data.limits.max_concurrent_executions == 1
```

```step
@id create-service-limited-sandbox
@name Create service-limited sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "{{service_organization_id}}"
}

[Captures]
service_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.organization_id == "{{service_organization_id}}"
data.effective_limits.max_concurrent_executions == 1
```

```step
@id execute-command-under-service-limit
@name Execute command under service limit

POST {{base_url}}/v1/exec/command
Content-Type: application/json
X-Request-ID: req_kest_service_execution_command

{
  "sandbox_id": "{{service_sandbox_id}}",
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
@id observer-service-command-event
@name Observer service command event

GET {{base_url}}/v1/observer/events?sandbox_id={{service_sandbox_id}}&type=exec.command&request_id=req_kest_service_execution_command&limit=1

[Asserts]
status == 200
code == 0
data.events.0.metadata.status == "success"
data.events.0.metadata.organization_id == "{{service_organization_id}}"
data.events.0.metadata.request_id == "req_kest_service_execution_command"
```

```step
@id delete-service-limited-sandbox
@name Delete service-limited sandbox

DELETE {{base_url}}/v1/sandboxes/{{service_sandbox_id}}

[Asserts]
status == 200
code == 0
```
