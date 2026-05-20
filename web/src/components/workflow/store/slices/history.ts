// History slice: undo/redo stacks and operations
import type { WorkflowStore } from '../store';
import type { GraphSnapshot } from '../helpers/history';
import {
  pushHistory as histPushHistory,
  undo as histUndo,
  redo as histRedo,
  makeGraphSnapshot,
} from '../helpers/history';

export interface HistorySlice {
  historyPast: GraphSnapshot[];
  historyFuture: GraphSnapshot[];
  pushHistory: () => void;
  undo: () => void;
  redo: () => void;
  // Batch flag and controls
  isHistoryBatching: boolean;
  // Baseline snapshot captured at beginHistoryBatch, pushed on endHistoryBatch
  historyBatchBaseline: GraphSnapshot | null;
  beginHistoryBatch: () => void;
  endHistoryBatch: () => void;
}

export function createHistorySlice(
  set: (
    partial: Partial<WorkflowStore> | ((state: WorkflowStore) => Partial<WorkflowStore>),
    replace?: boolean,
    action?: string
  ) => void,
  get: () => WorkflowStore
): HistorySlice {
  return {
    historyPast: [],
    historyFuture: [],
    isHistoryBatching: false,
    historyBatchBaseline: null,

    pushHistory: () => {
      const state = get();
      const { past, future } = histPushHistory(state.historyPast, state.nodes, state.edges);
      set({ historyPast: past, historyFuture: future }, false, 'pushHistory');
    },

    undo: () => {
      const state = get();
      const res = histUndo(state.historyPast, state.nodes, state.edges, state.historyFuture);
      if (!res) return;
      set(
        {
          nodes: res.nodes,
          edges: res.edges,
          isDirty: true,
          historyPast: res.past,
          historyFuture: res.future,
          // Prevent immediate React Flow layout event from pushing a new snapshot and wiping redo
          suppressNextLayoutHistoryPush: true,
        } as unknown as Partial<WorkflowStore>,
        false,
        'undo'
      );
      // Trigger validation after history state restore
      get().syncRunnableSets();
    },

    redo: () => {
      const state = get();
      const res = histRedo(state.historyPast, state.nodes, state.edges, state.historyFuture);
      if (!res) return;
      set(
        {
          nodes: res.nodes,
          edges: res.edges,
          isDirty: true,
          historyPast: res.past,
          historyFuture: res.future,
          // Prevent immediate React Flow layout event from pushing a new snapshot
          suppressNextLayoutHistoryPush: true,
        } as unknown as Partial<WorkflowStore>,
        false,
        'redo'
      );
      // Trigger validation after history state restore
      get().syncRunnableSets();
    },
    beginHistoryBatch: () => {
      const state = get();
      const baseline = makeGraphSnapshot(state.nodes, state.edges);
      set({ isHistoryBatching: true, historyBatchBaseline: baseline }, false, 'beginHistoryBatch');
    },
    endHistoryBatch: () => {
      const state = get();
      const baseline = state.historyBatchBaseline;
      const { past, future } = baseline
        ? histPushHistory(state.historyPast, baseline.nodes, baseline.edges)
        : histPushHistory(state.historyPast, state.nodes, state.edges);
      set(
        {
          isHistoryBatching: false,
          historyBatchBaseline: null,
          historyPast: past,
          historyFuture: future,
          suppressNextLayoutHistoryPush: true,
        } as unknown as Partial<WorkflowStore>,
        false,
        'endHistoryBatch'
      );
      // Trigger validation once at the end of the batch
      get().syncRunnableSets();
    },
  };
}
