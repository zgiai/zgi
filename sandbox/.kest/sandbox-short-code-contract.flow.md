# Sandbox Short Code Contract

```flow
@flow id=sandbox-short-code-contract
@name Sandbox short-code contract
```

```step
@id inspect-short-code-policy
@name Inspect short-code stateless policy

GET {{base_url}}/v1/policies

[Asserts]
status == 200
code == 0
data.command_profiles.0.name == "code-short"
data.command_profiles.0.stateless == true
data.command_profiles.0.max_request_bytes == 131072
data.command_profiles.0.max_result_json_bytes == 65536
data.command_profiles.0.network_allowed == false
data.command_profiles.0.network == "disabled"
```

```step
@id stateless-create-marker
@name Stateless short code creates temporary marker

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "language": "python3",
  "profile": "code-short",
  "strict_result_json": true,
  "timeout_ms": 5000,
  "code": "import json, pathlib\npathlib.Path('marker.txt').write_text('temporary')\nprint(json.dumps({'created': True}))",
  "enable_network": false
}

[Asserts]
status == 200
code == 0
data.exit_code == 0
data.result_json.created == true
```

```step
@id stateless-clean-workspace
@name Stateless short code gets a clean workspace

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "language": "python3",
  "profile": "code-short",
  "strict_result_json": true,
  "timeout_ms": 5000,
  "code": "import json, pathlib\nprint(json.dumps({'exists': pathlib.Path('marker.txt').exists()}))",
  "enable_network": false
}

[Asserts]
status == 200
code == 0
data.exit_code == 0
data.result_json.exists == false
```

```step
@id stateless-network-rejection
@name Reject network in stateless short code

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "language": "python3",
  "profile": "code-short",
  "timeout_ms": 5000,
  "code": "print('nope')",
  "enable_network": true
}

[Asserts]
status == 400
code == -400
```

```step
@id create-sandbox
@name Create sandbox

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
@id sandbox-scoped-stateless-write
@name Sandbox-scoped short code remains stateless by default

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "language": "python3",
  "profile": "code-short",
  "strict_result_json": true,
  "timeout_ms": 5000,
  "code": "import json, pathlib\npathlib.Path('session-marker.txt').write_text('temporary')\nprint(json.dumps({'created': True}))",
  "enable_network": false
}

[Asserts]
status == 200
code == 0
data.exit_code == 0
data.result_json.created == true
```

```step
@id stateless-write-not-persisted
@name Sandbox-scoped stateless write is not persisted

GET {{base_url}}/v1/files/info?sandbox_id={{sandbox_id}}&path=session-marker.txt

[Asserts]
status == 400
code == -400
```

```step
@id explicitly-bound-short-code-write
@name Explicitly bound short code can write workspace

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "language": "python3",
  "profile": "code-short",
  "strict_result_json": true,
  "bind_workspace": true,
  "timeout_ms": 5000,
  "code": "import json, pathlib\npathlib.Path('session-marker.txt').write_text('bound')\nprint(json.dumps({'created': True}))",
  "enable_network": false
}

[Asserts]
status == 200
code == 0
data.exit_code == 0
data.result_json.created == true
```

```step
@id bound-write-persisted
@name Explicitly bound short code write is persisted

GET {{base_url}}/v1/files/info?sandbox_id={{sandbox_id}}&path=session-marker.txt

[Asserts]
status == 200
code == 0
data.path == "session-marker.txt"
data.size == 5
```

```step
@id structured-json
@name Execute short code with structured JSON result

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "language": "python3",
  "profile": "code-short",
  "input_json": {
    "input": "hello from short code"
  },
  "strict_result_json": true,
  "timeout_ms": 5000,
  "stdout_limit_kb": 64,
  "stderr_limit_kb": 64,
  "code": "import json, sys\npayload = json.loads(sys.stdin.read())\nprint(json.dumps({'echo': payload['input'], 'ok': True}))",
  "enable_network": false
}

[Asserts]
status == 200
code == 0
data.exit_code == 0
data.result_json.echo == "hello from short code"
data.result_json.ok == true
```

```step
@id strict-json-rejection
@name Reject strict JSON when stdout is plain text

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "language": "python3",
  "profile": "code-short",
  "strict_result_json": true,
  "timeout_ms": 5000,
  "code": "print('plain text')",
  "enable_network": false
}

[Asserts]
status == 400
code == -400
```

```step
@id schema-valid-output
@name Validate short-code output schema

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "language": "python3",
  "profile": "code-short",
  "strict_result_json": true,
  "expected_output_schema": {
    "type": "object",
    "required": ["echo", "ok"],
    "additional_properties": false,
    "properties": {
      "echo": {"type": "string"},
      "ok": {"type": "boolean"},
      "count": {"type": "integer"}
    }
  },
  "timeout_ms": 5000,
  "code": "import json\nprint(json.dumps({'echo': 'schema ok', 'ok': True, 'count': 2}))",
  "enable_network": false
}

[Asserts]
status == 200
code == 0
data.exit_code == 0
data.result_json.echo == "schema ok"
data.result_json.ok == true
data.result_json.count == 2
```

```step
@id schema-reject-extra-output
@name Reject short-code output outside schema

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "language": "python3",
  "profile": "code-short",
  "strict_result_json": true,
  "expected_output_schema": {
    "type": "object",
    "required": ["echo", "ok"],
    "additional_properties": false,
    "properties": {
      "echo": {"type": "string"},
      "ok": {"type": "boolean"}
    }
  },
  "timeout_ms": 5000,
  "code": "import json\nprint(json.dumps({'echo': 'schema no', 'ok': True, 'extra': 'blocked'}))",
  "enable_network": false
}

[Asserts]
status == 400
code == -400
```

```step
@id result-json-size-rejection
@name Reject oversized short-code JSON result

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "language": "python3",
  "profile": "code-short",
  "strict_result_json": true,
  "timeout_ms": 5000,
  "stdout_limit_kb": 128,
  "stderr_limit_kb": 64,
  "code": "import json\nprint(json.dumps({'payload': 'x' * 70000}))",
  "enable_network": false
}

[Asserts]
status == 400
code == -400
```

```step
@id output-truncation
@name Truncate short-code stdout at request limit

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "language": "python3",
  "profile": "code-short",
  "timeout_ms": 5000,
  "stdout_limit_kb": 1,
  "stderr_limit_kb": 1,
  "code": "print('x' * 2048)",
  "enable_network": false
}

[Asserts]
status == 200
code == 0
data.exit_code == 0
data.truncated == true
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
