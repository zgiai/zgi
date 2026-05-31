# Sandbox Dependency Prepare

```flow
@flow id=sandbox-dependency-prepare
@name Sandbox dependency prepare
```

```step
@id prepare-skill-dependencies
@name Prepare dependency request from skill archive

POST {{base_url}}/v1/sandbox/dependencies/prepare
Content-Type: application/json

{
  "archive_base64": "{{dependency_prepare_archive_base64}}",
  "format": "zip",
  "strip_root": false,
  "base_runtime": "linux-secure"
}

[Asserts]
status == 200
code == 0
data.status == "build_required"
data.next_action == "queue_dependency_build"
data.package_count == 5
data.dependency_request.schema_version == 1
data.dependency_request.language == "python3"
data.dependency_request.base_runtime == "linux-secure"
data.packages.0.ecosystem == "nodejs"
data.packages.0.name == "@org/tool"
data.packages.1.ecosystem == "nodejs"
data.packages.1.name == "pdf-lib"
data.packages.1.version == "1.17.1"
data.packages.2.ecosystem == "python3"
data.packages.2.name == "pandas"
data.packages.2.version == "==2.2.3"
data.packages.3.ecosystem == "python3"
data.packages.3.name == "pillow"
data.packages.4.ecosystem == "python3"
data.packages.4.name == "pydantic"
data.packages.4.version == "==2.7.4"
```
