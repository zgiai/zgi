import type { WorkflowNode } from '../type';

function identity(node: WorkflowNode): string {
  const data = node.data as Record<string, unknown>;
  return JSON.stringify([
    data.provider_type,
    data.provider_id,
    data.tool_name,
    data.connector,
    data.connector_unavailable_ref,
  ]);
}

/**
 * Apply server-assigned references only while the submitted selection is unchanged.
 * Returns `current` itself when no node needed a server reference, so callers can
 * keep reference equality and avoid treating an unchanged graph as edited.
 */
export function mergeSavedConnectorNodes(
  current: WorkflowNode[],
  submitted: WorkflowNode[],
  canonical: WorkflowNode[]
): WorkflowNode[] {
  const before = new Map(submitted.map(node => [node.id, node]));
  const saved = new Map(canonical.map(node => [node.id, node]));
  let changed = false;
  const merged = current.map(node => {
    const source = before.get(node.id);
    const next = saved.get(node.id);
    if (
      !source ||
      !next ||
      source.data.provider_type !== 'connector' ||
      identity(node) !== identity(source)
    ) {
      return node;
    }
    const data = next.data as Record<string, unknown>;
    if (identity(source) === identity(next)) return node;
    changed = true;
    return {
      ...node,
      data: {
        ...node.data,
        provider_type: data.provider_type,
        provider_id: data.provider_id,
        tool_name: data.tool_name,
        connector: data.connector,
        connector_unavailable_ref: data.connector_unavailable_ref,
      },
    } as WorkflowNode;
  });
  return changed ? merged : current;
}
