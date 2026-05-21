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
  runtimeLogPopoverOpenByNodeId: Record<string, boolean>;
  runtimeLogPopoverManualByNodeId: Record<string, boolean>;
  runtimeLogAutoOpenEnabled: boolean;
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
  setRuntimeLogPopoverOpen: (nodeId: string, open: boolean) => void;
  beginRuntimeLogPopoverAutoOpen: () => void;
  openRuntimeLogPopoversForActiveRun: (nodeIds: string[]) => void;
  finalizeRuntimeLogPopoversAfterRun: () => void;
  resetRuntimeLogPopovers: () => void;
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
        runtimeLogPopoverOpenByNodeId: Record<string, boolean>;
        runtimeLogPopoverManualByNodeId: Record<string, boolean>;
        runtimeLogAutoOpenEnabled: boolean;
        isAutoFollow: boolean;
        currentRunningNodeId: string | null;
        lastDebugInputs: Record<string, unknown> | null;
        activeOutputHandleByNodeId: Record<string, string | null>;
        isProgrammaticPan: boolean;
      }>
    | ((state: {
        runStatusByNodeId: Record<string, RunStatus>;
        runtimeLogItemsByNodeId: Record<string, RuntimeLogItem[]>;
        runtimeLogPopoverOpenByNodeId: Record<string, boolean>;
        runtimeLogPopoverManualByNodeId: Record<string, boolean>;
        runtimeLogAutoOpenEnabled: boolean;
        isAutoFollow: boolean;
        currentRunningNodeId: string | null;
        lastDebugInputs: Record<string, unknown> | null;
        activeOutputHandleByNodeId: Record<string, string | null>;
        isProgrammaticPan: boolean;
      }) => Partial<{
        runStatusByNodeId: Record<string, RunStatus>;
        runtimeLogItemsByNodeId: Record<string, RuntimeLogItem[]>;
        runtimeLogPopoverOpenByNodeId: Record<string, boolean>;
        runtimeLogPopoverManualByNodeId: Record<string, boolean>;
        runtimeLogAutoOpenEnabled: boolean;
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
    runtimeLogPopoverOpenByNodeId: {},
    runtimeLogPopoverManualByNodeId: {},
    runtimeLogAutoOpenEnabled: false,
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
        (state: {
          runtimeLogPopoverOpenByNodeId: Record<string, boolean>;
          runtimeLogPopoverManualByNodeId: Record<string, boolean>;
          runtimeLogAutoOpenEnabled: boolean;
        }) => {
          if (items.length === 0) {
            return {
              runtimeLogItemsByNodeId: {},
              runtimeLogPopoverOpenByNodeId: {},
              runtimeLogPopoverManualByNodeId: {},
              runtimeLogAutoOpenEnabled: state.runtimeLogAutoOpenEnabled,
            };
          }
          const grouped: Record<string, RuntimeLogItem[]> = {};
          const nodeIds = new Set<string>();
          const appendItem = (item: RuntimeLogItem) => {
            if (item.nodeId) {
              grouped[item.nodeId] = [...(grouped[item.nodeId] ?? []), item];
              nodeIds.add(item.nodeId);
            }
            item.iterationRounds?.forEach(round => round.nodes.forEach(appendItem));
            item.loopRounds?.forEach(round => round.nodes.forEach(appendItem));
          };
          items.forEach(appendItem);
          const nextOpen = { ...state.runtimeLogPopoverOpenByNodeId };
          if (state.runtimeLogAutoOpenEnabled) {
            for (const nodeId of nodeIds) {
              if (!state.runtimeLogPopoverManualByNodeId[nodeId]) nextOpen[nodeId] = true;
            }
          }
          return { runtimeLogItemsByNodeId: grouped, runtimeLogPopoverOpenByNodeId: nextOpen };
        },
        false,
        'setRuntimeLogItems'
      ),

    resetRuntimeLogItems: () =>
      set(
        {
          runtimeLogItemsByNodeId: {},
          runtimeLogPopoverOpenByNodeId: {},
          runtimeLogPopoverManualByNodeId: {},
          runtimeLogAutoOpenEnabled: false,
        },
        false,
        'resetRuntimeLogItems'
      ),

    setRuntimeLogPopoverOpen: (nodeId, open) =>
      set(
        (state: {
          runtimeLogPopoverOpenByNodeId: Record<string, boolean>;
          runtimeLogPopoverManualByNodeId: Record<string, boolean>;
        }) => ({
          runtimeLogPopoverOpenByNodeId: {
            ...state.runtimeLogPopoverOpenByNodeId,
            [nodeId]: open,
          },
          runtimeLogPopoverManualByNodeId: {
            ...state.runtimeLogPopoverManualByNodeId,
            [nodeId]: true,
          },
        }),
        false,
        'setRuntimeLogPopoverOpen'
      ),

    beginRuntimeLogPopoverAutoOpen: () =>
      set(
        {
          runtimeLogPopoverOpenByNodeId: {},
          runtimeLogPopoverManualByNodeId: {},
          runtimeLogAutoOpenEnabled: true,
        },
        false,
        'beginRuntimeLogPopoverAutoOpen'
      ),

    openRuntimeLogPopoversForActiveRun: nodeIds =>
      set(
        (state: {
          runtimeLogPopoverOpenByNodeId: Record<string, boolean>;
          runtimeLogPopoverManualByNodeId: Record<string, boolean>;
          runtimeLogAutoOpenEnabled: boolean;
        }) => {
          const next = { ...state.runtimeLogPopoverOpenByNodeId };
          if (state.runtimeLogAutoOpenEnabled) {
            for (const nodeId of nodeIds) {
              if (!state.runtimeLogPopoverManualByNodeId[nodeId]) next[nodeId] = true;
            }
          }
          return { runtimeLogPopoverOpenByNodeId: next };
        },
        false,
        'openRuntimeLogPopoversForActiveRun'
      ),

    finalizeRuntimeLogPopoversAfterRun: () =>
      set(
        (state: {
          runStatusByNodeId: Record<string, RunStatus>;
          runtimeLogItemsByNodeId: Record<string, RuntimeLogItem[]>;
        }) => {
          const failedNodeIds = new Set<string>();
          let latestNodeId: string | null = null;
          let latestOrder = -1;
          let latestTime = -1;
          const appendFailed = (item: RuntimeLogItem) => {
            if (item.nodeId && item.status === 'failed') failedNodeIds.add(item.nodeId);
            if (item.nodeId) {
              const itemOrder = item.receivedOrder ?? -1;
              const itemTime = item.createdAtMs ?? -1;
              if (
                latestNodeId === null ||
                itemOrder > latestOrder ||
                (itemOrder === latestOrder && itemTime >= latestTime)
              ) {
                latestNodeId = item.nodeId;
                latestOrder = itemOrder;
                latestTime = itemTime;
              }
            }
            item.iterationRounds?.forEach(round => round.nodes.forEach(appendFailed));
            item.loopRounds?.forEach(round => round.nodes.forEach(appendFailed));
          };
          for (const [nodeId, status] of Object.entries(state.runStatusByNodeId)) {
            if (status === 'failed') failedNodeIds.add(nodeId);
          }
          Object.values(state.runtimeLogItemsByNodeId).forEach(items =>
            items.forEach(appendFailed)
          );

          const nextOpen: Record<string, boolean> = {};
          failedNodeIds.forEach(nodeId => {
            nextOpen[nodeId] = true;
          });
          if (latestNodeId) nextOpen[latestNodeId] = true;
          return {
            runtimeLogPopoverOpenByNodeId: nextOpen,
            runtimeLogPopoverManualByNodeId: {},
            runtimeLogAutoOpenEnabled: false,
          };
        },
        false,
        'finalizeRuntimeLogPopoversAfterRun'
      ),

    resetRuntimeLogPopovers: () =>
      set(
        {
          runtimeLogPopoverOpenByNodeId: {},
          runtimeLogPopoverManualByNodeId: {},
          runtimeLogAutoOpenEnabled: false,
        },
        false,
        'resetRuntimeLogPopovers'
      ),

    resetRunStatus: nodeIds =>
      set(
        (state: {
          runStatusByNodeId: Record<string, RunStatus>;
          runtimeLogItemsByNodeId: Record<string, RuntimeLogItem[]>;
          activeOutputHandleByNodeId: Record<string, string | null>;
          runtimeLogPopoverOpenByNodeId: Record<string, boolean>;
          runtimeLogPopoverManualByNodeId: Record<string, boolean>;
          runtimeLogAutoOpenEnabled: boolean;
        }) => {
          if (!nodeIds || nodeIds.length === 0) {
            return {
              runStatusByNodeId: {},
              runtimeLogItemsByNodeId: {},
              activeOutputHandleByNodeId: {},
              runtimeLogPopoverOpenByNodeId: {},
              runtimeLogPopoverManualByNodeId: {},
              runtimeLogAutoOpenEnabled: false,
            };
          }
          const next: Record<string, RunStatus> = { ...state.runStatusByNodeId };
          const nextRuntimeLogs: Record<string, RuntimeLogItem[]> = {
            ...state.runtimeLogItemsByNodeId,
          };
          const nextOutputHandles: Record<string, string | null> = {
            ...state.activeOutputHandleByNodeId,
          };
          const nextPopoverOpen: Record<string, boolean> = {
            ...state.runtimeLogPopoverOpenByNodeId,
          };
          const nextPopoverManual: Record<string, boolean> = {
            ...state.runtimeLogPopoverManualByNodeId,
          };
          for (const id of nodeIds) delete next[id];
          for (const id of nodeIds) delete nextRuntimeLogs[id];
          for (const id of nodeIds) delete nextOutputHandles[id];
          for (const id of nodeIds) delete nextPopoverOpen[id];
          for (const id of nodeIds) delete nextPopoverManual[id];
          return {
            runStatusByNodeId: next,
            runtimeLogItemsByNodeId: nextRuntimeLogs,
            activeOutputHandleByNodeId: nextOutputHandles,
            runtimeLogPopoverOpenByNodeId: nextPopoverOpen,
            runtimeLogPopoverManualByNodeId: nextPopoverManual,
            runtimeLogAutoOpenEnabled: state.runtimeLogAutoOpenEnabled,
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
