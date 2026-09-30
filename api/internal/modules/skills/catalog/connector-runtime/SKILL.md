---
name: connector-runtime
description: Use the native Connector operations attached to this conversation or Agent.
when_to_use: Use when the user requests information or an action from an attached personal connection.
display:
  name:
    en_US: Connectors
    zh_Hans: 连接器
  description:
    en_US: Designed for using personal connections attached to the current conversation or Agent.
    zh_Hans: 适用于使用当前对话或智能体已添加的个人连接。
  category: workflow_automation
  scenarios: [technical_development, knowledge_research]
  icon: cable
runtime_type: prompt
max_calls_per_turn: 20
timeout_seconds: 90
---

Use only the Connector operations exposed for the current target. Their parameters contain the business input; connection ownership and binding identity are supplied by the server.

Use an available read operation to gather information. Describe a write accurately before asking the user to confirm it. A pending confirmation continues the same operation and must not create a second call. If an operation has an unknown outcome, report that state and do not retry it automatically.

When a required connection is unavailable, ask the user to attach or repair it through the Connector picker. Never request a secret key in chat or place credentials in tool arguments.
