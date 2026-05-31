# Sandbox Egress Policy Decision

```flow
@flow id=sandbox-egress-policy-decision
@name Sandbox egress policy decision audit
```

```step
@id create-egress-decision-sandbox
@name Create sandbox with network disabled

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "organization_egress_decision_kest",
  "workspace_id": "workspace_egress_decision_kest",
  "dependency_profile": "stdlib",
  "network_enabled": false,
  "network_policy": "workflow-safe"
}

[Captures]
egress_decision_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.organization_id == "organization_egress_decision_kest"
data.network_enabled == false
data.network_policy == "workflow-safe"
```

```step
@id deny-egress-when-sandbox-network-disabled
@name Deny egress when sandbox network is disabled

POST {{base_url}}/v1/network/egress/check
Content-Type: application/json
X-Request-ID: req_kest_egress_network_disabled

{
  "sandbox_id": "{{egress_decision_sandbox_id}}",
  "organization_id": "organization_egress_decision_kest",
  "destination": "https://93.184.216.34/resource"
}

[Asserts]
status == 200
code == 0
data.allowed == false
data.code == "egress_denied_sandbox_network_disabled"
data.reason == "sandbox network access is disabled"
data.policy == "workflow-safe"
data.destination == "https://93.184.216.34/resource"
```

```step
@id inspect-egress-network-disabled-event
@name Inspect egress decision observer event

GET {{base_url}}/v1/observer/events?sandbox_id={{egress_decision_sandbox_id}}&type=network.egress.decision&request_id=req_kest_egress_network_disabled&limit=1

[Asserts]
status == 200
code == 0
data.events.0.type == "network.egress.decision"
data.events.0.metadata.allowed == "false"
data.events.0.metadata.code == "egress_denied_sandbox_network_disabled"
data.events.0.metadata.network_policy == "workflow-safe"
data.events.0.metadata.organization_id == "organization_egress_decision_kest"
data.events.0.metadata.workspace_id == "workspace_egress_decision_kest"
data.events.0.metadata.request_id == "req_kest_egress_network_disabled"
```

```step
@id reject-cross-organization-egress-check
@name Reject cross-organization egress check

POST {{base_url}}/v1/network/egress/check
Content-Type: application/json
X-Request-ID: req_kest_egress_cross_organization

{
  "sandbox_id": "{{egress_decision_sandbox_id}}",
  "organization_id": "organization_egress_decision_other",
  "destination": "https://93.184.216.34/resource"
}

[Asserts]
status == 403
code == -403
data.code == "cross_organization_sandbox_access_denied"
data.organization_id == "organization_egress_decision_other"
```

```step
@id delete-egress-decision-sandbox
@name Delete egress decision sandbox

DELETE {{base_url}}/v1/sandboxes/{{egress_decision_sandbox_id}}?organization_id=organization_egress_decision_kest

[Asserts]
status == 200
code == 0
```
