// Mode & Selection slice: editor mode, run history selection, selection, agent type
// Also exposes enter/exit history mode and run snapshots

import type { Viewport } from '@xyflow/react';
import type { WorkflowStore } from '../store';
import type { RunGraphSnapshot } from '../helpers/history';
import type { WorkflowNodeData } from '../type';
import { AgentType } from '@/services/types/agent';

import {
  prepareEnterHistoryMode,
  prepareExitHistoryMode,
  cloneRunSnapshot,
} from '../helpers/history';
import type { WorkflowNode, WorkflowEdge } from '../type';

export interface ModeSelectionSlice {
  // UI
  selectedNodeId: string | null;
  isLoading: boolean;
  isInitialized: boolean;
  isDirty: boolean;
  hasLayoutChanges: boolean;

  // Interaction
  interactionMode: 'pointer' | 'hand';
  setInteractionMode: (mode: 'pointer' | 'hand') => void;

  // Editor mode
  mode: 'edit' | 'history';
  selectedRunId: string | null;
  draftSnapshot: {
    nodes: WorkflowNode[];
    edges: WorkflowEdge[];
    viewport: Viewport;
    selectedNodeId: string | null;
    isDirty: boolean;
    hasLayoutChanges: boolean;
  } | null;

  // Agent type & ID
  agentType: AgentType;
  setAgentType: (agentType: AgentType) => void;
  agentId: string | null;
  setAgentId: (agentId: string | null) => void;

  // Selection
  selectNode: (nodeId: string | null) => void;
  // Source of selection for UI gating (e.g., click, create, drag, programmatic)
  selectionSource: 'none' | 'click' | 'create' | 'drag' | 'program';
  setSelectionSource: (src: 'none' | 'click' | 'create' | 'drag' | 'program') => void;

  // Run snapshots for history mode
  historySnapshots: Record<string, RunGraphSnapshot>;
  setHistorySnapshot: (runId: string, snapshot: RunGraphSnapshot) => void;

  // Clipboard
  clipboardNodeData: WorkflowNodeData | null;
  setClipboardNodeData: (data: WorkflowNodeData | null) => void;

  // Mouse pointer tracking for paste
  lastMouseClient: { x: number; y: number } | null;
  setLastMouseClient: (pos: { x: number; y: number } | null) => void;

  // Multi-node clipboard (support iteration grouping)
  clipboardNodes: Array<{
    data: WorkflowNodeData;
    offset: { x: number; y: number };
    parentRef?: string;
    refId?: string;
  }>;
  setClipboardNodes: (
    items: Array<{
      data: WorkflowNodeData;
      offset: { x: number; y: number };
      parentRef?: string;
      refId?: string;
    }>
  ) => void;

  // Interaction blocking
  isCreatingNode: boolean;
  setIsCreatingNode: (val: boolean) => void;
  isContextMenuOpen: boolean;
  setIsContextMenuOpen: (val: boolean) => void;
  edgeDescId: string | null;
  setEdgeDescId: (id: string | null) => void;
  edgeDescPosition: { x: number; y: number } | null;
  setEdgeDescPosition: (pos: { x: number; y: number } | null) => void;

  // Mode transitions
  enterHistoryMode: (runId: string) => void;
  exitHistoryMode: () => void;
}

export function createModeSelectionSlice(
  set: (
    partial: Partial<WorkflowStore> | ((state: WorkflowStore) => Partial<WorkflowStore>),
    replace?: boolean,
    action?: string
  ) => void,
  get: () => WorkflowStore
): ModeSelectionSlice {
  return {
    // defaults
    selectedNodeId: null,
    isLoading: false,
    isInitialized: false,
    isDirty: false,
    hasLayoutChanges: false,

    interactionMode: 'hand',
    setInteractionMode: mode => {
      if (get().interactionMode === mode) return;
      set({ interactionMode: mode }, false, 'setInteractionMode');
    },

    mode: 'edit',
    selectedRunId: null,
    draftSnapshot: null,

    agentType: AgentType.WORKFLOW,
    setAgentType: agentType => set({ agentType }, false, 'setAgentType'),
    agentId: null,
    setAgentId: agentId => set({ agentId }, false, 'setAgentId'),

    selectNode: nodeId => {
      const state = get();
      if (state.selectedNodeId === nodeId) return;
      const nextNodes = state.nodes.map(n => {
        const shouldSelect = !!nodeId && n.id === nodeId;
        if ((n as WorkflowNode).selected === shouldSelect) return n as WorkflowNode;
        return { ...(n as WorkflowNode), selected: shouldSelect } as WorkflowNode;
      });
      set({ selectedNodeId: nodeId, nodes: nextNodes }, false, 'selectNode');
    },

    // Selection source gating for UI behaviors
    selectionSource: 'none',
    setSelectionSource: src => set({ selectionSource: src }, false, 'setSelectionSource'),

    historySnapshots: {},
    setHistorySnapshot: (runId, snapshot) => {
      const cloned: RunGraphSnapshot = cloneRunSnapshot(snapshot);
      set(
        state => {
          const nextSnapshots = { ...state.historySnapshots, [runId]: cloned };
          const keys = Object.keys(nextSnapshots);
          // Limit capacity to 2 to prevent memory overflow.
          // Older snapshots will be re-fetched from server when requested in History Mode.
          if (keys.length > 2) {
            // Sort by insertion order is not guaranteed by Object.keys,
            // but for a small record of 3 items, deleting any but the current is safer than unbounded growth.
            // Ideally we'd track timestamp, but simple pruning is sufficient here.
            const oldestKey = keys[0] === runId ? keys[1] : keys[0];
            delete nextSnapshots[oldestKey];
          }
          return { historySnapshots: nextSnapshots };
        },
        false,
        'setHistorySnapshot'
      );
    },

    // Clipboard defaults and setters
    clipboardNodeData: null,
    setClipboardNodeData: data => set({ clipboardNodeData: data }, false, 'setClipboardNodeData'),

    // Mouse pointer tracking for paste
    lastMouseClient: null,
    setLastMouseClient: pos => set({ lastMouseClient: pos }, false, 'setLastMouseClient'),

    // Multi-node clipboard defaults and setters
    clipboardNodes: [],
    setClipboardNodes: items => set({ clipboardNodes: items }, false, 'setClipboardNodes'),

    isCreatingNode: false,
    setIsCreatingNode: val => set({ isCreatingNode: val }, false, 'setIsCreatingNode'),
    isContextMenuOpen: false,
    setIsContextMenuOpen: val => set({ isContextMenuOpen: val }, false, 'setIsContextMenuOpen'),
    edgeDescId: null,
    setEdgeDescId: id => set({ edgeDescId: id }, false, 'setEdgeDescId'),
    edgeDescPosition: null,
    setEdgeDescPosition: pos => set({ edgeDescPosition: pos }, false, 'setEdgeDescPosition'),

    // History mode transitions (restore removed functions)
    enterHistoryMode: runId => {
      const state = get();
      const prepared = prepareEnterHistoryMode({
        currentMode: state.mode,
        currentSelectedRunId: state.selectedRunId,
        runId,
        draftSnapshot: state.draftSnapshot,
        nodes: state.nodes,
        edges: state.edges,
        viewport: state.viewport,
        selectedNodeId: state.selectedNodeId,
        isDirty: state.isDirty,
        hasLayoutChanges: state.hasLayoutChanges,
      });
      if (!prepared) return;
      set(
        {
          mode: 'history',
          selectedRunId: prepared.selectedRunId,
          draftSnapshot: prepared.draftSnapshot,
        },
        false,
        'enterHistoryMode'
      );
    },

    exitHistoryMode: () => {
      const res = prepareExitHistoryMode(get().draftSnapshot);
      if ('nodes' in res) {
        set(
          {
            mode: res.mode,
            selectedRunId: res.selectedRunId,
            nodes: res.nodes,
            edges: res.edges,
            viewport: res.viewport,
            selectedNodeId: res.selectedNodeId,
            isDirty: res.isDirty,
            hasLayoutChanges: res.hasLayoutChanges,
            draftSnapshot: res.draftSnapshot,
          },
          false,
          'exitHistoryMode'
        );
      } else {
        set({ mode: res.mode, selectedRunId: res.selectedRunId }, false, 'exitHistoryMode');
      }
    },
  };
}
