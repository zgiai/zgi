'use client';

import { useCallback } from 'react';
import { useWorkflowStore } from '../store';
import type { WorkflowNodeData } from '../store/type';

/**
 * Hook to provide a referentially stable function for updating a specific node's data.
 * Ideal for passing to memoized child components to prevent unnecessary re-renders.
 */
export function useNodeDataUpdate<T extends WorkflowNodeData>(nodeId: string) {
  const updateNodeData = useWorkflowStore.use.updateNodeData();

  const updateData = useCallback(
    (patch: Partial<T> | ((prev: T) => Partial<T>)) => {
      // Pull latest from store state to avoid stale closures
      const storeState = useWorkflowStore.getState();
      const node = storeState.nodes.find(n => n.id === nodeId);
      if (!node) return;

      const currentData = node.data as T;
      const nextPatch = typeof patch === 'function' ? patch(currentData) : patch;

      updateNodeData(nodeId, nextPatch);
    },
    [nodeId, updateNodeData]
  );

  return updateData;
}

export default useNodeDataUpdate;
