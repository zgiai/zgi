---
name: agent-workflow
description: Call workflows explicitly bound to the current Agent as structured process tools.
when_to_use: Use when the current Agent needs to run a configured workflow for approval-driven or process-driven handling.
provider_type: builtin
provider_id: workflow
runtime_type: tool
max_calls_per_turn: 10
timeout_seconds: 600
supported_callers:
  - agent
required_config:
  - agent_workflow
tools:
  - list_agent_workflows
  - run_agent_workflow
  - get_workflow_run_status
display:
  icon: workflow
  category: system
  label:
    en_US: Agent Workflow
    zh_Hans: Agent 工作流
  description:
    en_US: Call workflows bound to this Agent.
    zh_Hans: 调用绑定到当前 Agent 的工作流。
  when_to_use:
    en_US: Use for configured approval or process workflows.
    zh_Hans: 用于已配置的审批或流程工作流。
---

Use this skill only for workflows that are already bound to the current Agent.

Workflow calls are tool-mode calls. They do not take over the conversation stream. Return the tool result to the skill loop and continue from the structured status:

- `succeeded`: use `outputs` to answer or continue.
- `pending_approval`: tell the user approval is waiting and include the safe approval entry details from the tool result when useful.
- `failed`: summarize the error and decide whether to retry or ask for corrected input.

Do not invent workflow IDs. First call `list_agent_workflows` when you need to choose a binding. Call `run_agent_workflow` only with a listed `binding_id`. After approval resumes, use `get_workflow_run_status` with the returned `workflow_run_id` to query the result.
