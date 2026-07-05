---
name: file-reader
description: Read text content from files the current AIChat user can access.
when_to_use: Use this skill when the user asks to inspect, quote, summarize, compare, translate, or answer from an uploaded file, historical file reference, or a file shown in the console files page.
provider_type: builtin
provider_id: files
runtime_type: tool
tools:
  - list_visible_files
  - read_file
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
display:
  icon: file-text
  category: productivity
  label:
    en_US: File Reader
  description:
    en_US: Lists visible file context and reads accessible uploaded, historical, or console file text.
  when_to_use:
    en_US: Use when an answer needs content from a specific file available to the current user.
  tags:
    en_US:
      - File
      - Reading
supported_callers:
  - aichat
---

# File Reader Skill

Use this skill to list visible file context or read content from a file that has already been resolved for the current AIChat turn.

## Workflow

1. Use this skill only when the user is asking about a specific file, uploaded attachment, historical file reference, or current console file context.
2. Do not invent file IDs. Use a file ID supplied by resolved page context, attachment context, or governed asset resolution.
3. For listing requests such as "what files do I have", "which files are visible", "current files", or "selected files", answer directly from the provided visible file context when it is present and sufficient. Call `list_visible_files` when that context is missing, ambiguous, stale, or needs an authoritative refresh. Do not use `read_file` for a list-only request unless the user asks for file contents.
4. For read, summary, translation, extraction, comparison, or question answering, call `read_file` with `file_id`. Set `max_chars` only when you need more or less returned content.
5. Inspect `content_status` before answering:
   - If `content_status` is `extracted`, answer from `content`.
   - If `content_status` is `empty`, say the file has no extractable text content.
   - If `content_status` is `error`, explain that the file could not be read and include the short error reason when useful.
6. The raw `content` returned by `read_file` is not durable across history reloads, approvals, navigation, refresh, or later tool phases. If later steps will use the file body, a summary, theme, topic, quote, title, prompt, config value, generated asset, or final answer derived from this file, call `submit_turn_state` before leaving the file-reading phase. Record a concise reusable fact with `kind=working_fact`, `visibility=model_only`, `source=file-reader/read_file`, and a meaningful key such as `source_file_summary`, `source_file_theme`, or `exact_short_text`. When the summary is useful for the user to verify, use `kind=user_deliverable`, `visibility=user_visible`, and a short `title`/`content` instead.
7. If `content_truncated` is true and the missing tail matters, ask for a narrower question or retry with a higher `max_chars` up to the tool limit.
8. If the user asks to delete, rename, move, or otherwise manage a File Management asset, use `file-manager` when it is available in the current files-page context. If `file-manager` is not available, explain that the current chat can read files but cannot perform that file-management operation from this surface.
9. Mention the file name when listing, summarizing, or quoting. Do not expose internal file IDs unless the user explicitly asks for them.

## Tool Usage

`read_file` accepts:

- `file_id`: required file ID from resolved context.
- `max_chars`: optional maximum returned content characters. Defaults to 4000 and is capped at 12000.
- Success evidence: the tool result must identify the resolved file and include `content_status`. Use `content` only when `content_status=extracted`. If `content_status=empty`, report that no extractable text was found. If `content_status=error`, report the actual read failure instead of summarizing from filename or page metadata.
- Handoff evidence: when `content_lifetime=current_tool_result_only` and `content_redacted_in_history=true`, do not assume the raw `content` will be available after continuation boundaries. If `handoff_recommended=true` and later steps depend on this content or a derived summary, call `submit_turn_state` before continuing to those steps.

`list_visible_files` accepts no parameters and returns:

- `count`: number of visible files supplied by the current page context.
- `selected_count`: number of selected files.
- `files`: ordered visible files with `visible_index`, `file_id`, `name`, `extension`, `mime_type`, optional `workspace_id`, and optional `selected`.
- Success evidence: use the returned `files` list and `count` as the current visible-file state. Do not infer file contents from this list-only result.

## Truthfulness Contract

- Treat file-reader results as authoritative. Do not answer from stale visible page metadata when a `read_file` result is required.
- If the target cannot be resolved or read, say so directly and ask for a narrower target only when the available context cannot identify the file.
- Retry at most once with corrected `file_id` or `max_chars` when the tool result indicates a recoverable issue.
