# Sandbox Policy Deny Audit

```flow
@flow id=sandbox-policy-deny-audit
@name Sandbox policy deny audit events
```

```step
@id reject-preview-network-request
@name Reject preview network request

POST {{base_url}}/v1/sandbox/run
Content-Type: application/json
X-Request-ID: req_kest_policy_network

{
  "language": "python3",
  "code": "print('blocked')",
  "enable_network": true
}

[Asserts]
status == 400
code == -400
```

```step
@id inspect-network-policy-deny-event
@name Inspect network policy deny event

GET {{base_url}}/v1/observer/events?type=policy.denied&request_id=req_kest_policy_network&limit=1

[Asserts]
status == 200
code == 0
data.events.0.type == "policy.denied"
data.events.0.metadata.code == "network_policy_not_enforced"
data.events.0.metadata.error_type == "policy_denied"
data.events.0.metadata.request_id == "req_kest_policy_network"
```

```step
@id create-policy-audit-sandbox
@name Create policy audit sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "organization_policy_audit_owner",
  "workspace_id": "workspace_policy_audit",
  "workflow_run_id": "run_policy_audit"
}

[Captures]
policy_audit_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.organization_id == "organization_policy_audit_owner"
```

```step
@id reject-cross-organization-policy-request
@name Reject cross-organization policy request

GET {{base_url}}/v1/sandboxes/{{policy_audit_sandbox_id}}?organization_id=organization_policy_audit_other
X-Request-ID: req_kest_policy_cross_organization

[Asserts]
status == 403
code == -403
data.code == "cross_organization_sandbox_access_denied"
data.organization_id == "organization_policy_audit_other"
```

```step
@id inspect-cross-organization-policy-deny-event
@name Inspect cross-organization policy deny event

GET {{base_url}}/v1/observer/events?sandbox_id={{policy_audit_sandbox_id}}&type=policy.denied&request_id=req_kest_policy_cross_organization&limit=1

[Asserts]
status == 200
code == 0
data.events.0.type == "policy.denied"
data.events.0.metadata.code == "cross_organization_sandbox_access_denied"
data.events.0.metadata.organization_id == "organization_policy_audit_owner"
data.events.0.metadata.requested_organization_id == "organization_policy_audit_other"
data.events.0.metadata.request_id == "req_kest_policy_cross_organization"
```

```step
@id delete-policy-audit-sandbox
@name Delete policy audit sandbox

DELETE {{base_url}}/v1/sandboxes/{{policy_audit_sandbox_id}}?organization_id=organization_policy_audit_owner

[Asserts]
status == 200
code == 0
```
