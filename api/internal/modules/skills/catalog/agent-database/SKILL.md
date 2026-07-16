---
name: agent-database
description: Operate database tables explicitly bound to the current Agent.
when_to_use: Use this skill when an Agent needs to inspect, query, insert, update, or delete records in its configured database tables.
provider_type: builtin
provider_id: database
runtime_type: tool
tools:
  - list_accessible_databases
  - list_database_tables
  - describe_database_table
  - query_table_records
  - insert_table_records
  - update_table_records
  - delete_table_records
max_calls_per_turn: 40
timeout_seconds: 30
tool_governance:
  list_accessible_databases:
    tool_id: database.list_agent_accessible
    skill_id: agent-database
    domain: database
    effect: read
    asset_type: database
    risk_level: low
    requires_asset_resolution: false
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - database:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  list_database_tables:
    tool_id: database.list_agent_tables
    skill_id: agent-database
    domain: database
    effect: read
    asset_type: database
    risk_level: low
    requires_asset_resolution: true
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - database:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  describe_database_table:
    tool_id: database.describe_agent_table
    skill_id: agent-database
    domain: database
    effect: read
    asset_type: database_table
    risk_level: low
    requires_asset_resolution: true
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - database:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  query_table_records:
    tool_id: database.query_agent_records
    skill_id: agent-database
    domain: database
    effect: read
    asset_type: database_table
    risk_level: low
    requires_asset_resolution: true
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - database:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  insert_table_records:
    tool_id: database.insert_agent_records
    skill_id: agent-database
    domain: database
    effect: create
    asset_type: database_table
    risk_level: medium
    requires_asset_resolution: true
    reversible: false
    bulk_sensitive: true
    external_side_effect: false
    permission_scopes:
      - database:write
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: true
  update_table_records:
    tool_id: database.update_agent_records
    skill_id: agent-database
    domain: database
    effect: update
    asset_type: database_table
    risk_level: medium
    requires_asset_resolution: true
    reversible: false
    bulk_sensitive: true
    external_side_effect: false
    permission_scopes:
      - database:write
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: true
  delete_table_records:
    tool_id: database.delete_agent_records
    skill_id: agent-database
    domain: database
    effect: delete
    asset_type: database_table
    risk_level: high
    requires_asset_resolution: true
    reversible: false
    bulk_sensitive: true
    external_side_effect: false
    permission_scopes:
      - database:write
    default_approval_policy: always_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: true
display:
  icon: table-properties
  category: data_analysis
  scenarios:
    - data_insights
    - technical_development
  label:
    en_US: Agent Database
    zh_Hans: 智能体数据库
  description:
    en_US: Designed for Agent tasks that need business data; searches, analyzes, and updates records only in tables bound to the current Agent.
    zh_Hans: 适用于智能体查询或维护业务数据，可在当前智能体绑定的数据库表中检索、分析和更新记录。
  when_to_use:
    en_US: Use for Agent answers or actions that need configured database records.
    zh_Hans: 当智能体回答或操作需要使用已配置的数据库记录时使用。
  tags:
    en_US:
      - Database
      - Agent
    zh_Hans:
      - 数据库
      - 智能体
supported_callers:
  - agent
required_config:
  - agent_database
---

# Agent Database Skill

Use this skill to work only with database tables configured on the current Agent.

## Workflow

1. Call `list_accessible_databases` to see the databases bound to this Agent by the binding editor.
2. Call `list_database_tables` to see the Agent-bound tables in a selected database.
3. Call `describe_database_table` before writing records.
4. Never ask for or invent database IDs or table IDs. The backend rejects resources outside the Agent binding.
5. Use only structured record tools. Do not generate or ask to run SQL.

## Tool Usage

Access is scoped to tables bound on the Agent. Permissions are authorized when the binding is saved by the editor, and runtime access uses that binding authorization. Mutation tools are available only for tables explicitly marked writable in the Agent binding.
