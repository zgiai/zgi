# Sandbox Execution Timeouts

```flow
@flow id=sandbox-execution-timeouts
@name Sandbox execution timeout behavior
```

```step
@id stateless-code-timeout
@name Stateless code returns timeout exit code

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "language": "python3",
  "profile": "code-short",
  "timeout_ms": 50,
  "stdout_limit_kb": 64,
  "stderr_limit_kb": 64,
  "code": "import time\nprint('started')\ntime.sleep(1)",
  "enable_network": false
}

[Asserts]
status == 200
code == 0
data.exit_code == 124
data.network_requested == false
```

```step
@id create-timeout-sandbox
@name Create sandbox for command timeout

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
@id stateless-code-cpu-bound-timeout
@name CPU-bound stateless code returns timeout exit code

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "language": "python3",
  "profile": "code-short",
  "timeout_ms": 50,
  "stdout_limit_kb": 64,
  "stderr_limit_kb": 64,
  "code": "while True:\n    pass",
  "enable_network": false
}

[Asserts]
status == 200
code == 0
data.exit_code == 124
data.network_requested == false
```

```step
@id command-timeout
@name Command returns timeout exit code

POST {{base_url}}/v1/exec/command
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "command": "python3",
  "args": ["-c", "import time; print('started'); time.sleep(1)"],
  "profile": "code-short",
  "timeout_ms": 50,
  "stdout_limit_kb": 64,
  "stderr_limit_kb": 64
}

[Asserts]
status == 200
code == 0
data.exit_code == 124
data.command == "python3"
```

```step
@id command-cpu-bound-timeout
@name CPU-bound command returns timeout exit code

POST {{base_url}}/v1/exec/command
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "command": "python3",
  "args": ["-c", "while True: pass"],
  "profile": "code-short",
  "timeout_ms": 50,
  "stdout_limit_kb": 64,
  "stderr_limit_kb": 64
}

[Asserts]
status == 200
code == 0
data.exit_code == 124
data.command == "python3"
```

```step
@id delete-timeout-sandbox
@name Delete timeout sandbox

DELETE {{base_url}}/v1/sandboxes/{{sandbox_id}}

[Asserts]
status == 200
code == 0
data.deleted == true
```
