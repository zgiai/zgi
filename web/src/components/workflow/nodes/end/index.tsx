import React from 'react';
import { useT } from '@/i18n';
import type { EndNodeData } from './config';
import OutputVariablesView from '../../common/output-variables-view';

export interface EndContentProps {
  nodeId: string;
  data: EndNodeData;
}

const EndContent: React.FC<EndContentProps> = ({ nodeId: _nodeId, data }) => {
  const t = useT('nodes');
  if (!data.outputs || data.outputs.length === 0) return null;
  return (
    <OutputVariablesView
      variant="compact"
      title={t('common.outputVariables')}
      variables={data.outputs.map((output, index) => ({
        name: output.variable || t('end.unnamed'),
        type: output.type,
        description: index === 0 ? t('end.outputsCount', { count: data.outputs.length }) : '',
      }))}
      maxItems={3}
    />
  );
};

export default EndContent;
