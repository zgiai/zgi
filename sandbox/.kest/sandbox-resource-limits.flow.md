# Sandbox Resource Limit Surface

```flow
@flow id=sandbox-resource-limits
@name Sandbox resource limit surface and structured failures
```

```step
@id policy-resource-limits
@name Inspect effective resource limits

GET {{base_url}}/v1/policies

[Asserts]
status == 200
code == 0
data.limits.max_active_sandboxes == 6
data.limits.max_archive_files == 256
data.limits.queue_timeout_ms == 5000
data.limits.workspace_byte_limit_enforced == false
```

```step
@id create-limited-sandbox-1
@name Create limited sandbox 1

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{"runtime_profile":"session","ttl_seconds":60}

[Captures]
sandbox_id_1 = data.id

[Asserts]
status == 200
code == 0
data.effective_limits.max_active_sandboxes == 6
```

```step
@id create-limited-sandbox-2
@name Create limited sandbox 2

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{"runtime_profile":"session","ttl_seconds":60}

[Captures]
sandbox_id_2 = data.id

[Asserts]
status == 200
code == 0
data.effective_limits.max_active_sandboxes == 6
```

```step
@id create-limited-sandbox-3
@name Create limited sandbox 3

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{"runtime_profile":"session","ttl_seconds":60}

[Captures]
sandbox_id_3 = data.id

[Asserts]
status == 200
code == 0
data.effective_limits.max_active_sandboxes == 6
```

```step
@id create-limited-sandbox-4
@name Create limited sandbox 4

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{"runtime_profile":"session","ttl_seconds":60}

[Captures]
sandbox_id_4 = data.id

[Asserts]
status == 200
code == 0
data.effective_limits.max_active_sandboxes == 6
```

```step
@id create-limited-sandbox-5
@name Create limited sandbox 5

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{"runtime_profile":"session","ttl_seconds":60}

[Captures]
sandbox_id_5 = data.id

[Asserts]
status == 200
code == 0
data.effective_limits.max_active_sandboxes == 6
```

```step
@id create-limited-sandbox-6
@name Create limited sandbox 6

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{"runtime_profile":"session","ttl_seconds":60}

[Captures]
sandbox_id_6 = data.id

[Asserts]
status == 200
code == 0
data.effective_limits.max_active_sandboxes == 6
```

```step
@id reject-active-sandbox-limit
@name Reject active sandbox limit

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{"runtime_profile":"session","ttl_seconds":60}

[Asserts]
status == 429
code == -429
data.error_type == "limit_exceeded"
data.code == "active_sandbox_limit_exceeded"
data.limit == "max_active_sandboxes"
data.maximum == 6
data.actual == 7
```

```step
@id delete-limited-sandbox-1
@name Delete limited sandbox 1

DELETE {{base_url}}/v1/sandboxes/{{sandbox_id_1}}

[Asserts]
status == 200
code == 0
```

```step
@id delete-limited-sandbox-2
@name Delete limited sandbox 2

DELETE {{base_url}}/v1/sandboxes/{{sandbox_id_2}}

[Asserts]
status == 200
code == 0
```

```step
@id delete-limited-sandbox-3
@name Delete limited sandbox 3

DELETE {{base_url}}/v1/sandboxes/{{sandbox_id_3}}

[Asserts]
status == 200
code == 0
```

```step
@id delete-limited-sandbox-4
@name Delete limited sandbox 4

DELETE {{base_url}}/v1/sandboxes/{{sandbox_id_4}}

[Asserts]
status == 200
code == 0
```

```step
@id delete-limited-sandbox-5
@name Delete limited sandbox 5

DELETE {{base_url}}/v1/sandboxes/{{sandbox_id_5}}

[Asserts]
status == 200
code == 0
```

```step
@id delete-limited-sandbox-6
@name Delete limited sandbox 6

DELETE {{base_url}}/v1/sandboxes/{{sandbox_id_6}}

[Asserts]
status == 200
code == 0
```
