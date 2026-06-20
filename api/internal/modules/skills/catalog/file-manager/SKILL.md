---
name: file-manager
description: Manage file assets in File Management when the current AIChat context provides concrete, user-authorized file targets.
when_to_use: Use this skill on the Console Files page when the user asks to delete a visible file or perform a governed file-management operation.
provider_type: builtin
provider_id: files
runtime_type: tool
tools:
  - delete_file
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
display:
  icon: folder-cog
  category: productivity
  label:
    en_US: File Manager
    zh_Hans: 文件管理器
  description:
    en_US: Performs governed File Management asset operations such as deleting a visible file.
    zh_Hans: 执行受治理保护的文件管理操作，例如删除当前可见文件。
  when_to_use:
    en_US: Use when the user explicitly asks to change files in File Management and the target is resolved from page context.
    zh_Hans: 当用户明确要求变更文件管理中的文件，并且目标已从页面上下文解析时使用。
  tags:
    en_US:
      - File
      - Management
    zh_Hans:
      - 文件
      - 管理
supported_callers:
  - aichat
---

# File Manager Skill

Use this skill for governed File Management asset operations. It is intentionally separate from `file-reader`: reading and summarizing file contents belongs to `file-reader`, while destructive or asset-changing file operations belong here.

## Workflow

1. Use this skill only when the current AIChat context provides a concrete File Management target, such as a resolved visible file on the Console Files page.
2. Do not invent file IDs. Use a file ID supplied by resolved page context or governed asset resolution.
3. For deletion requests with a resolved target, call `delete_file` with exactly that `file_id`.
4. Do not ask for a separate natural-language confirmation before deletion. Tool governance owns the approval card and will stop execution before deletion when approval is required.
5. If the target is ambiguous or missing, ask one concise clarification instead of calling `delete_file`.
6. If governance says approval is required, tell the user the deletion has not run yet and wait for the approval result. If approval is rejected, continue with a safe alternative.
7. Mention the file name when confirming a deletion. Do not expose internal file IDs unless the user explicitly asks for them.

## Tool Usage

`delete_file` accepts:

- `file_id`: required file ID from resolved context. Deletes exactly one file after governance allows execution.
