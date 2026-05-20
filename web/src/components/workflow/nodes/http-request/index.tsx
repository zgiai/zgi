import React from 'react';
import type { WorkflowNodeData } from '../../store';
import CustomHandle from '../../ui/custom-handle';
import { Position } from '@xyflow/react';
import { useT } from '@/i18n';
import MethodBadge from './method-badge';

type HttpRequestData = Extract<WorkflowNodeData, { type: 'http-request' }>;

interface HttpRequestContentProps {
  nodeId: string;
  data: HttpRequestData;
}

const HttpRequestContent: React.FC<HttpRequestContentProps> = ({ nodeId: _nodeId, data }) => {
  const t = useT();
  const method = data.method || 'GET';
  const url = data.url || '--';

  return (
    <div className="space-y-2">
      <MethodBadge method={method} />
      {url ? (
        <div
          className="truncate text-xs bg-background rounded-sm border p-1 text-wrap break-all min-h-[26px]"
          title={url}
        >
          {url}
        </div>
      ) : null}

      {data.error_strategy === 'fail-branch' ? (
        <div className="pt-1 border-t">
          <div className="relative h-[17px]">
            <div className="text-xs text-destructive font-medium text-right">
              {t('nodes.httpRequest.fields.failBranchLabel')}
            </div>
            <CustomHandle
              type="source"
              position={Position.Right}
              id="fail-branch"
              className="!bg-destructive"
              variant="destructive"
              style={{ top: '50%', right: -14 }}
            />
          </div>
        </div>
      ) : null}
    </div>
  );
};

export default HttpRequestContent;
