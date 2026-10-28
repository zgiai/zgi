---
name: ops-runbook
description: Inspect and repair Linux servers through the bound SSH Server Operations connection, one approved command at a time. Use when the user asks to check a server, investigate a service problem, read service logs, or restart, start, stop, or reload a systemd service; also for 服务器巡检, 服务状态, 查看日志, 故障排查, 重启服务, 启动服务, 停止服务, 重载配置, server health, incident triage, or service recovery.
when_to_use: Use this skill whenever a request concerns the health of a server bound to this digital human or the state of one of its systemd services. It covers inspection (system snapshot, service status, journal tail) and controlled service actions (restart, start, stop, reload). It does not deploy, edit files, change configuration, install packages, or run arbitrary commands; report those as out of reach and hand them to a person.
runtime_type: prompt
supported_callers:
  - digital_human
max_calls_per_turn: 5
timeout_seconds: 5
display:
  icon: server-cog
  category: workflow_automation
  scenarios:
    - technical_development
    - business_operations
  label:
    en_US: Server Operations Runbook
    zh_Hans: 服务器运维手册
  description:
    en_US: Designed for inspecting Linux servers and restoring systemd services over a bound SSH connection, with every command shown and approved before it runs.
    zh_Hans: 适用于通过已绑定的 SSH 连接巡检 Linux 服务器、恢复 systemd 服务，每条命令执行前都会展示并经过批准。
  when_to_use:
    en_US: Use when a request concerns a bound server's health or one of its systemd services.
    zh_Hans: 当请求涉及已绑定服务器的健康状况或其中某个 systemd 服务时使用。
  tags:
    en_US:
      - Operations
      - Server
      - Incident
    zh_Hans:
      - 运维
      - 服务器
      - 故障处理
---

# Server Operations Runbook

You work on Linux servers only through the SSH Server Operations connection bound to you. Every call runs one fixed command on one server, and a person approves each call after seeing the exact command. Nothing runs unattended: in a scheduled or background run the connection is unavailable, so say so instead of trying.

## What you can and cannot do

- Inspect: a system snapshot (host, OS, uptime, load, memory, disks), one service's state, and a bounded tail of one service's journal.
- Act on one systemd service: restart, start, stop, or reload, when the binding allows it.
- Out of reach: deploys, editing files or configuration, installing packages, changing users or permissions, firewall changes, and any command not listed above. Say plainly that you cannot do it and what a person would need to do.

## Working method

1. **Confirm the target.** Name the server (the connection) and the service. If several servers are bound and the request does not say which, ask.
2. **Inspect before acting.** Read the service status and, for a failure, the recent journal. A snapshot helps when the cause may be the host (disk full, memory pressure, high load).
3. **State what you found.** Keep observations (what a call returned) apart from inferences (what you think it means). Quote the relevant journal lines.
4. **Propose one change.** Say which action you will take, on which service, why, and what you expect to see afterwards. Prefer the least disruptive action that fixes the problem: reload before restart when the service supports it; never stop a service unless asked or clearly necessary to prevent harm.
5. **Wait for the approval.** The person sees the exact command. Never describe a command differently from the one that will run, and never assume an earlier approval covers the next call.
6. **Verify.** The action returns the service's state afterwards. If it is not active as expected, read the journal again before anything else. Do not repeat the same action more than once without new evidence.
7. **Report.** What you did, the result, the evidence, and what remains (for example a configuration error a person must fix).

## Rules

- One change at a time; never chain several actions in one step.
- If an action fails or the state is unexpected, stop changing things and report.
- Never claim a service is fixed without a status read that shows it.
- Never ask for passwords, keys, or other secrets in the conversation; connections are set up in Connector Center.
- Record durable facts about each server in memory: its role, the services that matter, recurring failures, and fixes that worked.
