# Agent Architecture Diagram Reference

Use this reference when `diagram_type` is `agent_architecture`, AI Agent architecture, LLM inference flow, RAG flow, tool-calling flow, memory management flow, or multi-agent workflow.

## Payload Shape

```json
{
  "diagram_type": "agent_architecture",
  "title": "RAG Agent Architecture",
  "data": {
    "groups": [
      {"id": "input", "label": "Input"},
      {"id": "agent", "label": "Agent Core"},
      {"id": "tools", "label": "Tools"},
      {"id": "memory", "label": "Memory"},
      {"id": "model", "label": "Model"}
    ],
    "nodes": [
      {"id": "user", "label": "User", "type": "actor", "group": "input", "layer": "input"},
      {"id": "orchestrator", "label": "Agent Orchestrator", "type": "agent", "group": "agent", "layer": "agent"},
      {"id": "retriever", "label": "Retriever", "type": "tool", "group": "tools", "layer": "tools"},
      {"id": "vector", "label": "Vector Store", "type": "memory", "group": "memory", "layer": "memory"},
      {"id": "llm", "label": "LLM", "type": "model", "group": "model", "layer": "model"},
      {"id": "answer", "label": "Answer", "type": "output", "group": "output", "layer": "output"}
    ],
    "edges": [
      {"from": "user", "to": "orchestrator", "label": "query"},
      {"from": "orchestrator", "to": "retriever", "label": "retrieve context"},
      {"from": "retriever", "to": "vector", "label": "similarity search"},
      {"from": "orchestrator", "to": "llm", "label": "prompt + context"},
      {"from": "llm", "to": "answer", "label": "final response"}
    ]
  },
  "options": {"style": "technical"}
}
```

## Data Rules

- Include user/input, agent controller, model, tools, memory, retrieval, and output nodes when they are relevant.
- Include `groups` and node `group` values for semantic sections such as Input, Agent Core, Tools, Memory, Model, and Output.
- Edges should show the control flow or data flow. Use labels for query, tool call, retrieval, memory read/write, prompt, response, or observation.
- Do not add tools or memory systems unless the user described them or they are clearly implied by a requested RAG/Agent diagram.

## Clarification Rules

Ask before generating when the Agent pattern is unclear: single-agent vs multi-agent, RAG vs tool-calling, memory type, or required tools.
