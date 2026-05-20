import React from 'react';
import type { LLMNodeData } from './config';
import { ModelIcon } from '@lobehub/icons';

export interface LLMContentProps {
  nodeId: string;
  data: LLMNodeData;
}

const LLMContent: React.FC<LLMContentProps> = ({ data }) => {
  return (
    <div className="space-y-2">
      {data?.model?.name ? (
        <div className="flex items-center gap-2 border rounded-md px-2 h-8">
          <ModelIcon size={20} model={data.model.name} />
          <p
            className="text-sm text-secondary-foreground line-clamp-1 w-0 grow truncate"
            title={data.model.name}
          >
            {data.model.name}
          </p>
        </div>
      ) : null}
    </div>
  );
};

export default LLMContent;
