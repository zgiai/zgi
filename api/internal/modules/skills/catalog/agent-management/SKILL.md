---
name: agent-management
description: Manage Agent assets and governed Agent bindings from the contextual console assistant when the user has the required workspace permissions.
when_to_use: Use this hidden skill when the user asks the assistant to create an Agent, edit Agent details, delete an Agent, inspect Agent draft config, query available Agent runtime models, update supported draft config fields, or replace Agent skill/knowledge/database/workflow bindings.
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
  - list_available_models
max_calls_per_turn: 14
timeout_seconds: 90
tool_governance:
  list_agents:
    tool_id: agent.list
    skill_id: agent-management
    domain: agents
    effect: read
    asset_type: agent
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
  get_agent:
    tool_id: agent.get
    skill_id: agent-management
    domain: agents
    effect: read
    asset_type: agent
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
  get_agent_config:
    tool_id: agent.get_config
    skill_id: agent-management
    domain: agents
    effect: read
    asset_type: agent
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
  list_available_models:
    tool_id: agent.list_available_models
    skill_id: agent-management
    domain: agents
    effect: read
    asset_type: llm_model
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
    en_US: Performs governed Agent asset operations from the contextual console assistant.
    zh_Hans: 在控制台操作助手中执行受治理保护的智能体资产操作。
  when_to_use:
    en_US: Use when the user explicitly asks to manage Agents in the current console.
    zh_Hans: 当用户明确要求在当前控制台管理智能体时使用。
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

Use this skill for governed Agent asset operations in the contextual console assistant. It is intentionally separate from Agent runtime skills: this skill changes Agent assets in the console; it does not invoke Agents, publish Agents, roll back versions, expose API keys, or run bound workflows.

## Workflow

1. Treat backend-backed current page `visible_agents` as authoritative resolved targets for list-page ordinal references. Agent detail pages expose only `current_agent`; they do not define a visible Agent list.
2. Use `list_agents` when no fresh backend-backed list context is available, when a mutation made that list stale, or when the user asks to search beyond the current page query.
3. When resolving a named Agent for mutation, do at most one exact-name `list_agents` search and, if needed, one broader workspace list/check. If neither proves a target, stop without requesting governance approval or deleting/modifying anything, and report the missing target with the evidence you checked. Do not keep retrying with near-duplicate keywords.
4. Use `get_agent` when the user asks about one Agent's basic information and visible/page context does not already answer it.
5. Use `create_agent` when the user asks to create a new Agent. Create only `AGENT` type drafts in the current workspace unless a target workspace is explicitly available in context. This skill does not create, edit, delete, or configure Workflow assets; Workflows live under `/console/workflows` and need a separate Workflow-specific capability.
6. Use `update_agent_identity` for Agent name, description, or icon changes. `update_agent_config` does not change Agent name, description, icon text, or icon background.
   When one user turn asks to change both identity fields and runtime/draft config fields, plan and execute both tools: `update_agent_identity` for identity fields and `update_agent_config` for config fields. Do not finish after only one of them unless the other tool failed or the user no longer wants that part.
7. Use `get_agent_config` before changing draft runtime configuration if the current config is not already known from page context. Exception: for an incremental system-prompt append or managed-section upsert, call `update_agent_config.system_prompt_patch` directly; the service reads and locks the current prompt at execution time, so do not fetch and retransmit it.
8. For read-only current configuration checks, `get_agent_config` is enough to answer the Agent's name, description, icon, model/provider, prompt, memory/file-upload settings, and currently bound Skill/knowledge/database/workflow counts. Do not call candidate-list tools or table-list tools just to inspect existing bindings or counts; call them only when the user asks what resources are available/bindable/selectable, or when a bind/unbind/replace operation needs exact candidate IDs. Do not call `get_current_page_context`; current page context is injected by the runtime and is not a skill tool.
9. Use `list_available_models` before replacing an Agent model unless the user already provided an exact provider/model pair from current page context. Default to `use_case: "agent"` for ordinary Agent runtime model replacement.
10. Use `update_agent_config` for supported draft fields: system prompt, model provider/model, model parameters, Agent memory switch, file upload switch, home title, input placeholder, theme color, suggested questions, and Agent skill/knowledge/database/workflow binding edits. Prefer one `update_agent_config` call when the user asks to change multiple config sections in the same turn. When the desired system prompt is already saved as a TXT or Markdown file in File Management, pass `system_prompt_source: {"type":"managed_file","file_id":"..."}` to replace the prompt without reading and retransmitting the full file content. To append saved file content exactly, use `system_prompt_patch.operation=append`. To integrate a derived summary or reusable context while preserving unrelated prompt structure, use `operation=upsert_section` with a stable `section_id`, optional `section_title`, and a managed-file or short-text source; the same section ID replaces only that managed section on later updates. The optional `separator` defaults to `\n\n` and is used when a new block is appended.
11. To replace the Agent model, choose one item from `list_available_models` and pass that item's `provider` as `model_provider` and `model` as `model` to `update_agent_config`.
12. Use `replace_agent_memory_slots` to replace the full Agent memory slot list. If the user asks to enable memory and create slots, call `update_agent_config` with `agent_memory_enabled: true`, then call `replace_agent_memory_slots`.
13. Use `list_agent_skill_candidates` before replacing Agent skill bindings unless the requested skills are already exact visible/page-context candidates. Pass the user's named Skill as `query` when available. Only bind skill IDs returned by that tool or already visible in current Agent context. If the requested Skill is not returned, stop that Skill binding change without requesting approval, and explain that no matching Skill candidate was found.
14. Use `update_agent_config` with `add_enabled_skill_ids` or `remove_enabled_skill_ids` when adding or removing specific Agent skills. Use `enabled_skill_ids` only when the user asks to replace or clear the full skill list. Preserve existing skills not mentioned by the user.
15. Use the candidate list tools before replacing Agent knowledge, database, or workflow bindings unless the requested resources are already exact visible/page-context candidates. Candidate tools are scoped by the backend to the resolved Agent's workspace; do not provide or infer a workspace ID. Select resources by visible name; do not invent skill IDs, dataset IDs, data source IDs, table IDs, workflow IDs, or binding IDs.
16. Prefer one `update_agent_config` call with `add_knowledge_dataset_ids` / `remove_knowledge_dataset_ids`, `add_database_bindings` / `remove_database_bindings`, and/or `add_workflow_bindings` / `remove_workflow_bindings` after the current binding set and exact candidates are known. Use full replacement fields (`knowledge_dataset_ids`, `database_bindings`, `workflow_bindings`) only when the user asks to replace or clear an entire section. Preserve existing bindings not mentioned by the user. Never pass the resources the user asked to unbind as a replacement list; use the matching `remove_*` field.
17. Binding replacement summaries must mention user-visible resource names such as skill names, knowledge base names, database table names, or workflow labels. Do not expose raw skill IDs, dataset IDs, table IDs, workflow IDs, binding IDs, workspace IDs, or grant/correlation IDs in the final answer unless the user explicitly asks for technical identifiers.
18. Do not change publishing state, API settings, or invocation behavior in this MVP.
19. Use `delete_agent` only when one target Agent is resolved by exact ID or exact visible/listed name. Use `delete_agents` once when the user asks to delete multiple Agents, a range such as the first N visible Agents, selected Agents, or all listed Agents. Deletion is irreversible and governance approval will pause execution when required.
20. Do not ask for a separate natural-language confirmation before governed operations. Tool governance owns approval. If approval is rejected, continue safely and explain that no mutation was performed.
21. If the target Agent or requested fields/resources are ambiguous, ask one concise clarification instead of guessing.
22. Navigation is not a default completion step for ordinary Agent edits, binding changes, unbinding, or list-page batch deletion. Prefer refreshed page context or asset observation after the mutation.
23. After `create_agent` succeeds, route to `/console/agents/{agentId}` only when the user asked to open the new Agent or the operation needs the detail page for follow-up edits. If the frontend client action already loaded that route, do not navigate again.
24. If `delete_agent` succeeds while the current page is that Agent's detail page, or `delete_agents` succeeds and includes the current detail Agent, use `console-navigator` to route to `/console/agents` before the final answer. When deleting multiple Agents from the list page, do not navigate after the first item; rely on page refresh/observation and the batch `item_results`.
25. A successful mutation result is the next step's primary evidence. After `create_agent`, use the returned `agent_id`/`detail_href` for follow-up `update_agent_config`, navigation, and verification in the same turn. Do not search for the newly created Agent by name unless the tool result is missing the ID or verification requires a fresh list.
26. Within one assistant turn, once this skill has been loaded, do not reload it just because tool governance approval, navigation, refresh, or client-action continuation resumed the loop. Continue from the latest tool result, client-action evidence, page context, and `turn_state`.
27. When a later Agent field must reuse a value derived from another tool, such as a file summary/theme, selected model, selected Skill, or chosen target Agent, first preserve the reusable fact with `submit_turn_state` before crossing approval, navigation, refresh, or another tool phase. Use the stored exact fact later instead of placeholders such as `file content`, `读取到的内容`, or `previous result`. A saved managed file ID is already the durable handoff for `system_prompt_source` or `system_prompt_patch`; do not submit the file body to turn state, navigate back to File Management, or read the file again solely to update the Agent prompt.

## Agent Capability Semantics

Use capability semantics to decide what configuration or binding actually proves that an Agent has the capability the user requested. Do not equate a natural-language prompt change with tool/data access unless the matching configuration evidence also exists.

- **Autonomous operation loop**: Treat any external turn strategy as phase guidance, not as a fixed tool script. In each step, choose the next tool from the enabled schemas, the latest tool result, current page evidence, and the remaining user-visible goal. After governance approval resumes, continue from the latest successful or failed tool result instead of restarting discovery.
- **Concrete capability mapping**: Convert user-facing capability requests into the minimum config or binding that actually grants that capability. If one request has multiple independent capability parts, update all requested parts before the final answer.
- **Model capability**: powered by the pair `model_provider` + `model`. Resolve candidates with `list_available_models`, update both fields together with `update_agent_config`, then verify the same pair with `get_agent_config`.
- **Persona or behavior**: powered by `system_prompt`. This changes how the Agent should behave, but it does not add tools, file generation, databases, knowledge, workflow access, or memory by itself.
- **File upload capability**: powered by `file_upload_enabled`. This lets users upload files into the Agent chat surface; it does not let the Agent generate files or manage File Management assets.
- **Skill-backed capability**: powered by `enabled_skill_ids`. For requests such as “make this Agent able to generate files/charts/images or use a tool,” resolve a matching Skill with `list_agent_skill_candidates`, bind the returned Skill ID with `update_agent_config.add_enabled_skill_ids`, and verify `get_agent_config.enabled_skill_ids`.
- **Memory capability**: powered by `agent_memory_enabled` and, when the user asks for concrete memory slots, `replace_agent_memory_slots`. A prompt saying “remember things” is not persistent memory.
- **Knowledge access**: powered by `knowledge_dataset_ids`. Resolve exact knowledge candidates when needed, bind or unbind with the matching `add_knowledge_dataset_ids` / `remove_knowledge_dataset_ids`, then verify `get_agent_config`.
- **Database table access**: powered by `database_bindings`. Resolve database and table candidates with the candidate tools, copy returned binding objects into `add_database_bindings` / `remove_database_bindings`, then verify `get_agent_config`.
- **Workflow access**: powered by `workflow_bindings`. Resolve workflow binding candidates, use `add_workflow_bindings` / `remove_workflow_bindings`, then verify `get_agent_config`.
- **Suggested questions**: powered by `suggested_questions`. This only changes starter prompts shown to users.

Common examples:

- "Make this Agent generate files" means resolve and bind a file-generation Skill. It does not mean only enabling `file_upload_enabled`.
- "Let users upload files to this Agent" means set `file_upload_enabled: true`. It does not mean binding file generation.
- "Make this Agent generate files and accept uploads" means do both: bind a file-generation Skill and set `file_upload_enabled: true`.
- "Use deepseek flash" means call `list_available_models` with the phrase, then update both `model_provider` and `model` from one returned candidate.
- "Write a prompt so it can do X" changes `system_prompt`; if X requires tools/data/workflows, also add the matching skill or resource bindings.
- If a config value is derived from a previous read tool, use the actual tool result text or stored turn-state fact. Never substitute placeholder words such as `file content`, `read content`, or `content value`.

Result chaining examples:

- Create then configure: after `create_agent` returns `agent_id`, use that same `agent_id` for `update_agent_config`; do not call `list_agents` only to find the Agent you just created.
- Delete then create: after `delete_agent` or `delete_agents` succeeds, continue with the next requested create/config step instead of re-checking the deleted target unless the result reports a failure.
- Read file then create Agent from theme: after `file-reader/read_file`, summarize the reusable theme with `submit_turn_state`, then use that exact summary in `create_agent`/`update_agent_config`.
- Configure then verify: after `update_agent_config`, call `get_agent_config` only when the user needs verification or the next step depends on confirmed config; final claims must follow `updated_fields`, `config_changes`, and the returned draft config.

For read-only questions such as “can this Agent generate files?” or “does this Agent have memory?”, inspect the relevant config and candidate evidence, then answer from that evidence without mutating. If the capability is missing and the user later says “进行处理/继续/那就做,” continue from the inspected capability goal instead of starting an unrelated old action.

## Tool Usage

`list_agents` accepts:

- `workspace_id`: optional. Usually omit it so the current assistant workspace context is used.
- `keyword`: optional search keyword.
- `page`: optional one-based page number.
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
- If the requested Agent name, description, prompt, or config value is derived from a previous tool result, use the actual returned value from that tool. For example, if the user says to name the Agent after the content you read from a file, use the `file-reader/read_file` content value, not the literal placeholder text such as `file content`, `文件内容`, or `读取到的内容`.

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
- `system_prompt_source`: optional managed-file reference with `type: managed_file` and `file_id`. It is mutually exclusive with `system_prompt`. Prefer this for saved TXT/Markdown content so the service revalidates and reads the file at execution time without putting the full prompt in the model tool arguments.
- `system_prompt_patch`: optional incremental prompt update. `operation: append` appends the source exactly. `operation: upsert_section` requires a stable ASCII `section_id` and accepts an optional one-line `section_title`; it inserts a managed section when absent and replaces only that section when present. Set `source` to either `{"type":"managed_file","file_id":"..."}` or `{"type":"text","text":"..."}`. Optional `separator` defaults to `\n\n` and is used when appending a new block. It is mutually exclusive with `system_prompt` and `system_prompt_source`. The service reads and locks the current prompt, verifies the frozen baseline, and applies the patch at execution time; do not call `get_agent_config` only to reconstruct the old prompt. For a managed file that was just generated and saved, reuse its returned `file_id` directly without navigating back to File Management or reading its content.
- For model replacement, `model_provider` and `model` must be provided together from the same `list_available_models` result item. Never pass only one of them, because model IDs can collide across providers and provider-only changes can leave an invalid pair.
- For model replacement from a natural-language model phrase, call `list_available_models` with `query` set to the user's phrase (for example `deepseek flash`). Use the returned ranking and `match` evidence to choose one returned provider/model pair, then pass only that pair to `update_agent_config`. If no returned item matches the requested phrase, do not guess; ask for clarification or explain the available options.
- `enabled_skill_ids`: optional full list of enabled user-selectable skill IDs. Use `[]` to clear all user-selectable skills.
- `add_enabled_skill_ids`: optional skill IDs to add while preserving all existing skills.
- `remove_enabled_skill_ids`: optional skill IDs to unbind while preserving all other existing skills.
- `knowledge_dataset_ids`: optional full list of knowledge dataset IDs. Use `[]` to clear knowledge bindings.
- `add_knowledge_dataset_ids`: optional knowledge dataset IDs to bind while preserving existing knowledge bindings.
- `remove_knowledge_dataset_ids`: optional knowledge dataset IDs to unbind while preserving other knowledge bindings.
- `knowledge_retrieval_config`: optional replacement knowledge retrieval config. Omit to preserve it.
- `database_bindings`: optional JSON array replacing database bindings. Each item supports `data_source_id`, `table_ids`, optional `writable_table_ids`, or `id` / `database_table_ids` values in `data_source_id:table_id` form. Use `[]` to clear database bindings.
- `add_database_bindings`: optional JSON array of database table bindings to add while preserving other database table bindings. Prefer copying `binding_candidates[].binding` from `list_agent_database_tables`.
- `remove_database_bindings`: optional JSON array of database table bindings to unbind while preserving other database table bindings. Prefer copying current `database_bindings` from `get_agent_config`.
- `workflow_bindings`: optional JSON array replacing workflow bindings. Each item uses a candidate `binding_id` and preserves returned fields such as `label`, `agent_id`, `workflow_id`, `version_strategy`, and optional `version_uuid`. Use `[]` to clear workflow bindings.
- `add_workflow_bindings`: optional JSON array of workflow bindings to add while preserving other workflow bindings.
- `remove_workflow_bindings`: optional JSON array of workflow bindings to unbind while preserving other workflow bindings.
- For specific bind/unbind requests, use the matching `add_*` or `remove_*` parameter. Use `[]` for a full replacement field only when the user asks to clear that whole binding section. Candidate list results are evidence for choosing targets, not the desired replacement state for unbind requests.
- `display_names`: optional evidence-only object for governance cards, event summaries, and final answers. It does not change execution or validation. When you use candidate/list results, pass maps such as `skills`, `knowledge_bases`, `database_tables`, and `workflows`; database table keys should prefer `data_source_id:table_id`.
- The result includes `updated_fields` and may include `config_changes`/`binding_changes` with `change_action` (`bind`, `unbind`, `replace`, or `update`). Only claim a field or binding changed when those fields or the returned draft `config` prove it.

`list_available_models` accepts:

- `use_case`: optional model use case. Defaults to `agent` for Agent runtime model replacement. Use `all` only when the user asks to inspect every available model.
- `provider`: optional provider slug filter.
- `query`: optional natural-language model phrase. Matching models are ranked first and include `match` evidence.
- `limit`: optional maximum result count, capped at 100.

The result includes `models[].provider`, `models[].model`, `models[].model_name`, `models[].use_cases`, optional `models[].match`, and key capability flags. Use exactly those returned `provider` and `model` values together when calling `update_agent_config`.

`replace_agent_memory_slots` accepts:

- `agent_id`: required resolved Agent ID.
- `agent_memory_slots`: required JSON array replacing the complete slot list. Each item supports `key`, `description`, `enabled`, and optional `sort_order`; use `[]` to clear all slots. Preserve existing slots unless the user asked to replace or remove them.

`list_agent_skill_candidates` accepts:

- `agent_id`: required resolved Agent ID.
- `query`: optional search text for narrowing candidate skills.
- `limit`: optional maximum result count.
- `include_selected`: optional. Defaults to true; set false to exclude currently enabled skills.

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
- The result includes `binding_candidates`; for binding, copy a candidate's `binding` object directly into `update_agent_config.add_database_bindings`. Do not manually recombine a database ID from one result with a table ID from another result.

`list_agent_workflow_binding_candidates` accepts:

- `agent_id`: required resolved Agent ID.
- `query`: optional search text for narrowing candidate workflows.
- `agent_type`: optional filter, usually `WORKFLOW` or `CONVERSATIONAL_WORKFLOW`.
- `limit`: optional maximum result count.
- `include_start_inputs`: optional. Defaults to true.
- `include_selected`: optional. Defaults to true; set false to exclude currently bound workflows.

## Success Evidence

- `create_agent` succeeds only when the tool result includes an `agent_id` and the created Agent name or detail href.
- `update_agent_identity` and `replace_agent_memory_slots` succeed only when the tool result confirms the requested `agent_id` and the changed fields or returned draft state.
- `update_agent_config` succeeds only for fields listed in `updated_fields` for the requested `agent_id`. For binding changes, use `config_changes`/`binding_changes` as the authoritative action summary; do not claim omitted fields were changed, even if they were part of the user's original request.
- `delete_agent` succeeds only when the tool result confirms deletion for the requested `agent_id`.
- `delete_agents` succeeds as a batch only according to `operation_group.item_results`: report exactly how many targets succeeded and failed, and name failed targets when present. Do not treat one succeeded item as proof that the whole batch finished.
- `list_available_models` is read-only evidence. When replacing a model, use one returned item and pass its `provider` and `model` together; final answers must not claim the model changed until `update_agent_config` succeeds.

## Truthfulness Contract

- Treat Agent Management tool results as authoritative. Do not claim an Agent was created, edited, deleted, opened, or rebound unless the corresponding tool result and route/client action evidence support that claim.
- If a mutation tool fails, approval is rejected, or success evidence is missing, say the operation was not confirmed and include the short failure reason when useful.
- Retry at most once with corrected arguments when the error is recoverable. Do not repeat the same mutating call with identical arguments after a failure.
