---
name: console-navigator
description: Navigate the visible ZGI console to a safe internal page when the user asks to open, switch to, or go to another module.
when_to_use: Use this skill when the user wants AIChat to route the current ZGI sidebar experience to files, agents, knowledge bases, databases, prompts, scheduled tasks, workspace, settings, or another whitelisted internal console page.
provider_type: builtin
provider_id: console_navigation
runtime_type: tool
tools:
  - navigate
max_calls_per_turn: 2
timeout_seconds: 15
display:
  icon: route
  category: productivity
  label:
    en_US: Console Navigator
  description:
    en_US: Routes AIChat to whitelisted internal ZGI console pages without mutating assets.
  when_to_use:
    en_US: Use when the user asks to open or switch to another ZGI module.
  tags:
    en_US:
      - Navigation
      - Console
supported_callers:
  - aichat
---

# Console Navigator Skill

Use this skill only to request safe internal ZGI console navigation. It does not create, edit, delete, publish, run, schedule, send, or otherwise mutate user assets.

## Available Routes

- Home: `/console`
- Conversations: `/console/work/chat`
- Images: `/console/work/image`
- Apps: `/console/work/app`
- Scheduled tasks: `/console/work/task`
- Agents: `/console/agents`
- Agent detail pages from current context when an exact href is known: `/console/agents/{agentId}/agent`, `/console/agents/{agentId}/workflow`, `/console/agents/{agentId}/logs`, `/console/agents/{agentId}/api`, `/console/agents/{agentId}/batch-test`
- Knowledge bases: `/console/dataset`
- Databases: `/console/db`
- Files: `/console/files`
- Prompts: `/console/prompts`
- File recognition: `/console/developer/content-parse`
- Workspace: `/console/workspace`, `/console/workspace/members`, `/console/workspace/settings`
- System settings: `/console/settings`

## Workflow

1. Use `navigate` when the user asks to go to, open, switch to, show, or continue in a ZGI module and the destination is in the route list.
2. If the user only asks what AIChat can do or what a module is for, answer directly from the site map instead of navigating.
3. If the requested destination is ambiguous, ask one concise clarification.
4. Do not navigate to external URLs or non-console paths.
5. After navigation, continue the same conversation normally. The frontend will switch routes and provide the updated page context on the next user turn.
6. Never claim that navigation performed an asset operation. If the user asks to delete, publish, run, schedule, create, or modify assets, explain that those actions need a supported governed tool and user approval when available.

## Tool Usage

`navigate` accepts:

- `href`: required whitelisted internal console route.
- `reason`: optional short reason for why that route is relevant.

The tool returns a `page_navigation_requested` event for the sidebar frontend. The frontend performs a second whitelist check before calling Next Router.
