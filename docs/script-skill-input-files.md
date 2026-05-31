# Script Skill Input Files

Custom script skills can request uploaded files through `skill.manifest.json`.
The API downloads declared files before execution, uploads them into the sandbox
under `inputs/`, and passes their sandbox paths through stdin. The sandbox never
receives ZGI file-service credentials and does not need network access.

Single-file example manifest:

```json
{
  "entrypoint": "scripts/run.py",
  "language": "python3",
  "input_files": [
    {
      "name": "confirmation",
      "argument": "confirmation_file_id",
      "required": true,
      "extensions": [".xlsx"],
      "mime_types": [
        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
      ],
      "max_bytes": 10485760
    }
  ]
}
```

`name` must be a safe path segment and becomes the key under `input_files`.
`argument` names the tool argument that contains the uploaded `file_id`.
`max_bytes` may tighten the default 10 MiB limit, but cannot raise it.
Do not set `dependency_profile` in skill packages. The platform scans the skill
archive, prepares or queues the required dependency build, and selects a verified
runtime profile before execution.
For third-party Python packages, include a pinned `requirements.txt` in the skill
package, for example `openpyxl==3.1.5`. The package list is only a dependency
request; the sandbox still installs it outside request execution through the
managed build flow.

When `confirmation_file_id` is supplied, stdin keeps the original arguments and
adds file metadata:

```json
{
  "confirmation_file_id": "file_123",
  "input_files": {
    "confirmation": {
      "path": "inputs/confirmation/original.xlsx",
      "file_id": "file_123",
      "filename": "original.xlsx",
      "mime_type": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
      "size": 12345
    }
  }
}
```

Scripts should read only from the provided `inputs/` paths and write generated
files to the manifest's allowed `artifacts/` paths.

For multi-file skills, declare a separate input item with `multiple: true`.
The argument must contain an array of uploaded file IDs. `max_count` may tighten
the platform limit, but cannot raise it above the API maximum.

```json
{
  "entrypoint": "scripts/run.py",
  "language": "python3",
  "input_files": [
    {
      "name": "confirmations",
      "argument": "confirmation_file_ids",
      "multiple": true,
      "max_count": 10,
      "extensions": [".xlsx"],
      "mime_types": [
        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
      ],
      "max_bytes": 10485760
    }
  ]
}
```

When `confirmation_file_ids` is supplied, stdin keeps the original array and
adds an array under `input_files.<name>`:

```json
{
  "confirmation_file_ids": ["file_a", "file_b"],
  "input_files": {
    "confirmations": [
      {
        "path": "inputs/confirmations/file_a/a.xlsx",
        "file_id": "file_a",
        "filename": "a.xlsx",
        "mime_type": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        "size": 12345
      },
      {
        "path": "inputs/confirmations/file_b/b.xlsx",
        "file_id": "file_b",
        "filename": "b.xlsx",
        "mime_type": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        "size": 23456
      }
    ]
  }
}
```
