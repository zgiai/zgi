import React from 'react';
import type { ImageGenNodeData } from './config';
import { ModelIcon } from '@lobehub/icons';

export interface ImageGenContentProps {
  nodeId: string;
  data: ImageGenNodeData;
}

const ImageGenContent: React.FC<ImageGenContentProps> = ({ data }) => {
  const modelName = data?.model?.name;

  return (
    <div className="space-y-2">
      {modelName ? (
        <div className="flex items-center gap-2 border rounded-md px-2 h-8">
          <ModelIcon size={20} model={modelName} />
          <p
            className="text-sm text-secondary-foreground line-clamp-1 w-0 grow truncate"
            title={modelName}
          >
            {modelName}
          </p>
        </div>
      ) : null}
    </div>
  );
};

export default ImageGenContent;
