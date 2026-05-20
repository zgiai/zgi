// History helpers: pure functions for snapshots, undo/redo, and history mode.
// Strictly typed. No runtime imports from store to avoid circular deps.

import type { WorkflowEdge, WorkflowNode } from '../type';
import type { Viewport } from '@xyflow/react';
import { deepClone } from '@/utils/object';

// Graph snapshot for undo/redo stack
export interface GraphSnapshot {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
}

// Snapshot of a run's graph at execution time (used for history mode rendering)
export interface RunGraphSnapshot {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  viewport: Viewport;
}

// Graph snapshots for undo/redo stack no longer clone the entire arrays.
// We rely on structural sharing: the Zustand store ensures that unchanged nodes/edges
// keep their original references.
export function makeGraphSnapshot(nodes: WorkflowNode[], edges: WorkflowEdge[]): GraphSnapshot {
  return {
    nodes, // structural sharing: reference current nodes array
    edges, // structural sharing: reference current edges array
  };
}

export const MAX_HISTORY_CAP = 30;

// Equality helpers tuned for workflow graphs (avoid heavy deep equality where possible)
function jsonEqual(a: unknown, b: unknown): boolean {
  // JSON.stringify is acceptable here because node/edge data are plain objects
  // and graphs are relatively small (< few hundred items)
  try {
    return JSON.stringify(a) === JSON.stringify(b);
  } catch {
    return false;
  }
}

export function nodesDeepEqual(a: WorkflowNode[], b: WorkflowNode[]): boolean {
  if (a.length !== b.length) return false;
  const mapA = new Map<string, WorkflowNode>(a.map(n => [n.id, n]));
  for (const nb of b) {
    const na = mapA.get(nb.id);
    if (!na) return false;
    // Compare essential semantic fields. Position/selection/drag states are layout/UI and intentionally ignored here.
    if (na.type !== nb.type) return false;
    // Compare structural relationship: parentId must match
    const pa = (na as unknown as { parentId?: string }).parentId;
    const pb = (nb as unknown as { parentId?: string }).parentId;
    if (pa !== pb) return false;
    // Compare data payload deeply
    if (!jsonEqual(na.data, nb.data)) return false;
  }
  return true;
}

function edgesDeepEqual(a: WorkflowEdge[], b: WorkflowEdge[]): boolean {
  if (a === b) return true;
  if (a.length !== b.length) return false;

  const mapA: Record<string, WorkflowEdge> = Object.create(null);
  for (const e of a) mapA[e.id] = e;

  for (const eb of b) {
    const ea = mapA[eb.id];
    if (!ea) return false;
    // Compare structural fields that define an edge
    if (ea.source !== eb.source) return false;
    if (ea.target !== eb.target) return false;
    const sah = ea.sourceHandle ?? 'source';
    const sbd = eb.sourceHandle ?? 'source';
    const tah = ea.targetHandle ?? 'target';
    const tbd = eb.targetHandle ?? 'target';
    if (sah !== sbd || tah !== tbd) return false;
    if (ea.type !== eb.type) return false;
    // Compare data payload deeply (edge metadata)
    if (!jsonEqual((ea as { data?: unknown }).data, (eb as { data?: unknown }).data)) return false;
  }
  return true;
}

/**
 * Hybrid comparison strategy for workflow graphs:
 * 1. Reference check: O(1) if same arrays
 * 2. Size check: O(1)
 * 3. Shallow item check: O(N) if every node/edge reference is identical
 * 4. Deep check: Fallback to detailed comparison only if references differ
 */
function graphsDeepEqual(
  nodesA: WorkflowNode[],
  edgesA: WorkflowEdge[],
  nodesB: WorkflowNode[],
  edgesB: WorkflowEdge[]
): boolean {
  // 1. Array reference match (fast path)
  if (nodesA === nodesB && edgesA === edgesB) return true;

  // 2. Nodes comparison
  const nodesEqual = (() => {
    if (nodesA === nodesB) return true;
    if (nodesA.length !== nodesB.length) return false;
    // Shallow check of individual node references
    for (let i = 0; i < nodesA.length; i++) {
      if (nodesA[i] !== nodesB[i]) {
        // Fallback to semantic deep equality if references differ
        return nodesDeepEqual(nodesA, nodesB);
      }
    }
    return true;
  })();
  if (!nodesEqual) return false;

  // 3. Edges comparison
  const edgesEqual = (() => {
    if (edgesA === edgesB) return true;
    if (edgesA.length !== edgesB.length) return false;
    // Shallow check of individual edge references
    for (let i = 0; i < edgesA.length; i++) {
      if (edgesA[i] !== edgesB[i]) {
        // Fallback to semantic deep equality
        return edgesDeepEqual(edgesA, edgesB);
      }
    }
    return true;
  })();

  return edgesEqual;
}

export function pushHistory(
  historyPast: GraphSnapshot[],
  nodes: WorkflowNode[],
  edges: WorkflowEdge[]
): { past: GraphSnapshot[]; future: GraphSnapshot[] } {
  const last = historyPast.length > 0 ? historyPast[historyPast.length - 1] : null;

  // Optimized skip check: O(1) reference comparison first, then O(N) shallow item, then deep
  if (last && graphsDeepEqual(last.nodes, last.edges, nodes, edges)) {
    return { past: historyPast, future: [] };
  }

  const snapshot = makeGraphSnapshot(nodes, edges);
  const nextPast = [...historyPast, snapshot];
  const cappedPast =
    nextPast.length > MAX_HISTORY_CAP
      ? nextPast.slice(nextPast.length - MAX_HISTORY_CAP)
      : nextPast;
  return { past: cappedPast, future: [] };
}

export function undo(
  historyPast: GraphSnapshot[],
  nodes: WorkflowNode[],
  edges: WorkflowEdge[],
  historyFuture: GraphSnapshot[]
): null | {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  past: GraphSnapshot[];
  future: GraphSnapshot[];
} {
  if (historyPast.length === 0) return null;
  // Use current state to build future if we need to (history helpers are pure)
  const current = makeGraphSnapshot(nodes, edges);
  const prev = historyPast[historyPast.length - 1];
  const past = historyPast.slice(0, -1);
  const future = [...historyFuture, current];
  return {
    nodes: prev.nodes,
    edges: prev.edges,
    past,
    future,
  };
}

export function redo(
  historyPast: GraphSnapshot[],
  nodes: WorkflowNode[],
  edges: WorkflowEdge[],
  historyFuture: GraphSnapshot[]
): null | {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  past: GraphSnapshot[];
  future: GraphSnapshot[];
} {
  if (historyFuture.length === 0) return null;
  const current = makeGraphSnapshot(nodes, edges);
  const next = historyFuture[historyFuture.length - 1];
  const future = historyFuture.slice(0, -1);
  const past = [...historyPast, current];
  return {
    nodes: next.nodes,
    edges: next.edges,
    past,
    future,
  };
}

export function cloneRunSnapshot(snapshot: RunGraphSnapshot): RunGraphSnapshot {
  // In-memory snapshots used for run results skip heavy cloning
  return {
    nodes: snapshot.nodes,
    edges: snapshot.edges,
    viewport: { ...snapshot.viewport },
  };
}

export function prepareEnterHistoryMode(args: {
  currentMode: 'edit' | 'history';
  currentSelectedRunId: string | null;
  runId: string;
  draftSnapshot: {
    nodes: WorkflowNode[];
    edges: WorkflowEdge[];
    viewport: Viewport;
    selectedNodeId: string | null;
    isDirty: boolean;
    hasLayoutChanges: boolean;
  } | null;
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  viewport: Viewport;
  selectedNodeId: string | null;
  isDirty: boolean;
  hasLayoutChanges: boolean;
}): null | {
  mode: 'history';
  selectedRunId: string;
  draftSnapshot: {
    nodes: WorkflowNode[];
    edges: WorkflowEdge[];
    viewport: Viewport;
    selectedNodeId: string | null;
    isDirty: boolean;
    hasLayoutChanges: boolean;
  };
} {
  const {
    currentMode,
    currentSelectedRunId,
    runId,
    draftSnapshot,
    nodes,
    edges,
    viewport,
    selectedNodeId,
    isDirty,
    hasLayoutChanges,
  } = args;

  if (currentMode === 'history' && currentSelectedRunId === runId) return null;

  const snapshot = draftSnapshot ?? {
    nodes,
    edges,
    viewport: { ...viewport },
    selectedNodeId,
    isDirty,
    hasLayoutChanges,
  };

  return {
    mode: 'history',
    selectedRunId: runId,
    draftSnapshot: snapshot,
  };
}

export function prepareExitHistoryMode(
  draftSnapshot: {
    nodes: WorkflowNode[];
    edges: WorkflowEdge[];
    viewport: Viewport;
    selectedNodeId: string | null;
    isDirty: boolean;
    hasLayoutChanges: boolean;
  } | null
):
  | {
      mode: 'edit';
      selectedRunId: null;
      nodes: WorkflowNode[];
      edges: WorkflowEdge[];
      viewport: Viewport;
      selectedNodeId: string | null;
      isDirty: boolean;
      hasLayoutChanges: boolean;
      draftSnapshot: null;
    }
  | { mode: 'edit'; selectedRunId: null } {
  if (!draftSnapshot) return { mode: 'edit', selectedRunId: null };
  return {
    mode: 'edit',
    selectedRunId: null,
    nodes: draftSnapshot.nodes,
    edges: draftSnapshot.edges,
    viewport: { ...draftSnapshot.viewport },
    selectedNodeId: draftSnapshot.selectedNodeId,
    isDirty: draftSnapshot.isDirty,
    hasLayoutChanges: draftSnapshot.hasLayoutChanges,
    draftSnapshot: null,
  };
}
