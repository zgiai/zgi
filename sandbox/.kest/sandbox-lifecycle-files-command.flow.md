# Sandbox Lifecycle, Files, and Commands

```flow
@flow id=sandbox-lifecycle-files-command
@name Sandbox lifecycle, file I/O, code, and command execution
```

```step
@id health
@name Health check

GET {{base_url}}/health

[Asserts]
status == 200
runtime_backend == "preview-process"
network_policy_enforced == false
shutdown_timeout_secs == 10
```

```step
@id readiness
@name Readiness check

GET {{base_url}}/ready

[Asserts]
status == 200
service == "zgi-sandbox"
ready == true
checks.postgres == "ok"
checks.runtime == "ok"
```

```step
@id policies
@name Policy catalog

GET {{base_url}}/v1/policies

[Asserts]
status == 200
code == 0
data.limits.runtime_backend == "preview-process"
data.limits.network_policy_enforced == false
data.limits.max_archive_files == 256
data.limits.max_active_sandboxes == 6
data.limits.queue_timeout_ms == 5000
data.limits.output_limit_kb == 1024
data.limits.max_file_size_kb == 256
```

```step
@id dependency-catalog
@name Dependency catalog

GET {{base_url}}/v1/sandbox/dependencies?language=python3

[Asserts]
status == 200
code == 0
```

```step
@id create-sandbox
@name Create session sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 120,
  "organization_id": "organization_kest",
  "workspace_id": "workspace_kest",
  "app_id": "app_kest",
  "workflow_run_id": "workflow_run_kest",
  "user_id": "user_kest",
  "dependency_profile": "stdlib",
  "network_enabled": false,
  "network_policy": "deny-by-default"
}

[Captures]
sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.effective_limits.runtime_backend == "preview-process"
data.effective_limits.max_archive_files == 256
data.effective_limits.max_active_sandboxes == 6
data.effective_limits.queue_timeout_ms == 5000
data.organization_id == "organization_kest"
data.workspace_id == "workspace_kest"
data.app_id == "app_kest"
data.workflow_run_id == "workflow_run_kest"
data.user_id == "user_kest"
```

```step
@id get-sandbox
@name Get sandbox

GET {{base_url}}/v1/sandboxes/{{sandbox_id}}

[Asserts]
status == 200
code == 0
data.status == "active"
data.organization_id == "organization_kest"
data.workspace_id == "workspace_kest"
data.workflow_run_id == "workflow_run_kest"
```

```step
@id renew-sandbox
@name Renew sandbox

POST {{base_url}}/v1/sandboxes/{{sandbox_id}}/renew-expiration
Content-Type: application/json

{
  "ttl_seconds": 180
}

[Asserts]
status == 200
code == 0
data.status == "active"
```

```step
@id upload-file
@name Upload base64 file

POST {{base_url}}/v1/files/upload
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "path": "notes/hello.txt",
  "content": "aGVsbG8ga2VzdAo=",
  "encoding": "base64"
}

[Asserts]
status == 200
code == 0
data.path == "notes/hello.txt"
data.size == 11
```

```step
@id file-info
@name File info

GET {{base_url}}/v1/files/info?sandbox_id={{sandbox_id}}&path=notes/hello.txt

[Asserts]
status == 200
code == 0
data.path == "notes/hello.txt"
data.size == 11
```

```step
@id download-file
@name Download base64 file

GET {{base_url}}/v1/files/download?sandbox_id={{sandbox_id}}&path=notes/hello.txt&encoding=base64

[Asserts]
status == 200
code == 0
data.content == "aGVsbG8ga2VzdAo="
```

```step
@id observer-file-download-event
@name Observer file download event

GET {{base_url}}/v1/observer/events?sandbox_id={{sandbox_id}}&type=files.download&limit=1

[Asserts]
status == 200
code == 0
data.events.0.metadata.path == "notes/hello.txt"
data.events.0.metadata.organization_id == "organization_kest"
data.events.0.metadata.workspace_id == "workspace_kest"
data.events.0.metadata.workflow_run_id == "workflow_run_kest"
```

```step
@id execute-code
@name Execute short Python code

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "language": "python3",
  "code": "print('code-ok')",
  "enable_network": false
}

[Asserts]
status == 200
code == 0
data.exit_code == 0
```

```step
@id execute-command
@name Execute command with stdin and env

POST {{base_url}}/v1/exec/command
Content-Type: application/json
X-Request-ID: req_kest_command

{
  "sandbox_id": "{{sandbox_id}}",
  "command": "python3",
  "args": ["-c", "import os,sys; print(os.environ['ZGI_TEST_TOKEN'] + ':' + sys.stdin.read())"],
  "stdin": "payload",
  "env": {
    "ZGI_TEST_TOKEN": "ok"
  },
  "profile": "code-short",
  "timeout_ms": 5000,
  "stdout_limit_kb": 64,
  "stderr_limit_kb": 64,
  "working_subpath": "."
}

[Asserts]
status == 200
code == 0
data.exit_code == 0
```

```step
@id list-files
@name List files

GET {{base_url}}/v1/files/tree?sandbox_id={{sandbox_id}}

[Asserts]
status == 200
code == 0
```

```step
@id observer-events
@name Observer events

GET {{base_url}}/v1/observer/events?sandbox_id={{sandbox_id}}&limit=20

[Asserts]
status == 200
code == 0
data.events.0.metadata.request_id == "req_kest_command"
data.events.0.metadata.organization_id == "organization_kest"
data.events.0.metadata.workspace_id == "workspace_kest"
data.events.0.metadata.workflow_run_id == "workflow_run_kest"
```

```step
@id observer-events-scope-filter
@name Observer events scope filter

GET {{base_url}}/v1/observer/events?organization_id=organization_kest&workspace_id=workspace_kest&workflow_run_id=workflow_run_kest&limit=5

[Asserts]
status == 200
code == 0
data.events.0.metadata.organization_id == "organization_kest"
data.events.0.metadata.workspace_id == "workspace_kest"
data.events.0.metadata.workflow_run_id == "workflow_run_kest"
```

```step
@id observer-events-request-filter
@name Observer events request filter

GET {{base_url}}/v1/observer/events?sandbox_id={{sandbox_id}}&request_id=req_kest_command&limit=5

[Asserts]
status == 200
code == 0
data.events.0.metadata.request_id == "req_kest_command"
data.events.0.metadata.organization_id == "organization_kest"
data.events.0.metadata.workspace_id == "workspace_kest"
data.events.0.metadata.workflow_run_id == "workflow_run_kest"
```

```step
@id observer-events-page-one
@name Observer events first page

GET {{base_url}}/v1/observer/events?sandbox_id={{sandbox_id}}&limit=1

[Captures]
observer_next_cursor = data.next_cursor

[Asserts]
status == 200
code == 0
data.limit == 1
data.has_more == true
```

```step
@id observer-events-page-two
@name Observer events next page

GET {{base_url}}/v1/observer/events?sandbox_id={{sandbox_id}}&limit=1&before={{observer_next_cursor}}

[Asserts]
status == 200
code == 0
data.limit == 1
```

```step
@id metrics
@name Metrics endpoint

GET {{base_url}}/v1/metrics

[Asserts]
status == 200
code == 0
data.active_sandboxes == 1
data.runner.max_workers == 4
data.runner.active_workers == 0
data.runner.queued_executions == 0
data.observer_retention.retention_days == 7
data.observer_retention.max_events == 10000
```

```step
@id delete-file
@name Delete uploaded file

DELETE {{base_url}}/v1/files?sandbox_id={{sandbox_id}}&path=notes/hello.txt

[Asserts]
status == 200
code == 0
data.deleted == true
```

```step
@id observer-file-delete-event
@name Observer file delete event

GET {{base_url}}/v1/observer/events?sandbox_id={{sandbox_id}}&type=files.delete&limit=1

[Asserts]
status == 200
code == 0
data.events.0.metadata.path == "notes/hello.txt"
data.events.0.metadata.organization_id == "organization_kest"
data.events.0.metadata.workspace_id == "workspace_kest"
data.events.0.metadata.workflow_run_id == "workflow_run_kest"
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
