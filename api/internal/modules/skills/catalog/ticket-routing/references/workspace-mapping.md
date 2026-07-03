# Workspace Mapping

## Use When

Use when the user wants to map a ticket to a platform workspace, department workspace, owner role, or department负责人.

## Mapping Rules

- Use explicit mapping rules from the user, platform context, tenant configuration, or previous conversation.
- If mapping rules provide workspace names or IDs, preserve them exactly.
- If only department names are known, recommend department-level routing and mark workspace as `待映射`.
- If multiple workspaces could match, list candidates and mark `需人工确认`.
- Do not invent workspace IDs, owner user IDs, duty schedules, or负责人 names.

## Suggested Mapping Table Shape

If mapping rules are provided, apply them in this order:

1. Exact issue type mapping.
2. Department mapping.
3. Property/project/building mapping.
4. Urgency escalation mapping.
5. Fallback customer service workspace.

## Output Fields

Include:

- 建议工作空间
- 映射依据
- 是否需要人工确认
- 候选工作空间, when more than one target is plausible.

## Clarification Rules

Call `request_user_input` when:

- The user asks to route to a workspace but no workspace mapping is available.
- The issue involves several departments and no primary workspace can be selected.
- A project/building/property scope is required but missing.
- The user expects real dispatch rather than a recommendation.
