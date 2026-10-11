---
name: zgi-node-script-skill-template
description: Transform structured values and optional uploaded files with a deterministic Node.js script. Use this as the complete Node.js reference when developing a ZGI custom Script Skill.
runtime_type: prompt
timeout_seconds: 30
script_input_schema:
  type: object
  additionalProperties: false
  required: [mode, text, count, ratio, enabled, tags, options]
  properties:
    mode:
      type: string
      enum: [upper, lower, title]
    text:
      type: string
      minLength: 1
      maxLength: 20000
    count:
      type: integer
      minimum: 0
      maximum: 100
    ratio:
      type: number
      minimum: 0
      maximum: 1
    enabled:
      type: boolean
    tags:
      type: array
      maxItems: 8
      uniqueItems: true
      items:
        type: string
        minLength: 1
        maxLength: 40
    options:
      type: object
      additionalProperties: false
      required: [prefix, repeat, emit_diagnostic]
      properties:
        prefix:
          type: string
          maxLength: 100
        repeat:
          type: integer
          minimum: 1
          maximum: 3
        emit_diagnostic:
          type: boolean
    note:
      type: [string, "null"]
      maxLength: 500
    source_file_id:
      type: string
      minLength: 1
    attachment_file_ids:
      type: array
      maxItems: 3
      items:
        type: string
        minLength: 1
---

# ZGI Node.js Script Skill Template

Use this Skill to transform structured values and authorized text files deterministically.

## Workflow

1. Collect the required structured arguments and any optional file IDs.
2. Call `run_script` once with those arguments.
3. Treat a zero exit code and the structured Tool result as the source of truth.
4. Return the structured result and surface the generated file attachment.

Do not invent local paths, invoke package installation, or send source content to
an external service. ZGI injects authorized files under `input_files`.

Read only the reference needed for the change:

- For argument types and file inputs, read [`references/input-contract.md`](references/input-contract.md).
- For stdout, stderr, result modes, and artifacts, read [`references/output-contract.md`](references/output-contract.md).
- For local and third-party packages, read [`references/dependencies.md`](references/dependencies.md).
