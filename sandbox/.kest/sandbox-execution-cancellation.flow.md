# Sandbox Execution Cancellation

```flow
@flow id=sandbox-execution-cancellation
@name Sandbox execution cancellation cleanup
```

```step
@id cancellation-releases-runner
@name Canceled request releases runner worker

GET {{base_url}}/v1/metrics

[Asserts]
status == 200
code == 0
data.runner.active_workers == 0
data.runner.queued_executions == 0
```

```step
@id observer-records-command-cancellation
@name Observer records canceled command

GET {{base_url}}/v1/observer/events?sandbox_id={{cancellation_sandbox_id}}&type=exec.command.failed&request_id={{cancellation_request_id}}&limit=1

[Asserts]
status == 200
code == 0
data.events.0.metadata.status == "failure"
data.events.0.metadata.error_type == "execution_canceled"
data.events.0.metadata.code == "request_canceled"
data.events.0.metadata.phase == "execution"
data.events.0.metadata.request_id == "{{cancellation_request_id}}"
```

```step
@id delete-cancellation-sandbox
@name Delete cancellation sandbox

DELETE {{base_url}}/v1/sandboxes/{{cancellation_sandbox_id}}

[Asserts]
status == 200
code == 0
data.deleted == true
```
