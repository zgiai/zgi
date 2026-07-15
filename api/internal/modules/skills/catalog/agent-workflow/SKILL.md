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
tool_governance:
  list_agent_workflows:
    tool_id: workflow.list_agent_workflows
    skill_id: agent-workflow
    domain: workflow
    effect: read
    asset_type: workflow
    risk_level: low
    requires_asset_resolution: false
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - workflow:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  run_agent_workflow:
    tool_id: workflow.run_agent_workflow
    skill_id: agent-workflow
    domain: workflow
    effect: invoke
    asset_type: workflow
    risk_level: high
    requires_asset_resolution: true
    reversible: false
    bulk_sensitive: false
    external_side_effect: true
    permission_scopes:
      - workflow:invoke
    default_approval_policy: always_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: true
  get_workflow_run_status:
    tool_id: workflow.get_run_status
    skill_id: agent-workflow
    domain: workflow
    effect: read
    asset_type: workflow_run
    risk_level: low
    requires_asset_resolution: true
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - workflow:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
display:
  icon: workflow
  category: workflow_automation
  scenarios:
    - business_operations
    - technical_development
  label:
    en_US: Agent Workflow
    zh_Hans: Agent 工作流
  description:
    en_US: Designed for running configured approvals or business processes; invokes workflows bound to the current Agent and returns their results.
    zh_Hans: 适用于执行已配置的审批或业务流程，可调用当前智能体绑定的工作流并返回运行结果。
  when_to_use:
    en_US: Use for configured approval or process workflows.
    zh_Hans: 用于已配置的审批或流程工作流。
---

Use this skill only for workflows that are already bound to the current Agent.

Workflow calls are tool-mode calls. They do not take over the conversation stream. Return the tool result to the skill loop and continue from the structured status:

- `succeeded`: use `primary_output` first, then `outputs`, to answer or continue. Do not claim that the workflow produced content that is not present in `primary_output` or `outputs`. If the workflow succeeded but returned no displayable output, tell the user the workflow ran but returned no displayable output and include `workflow_run_id`.
- `pending_approval`: tell the user approval is waiting and include the safe approval entry details from the tool result when useful.
- `failed`: summarize the error and decide whether to retry or ask for corrected input.

Do not invent workflow IDs. The Agent runtime injects an `available_workflows` JSON list when workflows are bound. Use that injected list first to choose a binding. Call `list_agent_workflows` only if the injected list is missing, ambiguous, or stale.

Call `run_agent_workflow` only with a `binding_id` from `available_workflows` or the fallback list result. Pass the user's current request as `inputs.query` unless the binding's `input_schema`, `required_inputs`, or `default_input_key` explicitly says otherwise. After approval resumes, use `get_workflow_run_status` with the returned `workflow_run_id` to query the result.
