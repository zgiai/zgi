'use client';

import { useCallback } from 'react';
import { useWorkflowStore } from '../store';

/**
 * Hook to select a specific node's data from the workflow store.
 * Only triggers re-render when this specific node's data changes.
 *
 * @param nodeId - The node ID to select data for
 * @returns The node data cast to type T, or undefined if node not found
 */
export function useNodeData<T = unknown>(nodeId: string): T | undefined {
  return useWorkflowStore(
    useCallback(
      state => {
        if (!nodeId) return undefined;
        // In history mode, find node in the active snapshot instead of current nodes
        if (state.mode === 'history' && state.selectedRunId) {
          const snap = state.historySnapshots[state.selectedRunId];
          if (snap) {
            const snapNode = snap.nodes.find(n => n.id === nodeId);
            return (snapNode?.data as T) ?? undefined;
          }
        }
        const node = state.nodes.find(n => n.id === nodeId);
        return node?.data as T | undefined;
      },
      [nodeId]
    )
  );
}

export default useNodeData;
