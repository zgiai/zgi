'use client';

import { useMemo } from 'react';
import { useT } from '@/i18n';
import { useWorkflowStore } from '../store';
import {
  buildVariableSelectionKey,
  isSpecialVariableSource,
  normalizeVariableSelector,
  parseTemplateToSelector,
  resolveVariableReference,
  type ResolvedVariableReference,
} from '../common/variable-reference';
import { useWorkflowVariableCatalog } from './use-workflow-variable-catalog';
import { resolveWorkflowVariableReferenceHealth } from '../common/variable-reference-health';

interface UseResolvedVariableReferenceOptions {
  selector?: string[];
  template?: string;
  currentNodeId?: string;
}

export function useResolvedVariableReference({
  selector,
  template,
  currentNodeId,
}: UseResolvedVariableReferenceOptions): ResolvedVariableReference | null {
  const t = useT();
  const normalizedSelector = useMemo(
    () => normalizeVariableSelector(selector ?? parseTemplateToSelector(template)),
    [selector, template]
  );

  const { selectionIndex } = useWorkflowVariableCatalog({
    nodeId: currentNodeId,
  });
  const nodeIdToTitle = useWorkflowStore.use.nodeIdToTitle();
  const nodes = useWorkflowStore.use.nodes();
  const edges = useWorkflowStore.use.edges();
  const agentType = useWorkflowStore.use.agentType();
  const mode = useWorkflowStore.use.mode();
  const selectedRunId = useWorkflowStore.use.selectedRunId();
  const historySnapshots = useWorkflowStore.use.historySnapshots();

  return useMemo(() => {
    if (!normalizedSelector) return null;

    const [sourceId] = normalizedSelector;
    const matched = selectionIndex.get(
      buildVariableSelectionKey(normalizedSelector) ?? normalizedSelector.join('::')
    );
    const isSpecialSource = isSpecialVariableSource(sourceId);

    const snapshot = mode === 'history' && selectedRunId ? historySnapshots[selectedRunId] : null;
    const effectiveNodes = snapshot?.nodes ?? nodes;
    const effectiveEdges = snapshot?.edges ?? edges;
    const health = currentNodeId
      ? resolveWorkflowVariableReferenceHealth({
          nodes: effectiveNodes,
          edges: effectiveEdges,
          consumerNodeId: currentNodeId,
          selector: normalizedSelector,
          agentType,
        })
      : null;

    let sourceTitle = matched?.sourceTitle ?? '';
    if (!matched) {
      if (sourceId === 'sys') {
        sourceTitle = t('agents.workflow.systemVariables.title');
      } else if (sourceId === 'environment') {
        sourceTitle = t('agents.workflow.environmentVariables.title');
      } else if (sourceId === 'conversation') {
        sourceTitle = t('agents.workflow.conversationVariables.title');
      } else if (mode === 'history' && selectedRunId) {
        const node = snapshot?.nodes.find(item => item.id === sourceId);
        sourceTitle =
          node?.data?.title ||
          nodeIdToTitle.get(sourceId) ||
          t('nodes.validation.deletedVariableSource');
      } else {
        sourceTitle =
          health?.sourceNode?.data?.title ||
          nodeIdToTitle.get(sourceId) ||
          t('nodes.validation.deletedVariableSource');
      }
    }

    const status = isSpecialSource ? 'active' : (health?.status ?? 'active');
    const invalid = status !== 'active';

    return resolveVariableReference({
      selector: normalizedSelector,
      sourceTitle,
      invalid,
      status,
      type: matched?.type ?? health?.type,
    });
  }, [
    currentNodeId,
    agentType,
    edges,
    historySnapshots,
    mode,
    nodeIdToTitle,
    nodes,
    normalizedSelector,
    selectedRunId,
    selectionIndex,
    t,
  ]);
}
