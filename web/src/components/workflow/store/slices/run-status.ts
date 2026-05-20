// Run status & auto-follow slice
// Strict typing; encapsulates transient run-time UI state separate from graph data

export type RunStatus = 'idle' | 'running' | 'succeeded' | 'failed' | 'stopped' | 'paused';

export interface RuntimeLogItem {
  title: string;
  nodeId: string;
  executionId?: string;
  createdAtMs?: number;
  receivedOrder?: number;
  nodeType: string;
  status: 'running' | 'succeeded' | 'failed' | 'stopped' | 'paused';
  nodeInput?: unknown;
  nodeOutput?: unknown;
  modelInput?: unknown;
  processData?: unknown;
  executionMetadata?: unknown;
  elapsedTime?: number;
  error?: string | null;
  iterationInputs?: unknown;
  iterationOutputs?: unknown;
  iterationRounds?: Array<{
    index: number;
    nodes: RuntimeLogItem[];
    elapsedTime?: number;
  }>;
  loopInputs?: unknown;
  loopOutputs?: unknown;
  loopRounds?: Array<{
    index: number;
    nodes: RuntimeLogItem[];
    elapsedTime?: number;
    variables?: unknown;
  }>;
  steps?: number;
}

export interface RunStatusSlice {
  runStatusByNodeId: Record<string, RunStatus>;
  runtimeLogItemsByNodeId: Record<string, RuntimeLogItem[]>;
  isAutoFollow: boolean;
  currentRunningNodeId: string | null;
  lastDebugInputs: Record<string, unknown> | null;
  // Active source handle selected by node_finished.output_handle for route highlighting.
  activeOutputHandleByNodeId: Record<string, string | null>;
  // Programmatic pan guard to avoid disabling auto-follow on setCenter animations
  isProgrammaticPan: boolean;

  setNodeRunStatus: (nodeId: string, status: RunStatus) => void;
  setRuntimeLogItems: (items: RuntimeLogItem[]) => void;
  resetRuntimeLogItems: () => void;
  resetRunStatus: (nodeIds?: string[]) => void;
  setAutoFollow: (enabled: boolean) => void;
  setCurrentRunningNodeId: (nodeId: string | null) => void;
  setLastDebugInputs: (inputs: Record<string, unknown> | null) => void;
  setActiveOutputHandle: (nodeId: string, outputHandle: string | null) => void;
  resetActiveOutputHandles: (nodeIds?: string[]) => void;
  setProgrammaticPan: (enabled: boolean) => void;
}

export type StoreSet = (
  partial:
    | Partial<{
        runStatusByNodeId: Record<string, RunStatus>;
        runtimeLogItemsByNodeId: Record<string, RuntimeLogItem[]>;
        isAutoFollow: boolean;
        currentRunningNodeId: string | null;
        lastDebugInputs: Record<string, unknown> | null;
        activeOutputHandleByNodeId: Record<string, string | null>;
        isProgrammaticPan: boolean;
      }>
    | ((state: {
        runStatusByNodeId: Record<string, RunStatus>;
        runtimeLogItemsByNodeId: Record<string, RuntimeLogItem[]>;
        isAutoFollow: boolean;
        currentRunningNodeId: string | null;
        lastDebugInputs: Record<string, unknown> | null;
        activeOutputHandleByNodeId: Record<string, string | null>;
        isProgrammaticPan: boolean;
      }) => Partial<{
        runStatusByNodeId: Record<string, RunStatus>;
        runtimeLogItemsByNodeId: Record<string, RuntimeLogItem[]>;
        isAutoFollow: boolean;
        currentRunningNodeId: string | null;
        activeOutputHandleByNodeId: Record<string, string | null>;
        isProgrammaticPan: boolean;
      }>),
  replace?: boolean,
  action?: string
) => void;

export function createRunStatusSlice(set: StoreSet, _get: () => unknown): RunStatusSlice {
  return {
    runStatusByNodeId: {},
    runtimeLogItemsByNodeId: {},
    isAutoFollow: false,
    currentRunningNodeId: null,
    lastDebugInputs: null,
    activeOutputHandleByNodeId: {},
    isProgrammaticPan: false,

    setNodeRunStatus: (nodeId, status) =>
      set(
        (state: { runStatusByNodeId: Record<string, RunStatus> }) => ({
          runStatusByNodeId: {
            ...state.runStatusByNodeId,
            [nodeId]: status,
          },
        }),
        false,
        'setNodeRunStatus'
      ),

    setRuntimeLogItems: items =>
      set(
        () => {
          const grouped: Record<string, RuntimeLogItem[]> = {};
          const appendItem = (item: RuntimeLogItem) => {
            if (item.nodeId) {
              grouped[item.nodeId] = [...(grouped[item.nodeId] ?? []), item];
            }
            item.iterationRounds?.forEach(round => round.nodes.forEach(appendItem));
            item.loopRounds?.forEach(round => round.nodes.forEach(appendItem));
          };
          items.forEach(appendItem);
          return { runtimeLogItemsByNodeId: grouped };
        },
        false,
        'setRuntimeLogItems'
      ),

    resetRuntimeLogItems: () => set({ runtimeLogItemsByNodeId: {} }, false, 'resetRuntimeLogItems'),

    resetRunStatus: nodeIds =>
      set(
        (state: {
          runStatusByNodeId: Record<string, RunStatus>;
          runtimeLogItemsByNodeId: Record<string, RuntimeLogItem[]>;
          activeOutputHandleByNodeId: Record<string, string | null>;
        }) => {
          if (!nodeIds || nodeIds.length === 0) {
            return {
              runStatusByNodeId: {},
              runtimeLogItemsByNodeId: {},
              activeOutputHandleByNodeId: {},
            };
          }
          const next: Record<string, RunStatus> = { ...state.runStatusByNodeId };
          const nextRuntimeLogs: Record<string, RuntimeLogItem[]> = {
            ...state.runtimeLogItemsByNodeId,
          };
          const nextOutputHandles: Record<string, string | null> = {
            ...state.activeOutputHandleByNodeId,
          };
          for (const id of nodeIds) delete next[id];
          for (const id of nodeIds) delete nextRuntimeLogs[id];
          for (const id of nodeIds) delete nextOutputHandles[id];
          return {
            runStatusByNodeId: next,
            runtimeLogItemsByNodeId: nextRuntimeLogs,
            activeOutputHandleByNodeId: nextOutputHandles,
          };
        },
        false,
        'resetRunStatus'
      ),

    setAutoFollow: enabled => set({ isAutoFollow: enabled }, false, 'setAutoFollow'),
    setCurrentRunningNodeId: nodeId =>
      set({ currentRunningNodeId: nodeId }, false, 'setCurrentRunningNodeId'),
    setLastDebugInputs: inputs => set({ lastDebugInputs: inputs }, false, 'setLastDebugInputs'),

    setActiveOutputHandle: (nodeId, outputHandle) =>
      set(
        (state: { activeOutputHandleByNodeId: Record<string, string | null> }) => ({
          activeOutputHandleByNodeId: {
            ...state.activeOutputHandleByNodeId,
            [nodeId]: outputHandle,
          },
        }),
        false,
        'setActiveOutputHandle'
      ),

    resetActiveOutputHandles: nodeIds =>
      set(
        (state: { activeOutputHandleByNodeId: Record<string, string | null> }) => {
          if (!nodeIds || nodeIds.length === 0) return { activeOutputHandleByNodeId: {} };
          const next = { ...state.activeOutputHandleByNodeId } as Record<string, string | null>;
          for (const id of nodeIds) delete next[id];
          return { activeOutputHandleByNodeId: next };
        },
        false,
        'resetActiveOutputHandles'
      ),

    setProgrammaticPan: enabled => set({ isProgrammaticPan: enabled }, false, 'setProgrammaticPan'),
  };
}
