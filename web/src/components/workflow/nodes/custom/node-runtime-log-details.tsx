import React from 'react';
import { AlertCircle, Loader2 } from 'lucide-react';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import type { ScopedTranslations } from '@/i18n/translations';
import { formatMs } from '@/utils/format';
import WorkflowRunNodesList from '../../ui/workflow-run-nodes-list';
import type { RuntimeLogItem, RunStatus } from '../../store/slices/run-status';

interface NodeRuntimeLogDetailsProps {
  nodeId: string;
  items: RuntimeLogItem[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface RuntimeStatusMeta {
  label: string;
  dotClassName: string;
  textClassName: string;
  barClassName: string;
  icon: React.ReactNode;
}

const stopCanvasInteraction = (event: React.SyntheticEvent) => {
  event.stopPropagation();
};

const DETAILS_EXIT_ANIMATION_MS = 180;

const flattenRuntimeItems = (items: RuntimeLogItem[]): RuntimeLogItem[] => {
  const result: RuntimeLogItem[] = [];
  const append = (item: RuntimeLogItem) => {
    result.push(item);
    item.iterationRounds?.forEach(round => round.nodes.forEach(append));
    item.loopRounds?.forEach(round => round.nodes.forEach(append));
  };
  items.forEach(append);
  return result;
};

const getLatestRuntimeItem = (items: RuntimeLogItem[]): RuntimeLogItem | null => {
  const flattened = flattenRuntimeItems(items);
  if (flattened.length === 0) return null;
  return flattened.reduce((latest, item) => {
    const latestOrder = latest.receivedOrder ?? -1;
    const itemOrder = item.receivedOrder ?? -1;
    if (itemOrder !== latestOrder) return itemOrder > latestOrder ? item : latest;
    const latestTime = latest.createdAtMs ?? -1;
    const itemTime = item.createdAtMs ?? -1;
    return itemTime >= latestTime ? item : latest;
  });
};

function getStatusMeta(
  status: RunStatus,
  t: ScopedTranslations<'agents'>
): RuntimeStatusMeta {
  switch (status) {
    case 'running':
      return {
        label: t('workflow.running'),
        dotClassName: 'bg-sky-500 shadow-[0_0_0_3px_rgba(14,165,233,0.12)]',
        textClassName: 'text-sky-700 dark:text-sky-300',
        barClassName: 'border-sky-200/80 bg-sky-50/80 dark:border-sky-500/20 dark:bg-sky-500/10',
        icon: <Loader2 className="size-3.5 animate-spin" />,
      };
    case 'failed':
      return {
        label: t('workflow.failed'),
        dotClassName: 'bg-rose-500 shadow-[0_0_0_3px_rgba(244,63,94,0.12)]',
        textClassName: 'text-rose-700 dark:text-rose-300',
        barClassName:
          'border-rose-200/90 bg-rose-50/85 dark:border-rose-500/25 dark:bg-rose-500/10',
        icon: <AlertCircle className="size-3.5" />,
      };
    case 'paused':
      return {
        label: t('workflow.paused'),
        dotClassName: 'bg-amber-500 shadow-[0_0_0_3px_rgba(245,158,11,0.12)]',
        textClassName: 'text-amber-700 dark:text-amber-300',
        barClassName:
          'border-amber-200/90 bg-amber-50/80 dark:border-amber-500/25 dark:bg-amber-500/10',
        icon: <AlertCircle className="size-3.5" />,
      };
    case 'stopped':
      return {
        label: t('workflow.stopped'),
        dotClassName: 'bg-slate-400 shadow-[0_0_0_3px_rgba(100,116,139,0.12)]',
        textClassName: 'text-slate-600 dark:text-slate-300',
        barClassName:
          'border-slate-200/90 bg-slate-50/80 dark:border-slate-500/25 dark:bg-slate-500/10',
        icon: <AlertCircle className="size-3.5" />,
      };
    case 'succeeded':
    case 'idle':
    default:
      return {
        label: t('workflow.succeeded'),
        dotClassName: 'bg-emerald-500 shadow-[0_0_0_3px_rgba(16,185,129,0.12)]',
        textClassName: 'text-emerald-700 dark:text-emerald-300',
        barClassName:
          'border-emerald-200/80 bg-emerald-50/80 dark:border-emerald-500/20 dark:bg-emerald-500/10',
        icon: <AlertCircle className="size-3.5" />,
      };
  }
}

const NodeRuntimeLogDetails: React.FC<NodeRuntimeLogDetailsProps> = ({
  nodeId,
  items,
  open,
  onOpenChange,
}) => {
  const t = useT('agents');
  const [shouldRenderDetails, setShouldRenderDetails] = React.useState(open);
  const [detailsVisible, setDetailsVisible] = React.useState(open);

  React.useEffect(() => {
    if (open) {
      setShouldRenderDetails(true);
      const frame = window.requestAnimationFrame(() => setDetailsVisible(true));
      return () => window.cancelAnimationFrame(frame);
    }

    setDetailsVisible(false);
    const timeout = window.setTimeout(
      () => setShouldRenderDetails(false),
      DETAILS_EXIT_ANIMATION_MS
    );
    return () => window.clearTimeout(timeout);
  }, [open]);

  const latest = getLatestRuntimeItem(items);
  if (!latest) return null;

  const meta = getStatusMeta(latest.status, t);
  const duration = latest.status === 'running' ? null : formatMs(latest.elapsedTime ?? 0);
  const actionLabel = open
    ? t('workflow.runtimeLog.closeNodeRuntimeLog')
    : t('workflow.runtimeLog.openNodeRuntimeLog');
  const tooltipLabel = t('workflow.runtimeLog.openNodeRuntimeLog');

  return (
    <>
      <div
        data-workflow-runtime-log="true"
        className="nodrag nowheel cursor-pointer border-t border-border/40 px-2.5 py-1.5"
        onClick={stopCanvasInteraction}
        onDoubleClick={stopCanvasInteraction}
        onMouseDown={stopCanvasInteraction}
        onPointerDown={stopCanvasInteraction}
        onWheel={stopCanvasInteraction}
      >
        <div
          role="button"
          tabIndex={0}
          aria-expanded={open}
          aria-label={actionLabel}
          className={cn(
            'flex h-7 cursor-pointer select-none items-center gap-2 rounded-md border px-2 text-[11px] transition-colors',
            'hover:brightness-[0.98] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/35',
            meta.barClassName
          )}
          onClick={event => {
            event.stopPropagation();
            onOpenChange(!open);
          }}
          onKeyDown={event => {
            if (event.key !== 'Enter' && event.key !== ' ') return;
            event.preventDefault();
            event.stopPropagation();
            onOpenChange(!open);
          }}
        >
          <span className={cn('size-1.5 shrink-0 rounded-full', meta.dotClassName)} />
          <span className={cn('min-w-0 truncate font-medium', meta.textClassName)}>
            {meta.label}
          </span>
          <span className="ml-auto shrink-0 tabular-nums text-muted-foreground/70">
            {duration ?? t('workflow.runtimeLog.runningNow')}
          </span>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <span
                  className={cn(
                    'flex size-4 shrink-0 items-center justify-center',
                    meta.textClassName
                  )}
                >
                  {meta.icon}
                </span>
              </TooltipTrigger>
              <TooltipContent side="top">{tooltipLabel}</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>
      </div>
      {shouldRenderDetails ? (
        <div
          data-workflow-runtime-log="true"
          data-node-id={nodeId}
          data-state={detailsVisible ? 'open' : 'closed'}
          className={cn(
            'nodrag nowheel absolute left-0 top-[calc(100%+6px)] z-30 w-full cursor-text select-text',
            'origin-top-right',
            detailsVisible
              ? 'workflow-runtime-details-enter'
              : 'pointer-events-none workflow-runtime-details-exit'
          )}
          onClick={stopCanvasInteraction}
          onDoubleClick={stopCanvasInteraction}
          onMouseDown={stopCanvasInteraction}
          onPointerDown={stopCanvasInteraction}
          onWheel={stopCanvasInteraction}
        >
          <WorkflowRunNodesList
            items={items}
            showDetail={false}
            variant="canvas"
            hideCanvasNodeChrome
          />
        </div>
      ) : null}
    </>
  );
};

export default React.memo(NodeRuntimeLogDetails);
