import React from 'react';
import type { ParameterExtractorNodeData } from './config';
import { ModelIcon } from '@lobehub/icons';
import OutputVariablesView from '../../common/output-variables-view';
import { useNodeOutputVariables } from '../../hooks';

interface ParameterExtractorContentProps {
  nodeId: string;
  data: ParameterExtractorNodeData;
}

const ParameterExtractorContent: React.FC<ParameterExtractorContentProps> = ({ nodeId, data }) => {
  const modelName = data?.model?.name || '';
  const outputVariables = useNodeOutputVariables(nodeId);

  return (
    <div className="space-y-2">
      {modelName ? (
        <div className="flex items-center gap-2 border rounded-md px-2 h-8">
          <ModelIcon size={20} model={modelName} />
          <p className="text-sm text-secondary-foreground truncate">{modelName}</p>
        </div>
      ) : null}
      <OutputVariablesView variant="compact" variables={outputVariables} maxItems={2} />
    </div>
  );
};

export default ParameterExtractorContent;
