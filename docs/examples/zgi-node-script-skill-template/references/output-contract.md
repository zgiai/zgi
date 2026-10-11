# Output contract

Read this reference when changing stdout, stderr, or generated files.

- The template uses `mixed`: stdout contains one JSON object and the script also
  creates `artifacts/result.json`.
- A JSON object on stdout becomes a structured Tool message visible to the model.
- Plain stdout becomes a text Tool message. Diagnostics written to stderr become
  log messages and must not contain secrets or sensitive source content.
- Generated files must be ordinary files under `allowed_artifact_paths` and must
  be closed before exit. ZGI persists them before deleting the temporary Sandbox.
- The model receives a file Tool message plus an `artifacts` summary containing
  persistence status and the stable file identity. A Sandbox path is not a URL.
- Exit code `0` means success. Nonzero exits fail the Tool call, although authorized
  diagnostic artifacts may still be returned.

Other supported modes are `stdout_json`, `stdout_text`, and `artifacts`. Use
`stdout_json` for a JSON-only result, `stdout_text` for text-only output, and
`artifacts` when files are the only deliverable.
