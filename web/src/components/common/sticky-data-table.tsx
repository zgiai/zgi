'use client';

import type { ReactNode } from 'react';

import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';

export interface StickyDataTableColumn {
  key: string;
  header: ReactNode;
  className?: string;
  align?: 'left' | 'center' | 'right';
}

interface StickyDataTableProps<TData> {
  columns: StickyDataTableColumn[];
  data: TData[];
  getRowKey: (item: TData, index: number) => string;
  renderRow?: (item: TData, index: number) => ReactNode;
  isLoading?: boolean;
  loadingRows?: number;
  renderSkeletonRow?: (index: number) => ReactNode;
  emptyState?: ReactNode;
  pagination?: ReactNode;
  children?: ReactNode;
  className?: string;
  scrollClassName?: string;
  tableClassName?: string;
  headerClassName?: string;
  bodyClassName?: string;
}

/**
 * @component StickyDataTable
 * @category Common
 * @status Stable
 * @description Shared table shell with sticky headers, scroll container, loading, empty state, and pagination slots
 * @usage Use when a feature needs custom row rendering with consistent table chrome
 * @example
 * <StickyDataTable columns={columns} data={items} getRowKey={item => item.id} renderRow={renderRow} />
 */
export function StickyDataTable<TData>({
  columns,
  data,
  getRowKey,
  renderRow,
  isLoading = false,
  loadingRows = 6,
  renderSkeletonRow,
  emptyState,
  pagination,
  children,
  className,
  scrollClassName,
  tableClassName,
  headerClassName,
  bodyClassName,
}: StickyDataTableProps<TData>) {
  const hasRows = data.length > 0;
  const shouldUseChildren = children !== undefined && children !== null && children !== false;

  return (
    <div className={cn('flex-1 overflow-auto flex flex-col min-h-0', className)}>
      {isLoading || hasRows ? (
        <div
          className={cn(
            'overflow-x-auto h-full scrollbar-thin scrollbar-thumb-border/50',
            scrollClassName
          )}
        >
          <table className={cn('w-full text-left text-xs', tableClassName)}>
            <thead
              className={cn(
                'bg-bg-canvas/50 backdrop-blur-sm sticky top-0 z-10 border-b border-border/20',
                headerClassName
              )}
            >
              <tr>
                {columns.map(column => (
                  <th
                    key={column.key}
                    className={cn(
                      'font-semibold text-text-secondary h-11 text-[12px] uppercase tracking-wider',
                      column.align === 'center' && 'text-center',
                      column.align === 'right' && 'text-right',
                      column.className
                    )}
                  >
                    {column.header}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className={cn('bg-background', bodyClassName)}>
              {shouldUseChildren
                ? children
                : isLoading
                  ? Array.from({ length: loadingRows }).map((_, index) =>
                      renderSkeletonRow ? (
                        renderSkeletonRow(index)
                      ) : (
                        <tr
                          key={`sticky-data-table-skeleton-${index}`}
                          className="border-b border-border/10"
                        >
                          <td colSpan={columns.length} className="px-6 py-4">
                            <Skeleton className="h-10 w-full rounded-xl opacity-60" />
                          </td>
                        </tr>
                      )
                    )
                  : renderRow
                    ? data.map((item, index) => (
                        <tr
                          key={getRowKey(item, index)}
                          className="group border-b border-border/10 hover:bg-bg-canvas/40 transition-colors interactive-subtle"
                        >
                          {renderRow(item, index)}
                        </tr>
                      ))
                    : null}
            </tbody>
          </table>
        </div>
      ) : (
        emptyState
      )}

      {pagination}
    </div>
  );
}
