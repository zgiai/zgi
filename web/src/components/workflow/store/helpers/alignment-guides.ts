import type { NodeChange } from '@xyflow/react';
import type { WorkflowNode, WorkflowNodeData } from '../type';
import { NODE_THEMES } from '../../nodes/custom/config';

const DEFAULT_NODE_WIDTH = 280;
const DEFAULT_NODE_HEIGHT = 120;
const DEFAULT_THRESHOLD = 6;

export type AlignmentAnchorX = 'left' | 'center' | 'right';
export type AlignmentAnchorY = 'top' | 'middle' | 'bottom';

const X_ANCHORS: readonly AlignmentAnchorX[] = ['left', 'center', 'right'];
const Y_ANCHORS: readonly AlignmentAnchorY[] = ['top', 'middle', 'bottom'];

export interface AlignmentGuide {
  value: number;
  from: number;
  to: number;
}

export interface AlignmentGuideState {
  vertical?: AlignmentGuide;
  horizontal?: AlignmentGuide;
}

interface NodeRect {
  parentId?: string;
  local: Rect;
  absolute: Rect;
}

interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

interface AlignmentCandidate {
  delta: number;
  distance: number;
  guide: AlignmentGuide;
}

export interface ApplyNodeAlignmentResult {
  changes: Array<NodeChange<WorkflowNode>>;
  guides: AlignmentGuideState | null;
}

type WorkflowNodePositionChange = Extract<
  NodeChange<WorkflowNode>,
  { type: 'position'; id: string }
>;

function getNodeParentId(node: WorkflowNode): string | undefined {
  return (node as unknown as { parentId?: string }).parentId;
}

function getNodeSize(node: WorkflowNode): { width: number; height: number } {
  const dataType = (node.data as WorkflowNodeData | undefined)?.type;
  const theme = dataType ? NODE_THEMES[dataType] : undefined;

  return {
    width: node.measured?.width ?? node.width ?? theme?.width ?? DEFAULT_NODE_WIDTH,
    height: node.measured?.height ?? node.height ?? theme?.height ?? DEFAULT_NODE_HEIGHT,
  };
}

function getAbsolutePosition(
  node: WorkflowNode,
  nodeById: Map<string, WorkflowNode>
): { x: number; y: number } {
  const position = node.position ?? { x: 0, y: 0 };
  const absolute = { x: position.x, y: position.y };
  let current = node;
  const visited = new Set<string>([node.id]);

  while (getNodeParentId(current)) {
    const parentId = getNodeParentId(current);
    if (!parentId || visited.has(parentId)) break;
    const parent = nodeById.get(parentId);
    if (!parent) break;
    visited.add(parent.id);
    absolute.x += parent.position?.x ?? 0;
    absolute.y += parent.position?.y ?? 0;
    current = parent;
  }

  return absolute;
}

function getNodeRect(node: WorkflowNode, nodeById: Map<string, WorkflowNode>): NodeRect {
  const position = node.position ?? { x: 0, y: 0 };
  const size = getNodeSize(node);
  const absolutePosition = getAbsolutePosition(node, nodeById);

  return {
    parentId: getNodeParentId(node),
    local: {
      x: position.x,
      y: position.y,
      width: size.width,
      height: size.height,
    },
    absolute: {
      x: absolutePosition.x,
      y: absolutePosition.y,
      width: size.width,
      height: size.height,
    },
  };
}

function withPosition(rect: NodeRect, position: { x: number; y: number }): NodeRect {
  const dx = position.x - rect.local.x;
  const dy = position.y - rect.local.y;

  return {
    ...rect,
    local: {
      ...rect.local,
      x: position.x,
      y: position.y,
    },
    absolute: {
      ...rect.absolute,
      x: rect.absolute.x + dx,
      y: rect.absolute.y + dy,
    },
  };
}

function getXValue(rect: Rect, anchor: AlignmentAnchorX): number {
  if (anchor === 'center') return rect.x + rect.width / 2;
  if (anchor === 'right') return rect.x + rect.width;
  return rect.x;
}

function getYValue(rect: Rect, anchor: AlignmentAnchorY): number {
  if (anchor === 'middle') return rect.y + rect.height / 2;
  if (anchor === 'bottom') return rect.y + rect.height;
  return rect.y;
}

function getXDelta(width: number, anchor: AlignmentAnchorX): number {
  if (anchor === 'center') return width / 2;
  if (anchor === 'right') return width;
  return 0;
}

function getYDelta(height: number, anchor: AlignmentAnchorY): number {
  if (anchor === 'middle') return height / 2;
  if (anchor === 'bottom') return height;
  return 0;
}

function getVerticalGuide(
  active: NodeRect,
  target: NodeRect,
  value: number
): AlignmentGuide {
  return {
    value,
    from: Math.min(active.absolute.y, target.absolute.y),
    to: Math.max(
      active.absolute.y + active.absolute.height,
      target.absolute.y + target.absolute.height
    ),
  };
}

function getHorizontalGuide(
  active: NodeRect,
  target: NodeRect,
  value: number
): AlignmentGuide {
  return {
    value,
    from: Math.min(active.absolute.x, target.absolute.x),
    to: Math.max(
      active.absolute.x + active.absolute.width,
      target.absolute.x + target.absolute.width
    ),
  };
}

function pickBestCandidate(
  current: AlignmentCandidate | null,
  next: AlignmentCandidate
): AlignmentCandidate {
  if (!current) return next;
  if (next.distance < current.distance) return next;
  return current;
}

function getVerticalCandidate(
  active: NodeRect,
  target: NodeRect,
  threshold: number
): AlignmentCandidate | null {
  let best: AlignmentCandidate | null = null;

  for (const activeAnchor of X_ANCHORS) {
    const activeValue = getXValue(active.local, activeAnchor);
    for (const targetAnchor of X_ANCHORS) {
      const targetValue = getXValue(target.local, targetAnchor);
      const delta = targetValue - activeValue;
      const distance = Math.abs(delta);
      if (distance > threshold) continue;

      best = pickBestCandidate(best, {
        delta,
        distance,
        guide: getVerticalGuide(
          active,
          target,
          target.absolute.x + getXDelta(target.absolute.width, targetAnchor)
        ),
      });
    }
  }

  return best;
}

function getHorizontalCandidate(
  active: NodeRect,
  target: NodeRect,
  threshold: number
): AlignmentCandidate | null {
  let best: AlignmentCandidate | null = null;

  for (const activeAnchor of Y_ANCHORS) {
    const activeValue = getYValue(active.local, activeAnchor);
    for (const targetAnchor of Y_ANCHORS) {
      const targetValue = getYValue(target.local, targetAnchor);
      const delta = targetValue - activeValue;
      const distance = Math.abs(delta);
      if (distance > threshold) continue;

      best = pickBestCandidate(best, {
        delta,
        distance,
        guide: getHorizontalGuide(
          active,
          target,
          target.absolute.y + getYDelta(target.absolute.height, targetAnchor)
        ),
      });
    }
  }

  return best;
}

function isPositionChange(
  change: NodeChange<WorkflowNode>
): change is WorkflowNodePositionChange {
  return change.type === 'position' && 'id' in change;
}

export function applySingleNodeAlignment(
  nodes: WorkflowNode[],
  changes: Array<NodeChange<WorkflowNode>>,
  options: { threshold?: number } = {}
): ApplyNodeAlignmentResult {
  const threshold = options.threshold ?? DEFAULT_THRESHOLD;
  const alignablePositionChanges = changes.filter(
    (change): change is WorkflowNodePositionChange =>
      isPositionChange(change) && typeof change.dragging === 'boolean' && Boolean(change.position)
  );

  if (alignablePositionChanges.length !== 1) {
    return { changes, guides: null };
  }

  const activeChange = alignablePositionChanges[0];
  const nextPosition = activeChange.position;
  if (!nextPosition) return { changes, guides: null };

  const nodeById = new Map(nodes.map(node => [node.id, node]));
  const activeNode = nodeById.get(activeChange.id);
  if (!activeNode || activeNode.hidden) return { changes, guides: null };

  const activeBaseRect = getNodeRect(activeNode, nodeById);
  const activeRect = withPosition(activeBaseRect, nextPosition);
  const siblingRects = nodes
    .filter(node => node.id !== activeNode.id)
    .filter(node => !node.hidden)
    .filter(node => getNodeParentId(node) === activeBaseRect.parentId)
    .map(node => getNodeRect(node, nodeById));

  let vertical: AlignmentCandidate | null = null;
  let horizontal: AlignmentCandidate | null = null;

  for (const target of siblingRects) {
    const nextVertical = getVerticalCandidate(activeRect, target, threshold);
    if (nextVertical) vertical = pickBestCandidate(vertical, nextVertical);

    const nextHorizontal = getHorizontalCandidate(activeRect, target, threshold);
    if (nextHorizontal) horizontal = pickBestCandidate(horizontal, nextHorizontal);
  }

  if (!vertical && !horizontal) {
    return { changes, guides: null };
  }

  const snappedPosition = {
    x: nextPosition.x + (vertical?.delta ?? 0),
    y: nextPosition.y + (horizontal?.delta ?? 0),
  };

  const alignedChanges = changes.map(change => {
    if (change !== activeChange) return change;
    return {
      ...change,
      position: snappedPosition,
    };
  });

  return {
    changes: alignedChanges,
    guides: {
      vertical: vertical?.guide,
      horizontal: horizontal?.guide,
    },
  };
}
