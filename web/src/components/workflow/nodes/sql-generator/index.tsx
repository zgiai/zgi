import React from 'react';
import type { WorkflowNodeData } from '../../store';
import { cn } from '@/lib/utils';
import { ModelIcon } from 'modelicons';

type SqlGeneratorData = Extract<WorkflowNodeData, { type: 'sql-generator' }>;
type SqlGeneratorInnerData = SqlGeneratorData['data'];

interface SqlGeneratorContentProps {
  nodeId: string;
  data: SqlGeneratorData;
}

const DbTypeBadge: React.FC<{ type: SqlGeneratorInnerData['data_source']['source']['type'] }> = ({
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

const SqlGeneratorContent: React.FC<SqlGeneratorContentProps> = ({ nodeId: _nodeId, data }) => {
  const payload = data.data;
  const dataSource = payload?.data_source?.source;
  const dsName = dataSource?.name || dataSource?.id || '--';
  const dbType = dataSource?.type || 'postgres';
  const model = payload?.model;

  return (
    <div className="space-y-2">
      {model?.name ? (
        <div className="flex items-center gap-2 border rounded-md px-2 h-8">
          <ModelIcon size={20} model={model.name} />
          <p
            className="text-sm text-secondary-foreground line-clamp-1 w-0 grow truncate"
            title={model.name}
          >
            {model.name}
          </p>
        </div>
      ) : null}
      <div className="flex items-center gap-2">
        <DbTypeBadge type={dbType} />
        <div className="text-xs text-secondary-foreground truncate" title={dsName}>
          {dsName}
        </div>
      </div>
    </div>
  );
};

export default SqlGeneratorContent;
