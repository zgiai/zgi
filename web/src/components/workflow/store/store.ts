import { create } from 'zustand';
import type {
  WorkflowEdge,
  WorkflowNode,
  WorkflowData,
  WorkflowDraftData,
  WorkflowNodeData,
  WorkflowDragPreview,
} from './type';
import type { OnConnect, OnEdgesChange, OnNodesChange, Viewport } from '@xyflow/react';
import { devtools } from 'zustand/middleware';
import type { AgentType } from '@/services/types/agent';
import type { GraphSnapshot, RunGraphSnapshot } from './helpers/history';
import type { UpstreamExportItem } from './helpers/graph';
import { computeRunnableSets } from './helpers/graph';
import { validateWorkflow } from './helpers/validation-engine';
import type { StoreValidationResults } from './type';
import { createSelectors } from '@/store/utils/selectors';
import { createGraphSlice, type GraphSlice } from './slices/graph';
import { createViewportSlice, type ViewportSlice } from './slices/viewport';
import { createHistorySlice, type HistorySlice } from './slices/history';
import { createModeSelectionSlice, type ModeSelectionSlice } from './slices/mode-selection';
import { createWorkflowIOSlice, type WorkflowIOSlice } from './slices/workflow-io';
import { createRunStatusSlice, type RunStatusSlice } from './slices/run-status';
import { debounce } from '@/lib/utils';
import { useAuthStore } from '@/store/auth-store';
import type { BuiltinToolProvider } from '@/services/types/tool';
import type { Locale } from '@/lib/i18n';

export type { UpstreamExportItem } from './helpers/graph';

// Cached graph reachability sets for performance
export interface RunnableSets {
  mainRunnable: Set<string>;
  iterRunnableMap: Map<string, Set<string>>;
  commentSet: Set<string>;
}

export interface WorkflowStore
  extends GraphSlice,
    ViewportSlice,
    HistorySlice,
    ModeSelectionSlice,
    RunStatusSlice,
    WorkflowIOSlice {
  workflowData: WorkflowData;
  lastSavedAt: number | null;

  // React Flow state
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  viewport: Viewport;
  // Batch history flag
  isHistoryBatching: boolean;
  // Suppress the next layout history push (used right after endHistoryBatch)
  suppressNextLayoutHistoryPush?: boolean;
  // Suppress one initial React Flow layout/viewport sync from marking the draft dirty.
  suppressNextLayoutDirty?: boolean;
  suppressNextViewportDirty?: boolean;

  // Interaction mode
  interactionMode: 'pointer' | 'hand';
  // Editor mode: edit vs history (read-only)
  mode: 'edit' | 'history';
  // Selected workflow run id when in history mode
  selectedRunId: string | null;
  // Snapshot of current draft graph state to be restored when exiting history mode
  draftSnapshot: {
    nodes: WorkflowNode[];
    edges: WorkflowEdge[];
    viewport: Viewport;
    selectedNodeId: string | null;
    isDirty: boolean;
    hasLayoutChanges: boolean;
  } | null;

  // History state
  historyPast: GraphSnapshot[];
  historyFuture: GraphSnapshot[];
  // Per-run history snapshots captured at run start (in-memory)
  historySnapshots: Record<string, RunGraphSnapshot>;

  // Default agent type to WORKFLOW; will be set precisely by WorkflowEditor on mount
  agentType: AgentType;

  // Permission-based read-only flag (user lacks edit permission)
  canEdit: boolean;
  setCanEdit: (canEdit: boolean) => void;
  canRunDraft: boolean;
  setCanRunDraft: (canRunDraft: boolean) => void;
  canStopRun: boolean;
  setCanStopRun: (canStopRun: boolean) => void;
  canDebug: boolean;
  setCanDebug: (canDebug: boolean) => void;
  canViewRuntimeLogs: boolean;
  setCanViewRuntimeLogs: (canViewRuntimeLogs: boolean) => void;

  setNodes: (nodes: WorkflowNode[]) => void;
  setEdges: (edges: WorkflowEdge[]) => void;
  setViewport: (viewport: Viewport, options?: { markLayoutDirty?: boolean }) => void;
  setInteractionMode: (mode: 'pointer' | 'hand') => void;
  onNodesChange: OnNodesChange;
  onEdgesChange: OnEdgesChange;
  onConnect: OnConnect;

  addNode: (
    nodeData: Partial<WorkflowNodeData>,
    position: { x: number; y: number },
    parentId?: string
  ) => string | null;
  addNodes: (
    items: Array<{ data: Partial<WorkflowNodeData>; position: { x: number; y: number } }>,
    parentId?: string
  ) => string[];
  deleteNode: (nodeId: string) => void;
  updateNode: (nodeId: string, updates: Partial<WorkflowNode>) => void;
  updateNodeData: (nodeId: string, data: Partial<WorkflowNodeData>) => void;
  selectNode: (nodeId: string | null) => void;

  // Accept both draft and full workflow data; internally normalized via convertToWorkflowData
  loadWorkflow: (
    data: WorkflowData | WorkflowDraftData,
    agentId: string,
    preserveDirtyState?: boolean
  ) => void;
  saveWorkflow: () => Promise<void>;
  resetWorkflow: () => void;

  // History operations
  pushHistory: () => void;
  undo: () => void;
  redo: () => void;
  beginHistoryBatch: () => void;
  endHistoryBatch: () => void;

  // Utilities
  generateNodeId: () => string;
  canAddNode: (nodeType: string) => boolean;
  getUniqueNodeTypes: () => string[];

  // Graph analysis
  getIncomingSources: (nodeId: string) => string[];
  getAncestors: (nodeId: string) => string[];
  getUpstreamVariables: (nodeId: string) => UpstreamExportItem[];
  getUpstreamWritableVariables: (nodeId: string) => UpstreamExportItem[];

  // Agent type setter
  setAgentType: (agentType: AgentType) => void;

  // Set snapshot for a specific run; used by run panel to capture graph state at run start
  setHistorySnapshot: (runId: string, snapshot: RunGraphSnapshot) => void;
  // History mode controls
  enterHistoryMode: (runId: string) => void;
  exitHistoryMode: () => void;

  // Clipboard and mouse position for copy/paste UX
  clipboardNodeData: WorkflowNodeData | null;
  setClipboardNodeData: (data: WorkflowNodeData | null) => void;
  lastMouseClient: { x: number; y: number } | null;
  setLastMouseClient: (pos: { x: number; y: number } | null) => void;

  // UI hover state: currently hovered node id (for edge highlight on node hover)
  hoveredNodeId: string | null;
  setHoveredNodeId: (nodeId: string | null) => void;

  // Multi-node clipboard (support iteration grouping)
  clipboardNodes: Array<{
    data: WorkflowNodeData;
    offset: { x: number; y: number };
    parentRef?: string; // original iteration parent id if item is an iteration child
    refId?: string; // original node id for mapping (used for parents)
  }>;
  setClipboardNodes: (
    items: Array<{
      data: WorkflowNodeData;
      offset: { x: number; y: number };
      parentRef?: string;
      refId?: string;
    }>
  ) => void;

  // Validation issues dropdown control
  openValidationIssues: boolean;
  setOpenValidationIssues: (open: boolean) => void;

  // Cached results for performance (avoids re-computing on every render)
  runnableSets: RunnableSets;
  validationResults: StoreValidationResults;
  toolValidationProviders: BuiltinToolProvider[] | null;
  toolValidationLocale: Locale;
  setToolValidationContext: (providers: BuiltinToolProvider[] | null, locale: Locale) => void;
  syncRunnableSets: () => void;
  syncRunnableSetsDebounced: () => void;

  // Global version to track semantic graph changes (nodes/edges topology or data)
  graphVersion: number;
  // Internal cache for expensive graph analysis results
  _analysisCache: {
    upstreamVariables: Map<string, UpstreamExportItem[]>;
    ancestors: Map<string, string[]>;
    version: number;
    runId: string | null;
  };

  // Global drag tracking for container interactions
  draggingNodeType: string | null;
  setDraggingNodeType: (type: string | null) => void;
  draggingNodePreview: WorkflowDragPreview | null;
  setDraggingNodePreview: (preview: WorkflowDragPreview | null) => void;
  updateDraggingNodePreviewClient: (client: { x: number; y: number }) => void;
  clearDraggingNodePreview: () => void;
  dragOverContainerId: string | null;
  setDragOverContainerId: (id: string | null) => void;
  // Map node id to its title for quick lookup; updated on node title/list changes
  nodeIdToTitle: Map<string, string>;
}

export const useWorkflowStoreBase = create<WorkflowStore>()(
  devtools(
    (set, get) => ({
      // Initial state

      // Slice: Workflow IO (migrated)
      ...createWorkflowIOSlice(
        set as (
          partial: Partial<WorkflowStore> | ((state: WorkflowStore) => Partial<WorkflowStore>),
          replace?: boolean,
          action?: string
        ) => void,
        get as () => WorkflowStore
      ),

      // Slice: Graph (nodes/edges + RF handlers)
      ...createGraphSlice(
        set as (partial: unknown, replace?: boolean, action?: string) => void,
        get as () => WorkflowStore
      ),

      // Slice: Viewport
      ...createViewportSlice(
        set as (partial: unknown, replace?: boolean, action?: string) => void,
        get as () => WorkflowStore
      ),

      // Slice: History (undo/redo + batch)
      ...createHistorySlice(
        set as (
          partial: Partial<WorkflowStore> | ((state: WorkflowStore) => Partial<WorkflowStore>),
          replace?: boolean,
          action?: string
        ) => void,
        get as () => WorkflowStore
      ),

      // Slice: Mode Selection
      ...createModeSelectionSlice(
        set as (
          partial: Partial<WorkflowStore> | ((state: WorkflowStore) => Partial<WorkflowStore>),
          replace?: boolean,
          action?: string
        ) => void,
        get as () => WorkflowStore
      ),

      // Slice: Run Status
      ...createRunStatusSlice(
        set as (partial: unknown, replace?: boolean, action?: string) => void,
        get as () => WorkflowStore
      ),

      graphVersion: 0,
      _analysisCache: {
        upstreamVariables: new Map(),
        ancestors: new Map(),
        version: -1,
        runId: null,
      },

      hoveredNodeId: null,
      setHoveredNodeId: (nodeId: string | null) =>
        set({ hoveredNodeId: nodeId }, false, 'workflow:setHoveredNodeId'),
      lastSavedAt: null,
      openValidationIssues: false,
      setOpenValidationIssues: (open: boolean) =>
        set({ openValidationIssues: open }, false, 'workflow:setOpenValidationIssues'),

      // Permission-based read-only: default to true (editable)
      canEdit: true,
      setCanEdit: (canEdit: boolean) => set({ canEdit }, false, 'workflow:setCanEdit'),
      canRunDraft: false,
      setCanRunDraft: (canRunDraft: boolean) =>
        set({ canRunDraft }, false, 'workflow:setCanRunDraft'),
      canStopRun: false,
      setCanStopRun: (canStopRun: boolean) => set({ canStopRun }, false, 'workflow:setCanStopRun'),
      canDebug: false,
      setCanDebug: (canDebug: boolean) => set({ canDebug }, false, 'workflow:setCanDebug'),
      canViewRuntimeLogs: false,
      setCanViewRuntimeLogs: (canViewRuntimeLogs: boolean) =>
        set({ canViewRuntimeLogs }, false, 'workflow:setCanViewRuntimeLogs'),

      // Initial validation results (empty)
      validationResults: {
        errors: [],
        warnings: [],
        errorMap: new Map(),
        warningMap: new Map(),
      },
      toolValidationProviders: null,
      toolValidationLocale: 'zh-Hans',
      setToolValidationContext: (providers, locale) => {
        set(
          { toolValidationProviders: providers, toolValidationLocale: locale },
          false,
          'workflow:setToolValidationContext'
        );
        get().syncRunnableSets();
      },
      // Cache for graph reachability/sets
      runnableSets: {
        mainRunnable: new Set(),
        iterRunnableMap: new Map(),
        commentSet: new Set(),
      },
      syncRunnableSets: () => {
        const { nodes, edges, agentType, isHistoryBatching } = get();
        if (isHistoryBatching) return;

        // const start = performance.now();
        const runnable = computeRunnableSets(nodes, edges);
        // const mid = performance.now();
        const validation = validateWorkflow(
          nodes,
          edges,
          agentType,
          runnable,
          useAuthStore.getState().systemFeatures,
          get().toolValidationProviders,
          get().toolValidationLocale
        );
        // const end = performance.now();

        // console.log(
        //   `[Workflow Performance] syncRunnableSets took ${end - start}ms (compute: ${mid - start}ms, validate: ${end - mid}ms)`
        // );

        set(
          { runnableSets: runnable, validationResults: validation },
          false,
          'workflow:syncRunnableSets'
        );
      },

      syncRunnableSetsDebounced: debounce(() => {
        const { isHistoryBatching, syncRunnableSets } = get();
        if (isHistoryBatching) return;
        syncRunnableSets();
      }, 300),

      draggingNodeType: null,
      setDraggingNodeType: (type: string | null) =>
        set(
          type === null
            ? { draggingNodeType: null, draggingNodePreview: null }
            : { draggingNodeType: type },
          false,
          'workflow:setDraggingNodeType'
        ),
      draggingNodePreview: null,
      setDraggingNodePreview: (preview: WorkflowDragPreview | null) =>
        set({ draggingNodePreview: preview }, false, 'workflow:setDraggingNodePreview'),
      updateDraggingNodePreviewClient: (client: { x: number; y: number }) =>
        set(
          state => ({
            draggingNodePreview: state.draggingNodePreview
              ? { ...state.draggingNodePreview, client }
              : null,
          }),
          false,
          'workflow:updateDraggingNodePreviewClient'
        ),
      clearDraggingNodePreview: () =>
        set(
          { draggingNodeType: null, draggingNodePreview: null },
          false,
          'workflow:clearDraggingNodePreview'
        ),
      dragOverContainerId: null,
      setDragOverContainerId: (id: string | null) =>
        set({ dragOverContainerId: id }, false, 'workflow:setDragOverContainerId'),
      nodeIdToTitle: new Map(),
    }),
    { name: 'workflow-store' }
  )
);

export const useWorkflowStore = createSelectors(useWorkflowStoreBase);
export default useWorkflowStore;
