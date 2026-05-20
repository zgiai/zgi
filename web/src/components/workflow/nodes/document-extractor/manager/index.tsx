'use client';

import React, { useCallback } from 'react';
import type { DocumentExtractorNodeData } from '../config';
import NodeValueSelector from '../../../common/node-value-selector';
import type { WorkflowVariable } from '../../../store/type';
import OutputVariablesView from '../../../common/output-variables-view';
import { useMemo } from 'react';
import { useT } from '@/i18n';

import { useNodeData, useNodeDataUpdate, useNodeOutputVariables } from '../../../hooks';

interface DocumentExtractorManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

const DocumentExtractorManager: React.FC<DocumentExtractorManagerProps> = ({
  id,
  className,
  readOnly = false,
}) => {
  const t = useT();
  const nodeData = useNodeData<DocumentExtractorNodeData>(id);
  const updateNodeData = useNodeDataUpdate<DocumentExtractorNodeData>(id);
  // Use fine-grained selector to only get current node instead of entire nodes array
  // This prevents re-render when other nodes change (e.g., during hover)
  const outputs = useNodeOutputVariables(id);

  const handleVariableChange = useCallback(
    (payload: {
      sourceId: string;
      key: string;
      valuePath: string[];
      type: WorkflowVariable['type'];
    }) => {
      const isArrayFile = payload.type === 'array[file]';
      updateNodeData({
        variable_selector: payload.valuePath,
        is_array_file: isArrayFile,
      });
    },
    [updateNodeData]
  );

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-sm font-medium mb-2">{t('nodes.documentExtractor.fileVariable')}</h3>
        <NodeValueSelector
          nodeId={id}
          value={
            nodeData?.variable_selector &&
            Array.isArray(nodeData.variable_selector) &&
            nodeData.variable_selector.length >= 2
              ? nodeData.variable_selector
              : undefined
          }
          onChange={handleVariableChange}
          typeFilter={type => type === 'file' || type === 'array[file]'}
          placeholder={t('nodes.documentExtractor.placeholders.selectFileVar')}
          disabled={readOnly}
        />
      </div>
      <OutputVariablesView variables={outputs} />
    </div>
  );
};

export default DocumentExtractorManager;
