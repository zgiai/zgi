# Sandbox Profile Concurrent Execution Limit

```flow
@flow id=sandbox-profile-concurrent-execution-limit
@name Sandbox profile concurrent execution limit
```

```step
@id inspect-profile-concurrent-execution-limit
@name Inspect profile concurrent execution limit

GET {{base_url}}/v1/policies

[Asserts]
status == 200
code == 0
data.limits.max_concurrent_executions_per_profile == 1
data.limits.max_concurrent_executions == 1
```

```step
@id create-profile-limited-sandbox
@name Create profile-limited sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "{{profile_organization_id}}"
}

[Captures]
profile_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.organization_id == "{{profile_organization_id}}"
data.effective_limits.max_concurrent_executions_per_profile == 1
data.effective_limits.max_concurrent_executions == 1
```

```step
@id execute-command-under-profile-limit
@name Execute command under profile limit

POST {{base_url}}/v1/exec/command
Content-Type: application/json
X-Request-ID: req_kest_profile_execution_command

{
  "sandbox_id": "{{profile_sandbox_id}}",
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
@id observer-profile-command-event
@name Observer profile command event

GET {{base_url}}/v1/observer/events?sandbox_id={{profile_sandbox_id}}&type=exec.command&request_id=req_kest_profile_execution_command&limit=1

[Asserts]
status == 200
code == 0
data.events.0.metadata.status == "success"
data.events.0.metadata.profile == "code-short"
data.events.0.metadata.organization_id == "{{profile_organization_id}}"
data.events.0.metadata.request_id == "req_kest_profile_execution_command"
```

```step
@id delete-profile-limited-sandbox
@name Delete profile-limited sandbox

DELETE {{base_url}}/v1/sandboxes/{{profile_sandbox_id}}

[Asserts]
status == 200
code == 0
```
