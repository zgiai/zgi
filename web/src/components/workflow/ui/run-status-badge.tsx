'use client';

import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

interface RunStatusBadgeProps {
  status?: string | null;
  className?: string;
}

type RunStatusTone =
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'stopped'
  | 'paused'
  | 'pending'
  | 'unknown';

const STATUS_CLASSES: Record<RunStatusTone, { badge: string; dot: string }> = {
  running: {
    badge:
      'border-sky-200/80 bg-sky-50 text-sky-700 dark:border-sky-500/25 dark:bg-sky-500/10 dark:text-sky-300',
    dot: 'bg-sky-500 ring-sky-500/15 animate-pulse',
  },
  succeeded: {
    badge:
      'border-emerald-200/80 bg-emerald-50 text-emerald-700 dark:border-emerald-500/25 dark:bg-emerald-500/10 dark:text-emerald-300',
    dot: 'bg-emerald-500 ring-emerald-500/15',
  },
  failed: {
    badge:
      'border-rose-200/80 bg-rose-50 text-rose-700 dark:border-rose-500/25 dark:bg-rose-500/10 dark:text-rose-300',
    dot: 'bg-rose-500 ring-rose-500/15',
  },
  stopped: {
    badge:
      'border-slate-200/90 bg-slate-100 text-slate-700 dark:border-slate-500/30 dark:bg-slate-500/15 dark:text-slate-300',
    dot: 'bg-slate-500 ring-slate-500/15',
  },
  paused: {
    badge:
      'border-amber-200/90 bg-amber-50 text-amber-700 dark:border-amber-500/25 dark:bg-amber-500/10 dark:text-amber-300',
    dot: 'bg-amber-500 ring-amber-500/15',
  },
  pending: {
    badge:
      'border-violet-200/80 bg-violet-50 text-violet-700 dark:border-violet-500/25 dark:bg-violet-500/10 dark:text-violet-300',
    dot: 'bg-violet-500 ring-violet-500/15',
  },
  unknown: {
    badge: 'border-border bg-muted/60 text-muted-foreground',
    dot: 'bg-muted-foreground ring-muted-foreground/15',
  },
};

function normalizeRunStatus(status?: string | null): RunStatusTone {
  const normalized = status?.trim().toLowerCase().replaceAll('_', '-') ?? '';

  if (['running', 'in-progress', 'processing', 'streaming'].includes(normalized)) {
    return 'running';
  }
  if (['succeeded', 'success', 'completed', 'partial-succeeded'].includes(normalized)) {
    return 'succeeded';
  }
  if (['failed', 'error'].includes(normalized)) {
    return 'failed';
  }
  if (['stopped', 'cancelled', 'canceled', 'aborted'].includes(normalized)) {
    return 'stopped';
  }
  if (normalized === 'paused') {
    return 'paused';
  }
  if (['pending', 'queued', 'waiting'].includes(normalized)) {
    return 'pending';
  }
  return 'unknown';
}

export function RunStatusBadge({ status, className }: RunStatusBadgeProps) {
  const t = useT('agents');
  const tone = normalizeRunStatus(status);
  const styles = STATUS_CLASSES[tone];
  let label = status?.trim() || '-';
  if (tone === 'running') label = t('workflow.running');
  if (tone === 'succeeded') label = t('workflow.succeeded');
  if (tone === 'failed') label = t('workflow.failed');
  if (tone === 'stopped') label = t('workflow.stopped');
  if (tone === 'paused') label = t('workflow.paused');
  if (tone === 'pending') label = t('workflow.pending');

  return (
    <Badge
      variant="outline"
      className={cn('h-6 gap-1.5 border px-2.5 font-medium shadow-none', styles.badge, className)}
    >
      <span aria-hidden className={cn('size-1.5 rounded-full ring-2', styles.dot)} />
      <span>{label}</span>
    </Badge>
  );
}
