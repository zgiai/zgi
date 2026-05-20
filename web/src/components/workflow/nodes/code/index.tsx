import React, { useMemo } from 'react';
import { AlertCircle } from 'lucide-react';
import type { CodeNodeData } from './config';
import { checkValid } from './config';
import { useT } from '@/i18n';
import { useWorkflowStore } from '../../store';
import OutputVariablesView from '../../common/output-variables-view';
import { useNodeOutputVariables } from '../../hooks';

interface CodeContentProps {
  nodeId: string;
  data: CodeNodeData;
}

const CodeContent: React.FC<CodeContentProps> = ({ nodeId, data }) => {
  const t = useT('nodes');
  const outputVariables = useNodeOutputVariables(nodeId);

  const upstreamIds = useMemo(() => {
    const ids = new Set<string>();
    for (const variable of data.variables || []) {
      if (Array.isArray(variable.value_selector) && variable.value_selector.length >= 2) {
        const sourceId = variable.value_selector[0];
        if (sourceId && !['sys', 'conversation', 'environment'].includes(sourceId)) {
          ids.add(sourceId);
        }
      }
    }
    return Array.from(ids);
  }, [data.variables]);

  const nodesExist = useWorkflowStore(
    React.useCallback(state => upstreamIds.every(id => state.nodeIdToTitle.has(id)), [upstreamIds])
  );

  const validation = useMemo(() => {
    return checkValid(data, {
      nodes: nodesExist ? (upstreamIds.map(id => ({ id })) as any) : [],
    });
  }, [data, nodesExist, upstreamIds]);
  const hasErrors = !validation.isValid;

  return (
    <>
      <OutputVariablesView
        variant="compact"
        title={t('common.outputVariables')}
        variables={outputVariables}
        maxItems={3}
      />

      {hasErrors && validation.errors[0] ? (
        <div className="mt-2 space-y-1">
          <div className="text-xs text-red-600 flex items-center gap-1">
            <AlertCircle className="w-3 h-3" />
            {t(`${validation.errors[0].code}` as any, validation.errors[0].params as any)}
          </div>
        </div>
      ) : null}
    </>
  );
};

export default CodeContent;
