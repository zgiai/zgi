---
name: architecture-diagram-generator
description: Generate downloadable SVG and HTML technical diagrams from natural language or structured data, including system architecture, AI Agent architecture, data flow, flowchart, comparison matrix, sequence, state, and ER diagrams.
when_to_use: Use this skill when the user asks to create, generate, export, draw, visualize, or produce a technical architecture diagram, system diagram, microservice diagram, frontend/backend diagram, database architecture diagram, API gateway diagram, AI Agent architecture, Agent workflow, LLM workflow, RAG workflow, tool-calling workflow, memory workflow, data flow diagram, process flowchart, business process diagram, approval flow, task execution flow, user operation flow, product comparison, competitor comparison, solution comparison, feature matrix, sequence diagram, state diagram, ER diagram, database design diagram, or architecture visualization for documentation, reports, presentations, or embedding. For casual, vague, incomplete, or non-structured diagram requests, first route through prompt-professionalizer to optimize the diagram prompt and extract structured requirements, then call this skill.
provider_type: builtin
provider_id: architecture_diagram_generator
runtime_type: tool
tools:
  - generate_architecture_diagram
max_calls_per_turn: 5
timeout_seconds: 10
display:
  icon: network
  category: content_creation
  scenarios:
    - content_creation
    - technical_development
  label:
    en_US: Architecture Diagram Generator
    zh_Hans: 架构图生成器
  description:
    en_US: Designed for system architecture, process, and data-relationship planning; turns natural language or structured data into SVG and HTML architecture, flow, sequence, state, or ER diagrams.
    zh_Hans: 适用于设计系统架构、业务流程或数据关系，可将自然语言或结构化数据生成 SVG 和 HTML 架构图、流程图、时序图、状态图或 ER 图。
  when_to_use:
    en_US: Use when the answer should include generated technical diagram artifacts.
    zh_Hans: 当回答需要生成技术架构图文件时使用。
  tags:
    en_US:
      - Architecture
      - Diagram
      - Visualization
    zh_Hans:
      - 架构图
      - 技术图
      - 可视化
---

# Architecture Diagram Generator Skill

Use this skill to generate downloadable SVG and HTML technical diagram artifacts. Supported diagram types are `system_architecture`, `agent_architecture`, `data_flow`, `flowchart`, `comparison_matrix`, `sequence`, `state`, and `er`.

## Supported Diagram Types

- `system_architecture`: system architecture, microservice architecture, frontend/backend architecture, database architecture, API gateway architecture, deployment, and component diagrams.
- `agent_architecture`: AI Agent architecture, LLM inference flow, tool calling, memory management, multi-agent workflow, and RAG pipeline diagrams.
- `data_flow`: data movement from input through processing, storage, and output.
- `flowchart`: business process, approval flow, task execution flow, and user operation flow.
- `comparison_matrix`: product comparison, competitor analysis, solution selection, and feature matrices.
- `sequence`: service interactions, API call order, request/response lifecycle, and interface documentation.
- `state`: state machines, lifecycle transitions, and status flow.
- `er`: entity relationship diagrams for database design and development communication.

## Workflow

1. Determine whether the user explicitly requested a `diagram_type`: `system_architecture`, `agent_architecture`, `data_flow`, `flowchart`, `comparison_matrix`, `sequence`, `state`, or `er`.
2. If the request is casual, vague, incomplete, or not already structured for a technical diagram, first use `prompt-professionalizer` to produce an optimized architecture diagram prompt and structured requirements.
3. If the user only gives a generic request such as "draw an architecture diagram", "generate a system diagram", "visualize this process", "create a technical diagram", "画个架构图", "生成系统图", "流程可视化", or "做一个技术图", call `request_user_input` before calling `generate_architecture_diagram`.
4. Read exactly one reference document for the selected diagram type before calling `generate_architecture_diagram`.
5. Convert the user's natural-language description into the JSON payload documented in the selected reference.
6. Add semantic `groups` and each node's `group` when components belong to modules such as frontend, backend, data, tools, memory, model, output, external systems, infrastructure, or business domains. Use `layer` only for layout order.
7. Validate that all required nodes, relationships, participants, states, entities, rows, columns, or cells are present and internally consistent.
8. Call `call_skill_tool` with `tool_name` set to `generate_architecture_diagram`.
9. In the final answer, briefly mention the generated SVG and HTML filenames and any assumptions. Do not paste SVG or HTML source unless the user explicitly asks for it.

## Clarification Workflow

When any required decision is missing or ambiguous, call `request_user_input` instead of writing a plain clarification message. This ensures the UI renders the clarification as a structured confirmation card with optional quick replies.

Generic architecture or diagram requests are incomplete even when the domain can be guessed. Do not infer `system_architecture` just because the user says "architecture", do not infer `flowchart` just because the user describes steps, do not infer `data_flow` just because data is mentioned, and do not infer `sequence` just because services communicate. If the diagram type is not explicit, ask.

For generic diagram requests, diagram type, diagram title, scope or boundary, and rendering style are required decisions. Ask for them before reading a diagram reference or calling `generate_architecture_diagram`.

Use a brief `message` explaining what needs confirmation, then provide 1-4 focused `questions`. Include `options` only when each option is a concrete answer that can be used directly. Omit options for open-ended questions such as the diagram title or system boundary.

After calling `request_user_input`, stop the turn and wait for the user's answer. Do not call `generate_architecture_diagram` in the same turn.

Ask about:

- Diagram type when the user did not explicitly request one supported type.
- Diagram title when the user did not provide a title.
- Scope or boundary when components, actors, systems, data stores, or external services are unclear.
- Rendering style when the user asks for a polished, report-ready, or presentation-ready artifact. Supported styles: `simple`, `business`, `technical`, `presentation`, `paper`.
- Direction when the user cares about layout. Supported directions: `left_to_right`, `top_to_bottom`.

Example `request_user_input` payload:

```json
{
  "message": "I can generate the technical diagram, but need to confirm a few details first.",
  "questions": [
    {
      "id": "diagram_type",
      "question": "Which diagram type should I generate?",
      "options": [
        { "label": "system_architecture" },
        { "label": "agent_architecture" },
        { "label": "data_flow" },
        { "label": "flowchart" },
        { "label": "sequence" }
      ]
    },
    {
      "id": "title",
      "question": "What title should be shown on the diagram?"
    },
    {
      "id": "style",
      "question": "Which rendering style should I use?",
      "options": [
        { "label": "simple" },
        { "label": "business" },
        { "label": "technical" },
        { "label": "presentation" },
        { "label": "paper" }
      ]
    },
    {
      "id": "scope",
      "question": "What system boundary or key components must be included?"
    }
  ]
}
```

## References

Read exactly one reference after choosing the diagram type:

| Requested diagram | Read reference |
| --- | --- |
| `system_architecture`, microservices, frontend/backend, API gateway, database architecture | `diagram-system-architecture.md` |
| `agent_architecture`, AI Agent, LLM, RAG, tool calling, memory workflow | `diagram-agent-architecture.md` |
| `data_flow`, data pipeline, input-process-store-output chain | `diagram-data-flow.md` |
| `flowchart`, business process, approval flow, task flow, user operation flow | `diagram-flowchart.md` |
| `comparison_matrix`, feature matrix, product comparison, solution selection | `diagram-comparison-matrix.md` |
| `sequence`, API calls, service interaction, request lifecycle | `diagram-sequence.md` |
| `state`, state machine, lifecycle transitions | `diagram-state.md` |
| `er`, ERD, entity relationship, database design | `diagram-er.md` |

If the user requests a diagram type that is not listed, say it is not supported yet and offer to structure it as the closest supported type only after confirmation.

## Unified Payload

`generate_architecture_diagram` accepts:

- `diagram_type`: required. Supported values: `system_architecture`, `agent_architecture`, `data_flow`, `flowchart`, `comparison_matrix`, `sequence`, `state`, `er`.
- `title`: optional diagram title.
- `description`: optional subtitle or source summary.
- `output_filename`: optional ASCII filename without extension. Defaults to `architecture-diagram`.
- `data`: required diagram-specific data object.
- For node-edge diagrams, include `groups` and node `group` fields when there are clear modules. Use `layer` only for layout order.
- `options`: optional rendering options. Common keys: `formats`, `width`, `height`, `style`, `direction`, `show_legend`, `show_labels`.
- `lifecycle`: optional file lifecycle, `persistent` or `temporary`. Defaults to `persistent`.

Default output formats are `svg` and `html`.

## Style Rules

- Use `technical` for engineering documentation and architecture reviews.
- Use `business` for stakeholder reports and product comparisons.
- Use `presentation` for slide-ready diagrams.
- Use `paper` for warm, report-ready Paper.design-like visuals.
- Use `simple` when no visual style is specified.

## Constraints

- Before calling `generate_architecture_diagram`, use `prompt-professionalizer` when the user's request is casual, vague, incomplete, or not already structured for a technical diagram. Direct tool calls are allowed only when the diagram type, content, and key rendering requirements are already complete.
- Do not call `generate_architecture_diagram` until the selected diagram reference has been read.
- Do not read a diagram reference until the diagram type has been explicitly provided by the user or confirmed through `request_user_input`.
- Generate SVG and HTML artifacts only. Do not promise PNG, PDF, editable PPTX, or interactive diagrams.
- Do not invent components, services, data stores, actors, entities, fields, participants, or relationships that the user did not provide or imply clearly.
- Do not silently choose a diagram type, title, scope, or style for a generic diagram request.
- Do not use unsupported diagram types silently. Unsupported diagram types must be reported as unsupported.
- Keep filenames short, ASCII, and free of path separators.
- If the user's architecture description is ambiguous, ask for clarification or state the assumptions before generating a diagram.
