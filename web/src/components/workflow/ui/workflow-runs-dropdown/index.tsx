import React, { useMemo } from 'react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Skeleton } from '@/components/ui/skeleton';
import { Clock, Loader2 } from 'lucide-react';
import { formatDate, formatWorkflowElapsedMs } from '@/utils/format';
import { useWorkflowRunsInfinite } from '@/hooks';
import type { WorkflowRunItem, WorkflowRunsQuery } from '@/services/types/workflow';
import { useWorkflowRunDetail } from '@/hooks/workflow/use-workflow-run-detail';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { RunStatusBadge } from '../run-status-badge';

interface WorkflowRunsDropdownProps {
  agentId: string | null;
  limit?: number;
  icon?: React.ReactNode;
  tooltipLabel?: string;
  dropdownLabel?: string;
  triggerText?: string;
  triggerClassName?: string;
  triggerVariant?: 'default' | 'destructive' | 'outline' | 'secondary' | 'ghost' | 'link';
  triggerSize?: 'xs' | 'sm' | 'default' | 'lg' | 'xl' | '2xl';
  refreshOnOpen?: boolean;
  itemFilter?: (item: WorkflowRunItem) => boolean;
  query?: Omit<WorkflowRunsQuery, 'page' | 'limit'>;
  // Callback when a run is selected from the dropdown
  onSelect?: (runId: string) => void;
}

function RunItem({
  item,
  agentId,
  onClick,
}: {
  item: WorkflowRunItem;
  agentId: string | null;
  onClick?: () => void;
}) {
  const t = useT('agents');
  // Prefetch run detail on hover to accelerate opening
  const { prefetch } = useWorkflowRunDetail(
    { agentId, runId: item.id },
    { enabled: false, staleTime: 60_000, gcTime: 10 * 60_000, refetchOnWindowFocus: false }
  );

  return (
    <div
      className="px-3 py-2 hover:bg-accent transition-colors cursor-pointer"
      onMouseEnter={() => prefetch()}
      onFocus={() => prefetch()}
      onClick={onClick}
      role="button"
      tabIndex={0}
    >
      <div className="flex items-center justify-between gap-2">
        <RunStatusBadge status={item.status} />
        <div className="flex items-center gap-1 text-muted-foreground text-xs">
          <Clock className="w-3 h-3" />
          <span>{typeof item.created_at === 'number' ? formatDate(item.created_at) : '-'}</span>
        </div>
      </div>
      <div className="mt-1 grid grid-cols-3 gap-2 text-xs text-muted-foreground">
        {/* <div>
          <span className="mr-1">{t('workflow.tokens')}:</span>
          <span>{typeof item.total_tokens === 'number' ? item.total_tokens : '-'}</span>
        </div> */}
        <div>
          <span className="mr-1">{t('workflow.steps')}:</span>
          <span>{typeof item.total_steps === 'number' ? item.total_steps : '-'}</span>
        </div>
        <div>
          <span className="mr-1">{t('workflow.elapsed')}:</span>
          <span>
            {typeof item.elapsed_time === 'number'
              ? formatWorkflowElapsedMs(item.elapsed_time)
              : '-'}
          </span>
        </div>
      </div>
    </div>
  );
}

function FirstLoadSkeleton() {
  return (
    <div className="space-y-2 p-2">
      {Array.from({ length: 5 }).map((_, i) => (
        <div key={i} className="space-y-2">
          <div className="flex items-center justify-between">
            <Skeleton className="h-5 w-24" />
            <Skeleton className="h-4 w-32" />
          </div>
          <div className="grid grid-cols-3 gap-2">
            <Skeleton className="h-4" />
            <Skeleton className="h-4" />
            <Skeleton className="h-4" />
          </div>
        </div>
      ))}
    </div>
  );
}

const ErrorState = ({ message, onRetry }: { message: string; onRetry: () => void }) => {
  const t = useT('agents');
  return (
    <div className="p-3 text-sm">
      <div className="text-destructive mb-2">{message}</div>
      <Button size="sm" variant="outline" onClick={onRetry}>
        {t('workflow.retry')}
      </Button>
    </div>
  );
};

const WorkflowRunsDropdown: React.FC<WorkflowRunsDropdownProps> = ({
  agentId,
  limit = 10,
  icon,
  tooltipLabel,
  dropdownLabel,
  triggerText,
  triggerClassName,
  triggerVariant = 'ghost',
  triggerSize = 'sm',
  refreshOnOpen = false,
  itemFilter,
  query,
  onSelect,
}) => {
  const t = useT('agents');
  const [open, setOpen] = React.useState(false);
  const viewportRef = React.useRef<HTMLDivElement | null>(null);

  const { pages, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading, error, reload } =
    useWorkflowRunsInfinite(
      { agentId, limit, query },
      {
        enabled: open,
        staleTime: refreshOnOpen ? 0 : 30_000,
        gcTime: 10 * 60_000,
        refetchOnWindowFocus: false,
      }
    );

  const items = useMemo(() => {
    const flat = pages.flat();
    return itemFilter ? flat.filter(itemFilter) : flat;
  }, [itemFilter, pages]);
  const triggerLabel = tooltipLabel ?? t('workflow.recentRuns');
  const menuLabel = dropdownLabel ?? t('workflow.recentRuns');

  // Auto load next page when scrolled near bottom
  const handleScroll = React.useCallback(() => {
    const el = viewportRef.current;
    if (!el) return;
    const threshold = 48; // px to bottom to trigger loading
    const distanceToBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    if (distanceToBottom <= threshold && hasNextPage && !isFetchingNextPage && !isLoading) {
      void fetchNextPage();
    }
  }, [hasNextPage, isFetchingNextPage, isLoading, fetchNextPage]);

  // When opened or items updated, ensure viewport is filled (load more until enough)
  React.useEffect(() => {
    if (!open) return;
    const el = viewportRef.current;
    if (!el) return;
    const needsMore = el.scrollHeight <= el.clientHeight + 4;
    if (needsMore && hasNextPage && !isFetchingNextPage && !isLoading) {
      void fetchNextPage();
    }
  }, [open, items.length, hasNextPage, isFetchingNextPage, isLoading, fetchNextPage]);

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              variant={triggerVariant}
              size={triggerSize}
              isIcon={!triggerText}
              className={cn('gap-1.5', triggerClassName)}
              aria-label={triggerLabel}
            >
              {icon ?? <Clock size={20} />}
              {triggerText ? <span>{triggerText}</span> : null}
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{triggerLabel}</TooltipContent>
      </Tooltip>

      <DropdownMenuContent align="end" className="w-[380px] p-0">
        <DropdownMenuLabel className="text-xs text-muted-foreground px-3 py-2">
          {menuLabel}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <ScrollArea
          className="max-h-[360px] overflow-auto"
          viewportRef={viewportRef}
          viewportProps={{ onScroll: handleScroll }}
        >
          {isLoading ? (
            <FirstLoadSkeleton />
          ) : error ? (
            <ErrorState message={error} onRetry={reload} />
          ) : items.length === 0 ? (
            <div className="py-6 text-sm text-center text-muted-foreground">
              {t('workflow.noRuns')}
            </div>
          ) : (
            <div className="divide-y">
              {items.map(item => (
                <RunItem
                  key={item.id}
                  item={item}
                  agentId={agentId}
                  onClick={() => {
                    onSelect?.(item.id);
                    setOpen(false);
                  }}
                />
              ))}
              {hasNextPage && (
                <div className="p-2 flex items-center justify-center">
                  {isFetchingNextPage ? (
                    <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
                  ) : null}
                </div>
              )}
            </div>
          )}
        </ScrollArea>
      </DropdownMenuContent>
    </DropdownMenu>
  );
};

export default WorkflowRunsDropdown;
