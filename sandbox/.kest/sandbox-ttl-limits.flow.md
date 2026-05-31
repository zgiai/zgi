# Sandbox TTL Limits

```flow
@flow id=sandbox-ttl-limits
@name Sandbox TTL limit clamping
```

```step
@id policy-ttl-limits
@name Inspect TTL limits

GET {{base_url}}/v1/policies

[Asserts]
status == 200
code == 0
data.limits.session_ttl_seconds == 1800
data.limits.interactive_ttl_seconds == 3600
data.limits.max_compat_ttl_seconds == 300
```

```step
@id create-session-above-max-ttl
@name Create session sandbox clamps requested TTL

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 999999,
  "dependency_profile": "stdlib",
  "network_enabled": false,
  "network_policy": "deny-by-default"
}

[Captures]
ttl_session_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.ttl_seconds == 1800
data.effective_limits.session_ttl_seconds == 1800
```

```step
@id renew-session-above-max-ttl
@name Renew session sandbox clamps requested TTL

POST {{base_url}}/v1/sandboxes/{{ttl_session_sandbox_id}}/renew-expiration
Content-Type: application/json

{
  "ttl_seconds": 999999
}

[Asserts]
status == 200
code == 0
data.ttl_seconds == 1800
data.status == "active"
```

```step
@id create-interactive-above-max-ttl
@name Create interactive sandbox clamps requested TTL

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "interactive",
  "ttl_seconds": 999999,
  "dependency_profile": "stdlib",
  "network_enabled": false,
  "network_policy": "deny-by-default"
}

[Captures]
ttl_interactive_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.ttl_seconds == 3600
data.effective_limits.interactive_ttl_seconds == 3600
```

```step
@id delete-session-ttl-sandbox
@name Delete session TTL sandbox

DELETE {{base_url}}/v1/sandboxes/{{ttl_session_sandbox_id}}

[Asserts]
status == 200
code == 0
data.deleted == true
```

```step
@id delete-interactive-ttl-sandbox
@name Delete interactive TTL sandbox

DELETE {{base_url}}/v1/sandboxes/{{ttl_interactive_sandbox_id}}

[Asserts]
status == 200
code == 0
data.deleted == true
```
