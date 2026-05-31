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
  "timeout_ms": 2000,
  "output_limit_kb": 64
}

[Asserts]
status == 200
code == 0
data.content == "Hello ZGI"
data.truncated == false
```

```step
@id reject-missing-variable
@name Reject missing template variable

POST {{base_url}}/v1/exec/template
Content-Type: application/json

{
  "template": "{{template_missing}}",
  "variables": {}
}

[Asserts]
status == 400
code == -400
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
