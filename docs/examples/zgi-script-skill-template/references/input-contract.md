# Input contract

Read this reference when changing arguments or uploaded-file handling.

- `script_input_schema` uses JSON Schema Draft 2020-12 and must have an object root.
- The example covers strings, integers, numbers, booleans, arrays, nested objects,
  enums, optional values, and `null`.
- ZGI validates Tool arguments before starting the script. The script still checks
  types so direct local execution fails clearly.
- `source_file_id` maps to one metadata object under `input_files.source`.
- `attachment_file_ids` maps to an array under `input_files.attachments`.
- Scripts receive authorized relative paths under `inputs/`, never storage credentials.
- Never accept absolute paths, shell commands, arbitrary environment variables, or
  user-selected working directories.
