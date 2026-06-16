---
name: work-report-generator
description: Generate weekly or monthly work reports from user-provided progress, tasks, metrics, risks, and plans.
when_to_use: Use this skill when the user asks to write, organize, polish, export, or generate a weekly report, monthly report, work summary, team report, project report, or management update.
provider_type: builtin
provider_id: file_generator
runtime_type: hybrid
tools:
  - generate_file
max_calls_per_turn: 3
timeout_seconds: 5
tool_governance:
  generate_file:
    tool_id: file.generate_report
    skill_id: work-report-generator
    domain: files
    effect: create
    asset_type: file
    risk_level: medium
    requires_asset_resolution: false
    reversible: true
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - file:create
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
display:
  icon: clipboard-list
  category: productivity
  label:
    en_US: Work Report Generator
    zh_Hans: 周报月报生成
  description:
    en_US: Turns work notes, progress, metrics, risks, and plans into structured weekly or monthly reports.
    zh_Hans: 将工作记录、项目进展、关键数据、风险问题和计划整理成结构化周报或月报。
  when_to_use:
    en_US: Use when the user needs a weekly report, monthly report, work summary, project update, or management report.
    zh_Hans: 当用户需要生成周报、月报、工作总结、项目进展汇报或管理汇报时使用。
  tags:
    en_US:
      - Report
      - Productivity
      - Summary
    zh_Hans:
      - 周报
      - 月报
      - 工作总结
---

# Work Report Generator Skill

Use this skill to turn raw work notes into a clear weekly or monthly work report. The output should be practical, structured, and ready to share with a manager, team, or customer.

## Workflow

1. Identify the report type:
   - Use "weekly report" when the user mentions week, this week, last week, weekly, 周报, 本周, 上周, or 下周.
   - Use "monthly report" when the user mentions month, this month, last month, monthly, 月报, 本月, 上月, or 下月.
   - If the period is unclear, infer the safest generic "work report" and avoid inventing dates.
2. Extract available facts from the user's input:
   - completed work
   - in-progress work
   - key metrics or business results
   - blockers, risks, and issues
   - customer, project, or cross-team communication
   - next-period plan
   - support or decisions needed
3. Do not invent progress, metrics, owners, dates, customers, or conclusions. If data is missing, write a concise placeholder such as "To be added" only when the user asked for a reusable template; otherwise omit that section or mark it as "Not provided".
4. Choose a concise structure based on the request:
   - Personal report: Summary, Completed Work, Key Results, Problems/Risks, Next Plan.
   - Team report: Overall Progress, Workstream Updates, Metrics, Risks, Decisions Needed, Next Plan.
   - Project report: Project Status, Milestones, Deliverables, Risks/Issues, Dependencies, Next Steps.
   - Sales/customer report: Customer Progress, Opportunities, Risks, Follow-ups, Next Actions.
5. Keep the writing objective and professional. Prefer specific bullets over broad praise. Merge duplicate items and group related tasks.
6. If the user provides rough notes, preserve the meaning while polishing the wording. If the user provides a requested tone, follow it.
7. If the user asks for a file, export the final report with `generate_file`.
8. If the user did not ask for a file, answer directly in chat and do not call `generate_file`.
9. Never output an internal invocation payload such as `skill_id`, `input`, `arguments`, or JSON that represents this skill call unless the user explicitly asks to see implementation details or a call example.
10. When the user asks to generate, write, organize, or polish a weekly/monthly report, the primary response must be the final report content itself.

## Output Rules

- Use Markdown structure by default.
- Start with a clear title, such as "Weekly Work Report" or "Monthly Work Report".
- Include the reporting period only when the user provided it or it can be safely inferred from explicit context.
- Keep each bullet action-oriented and concrete.
- For risks, include impact and suggested next action when enough context is provided.
- For next-period plans, separate committed work from optional goals when the input makes that distinction.
- Do not expose internal reasoning or mention this skill.
- Do not wrap the answer in JSON unless the user explicitly asks for JSON.
- Do not invent dates, date ranges, metrics, hours, task counts, owners, customers, or progress percentages that were not provided by the user.
- If the user supplies only rough notes, generate a polished report from those notes directly instead of creating a structured input object.

## File Export

Use `generate_file` only when the user asks to export, download, save, or generate a file.

Recommended formats:

- Use `md` for normal reports and editable structured text.
- Use `docx` when the user asks for Word or a formal editable document.
- Use `pdf` when the user asks for a read-only sharing file.
- Use `txt` only for plain text.

When calling `generate_file`:

- `content`: the final report text.
- `format`: selected from `md`, `docx`, `pdf`, or `txt`.
- `filename`: use a short ASCII name such as `weekly-work-report` or `monthly-work-report`.
- `title`: use the report title.
- `lifecycle`: use `persistent` by default.

## Examples

User input:

"帮我根据这些内容写本周周报：完成客户 A 需求评审，推进订单导入功能，修复 3 个线上问题，下周准备联调。"

Expected report shape:

```md
# Weekly Work Report

## Summary

This week focused on customer requirement alignment, order import development, and production issue resolution.

## Completed Work

- Completed the requirement review for Customer A.
- Fixed 3 production issues.

## In Progress

- Continued development of the order import feature.

## Next Plan

- Prepare and start joint integration testing.
```
