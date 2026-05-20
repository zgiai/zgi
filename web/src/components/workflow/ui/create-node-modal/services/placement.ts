import type { WorkflowNode } from '../../../store';
import { ITER_HEADER_H, PAD_X, PAD_Y, GAP_X } from '../constants/iteration-layout';
import { NODE_THEMES } from '../../../nodes/custom/config';
import {
  STAGGER_STEP,
  GROUP_SIZE,
  COLUMN_SHIFT,
  ROW_SHIFT,
  INITIAL_SHIFT,
} from '../constants/place';

export interface NodeBox {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface PlacementInput {
  parent: { w: number; h: number };
  anchor: NodeBox;
  newSize: { w: number; h: number };
  mode: 'insert' | 'append';
  target?: NodeBox | null;
}

export interface PlacementOutput {
  nx: number;
  ny: number;
  nextPw: number;
  nextPh: number;
  targetShiftX?: number; // if provided, shift target.x to this value
}

// Minimal shape for rf node access without importing heavy types
interface FlowNodeLite {
  width?: number;
  height?: number;
  position?: { x: number; y: number };
  data?: { type?: string };
  parentNode?: string;
}
interface FlowRefLite {
  getNode: (id: string) => FlowNodeLite | undefined;
}

// Read node box from rf with fallback to theme size and default
export function getNodeBoxFromGraph(
  rf: FlowRefLite,
  id: string,
  fallback: { w: number; h: number } = { w: 280, h: 120 }
): NodeBox {
  const n = rf.getNode(id);
  const type = (n?.data as { type?: keyof typeof NODE_THEMES } | undefined)?.type;
  const theme = type
    ? (NODE_THEMES as Record<string, { width?: number; height?: number }>)[type as string]
    : undefined;
  const w = n?.width ?? theme?.width ?? fallback.w;
  const h = n?.height ?? theme?.height ?? fallback.h;
  const x = n?.position?.x ?? 0;
  const y = n?.position?.y ?? 0;
  return { x, y, w, h };
}

/**
 * Calculates the flow coordinates of the viewport center.
 */
export function computeViewportCenterFlowPos(viewport: { x: number; y: number; zoom: number }): {
  x: number;
  y: number;
} {
  const container = document.querySelector('.react-flow') as HTMLElement | null;
  const rect = container?.getBoundingClientRect();
  const width = rect?.width ?? 800;
  const height = rect?.height ?? 600;
  return {
    x: (width / 2 - viewport.x) / viewport.zoom,
    y: (height / 2 - viewport.y) / viewport.zoom,
  };
}

/**
 * Calculates a staggered position relative to a base point.
 */
export function computeStaggeredPosition(
  base: { x: number; y: number },
  index: number
): { x: number; y: number } {
  const groupIndex = Math.floor(index / GROUP_SIZE);
  const localIndex = index % GROUP_SIZE;
  const diagonal = localIndex * STAGGER_STEP;
  return {
    x: base.x - INITIAL_SHIFT - groupIndex * COLUMN_SHIFT + diagonal,
    y: base.y - INITIAL_SHIFT - groupIndex * ROW_SHIFT + diagonal,
  };
}

// Pure calculation for new child placement and parent resize
export function computeNewChildPlacement(inp: PlacementInput): PlacementOutput {
  const minY = ITER_HEADER_H + PAD_Y * 2; // 56
  const nx = inp.anchor.x + inp.anchor.w + GAP_X;
  const ny = Math.max(inp.anchor.y, minY);
  const requiredRight = nx + inp.newSize.w + PAD_X * 2;
  const requiredBottom = ny + inp.newSize.h + PAD_Y * 2;

  let tRequiredRight = 0;
  let targetShiftX: number | undefined;
  if (inp.mode === 'insert' && inp.target) {
    const tx = Math.max(inp.target.x, nx + inp.newSize.w + GAP_X);
    targetShiftX = tx;
    tRequiredRight = tx + inp.target.w + PAD_X * 2;
  }

  let nextPw = Math.max(inp.parent.w, requiredRight, tRequiredRight);
  if (inp.mode === 'insert') {
    nextPw = Math.max(nextPw, inp.parent.w + (inp.newSize.w + GAP_X));
  }
  const nextPh = Math.max(inp.parent.h, requiredBottom);

  return { nx, ny, nextPw, nextPh, targetShiftX };
}

// First pass application: expand parent then place child, optionally shift target
export function applyPlacementFirstPass(params: {
  updateNode: (id: string, data: Partial<WorkflowNode>) => void;
  parentId: string;
  targetId?: string;
  placement: PlacementOutput;
  newNodeId: string;
  targetBox?: NodeBox | null;
}) {
  const { updateNode, parentId, targetId, placement, newNodeId, targetBox } = params;
  updateNode(parentId, {
    width: placement.nextPw,
    height: placement.nextPh,
  } as unknown as WorkflowNode);
  updateNode(newNodeId, {
    position: { x: placement.nx, y: placement.ny },
  } as unknown as WorkflowNode);
  if (targetId && typeof placement.targetShiftX === 'number' && targetBox) {
    if (placement.targetShiftX !== targetBox.x) {
      updateNode(targetId, {
        position: { x: placement.targetShiftX, y: targetBox.y },
      } as unknown as WorkflowNode);
    }
  }
}

// Second pass: re-evaluate using measured sizes and adjust parent/target if needed
export function applyPlacementSecondPass(params: {
  rf: FlowRefLite;
  updateNode: (id: string, data: Partial<WorkflowNode>) => void;
  parentId: string;
  newNodeId: string;
  placement: PlacementOutput; // includes nx, ny
  mode: 'insert' | 'append';
  parentBaseW: number; // original parent width before expansion
  targetId?: string;
  targetBox?: NodeBox | null;
}) {
  const { rf, updateNode, parentId, newNodeId, placement, mode, parentBaseW, targetId, targetBox } =
    params;

  const refine = () => {
    const n = rf.getNode(newNodeId);
    const nw = (n?.width as number | undefined) ?? 280;
    const nh = (n?.height as number | undefined) ?? 120;
    const reqRight2 = placement.nx + nw + PAD_X * 2;
    const reqBottom2 = placement.ny + nh + PAD_Y * 2;
    const p2 = rf.getNode(parentId);
    const pw1 = (p2?.width as number | undefined) ?? placement.nextPw;
    const ph1 = (p2?.height as number | undefined) ?? placement.nextPh;

    let tReqRight2 = 0;
    if (mode === 'insert' && targetId && targetBox) {
      const tN = rf.getNode(targetId);
      const tw = (tN?.width as number | undefined) ?? targetBox.w ?? 240;
      const tx2 = Math.max(targetBox.x, placement.nx + nw + GAP_X);
      tReqRight2 = tx2 + tw + PAD_X * 2;
      const ty = targetBox.y;
      if (!tN || (tN.position?.x ?? 0) !== tx2) {
        updateNode(targetId, { position: { x: tx2, y: ty } } as unknown as WorkflowNode);
      }
    }

    let adjPw = Math.max(pw1, reqRight2, tReqRight2);
    if (mode === 'insert') {
      adjPw = Math.max(adjPw, parentBaseW + (nw + GAP_X));
    }
    const adjPh = Math.max(ph1, reqBottom2);
    if (adjPw !== pw1 || adjPh !== ph1) {
      updateNode(parentId, { width: adjPw, height: adjPh } as unknown as WorkflowNode);
    }
  };

  if (typeof window !== 'undefined') {
    requestAnimationFrame(refine);
  } else {
    refine();
  }
}

// Shift all downstream nodes of a root within a given parent by a uniform delta.
// This is used after inserting a node in an iteration to create horizontal space.
export function shiftDownstreamNodes(params: {
  rf: FlowRefLite;
  updateNode: (id: string, data: Partial<WorkflowNode>) => void;
  edges: Array<{ source: string; target: string }>;
  rootId: string;
  parentId?: string;
  dx: number;
  dy: number;
  skipRoot?: boolean;
}) {
  const { rf, updateNode, edges, rootId, parentId, dx, dy, skipRoot } = params;
  if (!dx && !dy) return;
  const visited = new Set<string>();
  const queue: string[] = [];

  // Seed queue with direct children of root
  for (const e of edges) {
    if (e.source === rootId) {
      queue.push(e.target);
    }
  }

  while (queue.length) {
    const nid = queue.shift()!;
    if (visited.has(nid)) continue;
    visited.add(nid);
    const n = rf.getNode(nid);
    if (!n) continue;
    if (parentId && n.parentNode && n.parentNode !== parentId) {
      continue; // keep shifts scoped within parent container if provided
    }
    const pos = n.position ?? { x: 0, y: 0 };
    updateNode(nid, { position: { x: pos.x + dx, y: pos.y + dy } } as unknown as WorkflowNode);
    // Enqueue children
    for (const e of edges) {
      if (e.source === nid) {
        queue.push(e.target);
      }
    }
  }

  // Optionally shift the root itself (not used for insert because target already positioned)
  if (!skipRoot) {
    const rt = rf.getNode(rootId);
    if (rt) {
      const rpos = rt.position ?? { x: 0, y: 0 };
      updateNode(rootId, {
        position: { x: rpos.x + dx, y: rpos.y + dy },
      } as unknown as WorkflowNode);
    }
  }
}

/**
 * Manual screen-to-flow conversion to match NodeLeftPanel's offset logic.
 */
export function screenToFlowPositionManual(
  screenPos: { x: number; y: number },
  viewport: { x: number; y: number; zoom: number }
): { x: number; y: number } {
  const container = document.querySelector('.react-flow') as HTMLElement | null;
  const rect = container?.getBoundingClientRect() || { left: 0, top: 0 };
  return {
    x: (screenPos.x - rect.left - viewport.x) / viewport.zoom,
    y: (screenPos.y - rect.top - viewport.y) / viewport.zoom,
  };
}
