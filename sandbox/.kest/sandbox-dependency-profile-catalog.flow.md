# Sandbox Dependency Profile Catalog

```flow
@flow id=sandbox-dependency-profile-catalog
@name Sandbox dependency profile catalog and selection policy
```

```step
@id inspect-dependency-catalog
@name Inspect dependency catalog

GET {{base_url}}/v1/sandbox/dependencies?language=python3

[Asserts]
status == 200
code == 0
data.mode == "managed-profiles"
data.supports_user_update == false
data.profiles.0.name == "stdlib"
data.profiles.0.version == "2026.05.01"
data.profiles.0.status == "ready"
data.profiles.0.enabled == true
data.profiles.0.owner_scope == "global"
data.profiles.0.base_runtime == "preview-process"
data.package_policy.mode == "managed-build-only"
data.package_policy.enforced == true
data.package_policy.default_action == "deny-unlisted"
data.package_policy.allowed_packages.0.ecosystem == "python3"
data.package_policy.allowed_packages.0.name == "data-tools"
data.package_policy.denied_packages.0.name == "remote-url"
data.build_policy.mode == "operator-managed"
data.build_policy.enforced == true
data.build_policy.max_profile_size_bytes == 536870912
data.build_policy.build_timeout_seconds == 600
data.build_policy.builds_allowed_during_runtime == false
data.profiles.0.size_bytes == 33554432
```

```step
@id create-versioned-profile-sandbox
@name Create sandbox with versioned dependency profile

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "organization_id": "organization_dependency_profile_kest",
  "dependency_profile": "workflow-safe"
}

[Captures]
profile_sandbox_id = data.id

[Asserts]
status == 200
code == 0
data.dependency_profile == "workflow-safe"
data.dependency_profile_version == "2026.05.01"
data.metadata.dependency_profile_version == "2026.05.01"
```

```step
@id get-versioned-profile-sandbox
@name Get versioned dependency profile sandbox

GET {{base_url}}/v1/sandboxes/{{profile_sandbox_id}}

[Asserts]
status == 200
code == 0
data.dependency_profile == "workflow-safe"
data.dependency_profile_version == "2026.05.01"
data.metadata.dependency_profile_version == "2026.05.01"
```

```step
@id execute-versioned-profile-code
@name Execute code in versioned dependency profile sandbox

POST {{base_url}}/v1/exec/code
Content-Type: application/json
X-Request-ID: req_kest_dependency_profile_execution

{
  "sandbox_id": "{{profile_sandbox_id}}",
  "language": "python3",
  "profile": "code-short",
  "code": "print('dependency-profile-ok')",
  "enable_network": false
}

[Asserts]
status == 200
code == 0
data.exit_code == 0
```

```step
@id reject-runtime-pip-install
@name Reject runtime dependency install

POST {{base_url}}/v1/exec/command
Content-Type: application/json
X-Request-ID: req_kest_dependency_install_rejected

{
  "sandbox_id": "{{profile_sandbox_id}}",
  "command": "python3",
  "args": ["-m", "pip", "install", "requests"],
  "profile": "skill-python",
  "timeout_ms": 30000,
  "stdout_limit_kb": 1024,
  "stderr_limit_kb": 1024
}

[Asserts]
status == 400
code == -400
message == "runtime dependency installation is disabled for managed dependency profiles"
data.error_type == "policy_denied"
data.code == "dependency_install_disabled"
data.package_manager == "pip"
data.action == "install"
```

```step
@id observer-runtime-pip-install-rejection
@name Observer records runtime dependency install rejection

GET {{base_url}}/v1/observer/events?sandbox_id={{profile_sandbox_id}}&type=exec.command.failed&request_id=req_kest_dependency_install_rejected&limit=1

[Asserts]
status == 200
code == 0
data.events.0.metadata.status == "failure"
data.events.0.metadata.error_type == "policy_denied"
data.events.0.metadata.code == "dependency_install_disabled"
data.events.0.metadata.package_manager == "pip"
data.events.0.metadata.action == "install"
data.events.0.metadata.dependency_profile == "workflow-safe"
data.events.0.metadata.dependency_profile_version == "2026.05.01"
```

```step
@id observer-versioned-profile-execution
@name Observer records execution dependency profile version

GET {{base_url}}/v1/observer/events?sandbox_id={{profile_sandbox_id}}&type=exec.code&request_id=req_kest_dependency_profile_execution&limit=1

[Asserts]
status == 200
code == 0
data.events.0.metadata.status == "success"
data.events.0.metadata.dependency_profile == "workflow-safe"
data.events.0.metadata.dependency_profile_version == "2026.05.01"
data.events.0.metadata.request_id == "req_kest_dependency_profile_execution"
```

```step
@id reject-disabled-profile
@name Reject disabled dependency profile

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "dependency_profile": "python-data-preview"
}

[Asserts]
status == 400
code == -400
message == "dependency profile is not enabled: python-data-preview"
```

```step
@id reject-unknown-profile
@name Reject unknown dependency profile

POST {{base_url}}/v1/sandboxes
Content-Type: application/json

{
  "runtime_profile": "session",
  "ttl_seconds": 60,
  "dependency_profile": "missing-profile"
}

[Asserts]
status == 400
code == -400
message == "unsupported dependency profile: missing-profile"
```

```step
@id delete-versioned-profile-sandbox
@name Delete versioned dependency profile sandbox

DELETE {{base_url}}/v1/sandboxes/{{profile_sandbox_id}}

[Asserts]
status == 200
code == 0
```
