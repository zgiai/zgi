# Sandbox Template Runtime

```flow
@flow id=sandbox-template-runtime
@name Sandbox template runtime
```

```step
@id template-policy
@name Inspect template profile policy

GET {{base_url}}/v1/policies

[Asserts]
status == 200
code == 0
data.template_profiles.0.profile == "template-short"
data.template_profiles.0.engine == "go-text"
data.template_profiles.0.output_limit_bytes == 65536
data.template_profiles.0.max_variable_depth == 8
```

```step
@id render-template
@name Render bounded template

POST {{base_url}}/v1/exec/template
Content-Type: application/json

{
  "engine": "go-text",
  "profile": "template-short",
  "template": "{{template_render}}",
  "variables": {
    "name": "zgi"
  },
  "organization_id": "organization_template_kest",
  "workspace_id": "workspace_template_kest",
  "app_id": "app_template_kest",
  "workflow_run_id": "workflow_run_template_kest",
  "user_id": "user_template_kest",
  "timeout_ms": 2000,
  "output_limit_kb": 64
}

[Captures]
template_execution_id = data.execution_id

[Asserts]
status == 200
code == 0
data.content == "Hello ZGI"
data.truncated == false
```

```step
@id inspect-template-ownership-event
@name Inspect template ownership event

GET {{base_url}}/v1/observer/events?type=exec.template&organization_id=organization_template_kest&workspace_id=workspace_template_kest&app_id=app_template_kest&workflow_run_id=workflow_run_template_kest&user_id=user_template_kest&limit=1

[Asserts]
status == 200
code == 0
data.events.0.type == "exec.template"
data.events.0.metadata.execution_id == "{{template_execution_id}}"
data.events.0.metadata.organization_id == "organization_template_kest"
data.events.0.metadata.workspace_id == "workspace_template_kest"
data.events.0.metadata.app_id == "app_template_kest"
data.events.0.metadata.workflow_run_id == "workflow_run_template_kest"
data.events.0.metadata.user_id == "user_template_kest"
```

```step
@id reject-missing-variable
@name Reject missing template variable

POST {{base_url}}/v1/exec/template
Content-Type: application/json

{
  "template": "{{template_missing}}",
  "variables": {},
  "organization_id": "organization_template_failure_kest",
  "workspace_id": "workspace_template_failure_kest",
  "workflow_run_id": "workflow_run_template_failure_kest"
}

[Asserts]
status == 400
code == -400
```

```step
@id inspect-template-failure-ownership-event
@name Inspect template failure ownership event

GET {{base_url}}/v1/observer/events?type=exec.template.failed&organization_id=organization_template_failure_kest&workspace_id=workspace_template_failure_kest&workflow_run_id=workflow_run_template_failure_kest&limit=1

[Asserts]
status == 200
code == 0
data.events.0.type == "exec.template.failed"
data.events.0.metadata.organization_id == "organization_template_failure_kest"
data.events.0.metadata.workspace_id == "workspace_template_failure_kest"
data.events.0.metadata.workflow_run_id == "workflow_run_template_failure_kest"
data.events.0.metadata.status == "failure"
```

```step
@id reject-unsafe-helper
@name Reject helper outside allowlist

POST {{base_url}}/v1/exec/template
Content-Type: application/json

{
  "template": "{{template_unsafe_helper}}",
  "variables": {
    "name": "HOME"
  }
}

[Asserts]
status == 400
code == -400
```

```step
@id reject-built-in-helper
@name Reject built-in helper outside allowlist

POST {{base_url}}/v1/exec/template
Content-Type: application/json

{
  "template": "{{template_builtin_helper}}",
  "variables": {
    "name": "HOME"
  }
}

[Asserts]
status == 400
code == -400
```

```step
@id truncate-template-output
@name Truncate template output

POST {{base_url}}/v1/exec/template
Content-Type: application/json

{
  "template": "{{template_value}}",
  "variables": {
    "value": "{{template_output_value}}"
  },
  "output_limit_kb": 1
}

[Asserts]
status == 200
code == 0
data.truncated == true
data.warnings.0 == "output truncated"
```

```step
@id reject-oversized-variable
@name Reject oversized template variable

POST {{base_url}}/v1/exec/template
Content-Type: application/json

{
  "template": "{{template_value}}",
  "variables": {
    "value": "{{oversized_template_value}}"
  }
}

[Asserts]
status == 400
code == -400
```
