---
name: internal-database
description: Discover and operate database tables the current AIChat user can access.
when_to_use: Use this skill when an internal AIChat answer needs to inspect, query, insert, update, or delete structured database records.
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
    en_US: Internal Database
    zh_Hans: 内部数据库
  description:
    en_US: Finds accessible databases, inspects tables, and performs structured record operations.
    zh_Hans: 查找可访问的数据库、查看表结构，并执行结构化记录操作。
  when_to_use:
    en_US: Use when AIChat needs facts or changes from workspace database tables.
    zh_Hans: 当 AIChat 需要从工作区数据库表读取事实或写入变更时使用。
  tags:
    en_US:
      - Database
      - Records
    zh_Hans:
      - 数据库
      - 记录
supported_callers:
  - aichat
---

# Internal Database Skill

Use this skill to work with database tables the current AIChat user can access.

## Workflow

1. Call `list_accessible_databases` before using a database ID.
2. Call `list_database_tables` before using a table ID.
3. Call `describe_database_table` before writing records so field names and types are known.
4. Use only database IDs and table IDs returned by these tools.
5. For record changes, call only the structured insert, update, or delete tools. Do not generate or ask to run SQL.

## Tool Usage

Read operations require database view and AI query permissions. Write operations require AI query permission plus database data edit or manage permission.
