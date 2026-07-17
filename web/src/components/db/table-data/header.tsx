'use client';

// Table header row with optional tooltips for column descriptions.
// English comments only as per project guidelines.

import type { FC } from 'react';
import React from 'react';
import { TableHeader, TableHead, TableRow } from '@/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import type { DbTableColumn } from '@/services/types/db';
import { cn } from '@/lib/utils';

export interface TableDataHeaderProps {
  columns: readonly DbTableColumn[];
  isEditing: boolean;
  actionsTitle: string;
  showRowActions?: boolean;
  stickyColumnNames?: readonly string[];
}

function getColumnDisplayName(col: DbTableColumn): string {
  return col.source_column_name?.trim() || col.name;
}

const Header: FC<TableDataHeaderProps> = ({
  columns,
  isEditing,
  actionsTitle,
  showRowActions,
  stickyColumnNames = [],
}) => {
  return (
    <TableHeader>
      <TableRow className="text-xs">
        {columns.map(col => {
          const showRequiredStar = isEditing && !col.is_system_field && !!col.is_required;
          const displayName = getColumnDisplayName(col);
          const sticky = stickyColumnNames.includes(col.name);
          const headClassName = cn(
            'h-8 min-w-[140px] max-w-[280px] border-r bg-muted/50 last:border-r-0',
            sticky && 'sticky left-0 z-20 min-w-[140px] shadow-[1px_0_0_hsl(var(--border))]'
          );
          const label = (
            <span className="flex min-w-0 items-center gap-0.5">
              <span className="truncate">{displayName}</span>
              {showRequiredStar && (
                <span className="shrink-0 text-destructive" aria-hidden="true" title="Required">
                  *
                </span>
              )}
            </span>
          );
          return (
            <Tooltip key={`head-${col.id}`}>
              <TooltipTrigger asChild>
                <TableHead className={cn(headClassName, 'cursor-help')}>{label}</TableHead>
              </TooltipTrigger>
              <TooltipContent side="top" className="max-w-[320px] break-words">
                <div className="space-y-1 text-xs">
                  <div>{displayName}</div>
                  {col.name !== displayName && <div>{col.name}</div>}
                  {col.description && <div>{col.description}</div>}
                </div>
              </TooltipContent>
            </Tooltip>
          );
        })}
        {(isEditing || showRowActions) && (
          <TableHead
            className="sticky right-0 z-20 h-8 max-w-[400px] border-r bg-muted/50 shadow-[-1px_0_0_hsl(var(--border))]"
            title={actionsTitle}
          >
            {actionsTitle}
          </TableHead>
        )}
      </TableRow>
    </TableHeader>
  );
};

export const TableDataHeader = React.memo(Header);

export default TableDataHeader;
