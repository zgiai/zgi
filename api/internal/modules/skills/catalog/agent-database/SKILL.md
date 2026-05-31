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
max_calls_per_turn: 12
timeout_seconds: 30
display:
  icon: database
  category: database
  label:
    en_US: Agent Database
    zh_Hans: 智能体数据库
  description:
    en_US: Uses only database tables bound to the current Agent configuration.
    zh_Hans: 仅使用当前智能体配置中绑定的数据库表。
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
