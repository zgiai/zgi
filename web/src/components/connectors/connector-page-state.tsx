'use client';

import Link from 'next/link';
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  CircleSlash2,
  Inbox,
  PlugZap,
  SearchX,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import type { ConnectorReadiness } from '@/services/types/connector';

interface ConnectorPageLoadingProps {
  label: string;
  variant?: 'catalog' | 'detail' | 'connection';
}

export function ConnectorPageLoading({ label, variant = 'catalog' }: ConnectorPageLoadingProps) {
  if (variant === 'connection') {
    return (
      <div className="space-y-5" role="status" aria-label={label}>
        <span className="sr-only">{label}</span>
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
          <Skeleton className="size-14 shrink-0 rounded-2xl" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-7 w-60 max-w-full" />
            <Skeleton className="h-4 w-72 max-w-full" />
            <Skeleton className="h-5 w-24 rounded-full" />
          </div>
          <div className="flex gap-2">
            <Skeleton className="h-9 w-32" />
            <Skeleton className="h-9 w-28" />
          </div>
        </div>
        <Skeleton className="h-12 w-full rounded-xl" />
        <div className="grid gap-5 lg:grid-cols-[minmax(280px,0.8fr)_minmax(0,1.7fr)]">
          <Skeleton className="h-72 rounded-xl" />
          <Skeleton className="h-72 rounded-xl" />
        </div>
        <Skeleton className="h-56 rounded-xl" />
        <Skeleton className="h-28 rounded-xl" />
      </div>
    );
  }

  if (variant === 'detail') {
    return (
      <div className="space-y-6" role="status" aria-label={label}>
        <span className="sr-only">{label}</span>
        <div className="flex gap-4">
          <Skeleton className="size-14 shrink-0 rounded-2xl" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-7 w-52" />
            <Skeleton className="h-4 w-full max-w-2xl" />
          </div>
        </div>
        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_380px]">
          <Skeleton className="h-96 rounded-xl" />
          <Skeleton className="h-80 rounded-xl" />
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6" role="status" aria-label={label}>
      <span className="sr-only">{label}</span>
      <div className="grid gap-4 lg:grid-cols-2">
        {Array.from({ length: 4 }, (_, index) => (
          <div key={index} className="rounded-xl border bg-background p-5">
            <div className="flex gap-3">
              <Skeleton className="size-10 shrink-0" />
              <div className="flex-1 space-y-2">
                <Skeleton className="h-5 w-32" />
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-4 w-2/3" />
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

interface ConnectorPageMessageProps {
  kind: 'error' | 'empty' | 'disabled' | 'not_found';
  title: string;
  description: string;
  actionLabel?: string;
  onAction?: () => void;
  actionHref?: string;
}

export function ConnectorPageMessage({
  kind,
  title,
  description,
  actionLabel,
  onAction,
  actionHref,
}: ConnectorPageMessageProps) {
  const Icon =
    kind === 'error'
      ? AlertCircle
      : kind === 'empty'
        ? Inbox
        : kind === 'not_found'
          ? SearchX
          : PlugZap;
  return (
    <div className="rounded-xl border bg-background px-6 py-12 text-center shadow-sm">
      <Icon aria-hidden="true" className="mx-auto size-9 text-muted-foreground" />
      <h2 className="mt-4 text-lg font-semibold">{title}</h2>
      <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-muted-foreground">{description}</p>
      {actionLabel && actionHref ? (
        <Button asChild className="mt-5" variant="outline">
          <Link href={actionHref}>{actionLabel}</Link>
        </Button>
      ) : actionLabel && onAction ? (
        <Button className="mt-5" variant="outline" onClick={onAction}>
          {actionLabel}
        </Button>
      ) : null}
    </div>
  );
}

interface ConnectorReadinessBannerProps {
  status: ConnectorReadiness;
  title: string;
  description?: string;
  className?: string;
}

const READINESS_STYLES: Record<ConnectorReadiness, string> = {
  ready: 'border-success/25 bg-success/5 text-success',
  reconnect_required: 'border-warning/30 bg-warning/10 text-warning',
  unavailable: 'border-border-strong bg-muted/40 text-foreground',
};

export function ConnectorReadinessBanner({
  status,
  title,
  description,
  className,
}: ConnectorReadinessBannerProps) {
  const Icon =
    status === 'ready'
      ? CheckCircle2
      : status === 'reconnect_required'
        ? AlertTriangle
        : CircleSlash2;
  return (
    <div
      role="status"
      className={cn(
        'flex items-start gap-3 rounded-xl border px-4 py-3 text-sm',
        READINESS_STYLES[status],
        className
      )}
    >
      <Icon aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
      <div className="min-w-0">
        <p className="font-medium leading-5">{title}</p>
        {description ? (
          <p className="mt-0.5 leading-5 text-muted-foreground">{description}</p>
        ) : null}
      </div>
    </div>
  );
}
