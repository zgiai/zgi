# Sandbox Organization Access Scope

```flow
@flow id=sandbox-organization-access-scope
@name Sandbox organization access scope enforcement
```

```step
@id create-owned-sandbox
@name Create organization-owned sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "organization_scope_owner",
  "workspace_id": "workspace_scope",
  "workflow_run_id": "run_scope"
}

[Captures]
scope_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.organization_id == "organization_scope_owner"
```

```step
@id reject-cross-organization-get
@name Reject cross-organization sandbox get

GET {{base_url}}/v1/sandboxes/{{scope_sandbox_id}}?organization_id=organization_scope_other

[Asserts]
status == 403
code == -403
data.code == "cross_organization_sandbox_access_denied"
data.organization_id == "organization_scope_other"
```

```step
@id reject-cross-organization-exec
@name Reject cross-organization execution

POST {{base_url}}/v1/exec/code
Content-Type: application/json

{
  "sandbox_id": "{{scope_sandbox_id}}",
  "organization_id": "organization_scope_other",
  "language": "python3",
  "code": "print('blocked')",
  "enable_network": false
}

[Asserts]
status == 403
code == -403
data.code == "cross_organization_sandbox_access_denied"
data.organization_id == "organization_scope_other"
```

```step
@id reject-cross-organization-files
@name Reject cross-organization file access

GET {{base_url}}/v1/files/tree?sandbox_id={{scope_sandbox_id}}
X-ZGI-Organization-ID: organization_scope_other

[Asserts]
status == 403
code == -403
data.code == "cross_organization_sandbox_access_denied"
data.organization_id == "organization_scope_other"
```

```step
@id hide-cross-organization-list
@name Hide sandbox from other organization list

GET {{base_url}}/v1/sandboxes?organization_id=organization_scope_other

[Asserts]
status == 200
code == 0
```

```step
@id allow-owner-sandbox-get
@name Allow owner organization sandbox get

GET {{base_url}}/v1/sandboxes/{{scope_sandbox_id}}?organization_id=organization_scope_owner

[Asserts]
status == 200
code == 0
data.id == "{{scope_sandbox_id}}"
data.organization_id == "organization_scope_owner"
```

```step
@id delete-owned-sandbox
@name Delete organization-owned sandbox

DELETE {{base_url}}/v1/sandboxes/{{scope_sandbox_id}}?organization_id=organization_scope_owner

[Asserts]
status == 200
code == 0
```
