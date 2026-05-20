import { useWorkflowStore } from '../../../store';
import { isContainerNode } from '../../../store/type';

export function resolveContainerContextId(args: {
  originatingHandle?: { nodeId: string } | null;
  originatingEdge?: { targetId: string } | null;
}): string | undefined {
  // From handle
  if (args.originatingHandle) {
    const node = useWorkflowStore
      .getState()
      .nodes.find(n => n.id === args.originatingHandle!.nodeId);
    if (!node) return undefined;

    // Use parentId if node is already inside a container
    // Note: Do NOT return container ID when the originating node is the container itself.
    // Clicking on a container's external handles (source/target) should create external nodes,
    // not child nodes inside the container.
    const parentId = (node as { parentId?: string } | undefined)?.parentId;
    if (parentId) return parentId;
  }
  // From edge insertion
  if (args.originatingEdge) {
    const target = useWorkflowStore
      .getState()
      .nodes.find(n => n.id === args.originatingEdge!.targetId);
    const parentId = (target as { parentId?: string } | undefined)?.parentId;
    if (parentId) return parentId;
  }
  return undefined;
}

export function resolveAnchorIdForAppend(args: {
  originatingHandle?: { nodeId: string } | null;
  iterationParentId: string;
  createdId: string;
}): string {
  if (args.originatingHandle?.nodeId) return args.originatingHandle.nodeId;
  const parent = useWorkflowStore.getState().nodes.find(n => n.id === args.iterationParentId);
  const sid = (parent?.data as { start_node_id?: string })?.start_node_id;
  return sid || args.createdId;
}
