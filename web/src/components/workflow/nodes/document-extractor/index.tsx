import React from 'react';
import type { DocumentExtractorNodeData } from './config';
import ValueBadge from '../../ui/value-badge';

export interface DocumentExtractorContentProps {
  nodeId: string;
  data: DocumentExtractorNodeData;
}

const DocumentExtractorContent: React.FC<DocumentExtractorContentProps> = ({ nodeId, data }) => {
  const hasVariable = Array.isArray(data.variable_selector) && data.variable_selector.length === 2;

  return (
    <div className="space-y-2">
      {hasVariable ? (
        <div className="rounded-md bg-muted p-1 flex items-center">
          <ValueBadge selector={data.variable_selector as [string, string]} currentNodeId={nodeId} />
        </div>
      ) : null}
    </div>
  );
};

export default DocumentExtractorContent;
