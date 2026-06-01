# Sandbox Archive Strip Root

```flow
@flow id=sandbox-archive-strip-root
@name Sandbox archive strip-root extraction
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
@id upload-strip-root-archive
@name Upload archive with strip root

POST {{base_url}}/v1/files/upload-archive
Content-Type: application/json

{
  "sandbox_id": "{{sandbox_id}}",
  "path": "skills/weather",
  "archive_base64": "{{strip_root_archive_base64}}",
  "format": "zip",
  "strip_root": true
}

[Asserts]
status == 200
code == 0
data.file_count == 3
```

```step
@id download-stripped-skill
@name Download stripped skill file

GET {{base_url}}/v1/files/download?sandbox_id={{sandbox_id}}&path=skills/weather/SKILL.md&encoding=base64

[Asserts]
status == 200
code == 0
data.content == "IyBXZWF0aGVyCg=="
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
