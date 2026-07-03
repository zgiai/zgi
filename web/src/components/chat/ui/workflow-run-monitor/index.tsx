import React, { useEffect, useMemo, useState } from 'react';
import { cn } from '@/lib/utils';
import { formatWorkflowElapsedMs } from '@/utils/format';
import type { RunStatus } from '@/components/chat/types';
import WorkflowRunNodesList, {
  type WorkflowRunNodeListItem,
} from '@/components/workflow/ui/workflow-run-nodes-list';
import { Check, ChevronDown, Clock3, Loader, Pause, X } from 'lucide-react';
import { useT } from '@/i18n';

interface WorkflowRunMonitorProps {
  status?: RunStatus;
  elapsedTime?: number; // backend duration value; current workflow streams use milliseconds
  items: WorkflowRunNodeListItem[];
  className?: string;
  defaultOpen?: boolean;
  error?: string;
  showDetail?: boolean;
  allowExpand?: boolean;
}

const WorkflowRunMonitor: React.FC<WorkflowRunMonitorProps> = ({
  status,
  elapsedTime,
  items,
  className,
  defaultOpen,
  error,
  showDetail = true,
  allowExpand = true,
}) => {
  const [open, setOpen] = useState<boolean>(allowExpand ? (defaultOpen ?? true) : false);
  const t = useT('agents');
  const totalElapsedMs = useMemo(() => (elapsedTime ? elapsedTime : 0), [elapsedTime]);

  useEffect(() => {
    setOpen(allowExpand ? (defaultOpen ?? true) : false);
  }, [allowExpand, defaultOpen]);

  if (!status && items.length === 0) return null;

  return (
    <div
      className={cn(
        'rounded-xl p-1.5 border border-border/40 bg-muted/30 backdrop-blur-md transition-all duration-300',
        allowExpand ? 'hover:shadow-md hover:border-border/60' : '',
        open ? 'shadow-sm border-border/60' : '',
        className
      )}
    >
      <div
        className={cn(
          'flex items-center justify-between',
          allowExpand ? 'cursor-pointer' : 'cursor-default'
        )}
        onClick={allowExpand ? () => setOpen(v => !v) : undefined}
      >
        <div className="flex items-center gap-2">
          {allowExpand ? (
            <ChevronDown
              className={cn(
                'h-3.5 w-3.5 transition-transform text-muted-foreground',
                open ? '' : '-rotate-90'
              )}
            />
          ) : null}
          <span className="text-[13px] font-semibold tracking-tight">
            {t('modes.workflow')}
            {status === 'running' && (
              <span className="ml-1.5 inline-block w-1 h-1 rounded-full bg-info animate-pulse-fast shadow-[0_0_4px_var(--info)]" />
            )}
          </span>
        </div>

        <div className="flex items-center gap-2">
          {status !== 'running' && status !== 'pending_approval' && status !== 'pending_question' && (
            <div className="text-xs text-muted-foreground">
              {formatWorkflowElapsedMs(totalElapsedMs)}
            </div>
          )}
          {status === 'running' && <Loader className="h-3.5 w-3.5 animate-spin" />}
          {(status === 'pending_approval' || status === 'pending_question') && (
            <div className="flex items-center gap-2 text-white bg-warning rounded-full p-0.5">
              <Pause className="h-3 w-3" />
            </div>
          )}
          {status === 'completed' && (
            <div className="flex items-center gap-2 text-primary-foreground bg-success rounded-full p-0.5">
              <Check className="h-3 w-3" />
            </div>
          )}
          {status === 'error' && (
            <div className="flex items-center gap-2 text-primary-foreground bg-destructive rounded-full p-0.5">
              <X className="h-3 w-3" />
            </div>
          )}
          {status === 'stopped' && (
            <div className="flex items-center gap-2 text-primary-foreground bg-muted-foreground rounded-full p-0.5">
              <Pause className="h-3 w-3" />
            </div>
          )}
          {status === 'expired' && (
            <div className="flex items-center gap-2 text-primary-foreground bg-warning rounded-full p-0.5">
              <Clock3 className="h-3 w-3" />
            </div>
          )}
        </div>
      </div>

      {allowExpand && open && items.length > 0 && (
        <div className="mt-2">
          <WorkflowRunNodesList items={items} showDetail={showDetail} />
        </div>
      )}
      {status === 'error' && (
        <div className="mt-2">
          <div className="text-xs text-destructive">{error}</div>
        </div>
      )}
    </div>
  );
};

export default WorkflowRunMonitor;
