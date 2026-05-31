# Sandbox Execution Queue Timeout

```flow
@flow id=sandbox-execution-queue-timeout
@name Sandbox execution queue timeout
```

```step
@id inspect-queue-timeout-policy
@name Inspect queue timeout policy

GET {{base_url}}/v1/policies

[Asserts]
status == 200
code == 0
data.limits.queue_timeout_ms == 100
data.limits.max_concurrent_executions == 0
data.limits.max_concurrent_executions_per_profile == 0
data.limits.max_concurrent_executions_per_organization == 1
data.limits.max_queued_executions_per_organization == 1
```

```step
@id reject-command-after-queue-timeout
@name Reject command after queue timeout

POST {{base_url}}/v1/exec/command
Content-Type: application/json
X-Request-ID: req_kest_queue_timeout

{
  "sandbox_id": "{{queue_timeout_sandbox_id}}",
  "command": "python3",
  "args": ["-c", "print('queued')"],
  "profile": "code-short",
  "timeout_ms": 5000
}

[Asserts]
status == 429
code == -429
data.error_type == "limit_exceeded"
data.code == "organization_execution_queue_timeout"
data.limit == "queue_timeout_ms"
data.maximum == 100
```

```step
@id observer-queue-timeout-event
@name Observer records queue timeout

GET {{base_url}}/v1/observer/events?sandbox_id={{queue_timeout_sandbox_id}}&type=exec.command.failed&request_id=req_kest_queue_timeout&limit=1

[Asserts]
status == 200
code == 0
data.events.0.metadata.status == "failure"
data.events.0.metadata.error_type == "limit_exceeded"
data.events.0.metadata.code == "organization_execution_queue_timeout"
data.events.0.metadata.limit == "queue_timeout_ms"
data.events.0.metadata.runtime_backend == "preview-process"
data.events.0.metadata.organization_id == "{{queue_timeout_organization_id}}"
data.events.0.metadata.request_id == "req_kest_queue_timeout"
```
