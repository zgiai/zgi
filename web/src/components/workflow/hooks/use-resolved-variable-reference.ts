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
  const getAncestors = useWorkflowStore.use.getAncestors();
  const nodeIdToTitle = useWorkflowStore.use.nodeIdToTitle();
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

    let sourceTitle = matched?.sourceTitle ?? sourceId;
    if (!matched) {
      if (sourceId === 'sys') {
        sourceTitle = t('agents.workflow.systemVariables.title');
      } else if (sourceId === 'environment') {
        sourceTitle = t('agents.workflow.environmentVariables.title');
      } else if (sourceId === 'conversation') {
        sourceTitle = t('agents.workflow.conversationVariables.title');
      } else if (mode === 'history' && selectedRunId) {
        const snapshot = historySnapshots[selectedRunId];
        const node = snapshot?.nodes.find(item => item.id === sourceId);
        sourceTitle = node?.data?.title || nodeIdToTitle.get(sourceId) || sourceId;
      } else {
        sourceTitle = nodeIdToTitle.get(sourceId) || sourceId;
      }
    }

    const invalid = currentNodeId
      ? !isSpecialSource &&
        (!matched || !getAncestors(currentNodeId).includes(sourceId))
      : false;

    return resolveVariableReference({
      selector: normalizedSelector,
      sourceTitle,
      invalid,
      type: matched?.type,
    });
  }, [
    currentNodeId,
    getAncestors,
    historySnapshots,
    mode,
    nodeIdToTitle,
    normalizedSelector,
    selectedRunId,
    selectionIndex,
    t,
  ]);
}
