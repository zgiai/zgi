# Sandbox Organization Execution Rate

```flow
@flow id=sandbox-organization-execution-rate
@name Sandbox organization execution rate limit
```

```step
@id create-rate-sandbox
@name Create rate-limited sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "{{rate_organization_id}}"
}

[Captures]
rate_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.effective_limits.max_executions_per_minute_per_organization == 1
```

```step
@id run-first-organization-command
@name Run first organization command

POST {{base_url}}/v1/exec/command
Content-Type: application/json
X-Request-ID: req_kest_rate_first

{
  "sandbox_id": "{{rate_sandbox_id}}",
  "command": "python3",
  "args": ["-c", "print('first')"],
  "profile": "code-short"
}

[Asserts]
status == 200
code == 0
data.exit_code == 0
```

```step
@id reject-second-organization-command
@name Reject second organization command

POST {{base_url}}/v1/exec/command
Content-Type: application/json
X-Request-ID: req_kest_rate_second

{
  "sandbox_id": "{{rate_sandbox_id}}",
  "command": "python3",
  "args": ["-c", "print('second')"],
  "profile": "code-short"
}

[Asserts]
status == 429
code == -429
data.code == "organization_execution_rate_limit_exceeded"
data.limit == "max_executions_per_minute_per_organization"
data.maximum == 1
data.organization_id == "{{rate_organization_id}}"
```

```step
@id observer-rate-limit-event
@name Observer rate limit event

GET {{base_url}}/v1/observer/events?sandbox_id={{rate_sandbox_id}}&type=exec.command.failed&request_id=req_kest_rate_second&limit=1

[Asserts]
status == 200
code == 0
data.events.0.metadata.code == "organization_execution_rate_limit_exceeded"
data.events.0.metadata.organization_id == "{{rate_organization_id}}"
data.events.0.metadata.request_id == "req_kest_rate_second"
```

```step
@id delete-rate-sandbox
@name Delete rate-limited sandbox

DELETE {{base_url}}/v1/sandboxes/{{rate_sandbox_id}}

[Asserts]
status == 200
code == 0
data.deleted == true
```
