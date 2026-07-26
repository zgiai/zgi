'use client';

import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

interface RunStatusBadgeProps {
  status?: string | null;
  className?: string;
}

type RunStatusKind =
  | 'running'
  | 'resuming'
  | 'succeeded'
  | 'partialSucceeded'
  | 'failed'
  | 'stopped'
  | 'stopping'
  | 'paused'
  | 'pending'
  | 'queued'
  | 'pendingApproval'
  | 'pendingQuestion'
  | 'pendingClientAction'
  | 'pendingUserInput'
  | 'retrying'
  | 'submitted'
  | 'approved'
  | 'rejected'
  | 'answered'
  | 'blocked'
  | 'skipped'
  | 'expired'
  | 'unknown';

const STATUS_CLASSES: Record<RunStatusKind, { badge: string; dot: string }> = {
  running: {
    badge:
      'border-sky-200/80 bg-sky-50 text-sky-700 dark:border-sky-500/25 dark:bg-sky-500/10 dark:text-sky-300',
    dot: 'bg-sky-500 ring-sky-500/15 animate-pulse',
  },
  resuming: {
    badge:
      'border-sky-200/80 bg-sky-50 text-sky-700 dark:border-sky-500/25 dark:bg-sky-500/10 dark:text-sky-300',
    dot: 'bg-sky-500 ring-sky-500/15 animate-pulse',
  },
  succeeded: {
    badge:
      'border-emerald-200/80 bg-emerald-50 text-emerald-700 dark:border-emerald-500/25 dark:bg-emerald-500/10 dark:text-emerald-300',
    dot: 'bg-emerald-500 ring-emerald-500/15',
  },
  partialSucceeded: {
    badge:
      'border-teal-200/80 bg-teal-50 text-teal-700 dark:border-teal-500/25 dark:bg-teal-500/10 dark:text-teal-300',
    dot: 'bg-teal-500 ring-teal-500/15',
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
  stopping: {
    badge:
      'border-slate-200/90 bg-slate-100 text-slate-700 dark:border-slate-500/30 dark:bg-slate-500/15 dark:text-slate-300',
    dot: 'bg-slate-500 ring-slate-500/15 animate-pulse',
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
  queued: {
    badge:
      'border-violet-200/80 bg-violet-50 text-violet-700 dark:border-violet-500/25 dark:bg-violet-500/10 dark:text-violet-300',
    dot: 'bg-violet-500 ring-violet-500/15 animate-pulse',
  },
  pendingApproval: {
    badge:
      'border-amber-200/90 bg-amber-50 text-amber-700 dark:border-amber-500/25 dark:bg-amber-500/10 dark:text-amber-300',
    dot: 'bg-amber-500 ring-amber-500/15 animate-pulse',
  },
  pendingQuestion: {
    badge:
      'border-indigo-200/80 bg-indigo-50 text-indigo-700 dark:border-indigo-500/25 dark:bg-indigo-500/10 dark:text-indigo-300',
    dot: 'bg-indigo-500 ring-indigo-500/15 animate-pulse',
  },
  pendingClientAction: {
    badge:
      'border-cyan-200/80 bg-cyan-50 text-cyan-700 dark:border-cyan-500/25 dark:bg-cyan-500/10 dark:text-cyan-300',
    dot: 'bg-cyan-500 ring-cyan-500/15 animate-pulse',
  },
  pendingUserInput: {
    badge:
      'border-indigo-200/80 bg-indigo-50 text-indigo-700 dark:border-indigo-500/25 dark:bg-indigo-500/10 dark:text-indigo-300',
    dot: 'bg-indigo-500 ring-indigo-500/15 animate-pulse',
  },
  retrying: {
    badge:
      'border-sky-200/80 bg-sky-50 text-sky-700 dark:border-sky-500/25 dark:bg-sky-500/10 dark:text-sky-300',
    dot: 'bg-sky-500 ring-sky-500/15 animate-pulse',
  },
  submitted: {
    badge:
      'border-blue-200/80 bg-blue-50 text-blue-700 dark:border-blue-500/25 dark:bg-blue-500/10 dark:text-blue-300',
    dot: 'bg-blue-500 ring-blue-500/15',
  },
  approved: {
    badge:
      'border-emerald-200/80 bg-emerald-50 text-emerald-700 dark:border-emerald-500/25 dark:bg-emerald-500/10 dark:text-emerald-300',
    dot: 'bg-emerald-500 ring-emerald-500/15',
  },
  rejected: {
    badge:
      'border-rose-200/80 bg-rose-50 text-rose-700 dark:border-rose-500/25 dark:bg-rose-500/10 dark:text-rose-300',
    dot: 'bg-rose-500 ring-rose-500/15',
  },
  answered: {
    badge:
      'border-emerald-200/80 bg-emerald-50 text-emerald-700 dark:border-emerald-500/25 dark:bg-emerald-500/10 dark:text-emerald-300',
    dot: 'bg-emerald-500 ring-emerald-500/15',
  },
  blocked: {
    badge:
      'border-rose-200/80 bg-rose-50 text-rose-700 dark:border-rose-500/25 dark:bg-rose-500/10 dark:text-rose-300',
    dot: 'bg-rose-500 ring-rose-500/15',
  },
  skipped: {
    badge: 'border-border bg-muted/60 text-muted-foreground',
    dot: 'bg-muted-foreground ring-muted-foreground/15',
  },
  expired: {
    badge:
      'border-slate-200/90 bg-slate-100 text-slate-700 dark:border-slate-500/30 dark:bg-slate-500/15 dark:text-slate-300',
    dot: 'bg-slate-500 ring-slate-500/15',
  },
  unknown: {
    badge: 'border-border bg-muted/60 text-muted-foreground',
    dot: 'bg-muted-foreground ring-muted-foreground/15',
  },
};

function normalizeRunStatus(status?: string | null): RunStatusKind {
  const normalized = status?.trim().toLowerCase().replaceAll('_', '-') ?? '';

  if (['running', 'in-progress', 'processing', 'streaming', 'loading'].includes(normalized)) {
    return 'running';
  }
  if (normalized === 'resuming') return 'resuming';
  if (['succeeded', 'success', 'completed'].includes(normalized)) {
    return 'succeeded';
  }
  if (['partial-succeeded', 'partial-success'].includes(normalized)) return 'partialSucceeded';
  if (['failed', 'error', 'exception'].includes(normalized)) {
    return 'failed';
  }
  if (['stopped', 'cancelled', 'canceled', 'aborted'].includes(normalized)) {
    return 'stopped';
  }
  if (normalized === 'stopping') return 'stopping';
  if (normalized === 'paused') {
    return 'paused';
  }
  if (['pending-approval', 'waiting-approval'].includes(normalized)) return 'pendingApproval';
  if (['pending-question', 'waiting-question'].includes(normalized)) return 'pendingQuestion';
  if (['pending-client-action', 'waiting-client-action'].includes(normalized)) {
    return 'pendingClientAction';
  }
  if (['pending-user-input', 'waiting-user-input', 'waiting-for-user'].includes(normalized)) {
    return 'pendingUserInput';
  }
  if (normalized === 'queued') return 'queued';
  if (['pending', 'waiting'].includes(normalized)) {
    return 'pending';
  }
  if (['retry', 'retrying'].includes(normalized)) return 'retrying';
  if (normalized === 'submitted') return 'submitted';
  if (normalized === 'approved') return 'approved';
  if (['rejected', 'user-rejected'].includes(normalized)) return 'rejected';
  if (normalized === 'answered') return 'answered';
  if (normalized === 'blocked') return 'blocked';
  if (normalized === 'skipped') return 'skipped';
  if (normalized === 'expired') return 'expired';
  return 'unknown';
}

export function RunStatusBadge({ status, className }: RunStatusBadgeProps) {
  const t = useT('agents');
  const tone = normalizeRunStatus(status);
  const styles = STATUS_CLASSES[tone];
  const label = t(`workflow.status.${tone}`);
  const rawStatus = status?.trim();

  return (
    <Badge
      variant="outline"
      className={cn('h-6 gap-1.5 border px-2.5 font-medium shadow-none', styles.badge, className)}
      title={tone === 'unknown' && rawStatus ? rawStatus : undefined}
    >
      <span aria-hidden className={cn('size-1.5 rounded-full ring-2', styles.dot)} />
      <span>{label}</span>
    </Badge>
  );
}
