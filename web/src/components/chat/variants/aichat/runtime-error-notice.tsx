'use client';

import { AlertCircle, RefreshCw, Settings } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface AIChatRuntimeErrorNoticeProps {
  title: string;
  description: string;
  retryLabel: string;
  configureLabel?: string;
  isBilling?: boolean;
  canRetry?: boolean;
  onRetry?: () => void;
  onConfigure?: () => void;
}

export function AIChatRuntimeErrorNotice({
  title,
  description,
  retryLabel,
  configureLabel,
  isBilling = false,
  canRetry = false,
  onRetry,
  onConfigure,
}: AIChatRuntimeErrorNoticeProps) {
  return (
    <div
      role="alert"
      aria-live="polite"
      className={cn(
        'flex flex-col gap-2 rounded-xl border px-3 py-2.5 shadow-sm sm:flex-row sm:items-center sm:justify-between',
        isBilling
          ? 'border-warning/35 bg-warning/10'
          : 'border-destructive/25 bg-destructive/[0.045]'
      )}
    >
      <div className="flex min-w-0 items-start gap-2.5">
        <span
          className={cn(
            'mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full',
            isBilling ? 'bg-warning/15 text-warning' : 'bg-destructive/10 text-destructive'
          )}
        >
          <AlertCircle className="size-3.5" aria-hidden="true" />
        </span>
        <div className="min-w-0">
          <div className="text-sm font-medium text-foreground">{title}</div>
          <p className="mt-0.5 text-xs leading-5 text-muted-foreground">{description}</p>
        </div>
      </div>

      <div className="flex shrink-0 items-center gap-1.5 pl-8 sm:pl-0">
        {canRetry && onRetry ? (
          <Button type="button" variant="ghost" size="sm" className="h-7 gap-1.5" onClick={onRetry}>
            <RefreshCw className="size-3.5" />
            {retryLabel}
          </Button>
        ) : null}
        {configureLabel && onConfigure ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-7 gap-1.5 bg-background/80"
            onClick={onConfigure}
          >
            <Settings className="size-3.5" />
            {configureLabel}
          </Button>
        ) : null}
      </div>
    </div>
  );
}
