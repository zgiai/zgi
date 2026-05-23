---
name: calculator
description: Perform deterministic arithmetic, percent-based math, and change-rate math.
when_to_use: Use this skill when the user asks for exact arithmetic, percents, growth or decrease rates, discounts, markups, ratios, or numeric comparisons that should not be estimated mentally.
provider_type: builtin
provider_id: calculator
runtime_type: tool
tools:
  - evaluate_expression
  - calculate
  - percentage
max_calls_per_turn: 100
timeout_seconds: 5
display:
  icon: calculator
  category: productivity
  label:
    en_US: Calculator
    zh_Hans: 计算器
  description:
    en_US: Handles exact arithmetic, percentages, discounts, and numeric comparisons.
    zh_Hans: 用于精确计算、百分比、折扣、变化率和数值比较。
  when_to_use:
    en_US: Use when answers require exact calculation instead of mental math.
    zh_Hans: 当问题需要精确计算，而不是让模型心算时启用。
  tags:
    en_US:
      - Math
      - Arithmetic
    zh_Hans:
      - 数学
      - 精确计算
---

# Calculator Skill

Use this skill for exact arithmetic and percentage tasks.

## Workflow

1. Prefer one `call_skill_tool` call with `tool_name` set to `evaluate_expression` when the task can be represented as one arithmetic expression.
2. For totals followed by discounts, markups, taxes, or fixed reductions, fold the full final arithmetic into one expression whenever the condition is clear from the problem statement.
3. If a condition must be verified numerically first, use at most two expression calls: one for the condition value and one for the final result.
4. Use `calculate` only when a single binary operation is clearer than a full expression.
5. Use `percentage` only when the user asks specifically for percent-of, percentage change, or applying a percentage increase/decrease and a single expression would be less clear.
6. Do not calculate mentally when the user expects an exact result.
7. In the final answer, include a concise summary of the calculation process and the result.
8. Mention rounding when a precision value affects the displayed result.

## Tool Usage

`evaluate_expression` accepts:

- `expression`: a restricted arithmetic expression using numbers, parentheses, `+`, `-`, `*`, `/`, `%`, and `^`.
- `precision`: optional decimal places from 0 to 12.

Use decimal percentages inside expressions. For example, use `0.85` for an 85 percent multiplier and `0.15` for a 15 percent decrease.

Example for a grocery total with a fixed reduction and member discount:

- `expression`: `(9.5*10 + 4.8*15 + 15*5 - 30) * 0.85`
- `precision`: `2`

`calculate` accepts:

- `operation`: `add`, `subtract`, `multiply`, `divide`, `power`, or `mod`.
- `left`: left operand.
- `right`: right operand.
- `precision`: optional decimal places from 0 to 12.

`percentage` accepts:

- `operation`: `percent_of`, `change`, `apply_increase`, or `apply_decrease`.
- `value`: base value for `percent_of`, `apply_increase`, and `apply_decrease`.
- `percent`: percentage value, for example `15` means 15 percent.
- `from`: original value for `change`.
- `to`: new value for `change`.
- `precision`: optional decimal places from 0 to 12.
