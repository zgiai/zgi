# Architecture Diagram Prompt

## Use When

Use for prompts targeting technical architecture diagrams, Mermaid, PlantUML, Draw.io, Excalidraw, C4 diagrams, flowcharts, sequence diagrams, deployment diagrams, and ER diagrams.

## Required Elements

Check for:

- System name.
- Business goal or diagram purpose.
- Diagram type: system architecture, flowchart, sequence diagram, deployment diagram, C4 container diagram, ER diagram.
- User roles.
- Major modules and services.
- Module relationships.
- Data flow or request flow.
- External dependencies.
- Deployment environment or layers.
- Data stores and message queues.
- Security, permissions, or integration boundaries when relevant.

## Prompt Shape

Include:

- Diagram purpose and audience.
- Recommended diagram type.
- Layered structure.
- Component list.
- Relationship and data-flow description.
- Visual style requirements, such as clear grouping, labels, arrows, and legend.

## Structured Output Shape

Prefer:

- 用户层
- 接入层
- 应用层
- 服务层
- 数据层
- AI 能力层
- 外部系统
- 运维与安全

Use only layers that are supported by the user's source description.

## Clarification Rules

Ask when the diagram type or major modules are missing. Do not invent technical modules, databases, queues, services, or deployment environments. If the user provides a rough chain, preserve it and mark additions as `默认假设`.

## Boundary Rules

- Do not fabricate architecture details.
- Do not output runnable Mermaid or PlantUML unless the user asks for diagram code.
- Do not claim the architecture is production-ready without review.
