---
name: agent-management
description: Manage Agent assets and governed Agent bindings from the contextual AIChat sidebar when the user has the required workspace permissions.
when_to_use: Use this hidden skill when the user asks AIChat to create an Agent, edit Agent details, delete an Agent, inspect Agent draft config, query available Agent runtime models, update supported draft config fields, or replace Agent skill/knowledge/database/workflow bindings.
provider_type: builtin
provider_id: agent_management
runtime_type: tool
tools:
  - list_agents
  - get_agent
  - create_agent
  - update_agent_identity
  - delete_agent
  - delete_agents
  - get_agent_config
  - update_agent_config
  - replace_agent_memory_slots
  - list_agent_skill_candidates
  - list_agent_knowledge_candidates
  - list_agent_database_candidates
  - list_agent_database_tables
  - list_agent_workflow_binding_candidates
  - replace_agent_skill_bindings
  - replace_agent_knowledge_bindings
  - replace_agent_database_bindings
  - replace_agent_workflow_bindings
  - list_available_models
max_calls_per_turn: 14
timeout_seconds: 90
tool_governance:
  create_agent:
    tool_id: agent.create
    skill_id: agent-management
    domain: agents
    effect: create
    asset_type: agent
    risk_level: medium
    requires_asset_resolution: false
    reversible: true
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - agent:manage
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  update_agent_identity:
    tool_id: agent.update_identity
    skill_id: agent-management
    domain: agents
    effect: update
    asset_type: agent
    risk_level: medium
    requires_asset_resolution: true
    reversible: true
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - agent:manage
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  update_agent_config:
    tool_id: agent.update_config
    skill_id: agent-management
    domain: agents
    effect: update
    asset_type: agent
    risk_level: medium
    requires_asset_resolution: true
    reversible: true
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - agent:manage
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  replace_agent_memory_slots:
    tool_id: agent.replace_memory_slots
    skill_id: agent-management
    domain: agents
    effect: update
    asset_type: agent
    risk_level: medium
    requires_asset_resolution: true
    reversible: true
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - agent:manage
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  list_agent_knowledge_candidates:
    tool_id: agent.list_knowledge_candidates
    skill_id: agent-management
    domain: agents
    effect: read
    asset_type: knowledge_base
    risk_level: low
    requires_asset_resolution: false
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - agent:manage
      - knowledge:read
    default_approval_policy: never_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  list_agent_skill_candidates:
    tool_id: agent.list_skill_candidates
    skill_id: agent-management
    domain: agents
    effect: read
    asset_type: agent_skill
    risk_level: low
    requires_asset_resolution: false
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - agent:manage
    default_approval_policy: never_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  list_agent_database_candidates:
    tool_id: agent.list_database_candidates
    skill_id: agent-management
    domain: agents
    effect: read
    asset_type: database_table
    risk_level: low
    requires_asset_resolution: false
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - agent:manage
      - database:read
    default_approval_policy: never_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  list_agent_database_tables:
    tool_id: agent.list_database_tables
    skill_id: agent-management
    domain: agents
    effect: read
    asset_type: database_table
    risk_level: low
    requires_asset_resolution: false
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - agent:manage
      - database:read
    default_approval_policy: never_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  list_agent_workflow_binding_candidates:
    tool_id: agent.list_workflow_binding_candidates
    skill_id: agent-management
    domain: agents
    effect: read
    asset_type: workflow
    risk_level: low
    requires_asset_resolution: false
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - agent:manage
      - workflow:read
    default_approval_policy: never_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  replace_agent_knowledge_bindings:
    tool_id: agent.replace_knowledge_bindings
    skill_id: agent-management
    domain: agents
    effect: update
    asset_type: knowledge_base
    risk_level: medium
    requires_asset_resolution: true
    reversible: true
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - agent:manage
      - knowledge:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  replace_agent_skill_bindings:
    tool_id: agent.replace_skill_bindings
    skill_id: agent-management
    domain: agents
    effect: update
    asset_type: agent_skill
    risk_level: medium
    requires_asset_resolution: true
    reversible: true
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - agent:manage
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  replace_agent_database_bindings:
    tool_id: agent.replace_database_bindings
    skill_id: agent-management
    domain: agents
    effect: update
    asset_type: database_table
    risk_level: high
    requires_asset_resolution: true
    reversible: true
    bulk_sensitive: true
    external_side_effect: false
    permission_scopes:
      - agent:manage
      - database:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  replace_agent_workflow_bindings:
    tool_id: agent.replace_workflow_bindings
    skill_id: agent-management
    domain: agents
    effect: update
    asset_type: workflow
    risk_level: high
    requires_asset_resolution: true
    reversible: true
    bulk_sensitive: false
    external_side_effect: true
    permission_scopes:
      - agent:manage
      - workflow:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  delete_agent:
    tool_id: agent.delete
    skill_id: agent-management
    domain: agents
    effect: delete
    asset_type: agent
    risk_level: high
    requires_asset_resolution: true
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - agent:manage
    default_approval_policy: always_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  delete_agents:
    tool_id: agent.delete.batch
    skill_id: agent-management
    domain: agents
    effect: delete
    asset_type: agent
    risk_level: high
    requires_asset_resolution: true
    reversible: false
    bulk_sensitive: true
    external_side_effect: false
    permission_scopes:
      - agent:manage
    default_approval_policy: always_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
display:
  icon: bot
  category: productivity
  label:
    en_US: Agent Manager
    zh_Hans: 智能体管理器
  description:
    en_US: Performs governed Agent asset operations from contextual AIChat.
    zh_Hans: 在侧栏 AIChat 中执行受治理保护的智能体资产操作。
  when_to_use:
    en_US: Use when the user explicitly asks to manage Agents in the ZGI console.
    zh_Hans: 当用户明确要求在 ZGI 控制台管理智能体时使用。
  tags:
    en_US:
      - Agent
      - Management
    zh_Hans:
      - 智能体
      - 管理
supported_callers:
  - aichat
---

# Agent Management Skill

Use this skill for governed Agent asset operations in the contextual AIChat sidebar. It is intentionally separate from Agent runtime skills: this skill changes Agent assets in the console; it does not invoke Agents, publish Agents, roll back versions, expose API keys, or run bound workflows.

## Workflow

1. Treat current page `visible_agents` / runtime `console_agents_visible_agents` as authoritative resolved targets when the user refers to visible Agents, selected Agents, the current page, the current Agent, first-N/top-N Agents, or these Agents. Use their `agent_id`, visible name, and `href` directly; do not call `list_agents` just to rediscover the same visible targets.
2. Use `list_agents` only when the user asks what Agents exist beyond the current visible page context, asks to search/find an Agent by name, gives an Agent name without an exact visible/page-context match, or when no usable visible Agent context is available.
3. Use `get_agent` when the user asks about one Agent's basic information and visible/page context does not already answer it.
4. Use `create_agent` when the user asks to create a new Agent. Create only `AGENT` type drafts in the current workspace unless a target workspace is explicitly available in context.
5. Use `update_agent_identity` for Agent name, description, or icon changes.
6. Use `get_agent_config` before changing draft runtime configuration if the current config is not already known from page context.
7. Use `list_available_models` before replacing an Agent model unless the user already provided an exact provider/model pair from current page context. Default to `use_case: "text-chat"` for ordinary Agent runtime model replacement; use `reasoning`, `vision`, or `function-calling` only when the user clearly asks for that capability.
8. Use `update_agent_config` for supported draft fields: system prompt, model provider/model, model parameters, Agent memory switch, file upload switch, home title, input placeholder, theme color, suggested questions, and complete skill/knowledge/database/workflow binding replacement lists. Prefer one `update_agent_config` call when the user asks to change multiple config sections in the same turn.
9. To replace the Agent model, choose one item from `list_available_models` and pass that item's `provider` as `model_provider` and `model` as `model` to `update_agent_config`.
10. Use `replace_agent_memory_slots` to replace the full Agent memory slot list. If the user asks to enable memory and create slots, call `update_agent_config` with `agent_memory_enabled: true`, then call `replace_agent_memory_slots`.
11. Use `list_agent_skill_candidates` before replacing Agent skill bindings unless the requested skills are already exact visible/page-context candidates. Only bind skill IDs returned by that tool or already visible in current Agent context.
12. Prefer `update_agent_config` with `enabled_skill_ids` when adding, removing, or replacing Agent skills. Use `replace_agent_skill_bindings` only as a single-section compatibility tool. Preserve existing skills not mentioned by the user.
13. Use the candidate list tools before replacing Agent knowledge, database, or workflow bindings unless the requested resources are already exact visible/page-context candidates. Candidate tools are scoped by the backend to the resolved Agent's workspace; do not provide or infer a workspace ID. Select resources by visible name; do not invent skill IDs, dataset IDs, data source IDs, table IDs, workflow IDs, or binding IDs.
14. Prefer one `update_agent_config` call with `knowledge_dataset_ids`, `database_bindings`, and/or `workflow_bindings` after the current binding set and exact candidates are known. Use `replace_agent_knowledge_bindings`, `replace_agent_database_bindings`, or `replace_agent_workflow_bindings` only as single-section compatibility tools. Preserve existing bindings not mentioned by the user.
15. Binding replacement summaries must mention user-visible resource names such as skill names, knowledge base names, database table names, or workflow labels. Do not expose raw skill IDs, dataset IDs, table IDs, workflow IDs, binding IDs, workspace IDs, or grant/correlation IDs in the final answer unless the user explicitly asks for technical identifiers.
16. Do not change publishing state, API settings, or invocation behavior in this MVP.
17. Use `delete_agent` only when one target Agent is resolved by exact ID or exact visible/listed name. Use `delete_agents` once when the user asks to delete multiple Agents, a range such as the first N visible Agents, selected Agents, or all listed Agents. Deletion is irreversible and governance approval will pause execution when required.
18. Do not ask for a separate natural-language confirmation before governed operations. Tool governance owns approval. If approval is rejected, continue safely and explain that no mutation was performed.
19. If the target Agent or requested fields/resources are ambiguous, ask one concise clarification instead of guessing.
20. Navigation is not a default completion step for ordinary Agent edits, binding changes, unbinding, or list-page batch deletion. Prefer refreshed page context or asset observation after the mutation.
21. After `create_agent` succeeds, route to `/console/agents/{agentId}/agent` only when the user asked to open the new Agent or the operation needs the detail page for follow-up edits. If the frontend client action already loaded that route, do not navigate again.
22. If `delete_agent` succeeds while the current page is that Agent's detail page, or `delete_agents` succeeds and includes the current detail Agent, use `console-navigator` to route to `/console/agents` before the final answer. When deleting multiple Agents from the list page, do not navigate after the first item; rely on page refresh/observation and the batch `item_results`.

## Tool Usage

`list_agents` accepts:

- `workspace_id`: optional. Usually omit it so current AIChat workspace context is used.
- `keyword`: optional search keyword.
- `limit`: optional maximum result count.

`get_agent` accepts:

- `agent_id`: required resolved Agent ID.

`create_agent` accepts:

- `name`: required Agent name.
- `description`: optional Agent description.
- `icon_type`: optional icon type. Use `text` for text or emoji icons, or `image` for an uploaded image file ID/URL.
- `icon`: optional icon value. For text icons pass the visible text, for example `AI` or `BOT`; the runtime will normalize it to the Agent UI icon JSON shape. If omitted, the runtime derives a visible text icon from the Agent name.
- `icon_background`: optional text icon background color, for example `#0f766e`. When the user asks for an icon background color, pass this field explicitly instead of embedding it in `icon`.
- `workspace_id`: optional target workspace ID.

`update_agent_identity` accepts:

- `agent_id`: required resolved Agent ID.
- `name`, `description`, `icon_type`, `icon`, `icon_background`: optional fields. Provide only fields the user asked to change. For text or emoji icons use `icon_type: "text"`, pass the visible icon text in `icon`, and pass the requested background color in `icon_background`.
- The result includes `updated_fields`; only claim a name, description, icon text, or icon background changed when that exact field appears in `updated_fields` or the returned Agent draft state explicitly proves it.

`delete_agent` accepts:

- `agent_id`: required resolved Agent ID.

`delete_agents` accepts:

- `agents`: required JSON array of frozen target Agents. Each item should include `agent_id` and the visible `name`; include `workspace_id` when available. Example: `[{"agent_id":"...","name":"Agent A"},{"agent_id":"...","name":"Agent B"}]`.
- `agent_ids`: optional fallback ID list. Prefer `agents` so governance approval cards and final answers can show user-visible names.
- The result includes `operation_group`, `target_count`, `deleted_count`, `failed_count`, and `item_results[]` with per-Agent `status` (`succeeded` or `failed`). Use those facts for the final answer.

`get_agent_config` accepts:

- `agent_id`: required resolved Agent ID.

`update_agent_config` accepts:

- `agent_id`: required resolved Agent ID.
- Optional supported config fields. Omitted fields are preserved by the tool; do not send publish fields.
- For model replacement, `model_provider` and `model` must be provided together from the same `list_available_models` result item. Never pass only `model`, because model IDs can collide across providers.
- `enabled_skill_ids`: optional full list of enabled user-selectable skill IDs. Use `[]` to clear all user-selectable skills.
- `knowledge_dataset_ids`: optional full list of knowledge dataset IDs. Use `[]` to clear knowledge bindings.
- `knowledge_retrieval_config`: optional replacement knowledge retrieval config. Omit to preserve it.
- `database_bindings`: optional JSON array replacing database bindings. Each item supports `data_source_id`, `table_ids`, and optional `writable_table_ids`. Use `[]` to clear database bindings.
- `workflow_bindings`: optional JSON array replacing workflow bindings. Each item uses a candidate `binding_id` and preserves returned fields such as `label`, `agent_id`, `workflow_id`, `version_strategy`, and optional `version_uuid`. Use `[]` to clear workflow bindings.
- The result includes `updated_fields` and may include `config_changes`/`binding_changes` with `change_action` (`bind`, `unbind`, `replace`, or `update`). Only claim a field or binding changed when those fields or the returned draft `config` prove it.

`list_available_models` accepts:

- `use_case`: optional model use case. Defaults to `text-chat` for Agent runtime model replacement. Use `all` only when the user asks to inspect every available model.
- `provider`: optional provider slug filter.
- `limit`: optional maximum result count, capped at 100.

The result includes `models[].provider`, `models[].model`, `models[].model_name`, `models[].use_cases`, and key capability flags. Use exactly those returned `provider` and `model` values together when calling `update_agent_config`.

`replace_agent_memory_slots` accepts:

- `agent_id`: required resolved Agent ID.
- `agent_memory_slots`: required JSON array replacing the complete slot list. Each item supports `key`, `description`, `enabled`, and optional `sort_order`; use `[]` to clear all slots. Preserve existing slots unless the user asked to replace or remove them.

`list_agent_skill_candidates` accepts:

- `agent_id`: required resolved Agent ID.
- `query`: optional search text for narrowing candidate skills.
- `limit`: optional maximum result count.
- `include_selected`: optional. Defaults to true; set false to exclude currently enabled skills.

`replace_agent_skill_bindings` accepts:

- `agent_id`: required resolved Agent ID.
- `skill_ids`: required JSON array replacing the complete enabled skill list. Use `[]` to clear all user-selected skills. Preserve existing skill IDs unless the user asked to replace or remove them.
- When available from candidates, mention user-visible skill names in the reasoning/final answer; the backend validates skill IDs against the Agent-supported skill catalog.

`list_agent_knowledge_candidates` accepts:

- `agent_id`: required resolved Agent ID.
- `query`: optional search text for narrowing candidate knowledge bases.
- `limit`: optional maximum result count.
- `include_selected`: optional. Defaults to true; set false to exclude currently bound knowledge bases.

`list_agent_database_candidates` accepts:

- `agent_id`: required resolved Agent ID.
- `query`: optional search text for narrowing candidate databases.
- `limit`: optional maximum result count.
- `include_selected`: optional. Defaults to true; set false to exclude currently bound databases.
- `require_write`: optional. Set true when the user wants writable table bindings.

`list_agent_database_tables` accepts:

- `agent_id`: required resolved Agent ID.
- `data_source_id`: required database ID returned by `list_agent_database_candidates`.
- `query`: optional search text for narrowing candidate tables.
- `limit`: optional maximum result count.
- `include_columns`: optional. Defaults to false; set true when column details are needed.
- `include_selected`: optional. Defaults to true; set false to exclude currently bound tables for the database.

`list_agent_workflow_binding_candidates` accepts:

- `agent_id`: required resolved Agent ID.
- `query`: optional search text for narrowing candidate workflows.
- `agent_type`: optional filter, usually `WORKFLOW` or `CONVERSATIONAL_WORKFLOW`.
- `limit`: optional maximum result count.
- `include_start_inputs`: optional. Defaults to true.
- `include_selected`: optional. Defaults to true; set false to exclude currently bound workflows.

`replace_agent_knowledge_bindings` accepts:

- `agent_id`: required resolved Agent ID.
- `dataset_ids`: required JSON array replacing the complete knowledge binding list. Use `[]` to clear all knowledge bindings.
- `retrieval_config`: optional JSON object replacing knowledge retrieval config. Omit to preserve the current retrieval config.
- When available from candidates, include user-visible knowledge base names in the reasoning/final answer; the backend still validates IDs against the Agent workspace.

`replace_agent_database_bindings` accepts:

- `agent_id`: required resolved Agent ID.
- `bindings`: required JSON array replacing the complete database binding list. Each item supports `data_source_id`, `table_ids`, and optional `writable_table_ids`. Use `[]` to clear all database bindings.
- Preserve candidate table names in each binding when possible, for example by including `tables: [{"table_id":"...","name":"Orders"}]`; these names help governance approval cards, while execution still trusts only backend-validated IDs.

`replace_agent_workflow_bindings` accepts:

- `agent_id`: required resolved Agent ID.
- `bindings`: required JSON array replacing the complete workflow binding list. Each item uses a candidate `binding_id` and preserves returned fields such as `label`, `agent_id`, `workflow_id`, `version_strategy`, and optional `version_uuid`. Use `[]` to clear all workflow bindings.
- Preserve candidate `label` values in each binding so governance and final summaries show workflow names rather than IDs.

## Success Evidence

- `create_agent` succeeds only when the tool result includes an `agent_id` and the created Agent name or detail href.
- `update_agent_identity` and `replace_agent_memory_slots` succeed only when the tool result confirms the requested `agent_id` and the changed fields or returned draft state.
- `update_agent_config` succeeds only for fields listed in `updated_fields` for the requested `agent_id`. For binding changes, use `config_changes`/`binding_changes` as the authoritative action summary; do not claim omitted fields were changed, even if they were part of the user's original request.
- Binding replacement tools succeed only when the tool result confirms the requested `agent_id` and returns the resulting selected/bound resource state, counts, or user-visible resource names.
- `delete_agent` succeeds only when the tool result confirms deletion for the requested `agent_id`.
- `delete_agents` succeeds as a batch only according to `operation_group.item_results`: report exactly how many targets succeeded and failed, and name failed targets when present. Do not treat one succeeded item as proof that the whole batch finished.
- `list_available_models` is read-only evidence. When replacing a model, use one returned item and pass its `provider` and `model` together; final answers must not claim the model changed until `update_agent_config` succeeds.

## Truthfulness Contract

- Treat Agent Management tool results as authoritative. Do not claim an Agent was created, edited, deleted, opened, or rebound unless the corresponding tool result and route/client action evidence support that claim.
- If a mutation tool fails, approval is rejected, or success evidence is missing, say the operation was not confirmed and include the short failure reason when useful.
- Retry at most once with corrected arguments when the error is recoverable. Do not repeat the same mutating call with identical arguments after a failure.
