# Sandbox Organization Dependency Profile Limit

```flow
@flow id=sandbox-organization-dependency-profile-limit
@name Sandbox organization dependency profile limit
```

```step
@id inspect-dependency-profile-limit-policy
@name Inspect dependency profile limit policy

GET {{base_url}}/v1/policies

[Asserts]
status == 200
code == 0
data.limits.max_dependency_profiles_per_organization == 1
data.limits.organization_dependency_profile_limit_enforced == true
```

```step
@id create-first-profile-sandbox
@name Create first organization dependency profile sandbox

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "{{dependency_profile_limit_organization_id}}",
  "dependency_profile": "workflow-safe"
}

[Captures]
first_profile_limit_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.dependency_profile == "workflow-safe"
data.effective_limits.max_dependency_profiles_per_organization == 1
data.effective_limits.organization_dependency_profile_limit_enforced == true
```

```step
@id reuse-existing-profile-sandbox
@name Reuse active organization dependency profile

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "{{dependency_profile_limit_organization_id}}",
  "dependency_profile": "workflow-safe"
}

[Captures]
reused_profile_limit_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.dependency_profile == "workflow-safe"
```

```step
@id reject-new-organization-profile
@name Reject new organization dependency profile above limit

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "{{dependency_profile_limit_organization_id}}",
  "dependency_profile": "node-basic"
}

[Asserts]
status == 429
code == -429
data.error_type == "limit_exceeded"
data.code == "organization_dependency_profile_limit_exceeded"
data.limit == "max_dependency_profiles_per_organization"
data.maximum == 1
data.actual == 2
data.organization_id == "{{dependency_profile_limit_organization_id}}"
data.dependency_profile == "node-basic"
```

```step
@id create-other-organization-profile
@name Create different organization dependency profile

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "{{dependency_profile_limit_organization_id}}_other",
  "dependency_profile": "node-basic"
}

[Captures]
other_profile_limit_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.organization_id == "{{dependency_profile_limit_organization_id}}_other"
data.dependency_profile == "node-basic"
```

```step
@id delete-first-profile-limit-sandbox
@name Delete first dependency profile limit sandbox

DELETE {{base_url}}/v1/sandboxes/{{first_profile_limit_sandbox_id}}?organization_id={{dependency_profile_limit_organization_id}}

[Asserts]
status == 200
code == 0
```

```step
@id delete-reused-profile-limit-sandbox
@name Delete reused dependency profile limit sandbox

DELETE {{base_url}}/v1/sandboxes/{{reused_profile_limit_sandbox_id}}?organization_id={{dependency_profile_limit_organization_id}}

[Asserts]
status == 200
code == 0
```

```step
@id delete-other-profile-limit-sandbox
@name Delete other dependency profile limit sandbox

DELETE {{base_url}}/v1/sandboxes/{{other_profile_limit_sandbox_id}}?organization_id={{dependency_profile_limit_organization_id}}_other

[Asserts]
status == 200
code == 0
```
