---
name: file-reader
description: Read text content from files the current AIChat user can access, and request governed deletion for files the user asks to remove.
when_to_use: Use this skill when the user asks to inspect, quote, summarize, compare, answer from, or delete an uploaded file or a file shown in the console files page.
provider_type: builtin
provider_id: files
runtime_type: tool
tools:
  - list_visible_files
  - read_file
  - delete_file
max_calls_per_turn: 10
timeout_seconds: 120
tool_governance:
  list_visible_files:
    tool_id: file.list_visible
    skill_id: file-reader
    domain: files
    effect: read
    asset_type: file
    risk_level: low
    requires_asset_resolution: false
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - file:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  read_file:
    tool_id: file.read
    skill_id: file-reader
    domain: files
    effect: read
    asset_type: file
    risk_level: low
    requires_asset_resolution: true
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - file:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  delete_file:
    tool_id: file.delete
    skill_id: file-reader
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
display:
  icon: file-text
  category: productivity
  label:
    en_US: File Reader
  description:
    en_US: Lists visible file context, reads accessible uploaded or console file text, and deletes files only through governed approval.
  when_to_use:
    en_US: Use when an answer needs content from a specific file available to the current user, or when the user asks to delete a visible file.
  tags:
    en_US:
      - File
      - Reading
supported_callers:
  - aichat
---

# File Reader Skill

Use this skill to list visible file context, read content from a file that has already been resolved for the current AIChat turn, or delete a resolved file after governance approval.

## Workflow

1. Use this skill only when the user is asking about a specific file, uploaded attachment, or current console file context.
2. Do not invent file IDs. Use a file ID supplied by resolved page context, attachment context, or governed asset resolution.
3. For listing requests such as "what files do I have", "which files are visible", "current files", or "selected files", call `list_visible_files` first and answer from its `files` result. Do not use `read_file` for a list-only request unless the user asks for file contents.
4. For read, summary, translation, extraction, comparison, or question answering, call `read_file` with `file_id`. Set `max_chars` only when you need more or less returned content.
5. Inspect `content_status` before answering:
   - If `content_status` is `extracted`, answer from `content`.
   - If `content_status` is `empty`, say the file has no extractable text content.
   - If `content_status` is `error`, explain that the file could not be read and include the short error reason when useful.
6. If `content_truncated` is true and the missing tail matters, ask for a narrower question or retry with a higher `max_chars` up to the tool limit.
7. For deletion requests with a resolved target, call `delete_file` with exactly that `file_id`. Do not ask for a separate natural-language confirmation first; tool governance owns the approval card and will stop execution before deletion when approval is required.
8. If the target is ambiguous or missing, ask one concise clarification instead of calling `delete_file`.
9. If governance says approval is required, tell the user the deletion has not run yet and wait for the approval result. If approval is rejected, continue with a safe alternative.
10. Mention the file name when listing, summarizing, quoting, or confirming a deletion. Do not expose internal file IDs unless the user explicitly asks for them.

## Tool Usage

`read_file` accepts:

- `file_id`: required file ID from resolved context.
- `max_chars`: optional maximum returned content characters. Defaults to 4000 and is capped at 12000.

`delete_file` accepts:

- `file_id`: required file ID from resolved context. Deletes exactly one file after governance allows execution.

`list_visible_files` accepts no parameters and returns:

- `count`: number of visible files supplied by the current page context.
- `selected_count`: number of selected files.
- `files`: ordered visible files with `visible_index`, `file_id`, `name`, `extension`, `mime_type`, optional `workspace_id`, and optional `selected`.
