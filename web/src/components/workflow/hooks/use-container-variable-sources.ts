'use client';

import { useMemo } from 'react';
import { useShallow } from 'zustand/react/shallow';
import { useWorkflowStore } from '../store';
import type { WorkflowStore } from '../store';
import {
  collectForNode,
  collectScopedVariablesForNode,
  type UpstreamExportItem,
} from '../store/helpers/graph';
import type { WorkflowEdge, WorkflowNode, WorkflowNodeData } from '../store/type';

interface UseContainerVariableSourcesOptions {
  includeUpstream?: boolean;
  includeScopedSelf?: boolean;
  includeChildOutputs?: boolean;
  excludeChildNodeTypes?: WorkflowNodeData['type'][];
}

interface WorkflowGraphSnapshot {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  selfNode?: WorkflowNode;
  childNodes: WorkflowNode[];
}

export function useContainerVariableSources(
  nodeId: string,
  {
    includeUpstream = false,
    includeScopedSelf = false,
    includeChildOutputs = false,
    excludeChildNodeTypes = [],
  }: UseContainerVariableSourcesOptions = {}
): UpstreamExportItem[] {
  const agentType = useWorkflowStore.use.agentType();
  const getUpstreamVariables = useWorkflowStore.use.getUpstreamVariables();
  const { mode, selectedRunId, nodes, edges, historySnapshot } = useWorkflowStore(
    useShallow((state: WorkflowStore) => ({
      mode: state.mode,
      selectedRunId: state.selectedRunId,
      nodes: state.nodes,
      edges: state.edges,
      historySnapshot: state.selectedRunId ? state.historySnapshots[state.selectedRunId] : undefined,
    }))
  );
  const graphSnapshot = useMemo<WorkflowGraphSnapshot>(() => {
    const snapshot = mode === 'history' && selectedRunId ? historySnapshot : null;
    const currentNodes = (snapshot?.nodes ?? nodes) as WorkflowNode[];
    const currentEdges = (snapshot?.edges ?? edges) as WorkflowEdge[];

    return {
      nodes: currentNodes,
      edges: currentEdges,
      selfNode: currentNodes.find(node => node.id === nodeId),
      childNodes: currentNodes.filter(node => node.parentId === nodeId),
    };
  }, [edges, historySnapshot, mode, nodeId, nodes, selectedRunId]);

  return useMemo(() => {
    const groups: UpstreamExportItem[] = [];

    if (includeUpstream) {
      groups.push(...(getUpstreamVariables(nodeId) || []));
    }

    if (includeScopedSelf && graphSnapshot.selfNode) {
      const scopedGroup = collectScopedVariablesForNode(graphSnapshot.selfNode, agentType, {
        nodes: graphSnapshot.nodes,
        edges: graphSnapshot.edges,
      });
      if ((scopedGroup.variables?.length || 0) > 0) {
        groups.push(scopedGroup);
      }
    }

    if (includeChildOutputs) {
      const childGroups = graphSnapshot.childNodes
        .filter(child => !excludeChildNodeTypes.includes(child.data.type))
        .map(child => collectForNode(child, agentType))
        .filter(group => (group.variables?.length || 0) > 0);
      groups.push(...childGroups);
    }

    return groups;
  }, [
    agentType,
    excludeChildNodeTypes,
    getUpstreamVariables,
    graphSnapshot,
    includeChildOutputs,
    includeScopedSelf,
    includeUpstream,
    nodeId,
  ]);
}
