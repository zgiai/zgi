import { isContainerNode } from '../type';

const CONTAINER_BLOCKED_NODE_TYPES = new Set([
  'start',
  'end',
  'approval',
  'question-answer',
]);

export function canPlaceNodeInContainer(nodeType?: string, containerType?: string): boolean {
  if (!nodeType || !isContainerNode(containerType)) return false;
  if (isContainerNode(nodeType)) return false;
  return !CONTAINER_BLOCKED_NODE_TYPES.has(nodeType);
}
