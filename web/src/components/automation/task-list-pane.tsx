'use client';

import * as React from 'react';
import { Clock3, List, Sparkles } from 'lucide-react';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Pagination } from '@/components/ui/pagination';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import type { AutomationTaskListItem } from '@/services/types/automation';
import { TASK_STATUS_FILTERS } from './registry';
import { getScheduleSummary, getTaskNextRunLabel, getTaskStatusBadgeVariant } from './utils';
import type { TaskStatusFilterKey } from './types';

interface TaskListPaneProps {
  items: AutomationTaskListItem[];
  total: number;
  page: number;
  pageSize: number;
  isLoading: boolean;
  isFetching: boolean;
  counts?: Partial<Record<TaskStatusFilterKey, number>>;
  selectedTaskId: string | null;
  filterKey: TaskStatusFilterKey;
  panelOpen: boolean;
  canManage: boolean;
  onOpenCreate: () => void;
  onFilterChange: (filterKey: TaskStatusFilterKey) => void;
  onPageChange: (page: number) => void;
  onSelectTask: (taskId: string) => void;
}

interface TaskListHeaderProps {
  filterKey: TaskStatusFilterKey;
  total: number;
  isFetching: boolean;
  counts?: Partial<Record<TaskStatusFilterKey, number>>;
  compactMode: boolean;
  compactLocked: boolean;
  canManage: boolean;
  onOpenCreate: () => void;
  onFilterChange: (filterKey: TaskStatusFilterKey) => void;
  onCompactModeChange: (compactMode: boolean) => void;
}

interface TaskListItemCardProps {
  item: AutomationTaskListItem;
  selected: boolean;
  compact: boolean;
  onSelect: () => void;
}

/**
 * @component TaskListPane
 * @category Feature
 * @status Stable
 * @description Workspace task list with filters, pagination, and row-level quick actions.
 * @usage Render as the primary list column inside the automation task workbench.
 */
export function TaskListPane({
  items,
  total,
  page,
  pageSize,
  isLoading,
  isFetching,
  counts,
  selectedTaskId,
  filterKey,
  panelOpen,
  canManage,
  onOpenCreate,
  onFilterChange,
  onPageChange,
  onSelectTask,
}: TaskListPaneProps) {
  const t = useT('automation');
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const [compactMode, setCompactMode] = React.useState(false);
  const compactCards = panelOpen || compactMode;

  const isEmpty = !isLoading && items.length === 0;

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      <TaskListHeader
        filterKey={filterKey}
        total={total}
        isFetching={isFetching}
        counts={counts}
        compactMode={compactMode}
        compactLocked={panelOpen}
        canManage={canManage}
        onOpenCreate={onOpenCreate}
        onFilterChange={onFilterChange}
        onCompactModeChange={setCompactMode}
      />

      <ScrollArea className="h-0 grow">
        <div className="mx-auto flex w-full max-w-[1160px] flex-col gap-4 px-4 pb-8 pt-5 md:px-6 md:pt-6">
          {isLoading ? (
            <div className="flex flex-col gap-3">
              {Array.from({ length: 6 }).map((_, index) => (
                <div
                  key={`task-card-skeleton-${index}`}
                  className="rounded-[24px] border border-border/70 bg-background px-4 py-4"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="space-y-2">
                      <Skeleton className="h-5 w-40" />
                      <Skeleton className="h-4 w-28" />
                    </div>
                    <Skeleton className="h-6 w-16 rounded-full" />
                  </div>
                  <div className="mt-4 grid gap-3 lg:grid-cols-[minmax(0,1fr)_260px]">
                    <Skeleton className="h-6 rounded-lg" />
                    <Skeleton className="h-6 rounded-lg" />
                  </div>
                </div>
              ))}
            </div>
          ) : null}

          {!isLoading ? (
            <div className="flex flex-col gap-3">
              {items.map(item => (
                <TaskListItemCard
                  key={item.task.id}
                  item={item}
                  selected={item.task.id === selectedTaskId}
                  compact={compactCards}
                  onSelect={() => onSelectTask(item.task.id)}
                />
              ))}
            </div>
          ) : null}

          {isEmpty ? (
            <div className="rounded-[26px] border border-dashed border-border/80 bg-background/90">
              <div className="flex flex-col items-center justify-center gap-4 py-14 text-center">
                <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/10 text-primary">
                  <Sparkles className="size-6" />
                </div>
                <div className="space-y-2">
                  <h3 className="text-lg font-semibold text-foreground">
                    {filterKey === 'all' ? t('empty.title') : t('empty.filteredTitle')}
                  </h3>
                  <p className="max-w-lg text-sm leading-6 text-muted-foreground">
                    {filterKey === 'all' ? t('empty.description') : t('empty.filteredDescription')}
                  </p>
                </div>
                <Button
                  onClick={onOpenCreate}
                  disabled={!canManage}
                  title={!canManage ? t('noManagePermission') : undefined}
                >
                  {t('createTask')}
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      </ScrollArea>

      {!isLoading && totalPages > 1 ? (
        <div className="border-t border-border/70 bg-background px-4 py-3 md:px-6">
          <div className="mx-auto max-w-[1180px]">
            <Pagination
              currentPage={page}
              totalPages={totalPages}
              total={total}
              pageSize={pageSize}
              onPageChange={onPageChange}
              renderInfo={(start, end, totalItems) =>
                t('list.paginationInfo', {
                  start,
                  end,
                  total: totalItems,
                })
              }
            />
          </div>
        </div>
      ) : null}
    </div>
  );
}

export function TaskListHeader({
  filterKey,
  total,
  isFetching,
  counts,
  compactMode,
  compactLocked,
  canManage,
  onOpenCreate,
  onFilterChange,
  onCompactModeChange,
}: TaskListHeaderProps) {
  const t = useT('automation');
  const tCommon = useT('common');

  return (
    <div className="border-b border-border/70 bg-background">
      <div className="mx-auto flex w-full max-w-[1160px] flex-col gap-4 px-4 py-4 md:px-6 md:py-5">
        <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <h1 className="text-[26px] font-semibold tracking-tight text-foreground">
                {t('title')}
              </h1>
              {isFetching ? <Badge variant="info">{tCommon('statusLabels.loading')}</Badge> : null}
            </div>
            <div className="flex flex-wrap items-center gap-2.5 text-sm text-muted-foreground">
              <p className="max-w-2xl leading-6">{t('description')}</p>
              <span className="hidden h-1 w-1 rounded-full bg-border md:inline-block" />
              <span className="text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
                {t('taskCount', { count: total })}
              </span>
            </div>
          </div>

          <Button
            onClick={onOpenCreate}
            disabled={!canManage}
            title={!canManage ? t('noManagePermission') : undefined}
          >
            {t('createTask')}
          </Button>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-wrap items-center gap-2">
            {TASK_STATUS_FILTERS.map(filter => {
              const count = counts?.[filter.key];

              return (
                <button
                  key={filter.key}
                  type="button"
                  onClick={() => onFilterChange(filter.key)}
                  className={cn(
                    'rounded-xl border px-4 py-2 text-left transition-all',
                    filterKey === filter.key
                      ? 'border-primary bg-primary text-primary-foreground shadow-sm'
                      : 'border-border/70 bg-background hover:border-primary/25 hover:bg-muted/20'
                  )}
                >
                  <div className="flex items-center justify-between gap-3">
                    <span
                      className={cn(
                        'text-sm font-medium',
                        filterKey === filter.key ? 'text-primary-foreground' : 'text-foreground/90'
                      )}
                    >
                      {t(filter.labelKey as never)}
                    </span>
                    {typeof count === 'number' ? (
                      <span
                        className={cn(
                          'min-w-5 rounded-full px-1.5 py-0.5 text-center text-[11px] font-semibold leading-4',
                          filterKey === filter.key
                            ? 'bg-primary-foreground/15 text-primary-foreground'
                            : 'bg-muted text-muted-foreground'
                        )}
                      >
                        {count}
                      </span>
                    ) : (
                      <span
                        className={cn(
                          'size-2 rounded-full transition-colors',
                          filterKey === filter.key ? 'bg-primary-foreground' : 'bg-border'
                        )}
                      />
                    )}
                  </div>
                </button>
              );
            })}
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="gap-2"
            disabled={compactLocked}
            title={compactLocked ? t('list.compactAuto') : undefined}
            onClick={() => onCompactModeChange(!compactMode)}
          >
            <List className="size-4" />
            {compactLocked || !compactMode ? t('list.compactMode') : t('list.comfortableMode')}
          </Button>
        </div>
      </div>
    </div>
  );
}

export function TaskListItemCard({ item, selected, compact, onSelect }: TaskListItemCardProps) {
  const t = useT('automation');
  const translate = React.useCallback(
    (key: string, values?: Record<string, string | number>) => t(key as never, values as never),
    [t]
  );
  const schedule = getScheduleSummary(item.task, translate);

  return (
    <div
      data-task-card="true"
      className={cn(
        'group relative w-full cursor-pointer border bg-background text-left transition-colors',
        compact ? 'rounded-2xl px-3 py-3' : 'rounded-[24px] px-4 py-3.5',
        selected
          ? 'border-primary bg-primary/5 border-l-4'
          : 'hover:bg-muted/20 border-y-border/70 pl-[19px]'
      )}
      onClick={onSelect}
    >
      <div
        className={cn(
          'grid min-w-0 gap-3',
          compact ? 'lg:grid-cols-[minmax(0,1fr)_170px]' : 'lg:grid-cols-[minmax(0,1fr)_240px]'
        )}
      >
        <div className={cn('min-w-0', compact ? 'space-y-1.5' : 'space-y-2')}>
          <div className="flex flex-wrap items-center gap-2">
            <h2
              className={cn(
                'min-w-0 break-words font-semibold text-foreground',
                compact ? 'text-sm leading-5' : 'text-base leading-6'
              )}
            >
              {item.task.name}
            </h2>
            <Badge variant={getTaskStatusBadgeVariant(item.task.status)}>
              {t(`status.${item.task.status}`)}
            </Badge>
            <Badge variant="outline">{schedule.title}</Badge>
          </div>
          {item.task.description?.trim() ? (
            <p
              className={cn(
                'line-clamp-1 break-words text-muted-foreground',
                compact ? 'text-xs leading-5' : 'text-sm leading-5'
              )}
            >
              {item.task.description}
            </p>
          ) : null}
          <div
            className={cn(
              'flex flex-wrap items-center gap-3 text-muted-foreground',
              compact ? 'text-xs' : 'text-sm'
            )}
          >
            <div className="flex min-w-0 items-center gap-2">
              <Clock3 className="size-3.5" />
              <p className="line-clamp-1 break-words">{schedule.description}</p>
            </div>
          </div>
        </div>

        <div
          className={cn(
            'flex min-w-0 items-start justify-between gap-3 lg:flex-col lg:items-end lg:text-right',
            compact ? 'lg:justify-center' : 'lg:justify-center'
          )}
        >
          <div className="space-y-1">
            <p className="text-[11px] font-medium uppercase tracking-[0.12em] text-primary">
              {t('list.nextRun')}
            </p>
            <p
              className={cn(
                'break-words font-semibold text-foreground',
                compact ? 'text-xs leading-5' : 'text-sm leading-5'
              )}
            >
              {getTaskNextRunLabel(item.task, key => t(key as never))}
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
