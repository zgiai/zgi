---
name: time
description: Answer questions about current time, timezone-aware time lookup, date arithmetic, and date differences.
when_to_use: Use this skill when the user asks for the current time, today's date, timezone conversion, date addition or subtraction, deadlines, durations, or the number of days between dates.
provider_type: builtin
provider_id: time
runtime_type: tool
tools:
  - current_time
  - date_calculate
supported_callers:
  - aichat
  - agent
  - workflow
max_calls_per_turn: 20
timeout_seconds: 5
display:
  icon: clock
  category: productivity
  label:
    en_US: Time
    zh_Hans: 时间
  description:
    en_US: Looks up current time and performs timezone-aware date calculations.
    zh_Hans: 查询当前时间，并进行带时区的日期计算。
  when_to_use:
    en_US: Use when answers depend on current time, dates, deadlines, or date differences.
    zh_Hans: 当问题依赖当前时间、日期、截止日期或日期差时启用。
  tags:
    en_US:
      - Time
      - Date
    zh_Hans:
      - 时间
      - 日期
---

# Time Skill

Use this skill for current time and date calculation tasks.

## Workflow

1. For current time or today's date, call `call_skill_tool` with `tool_name` set to `current_time`.
2. For date addition, date subtraction, deadlines, or date differences, call `call_skill_tool` with `tool_name` set to `date_calculate`.
3. Always include the relevant timezone in the final answer when current time or today is involved.
4. Do not guess current time or today's date without calling the tool.

## Tool Usage

`current_time` accepts:

- `timezone`: IANA timezone such as `UTC`, `Asia/Shanghai`, or `America/New_York`.
- `format`: strftime-style output format such as `%Y-%m-%d %H:%M:%S`.

`date_calculate` accepts:

- `operation`: `add`, `subtract`, or `diff`.
- `base_date`: date in `YYYY-MM-DD` format, or omitted for today.
- `amount`: integer amount for add or subtract.
- `unit`: `day`, `week`, `month`, or `year`.
- `target_date`: required for `diff`.
- `timezone`: IANA timezone used when `base_date` is omitted.
