'use client';

import { useState, type ReactNode } from 'react';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';

/** Presentation extracted from the active Skill row; hosts supply safe details. */
export function ToolInvocationRow({
  tone,
  statusIcon,
  icon,
  title,
  duration,
  children,
}: {
  tone: 'running' | 'success' | 'warning' | 'error';
  statusIcon: ReactNode;
  icon: ReactNode;
  title: ReactNode;
  duration?: string | null;
  children?: ReactNode;
}) {
  const [open, setOpen] = useState(false);
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
      <span className="min-w-0 flex-1 truncate text-foreground">{title}</span>
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
        'rounded-md border bg-background/80 text-xs',
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
          className="flex min-h-8 w-full min-w-0 items-center gap-2 px-2.5 py-1.5 text-left"
          aria-expanded={open}
          onClick={() => setOpen(value => !value)}
        >
          {content}
        </button>
      ) : (
        <div className="flex min-h-8 w-full min-w-0 items-center gap-2 px-2.5 py-1.5 text-left">
          {content}
        </div>
      )}
      {children && open ? <div className="border-t bg-muted/20 px-2.5 py-2">{children}</div> : null}
    </div>
  );
}
