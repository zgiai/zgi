'use client';

import { useCallback, useMemo } from 'react';
import { useT } from '@/i18n';
import { useWorkflowStore } from '../store';
import { collectForNode } from '../store/helpers/graph';
import type { WorkflowNode, WorkflowVariable } from '../store/type';

interface UseNodeOutputVariablesOptions {
  excludeSystem?: boolean;
}

export function useNodeOutputVariables(
  nodeId: string,
  { excludeSystem = false }: UseNodeOutputVariablesOptions = {}
) {
  const t = useT('nodes');
  const agentType = useWorkflowStore.use.agentType();
  const node = useWorkflowStore(
    useCallback(
      state => {
        if (state.mode === 'history' && state.selectedRunId) {
          const snapshot = state.historySnapshots[state.selectedRunId];
          if (snapshot) {
            return snapshot.nodes.find(item => item.id === nodeId) as WorkflowNode | undefined;
          }
        }

        return state.nodes.find(item => item.id === nodeId) as WorkflowNode | undefined;
      },
      [nodeId]
    )
  );

  return useMemo<Array<Pick<WorkflowVariable, 'name' | 'type' | 'description'>>>(() => {
    if (!node) return [];

    return (collectForNode(node, agentType).variables || [])
      .filter(variable => (excludeSystem ? !String(variable.key).startsWith('sys.') : true))
      .map(variable => ({
        name: variable.key,
        type: variable.type,
        description: variable.descriptionKey
          ? t(variable.descriptionKey as never, { innerType: variable.description || 'string' })
          : (variable.description ?? ''),
      }));
  }, [agentType, excludeSystem, node, t]);
}
