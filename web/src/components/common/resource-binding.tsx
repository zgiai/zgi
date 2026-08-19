'use client';

import type { ReactNode } from 'react';
import { ChevronDown, Plus } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';

interface ResourceBindingSectionProps {
  id: string;
  title: string;
  open: boolean;
  count: number;
  countLabel: string;
  addLabel: string;
  helpText?: string;
  emptyText: string;
  isLoading: boolean;
  onToggle: () => void;
  onAdd: () => void;
  addTooltip?: string;
  readOnly?: boolean;
  children: ReactNode;
}

interface ResourceBindingCardProps {
  icon: ReactNode;
  title: string;
  description?: ReactNode;
  action?: ReactNode;
  status?: ReactNode;
  children?: ReactNode;
  error?: boolean;
}

export function ResourceBindingSection({
  id,
  title,
  open,
  count,
  countLabel,
  addLabel,
  helpText,
  emptyText,
  isLoading,
  onToggle,
  onAdd,
  addTooltip,
  readOnly = false,
  children,
}: ResourceBindingSectionProps) {
  const addButton = (
    <Button
      type="button"
      variant="outline"
      size="sm"
      isIcon
      className="size-8"
      aria-label={addLabel}
      disabled={readOnly}
      onClick={event => {
        event.stopPropagation();
        if (!readOnly) onAdd();
      }}
    >
      <Plus className="size-4" />
    </Button>
  );

  return (
    <section id={id} className="scroll-mt-16 space-y-3">
      <div className="flex items-center justify-between gap-3">
        <button
          type="button"
          className="flex min-w-0 items-center gap-2 text-sm font-medium"
          aria-expanded={open}
          aria-controls={`${id}-content`}
          onClick={onToggle}
        >
          <ChevronDown
            className={cn(
              'size-4 shrink-0 text-muted-foreground transition-transform',
              !open && '-rotate-90'
            )}
          />
          <span className="truncate">{title}</span>
        </button>
        <div className="flex items-center gap-2">
          <Badge variant="subtle" aria-label={countLabel}>
            {countLabel}
          </Badge>
          {addTooltip ? (
            <Tooltip>
              <TooltipTrigger asChild>{addButton}</TooltipTrigger>
              <TooltipContent>{addTooltip}</TooltipContent>
            </Tooltip>
          ) : (
            addButton
          )}
        </div>
      </div>

      {open ? (
        <div id={`${id}-content`} className="space-y-3">
          {helpText ? (
            <div className="rounded-md border bg-muted/25 p-3 text-xs leading-5 text-muted-foreground">
              {helpText}
            </div>
          ) : null}

          {isLoading ? (
            <div className="space-y-2">
              <Skeleton className="h-20 w-full" />
              <Skeleton className="h-20 w-full" />
            </div>
          ) : count === 0 ? (
            <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
              {emptyText}
            </div>
          ) : (
            children
          )}
        </div>
      ) : null}
    </section>
  );
}

export function ResourceBindingCard({
  icon,
  title,
  description,
  action,
  status,
  children,
  error,
}: ResourceBindingCardProps) {
  return (
    <div className="rounded-md border bg-background p-3">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted text-primary">
          <span className={error ? 'text-destructive' : undefined}>{icon}</span>
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <div className="min-w-0 truncate text-sm font-medium">{title}</div>
            {status}
          </div>
          {description ? (
            <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
              {description}
            </div>
          ) : null}
        </div>
        {action}
      </div>
      {children ? <div className="mt-3">{children}</div> : null}
    </div>
  );
}
