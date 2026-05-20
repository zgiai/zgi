import React, { useEffect, useMemo, useRef } from 'react';
import { cn } from '@/lib/utils';
import type { RunStatus } from '@/components/chat/types';
import type { WorkflowRunNodeListItem } from '@/components/workflow/ui/workflow-run-nodes-list';
import { NODE_THEMES } from '@/components/workflow/nodes/custom/config';
import { Check, Clock3, Loader, Pause, X } from 'lucide-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';

interface WorkflowRunHeaderProps {
  status?: RunStatus;
  items: WorkflowRunNodeListItem[];
  className?: string;
  visible?: boolean;
}

// Map node status to visual style
const getNodeStatusStyle = (nodeStatus: WorkflowRunNodeListItem['status']) => {
  switch (nodeStatus) {
    case 'running':
      return {
        frame: 'bg-info/10 ring-2 ring-info/25 shadow-sm',
        bar: 'bg-info',
      };
    case 'succeeded':
      return {
        frame: 'bg-success/20 shadow-[0_0_0_1px_var(--success)]',
        bar: 'bg-success',
      };
    case 'failed':
      return {
        frame: 'bg-destructive/20 shadow-[0_0_0_1px_var(--destructive)]',
        bar: 'bg-destructive',
      };
    case 'stopped':
      return {
        frame: 'bg-muted/80 shadow-[0_0_0_1px_var(--muted-foreground)]',
        bar: 'bg-muted-foreground',
      };
    case 'paused':
      return {
        frame: 'bg-warning/25 shadow-[0_0_0_1px_var(--warning)]',
        bar: 'bg-warning',
      };
    default:
      return {
        frame: 'bg-muted shadow-[0_0_0_1px_var(--border)]',
        bar: 'bg-border',
      };
  }
};

const solidConnectorClass = (status: WorkflowRunNodeListItem['status']): string => {
  switch (status) {
    case 'succeeded':
      return 'bg-success';
    case 'failed':
      return 'bg-destructive';
    case 'stopped':
      return 'bg-muted-foreground';
    case 'paused':
      return 'bg-warning';
    default:
      return 'bg-info';
  }
};

const gradientFromClass = (status: WorkflowRunNodeListItem['status']): string => {
  switch (status) {
    case 'succeeded':
      return 'from-success';
    case 'failed':
      return 'from-destructive';
    case 'stopped':
      return 'from-muted-foreground';
    case 'paused':
      return 'from-warning';
    default:
      return 'from-info';
  }
};

const gradientToClass = (status: WorkflowRunNodeListItem['status']): string => {
  switch (status) {
    case 'succeeded':
      return 'to-success';
    case 'failed':
      return 'to-destructive';
    case 'stopped':
      return 'to-muted-foreground';
    case 'paused':
      return 'to-warning';
    default:
      return 'to-info';
  }
};

// Compute connector line class based on adjacent node statuses
const connectorClass = (
  left: WorkflowRunNodeListItem['status'],
  right: WorkflowRunNodeListItem['status']
): string => {
  if (left === 'paused') {
    return 'bg-muted-foreground/25';
  }
  // Solid colors when both ends share the same status
  if (left === right) {
    return solidConnectorClass(left);
  }
  // Gradient from left status color to right status color
  const from = gradientFromClass(left);
  const to = gradientToClass(right);
  return cn(
    'bg-gradient-to-r',
    from,
    to,
    (left === 'running' || right === 'running') && 'opacity-80',
    left === 'succeeded' && right === 'running' && 'bg-[length:200%_100%]'
  );
};

const WorkflowRunHeader: React.FC<WorkflowRunHeaderProps> = ({
  status,
  items,
  className,
  visible = true,
}) => {
  // Only show when workflow is running or recently finished
  const shouldShow = useMemo(() => {
    if (!visible) return false;
    // Show when there is any status or any item to render (not only during running)
    return Boolean(status) || items.length > 0;
  }, [visible, status, items.length]);

  // Horizontal auto-scroll to the right on updates when overflowing
  const scrollerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const el = scrollerRef.current;
    if (!el) return;
    if (el.scrollWidth <= el.clientWidth) return;
    // Defer until after layout to ensure widths are correct
    const raf = requestAnimationFrame(() => {
      try {
        el.scrollTo({ left: el.scrollWidth, behavior: 'smooth' });
      } catch {
        // Fallback for environments without smooth scroll
        el.scrollLeft = el.scrollWidth;
      }
    });
    return () => cancelAnimationFrame(raf);
  }, [items]);

  if (!shouldShow) return null;

  return (
    <div
      className={cn(
        'flex items-center gap-3 px-2 py-1.5 backdrop-blur-sm bg-background/30 border-b border-border transition-all duration-500 ease-in-out',
        className
      )}
    >
      <div
        ref={scrollerRef}
        className="flex items-center justify-center gap-1 overflow-x-auto flex-1 py-1 [&::-webkit-scrollbar]:h-1 [&::-webkit-scrollbar-track]:bg-muted/50 [&::-webkit-scrollbar-thumb]:bg-muted-foreground/30 [&::-webkit-scrollbar-thumb]:rounded-full"
      >
        {items.map((item, idx) => {
          const theme =
            item.nodeType in NODE_THEMES
              ? NODE_THEMES[item.nodeType as keyof typeof NODE_THEMES]
              : undefined;
          const Icon = theme?.icon;
          const nodeStyle = getNodeStatusStyle(item.status);

          return (
            <React.Fragment key={`header-node-${item.nodeId}`}>
              {/* Node representation */}
              <div
                className={cn('flex items-center gap-1 rounded-md transition-all duration-300')}
                title={item.title}
              >
                {/* Node icon */}
                {Icon ? (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <div
                        className={cn(
                          'relative flex h-6 w-6 shrink-0 items-center justify-center rounded-sm p-0.5 transition-all duration-300',
                          nodeStyle.frame
                        )}
                        aria-label={item.title}
                      >
                        <span
                          className={cn(
                            'flex h-full w-full items-center justify-center rounded-sm text-white',
                            theme?.classNames.iconBg
                          )}
                        >
                          <Icon className="h-3.5 w-3.5" />
                        </span>
                        {item.status === 'running' ? (
                          <span className="absolute -right-0.5 -top-0.5 flex h-2 w-2 rounded-full bg-info ring-2 ring-background" />
                        ) : null}
                        {/* {item.status !== 'running' ? (
                          <span
                            className={cn(
                              'absolute bottom-px left-1/2 h-0.5 w-3 -translate-x-1/2 rounded-full',
                              nodeStyle.bar
                            )}
                          />
                        ) : null} */}
                      </div>
                    </TooltipTrigger>
                    <TooltipContent side="top" className="z-50">
                      <p className="max-w-xs">{item.title}</p>
                    </TooltipContent>
                  </Tooltip>
                ) : null}
                {/* Node status indicator */}
                {item.status === 'running' ? (
                  <span className="h-1.5 w-5 rounded-full bg-info/35" />
                ) : null}
              </div>

              {idx < items.length - 1 && (
                <div
                  className={cn(
                    'h-0.5 w-6 shrink-0 rounded-full transition-all duration-500',
                    connectorClass(item.status, items[idx + 1].status)
                  )}
                />
              )}
            </React.Fragment>
          );
        })}
      </div>

      <div className="flex items-center gap-2 px-2">
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
          <div className="flex items-center gap-2 text-primary-foreground bg-gray-500 rounded-full p-0.5">
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
  );
};

export default WorkflowRunHeader;
