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

The dependency scanner resolves absolute local Python imports from the directory
that contains the manifest `entrypoint`, matching `python3 <entrypoint>` runtime
semantics. Code in nested packages should use package-qualified or relative
imports. Dynamic `sys.path` changes do not expand the scanner's local import root.

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
files to the manifest's allowed `artifacts/` paths. See
[`script-skill-output-files.md`](script-skill-output-files.md) for the complete
artifact persistence, chat event, frontend rendering, and troubleshooting
contract.

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

## Package argument contracts

Script packages can declare `script_input_schema` in `SKILL.md` YAML frontmatter.
The schema must have an object root and follow JSON Schema Draft 2020-12. It is
validated during import and attached to the package's `run_script` tool. Invalid
arguments are rejected by the runtime before execution. This grants no additional
tools, providers, file permissions, or execution privileges.

```yaml
script_input_schema:
  type: object
  required: [count]
  additionalProperties: false
  properties:
    count:
      type: integer
      minimum: 1
```

Schemas are limited to 128 KiB and 32 nesting levels. Only local references are
allowed; `$id` is rejected to prevent changes to reference resolution scope.

## Administrator runtime grants

Set `ZGI_SKILL_RUNTIME_GRANTS_PATH` to a server-controlled JSON file outside the
package storage tree. The API loads it at startup. A configured unreadable or
invalid file causes script executions to fail closed. Each entry binds an
`organization_id` and `package_sha256` to a `dependency_profile`, `profile_version`,
`artifact_checksum`, `max_artifact_count`, `max_artifact_bytes`, and
`max_input_file_bytes`. Package names and declared source do not confer a grant.
Without a matching grant, explicit non-stdlib environments are rejected.

The package hash is SHA-256 over sorted relative file names, a NUL byte, the
lowercase SHA-256 hex digest of each file, and a newline for each entry. Symlinks
and nonregular files are rejected. ZIP metadata is excluded. Any content change
requires a new grant. The created sandbox must match the granted runtime version
and artifact checksum before package upload. Successful traces include the
package hash and granted runtime identity.

Imports retain content-addressed directories under `versions/<package_sha256>`.
Upgrading changes the active database pointer without overwriting earlier package
bytes. To roll back, preview and confirm the previous ZIP again with overwrite
confirmation. Its original hash must still have a server runtime grant. Reimport
verifies existing content before reusing the version. Runs snapshot package files
before hashing and upload, so concurrent imports cannot change running scripts.
Runtime storage is excluded from Docker build contexts.
# Diagnostic output from failed scripts

A script may write diagnostic output and authorized artifacts before exiting
with a nonzero code. These outputs remain available in the tool error response
and as downloadable conversation artifacts. The invocation and operation plan
still record execution failure; the presence of a diagnostic file does not
prove that the requested work succeeded. Validation and provider errors retain
their existing public error and redaction behavior.

Failed script events retain the verified package and runtime profile identity with
the exit code. Marking required, unverified tool work as skipped does not turn a
failed runtime attempt into passing plan completion.
