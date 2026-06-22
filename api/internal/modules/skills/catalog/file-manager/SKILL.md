---
name: file-manager
description: Manage File Management assets when AIChat has concrete, user-authorized file targets.
when_to_use: Use this hidden skill when the user explicitly asks AIChat to create, save, import, or delete files in File Management.
provider_type: builtin
provider_id: files
runtime_type: tool
tools:
  - delete_file
  - save_file_to_management
max_calls_per_turn: 5
timeout_seconds: 120
tool_governance:
  delete_file:
    tool_id: file.delete
    skill_id: file-manager
    domain: files
    effect: delete
    asset_type: file
    risk_level: high
    requires_asset_resolution: true
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - file:manage
    default_approval_policy: always_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  save_file_to_management:
    tool_id: file.save_to_management
    skill_id: file-manager
    domain: files
    effect: create
    asset_type: file
    risk_level: medium
    requires_asset_resolution: false
    reversible: true
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - file:create
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
display:
  icon: folder-cog
  category: productivity
  label:
    en_US: File Manager
    zh_Hans: File Manager
  description:
    en_US: Performs governed File Management asset operations such as saving generated files or deleting visible files.
    zh_Hans: Governed File Management asset operations.
  when_to_use:
    en_US: Use when the user explicitly asks to change files in File Management and the target is resolved from page context.
    zh_Hans: Use for explicit File Management changes.
  tags:
    en_US:
      - File
      - Management
    zh_Hans:
      - File
      - Management
supported_callers:
  - aichat
---

# File Manager Skill

Use this skill for governed File Management asset operations. It is intentionally separate from `file-reader` and `file-generator`: reading and summarizing file contents belongs to `file-reader`, generating temporary downloadable artifacts belongs to `file-generator`, and creating or deleting File Management assets belongs here.

## Workflow

1. Use this skill only when the user explicitly asks to change File Management, such as deleting a visible file or saving a generated/external file into the current Files page.
2. Do not invent file IDs. For existing files, use a file ID supplied by resolved page context or governed asset resolution.
3. For deletion requests with a resolved target, call `delete_file` with exactly that `file_id`.
4. For create/save/import requests where file content still needs to be produced, first use the appropriate artifact-producing skill to create a temporary artifact, then call `save_file_to_management` with `source_type=tool_file`, the returned `tool_file_id`/`file_id`, and the destination filename.
   - Use `file-generator` for regular files and generic SVG/vector files.
   - Use `chart-generator` only for chart, graph, and data visualization artifacts.
5. For create/save/import requests where the user supplied a public file URL, call `save_file_to_management` with `source_type=url`, the URL, and the destination filename.
6. Do not ask for a separate natural-language confirmation before governed operations. Tool governance owns the approval card and will stop execution when approval is required.
7. If the target or destination filename is ambiguous or missing, ask one concise clarification instead of calling a mutating tool.
8. If governance says approval is required, tell the user the operation has not run yet and wait for the approval result. If approval is rejected, continue with a safe alternative.
9. Mention the file name when confirming a create or deletion. Do not expose internal file IDs unless the user explicitly asks for them.

## Tool Usage

`delete_file` accepts:

- `file_id`: required file ID from resolved context. Deletes exactly one file after governance allows execution.

`save_file_to_management` accepts:

- `source_type`: required, `tool_file` for a generated artifact or `url` for a public external URL.
- `tool_file_id`: required when `source_type=tool_file`; use the generated artifact ID returned by the generation tool.
- `url`: required when `source_type=url`; must be an absolute public HTTP or HTTPS URL supplied by the user.
- `filename`: required destination filename shown in File Management. Include a suitable extension.
- `workspace_id`: optional target workspace ID. Usually omit it so current AIChat workspace context is used. Do not invent IDs.
