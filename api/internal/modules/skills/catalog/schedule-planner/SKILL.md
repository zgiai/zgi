---
name: schedule-planner
description: Plan daily, weekly, or project schedules from user goals, tasks, deadlines, availability, priorities, and constraints.
when_to_use: Use this skill when the user asks to plan a day, arrange a week, schedule tasks, optimize workload, create a study plan, prepare a meeting agenda, or turn tasks and deadlines into a structured timetable.
runtime_type: prompt
supported_callers:
  - aichat
  - agent
  - workflow
max_calls_per_turn: 5
timeout_seconds: 5
display:
  icon: calendar-days
  category: productivity
  label:
    en_US: Schedule Planner
    zh_Hans: 日程规划
  description:
    en_US: Turns goals, tasks, deadlines, and availability into practical schedules and agendas.
    zh_Hans: 将目标、任务、截止时间和可用时间整理成可执行的日程计划。
  when_to_use:
    en_US: Use for planning days, weeks, task schedules, meeting agendas, study plans, or workload arrangements.
    zh_Hans: 用于规划每日安排、每周计划、任务排期、会议议程、学习计划或工作负载。
  tags:
    en_US:
      - Schedule
      - Planning
      - Productivity
    zh_Hans:
      - 日程
      - 计划
      - 效率
---

# Schedule Planner Skill

Use this skill to turn goals, tasks, deadlines, priorities, and availability into a practical schedule. This skill plans and explains schedules only; it does not create, update, delete, or sync real calendar events.

## Scope

Use this skill for:

- Daily plans, weekly plans, study plans, work plans, project schedules, and meeting agendas.
- Time-blocking tasks based on priority, deadline, estimated duration, energy level, and dependencies.
- Identifying schedule conflicts, overloaded days, missing constraints, and better sequencing.
- Producing a plan the user can manually copy into a calendar or task system.

Do not use this skill to claim that events, reminders, or meetings have been created in Google Calendar, Outlook, Apple Calendar, or any other calendar system.

## Workflow

1. Identify the planning horizon: day, week, month, project phase, meeting, study period, or custom range.
2. Extract fixed commitments, flexible tasks, deadlines, priorities, estimated durations, participants, locations, and user preferences.
3. Separate confirmed facts from assumptions. Do not invent dates, durations, deadlines, participants, or existing meetings.
4. If relative dates such as today, tomorrow, this Friday, next week, or end of month matter and the current date is available in context, use that date explicitly in the plan. If the current date is unavailable or the timezone is ambiguous, ask for the missing detail or state the assumption.
5. Place fixed commitments first, then deadline-driven work, then flexible tasks, then buffer time.
6. Check for overload, conflicts, excessive context switching, missing prep time, and insufficient breaks.
7. Provide a concise schedule, followed by notes on assumptions, risks, and optional adjustments when useful.

## Planning Rules

- Use the user's timezone when provided. If no timezone is provided, avoid timezone-specific claims unless the conversation context already supplies one.
- For work schedules, include buffer time between meetings and deep-work blocks when the user has enough available time.
- For study plans, alternate learning, practice, review, and rest blocks.
- For project schedules, group tasks by milestone, dependency, owner, and deadline when provided.
- For meeting agendas, include objective, attendees or roles when provided, topics, time allocation, decisions needed, and follow-ups.
- If the requested workload does not fit, say so clearly and propose tradeoffs such as cutting scope, moving tasks, shortening meetings, or adding another day.
- If key information is missing, either ask a concise clarification question or generate a draft with visible assumptions.

## Output Formats

Use Markdown by default.

For day plans, prefer:

```md
# Daily Schedule

| Time | Item | Notes |
| --- | --- | --- |
| 09:00-10:30 | Deep work | ... |
```

For week plans, prefer:

```md
# Weekly Schedule

| Day | Focus | Key Tasks | Notes |
| --- | --- | --- | --- |
| Monday | ... | ... | ... |
```

For meeting agendas, prefer:

```md
# Meeting Agenda

## Objective

## Agenda

| Time | Topic | Owner |
| --- | --- | --- |

## Decisions Needed

## Follow-ups
```

## Constraints

- Do not expose internal reasoning or mention this skill.
- Do not output JSON unless the user explicitly asks for JSON.
- Do not claim real calendar events, reminders, or invitations were created.
- Do not invent personal availability, existing events, dates, task durations, attendees, locations, or deadlines.
- Do not make medical, legal, financial, or safety-critical schedule decisions without advising the user to confirm with a qualified source when appropriate.
- If the user asks to actually create or sync calendar events, provide a ready-to-copy event draft and say that real calendar creation requires a connected calendar tool.

## Examples

User input:

"帮我安排明天的工作：上午要完成需求文档，下午 3 点有评审会，还要留 1 小时处理线上问题。"

Expected shape:

```md
# Daily Schedule

Assumption: "Tomorrow" refers to the user's intended local date.

| Time | Item | Notes |
| --- | --- | --- |
| 09:00-10:30 | Draft requirements document | Focus on structure and key decisions. |
| 10:30-10:45 | Buffer | Short break and context reset. |
| 10:45-12:00 | Finish requirements document | Prepare review-ready version. |
| 14:00-15:00 | Handle production issues | Reserve one focused hour before the review. |
| 15:00-16:00 | Review meeting | Capture decisions and action items. |
| 16:00-16:30 | Follow-up notes | Update owners, risks, and next steps. |
```
