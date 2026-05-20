// Node helpers: id generation and constraints, strictly typed.

import type { WorkflowNode } from '../type';

/**
 * Generate a unique node id.
 * Note: Current implementation uses timestamp for simplicity; can be replaced with nanoid in future.
 */
let __lastTs = 0;
let __seq = 0;
export function generateNodeId(): string {
  const now = Date.now();
  if (now === __lastTs) {
    __seq += 1;
  } else {
    __lastTs = now;
    __seq = 0;
  }
  const randomSuffix = Math.random().toString(36).substring(2, 7);
  const baseId = __seq > 0 ? `${now}_${__seq}` : `${now}`;
  return `${baseId}_${randomSuffix}`;
}

/**
 * Whether a node of given type can be added, respecting uniqueness constraints.
 * Unique types: 'start'
 */
export function canAddNode(nodes: WorkflowNode[], nodeType: string): boolean {
  const uniqueTypes = ['start'];
  if (uniqueTypes.includes(nodeType)) {
    const exists = nodes.some(n => (n.data as { type?: string })?.type === nodeType);
    return !exists;
  }
  return true;
}

/**
 * Get list of unique node types present in graph.
 */
export function getUniqueNodeTypes(nodes: WorkflowNode[]): string[] {
  const types = new Set<string>();
  nodes.forEach(n => {
    const t = (n.data as { type?: string })?.type;
    if (t) types.add(t);
  });
  return Array.from(types);
}

export function sanitizeNodes(nodes: WorkflowNode[]): WorkflowNode[] {
  return nodes.map(n => {
    const pos = (n as Partial<WorkflowNode>).position as { x?: unknown; y?: unknown } | undefined;
    const hasValidPosition =
      !!pos &&
      typeof pos.x === 'number' &&
      typeof pos.y === 'number' &&
      Number.isFinite(pos.x) &&
      Number.isFinite(pos.y);

    return {
      ...n,
      type: (() => {
        const dataType = (n.data as { type?: string } | undefined)?.type;
        return dataType === 'approval' || dataType === 'question-answer'
          ? dataType
          : n.type || 'custom';
      })(),
      position: hasValidPosition ? (pos as { x: number; y: number }) : { x: 0, y: 0 },
    } as WorkflowNode;
  });
}
