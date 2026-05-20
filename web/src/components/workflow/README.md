## Workflow Module – Structure & Onboarding Guide

This document provides a concise, high-signal overview of the workflow editor architecture to help you navigate quickly and implement features with confidence.

## What It Is

- ReactFlow-based visual editor for building agent workflows
- Strongly-typed node system with per-node configuration managers
- Centralized state via Zustand slices
- Run history (read-only) and live draft modes

## Quick Links

- Entry: `src/components/workflow/index.tsx`
- Store: `workflow/store/{store.ts, slices/*, helpers/*, type.ts, initial-data.ts}`
- Node registry: `workflow/nodes/index.ts`
- Node managers: `workflow/nodes/*/manager/*`
- UI chrome: `workflow/ui/*`
- Value editor: `workflow/ui/workflow-value-editor/*`
- Validation: `workflow/nodes/common/validation.ts`
- Types: `workflow/store/type.ts`, `workflow/types/input-var.ts`

## Directory Map (High-Level)

```text
workflow/
  index.tsx                   # WorkflowEditor (ReactFlow host + panels)
  hooks/                      # Editor lifecycle, keyboard, creation, validation
  nodes/                      # Node registry + each node's renderer and manager
  store/                      # Zustand store, slices, helpers, types
  ui/                         # Panels, chrome, minimap, context menu, value editor
  types/                      # Shared workflow-related types (e.g., input vars)
  common/                     # Shared small building blocks used by workflow
```

### nodes/

```text
nodes/
  index.ts                    # Aggregates nodeTypes for ReactFlow
  common/validation.ts        # Cross-node validation helpers
  start/                      # Input variables node (manager has edit modal)
  knowledge-retrieval/        # Retrieval node (with recall settings dialog)
  llm/                        # Prompt + model parameters (prompt editor, params modal)
  http-request/               # HTTP request node (cURL import dialog)
  if-else/                    # Conditional branching (types + utils)
  code/                       # Code execution node (IO sections + hooks)
  end/                        # Output aggregation node
  custom/                     # Example/extensible custom node
```

### store/

```text
store/
  store.ts                    # Store creation and typed selectors
  index.ts                    # Re-exports store accessors
  initial-data.ts             # Default graph seed
  type.ts                     # Core types: WorkflowNode, node data variants
  slices/
    graph.ts                  # Nodes/edges, onChange, connect logic
    workflow-io.ts            # Draft data, selected run, metadata
    history.ts                # Run snapshots, history (read-only) mode
    viewport.ts               # Controlled pan/zoom viewport state
    run-status.ts             # Execution state and run progress
    drag-preview.ts           # Creation/drag preview state
    mode-selection.ts         # Interaction mode (hand/select)
  helpers/
    drag-runtime.ts           # Drag helpers (runtime)
    drag.ts                   # Drag helpers
    graph.ts                  # Graph utilities
    history.ts                # History ops
    nodes.ts                  # Node helpers
    normalizers.ts            # Data normalizers
    titles.ts                 # Auto titles
```

### ui/

```text
ui/
  workflow-header.tsx         # Top bar: save, run, history controls
  workflow-bottom-toolbar.tsx # Zoom, modes, utilities
  workflow-minimap.tsx        # Overview map
  context-menu.tsx            # Right-click actions (global handler)
  node-floating-panel.tsx     # Right sidebar: node property editor (managers)
  workflow-run-panel/         # Run history/details + SSE hooks
  workflow-chat-panel/        # Chat for conversational agents
  create-node-modal/          # Node creation modal + auto-connect flow
  custom-edge.tsx             # Edge visual
  custom-handle.tsx           # Handle visual
  value-badge.tsx             # Small value tags
  workflow-skeleton.tsx       # Initial loading skeleton
  workflow-value-editor/      # Rich tokens, suggestions, transforms
```

## Entry Point – `workflow/index.tsx`

- Loads draft via `useWorkflowDraft(agentId)` and hydrates the store
- Saves via `useCombinedWorkflowSave(agentId)` (manual and autosave, idle when available)
- Hosts ReactFlow and registers `nodeTypes` and `edgeTypes`
- Panels via `PanelStackProvider`: minimap, run panel, chat panel, `NodeFloatingPanel`
- History mode: when a run is selected, renders run snapshots (nodes/edges/viewport) read-only

## Data Flow

- Load: server draft → `loadWorkflow()` populates store
- Edit: components call `useWorkflowStore.use.updateNodeData()` or `useWorkflowOperations()`
- Save: combined save merges semantic/layout changes; autosave on interval/idle
- History: `mode === 'history'` reads from `historySnapshots[runId]` without mutating draft

## Interaction Model

- Selection: click to select; right-click opens `WorkflowContextMenu`
- Creation: `CreateNodeModal` + drag-preview, auto-connect from originating handle
- Modes: select vs hand (pan). While connecting/creating, selection/pan behavior adapts
- Viewport: controlled via store; programmatic pans marked to avoid canceling auto-follow

## Validation & Types

- Core types in `store/type.ts`, with specific shapes for each node data variant
- Input variable types in `types/input-var.ts`
- Cross-node validation in `nodes/common/validation.ts`

## Adding a New Node Type

1. Create `nodes/<your-node>/index.tsx` (renderer) and optional `config.ts`
2. Add manager UI under `nodes/<your-node>/manager/index.tsx` (subcomponents as needed)
3. Register in `nodes/index.ts` to expose in `nodeTypes`
4. Extend types in `store/type.ts` for the node’s data shape
5. Add validation in `nodes/common/validation.ts` if needed
6. Render the manager in `ui/node-floating-panel.tsx` switch by `selectedNode.data.type`

## Common Tasks

- Add property to existing node: update `store/type.ts` → render controls in manager → commit via `updateNodeData`
- Customize run panel: edit `ui/workflow-run-panel/components/*` or `ui/workflow-run-panel/hooks/*`
- Add keyboard shortcut: extend `hooks/use-workflow-keyboard.ts`
- Extend context menu: update `ui/context-menu.tsx` and the global handler contract

## Performance Notes

- Fine-grained selectors `useWorkflowStore.use.xxx()` to minimize re-renders
- Memoized node/edge types and controlled viewport
- Debounced text inputs (`hooks/use-debounced-commit.ts`)
- RAF-throttled resize in `node-floating-panel.tsx`
- Idle autosave to reduce contention

## Development Tips

- Favor existing UI components (shadcn UI) to keep UX consistent
- Keep types strict (no `any`); prefer early returns and shallow nesting
- Place common helpers in `src/utils` to avoid duplication
- Ensure fast-first render: skeletons for initial loads, optimistic updates with rollback


