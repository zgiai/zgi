---
name: console-navigator
description: Navigate the visible console to a safe internal page when the user asks to open, switch to, or go to another module.
when_to_use: Use this skill when the user wants the assistant to route the current contextual sidebar to files, agents, knowledge bases, databases, prompts, scheduled tasks, workspace, settings, or another whitelisted internal console page.
provider_type: builtin
provider_id: console_navigation
runtime_type: tool
tools:
  - navigate
max_calls_per_turn: 2
timeout_seconds: 15
display:
  icon: compass
  category: workflow_automation
  scenarios:
    - general
    - technical_development
  label:
    en_US: Console Navigator
    zh_Hans: 控制台导航
  description:
    en_US: Designed for quickly reaching another console module; identifies the requested destination and opens an approved internal page without changing business data.
    zh_Hans: 适用于快速前往控制台其他模块，可识别用户要访问的目标页面并安全跳转，不会修改业务数据。
  when_to_use:
    en_US: Use when the user asks to open or switch to another console module.
  tags:
    en_US:
      - Navigation
      - Console
supported_callers:
  - aichat
---

# Console Navigator Skill

Use this skill only to request safe internal console navigation. It does not create, edit, delete, publish, run, schedule, send, or otherwise mutate user assets.

## Available Routes

- Home: `/console`
- Conversations: `/console/work/chat`
- Images: `/console/work/image`
- Apps: `/console/work/app`
- Scheduled tasks: `/console/work/task`
- Agents: `/console/agents`
- Agent detail pages from current context when an exact href is known: `/console/agents/{agentId}`, `/console/agents/{agentId}/logs`, `/console/agents/{agentId}/api`, `/console/agents/{agentId}/batch-test`
- Workflows: `/console/workflows`
- Workflow detail pages from current context when an exact href is known: `/console/workflows/{workflowId}`, `/console/workflows/{workflowId}/logs`, `/console/workflows/{workflowId}/api`, `/console/workflows/{workflowId}/batch-test`
- Knowledge bases: `/console/dataset`
- Databases: `/console/db`
- Files: `/console/files`
- Prompts: `/console/prompts`
- File recognition: `/console/developer/content-parse`
- Workspace: `/console/workspace`, `/console/workspace/members`, `/console/workspace/settings`
- System settings: `/console/settings`

## Workflow

1. Use `navigate` when the user asks to go to, open, switch to, show, or continue in a console module and the destination is in the route list.
2. If the user only asks what the assistant can do or what a module is for, answer directly from the site map instead of navigating.
3. If the requested destination is ambiguous, ask one concise clarification.
4. Do not navigate to external URLs or non-console paths.
5. If the current page context already matches the requested route, do not call `navigate` just to create proof. Continue from the current page context and visible resources.
6. After navigation, pause for the frontend client action result. The sidebar will switch routes, wait for supported target page context, and continue this same assistant turn with the updated page context when available.
7. Navigation is usually a substep, not the user's final goal. After the loaded-route/page-context evidence arrives, continue the remaining requested operation from the new page context instead of ending the turn just because navigation succeeded.
8. If a previous tool result or page fact will be needed after navigation, record it with `submit_turn_state` before calling `navigate`. Use the stored fact after the continuation instead of re-reading or guessing.
9. Never claim that navigation performed an asset operation. If the user asks to delete, publish, run, schedule, create, or modify assets, explain that those actions need a supported governed tool and user approval when available.
10. Agent and Workflow assets are separate console surfaces. Use `/console/agents` for Agent management and `/console/workflows` for Workflow management or inspection. Do not route Workflow requests to the Agents list unless the user explicitly asks for Agents.

## Tool Usage

`navigate` accepts:

- `href`: required whitelisted internal console route.
- `reason`: optional short reason for why that route is relevant.

The tool returns a `page_navigation_requested` event for the sidebar frontend. The frontend performs a second whitelist check before calling Next Router.

## Success Evidence

- A `navigate` tool result means the route request was accepted, not that the target page is already loaded.
- Final navigation success requires frontend client-action evidence such as `route_loaded`, `loaded_href`, `observed_path`, or an equivalent successful page-context continuation matching the requested `href`. If the current page context already matches the requested `href`, that current page context is already sufficient route evidence.
- Do not answer from the old page context after a navigation request. If the route request succeeded but no loaded-page evidence is available yet, say that navigation is pending or wait for the continuation.

## Truthfulness Contract

- Treat client-action route evidence as authoritative for what page is currently visible.
- If navigation fails, is blocked, or does not produce matching loaded-route evidence, do not claim the page is open. Report the actual navigation state.
- Retry at most once with a corrected whitelisted `href` when the route error is recoverable. Do not repeat the same navigation request with identical arguments after a failure.
