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
  return col.name;
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
            'border-r last:border-r-0 h-8 bg-muted/50',
            sticky && 'sticky left-0 z-20 min-w-[140px] shadow-[1px_0_0_hsl(var(--border))]'
          );
          const label = (
            <span className="inline-flex items-center gap-0.5">
              {displayName}
              {showRequiredStar && (
                <span className="text-destructive" aria-hidden="true" title="Required">
                  *
                </span>
              )}
            </span>
          );
          return col.description ? (
            <Tooltip key={`head-${col.id}`}>
              <TooltipTrigger asChild>
                <TableHead className={cn(headClassName, 'cursor-help')}>{label}</TableHead>
              </TooltipTrigger>
              <TooltipContent side="top" className="max-w-[320px] break-words">
                <div className="space-y-1 text-xs">
                  {col.display_name?.trim() && col.display_name.trim() !== col.name && (
                    <div>{col.display_name.trim()}</div>
                  )}
                  {col.source_column_name && <div>{col.source_column_name}</div>}
                  <div>{col.description}</div>
                </div>
              </TooltipContent>
            </Tooltip>
          ) : (
            <TableHead key={`head-${col.id}`} className={headClassName}>
              {label}
            </TableHead>
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
