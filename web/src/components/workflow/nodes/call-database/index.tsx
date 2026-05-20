import React from 'react';
import type { WorkflowNodeData } from '../../store';
import { cn } from '@/lib/utils';

type CallDatabaseData = Extract<WorkflowNodeData, { type: 'call-database' }>;
type CallDatabaseInnerData = CallDatabaseData['data'];

interface CallDatabaseContentProps {
  nodeId: string;
  data: CallDatabaseData;
}

const DbTypeBadge: React.FC<{ type: CallDatabaseInnerData['data_source']['type'] }> = ({
  type,
}) => {
  const color =
    type === 'postgres'
      ? 'bg-indigo-50 text-indigo-700 border-indigo-200'
      : type === 'mysql'
        ? 'bg-blue-50 text-blue-700 border-blue-200'
        : type === 'sqlite'
          ? 'bg-gray-50 text-gray-700 border-gray-200'
          : type === 'mssql'
            ? 'bg-violet-50 text-violet-700 border-violet-200'
            : 'bg-rose-50 text-rose-700 border-rose-200';
  return (
    <div className={cn('items-center flex h-5')}>
      <div className={cn('rounded border px-2 text-sm font-medium', color)}>{type}</div>
    </div>
  );
};

const CallDatabaseContent: React.FC<CallDatabaseContentProps> = ({ data }) => {
  const payload = data.data;
  const dataSource = payload?.data_source;
  const dsName = dataSource?.name || dataSource?.id || '--';
  const dbType = dataSource?.type || 'postgres';

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <DbTypeBadge type={dbType} />
        <div className="text-xs text-secondary-foreground truncate" title={dsName}>
          {dsName}
        </div>
      </div>
    </div>
  );
};

export default CallDatabaseContent;
