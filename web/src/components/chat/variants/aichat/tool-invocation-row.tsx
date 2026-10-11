'use client';

import { useId, useState, type ReactNode } from 'react';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';

/** Presentation extracted from the active Skill row; hosts supply safe details. */
export function ToolInvocationRow({
  tone,
  statusIcon,
  icon,
  title,
  duration,
  summary,
  defaultOpen = false,
  children,
}: {
  tone: 'running' | 'success' | 'warning' | 'error';
  statusIcon: ReactNode;
  icon: ReactNode;
  title: ReactNode;
  duration?: string | null;
  summary?: ReactNode;
  defaultOpen?: boolean;
  children?: ReactNode;
}) {
  const [userOpen, setUserOpen] = useState<boolean | undefined>();
  const open = userOpen ?? defaultOpen;
  const detailId = useId();
  const content = (
    <>
      <span
        className={cn(
          'flex size-5 shrink-0 items-center justify-center rounded-full border bg-background',
          tone === 'error'
            ? 'border-destructive/40 text-destructive'
            : tone === 'warning'
              ? 'border-amber-500/40 text-amber-700 dark:text-amber-300'
              : 'border-border text-muted-foreground'
        )}
      >
        {statusIcon}
      </span>
      {icon}
      <span className="min-w-0 flex-1 break-words text-foreground">{title}</span>
      {duration ? <span className="shrink-0 text-muted-foreground">{duration}</span> : null}
      {children ? (
        <ChevronDown
          className={cn('size-3.5 shrink-0 text-muted-foreground transition-transform', {
            'rotate-180': open,
          })}
        />
      ) : null}
    </>
  );
  return (
    <div
      className={cn(
        'rounded-lg border bg-background/80 text-[13px] leading-5',
        tone === 'error'
          ? 'border-destructive/30'
          : tone === 'warning'
            ? 'border-amber-500/30'
            : 'border-border'
      )}
    >
      {children ? (
        <button
          type="button"
          className="flex min-h-10 w-full min-w-0 items-center gap-2 px-3 py-2 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          aria-expanded={open}
          aria-controls={detailId}
          onClick={() => setUserOpen(!open)}
        >
          {content}
        </button>
      ) : (
        <div className="flex min-h-10 w-full min-w-0 items-center gap-2 px-3 py-2 text-left">
          {content}
        </div>
      )}
      {summary ? (
        <div className="px-3 pb-2 text-xs leading-5 text-muted-foreground">{summary}</div>
      ) : null}
      {children ? (
        <div id={detailId} hidden={!open} className="border-t bg-muted/20 px-3 py-2">
          {children}
        </div>
      ) : null}
    </div>
  );
}
