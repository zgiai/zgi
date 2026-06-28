'use client';

import {
  AlertCircle,
  CheckCircle2,
  CirclePause,
  CircleStop,
  Clock,
  Loader2,
  ServerCog,
} from 'lucide-react';
import { Progress } from '@/components/ui/progress';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type {
  HostedTaskCardProps,
  HostedTaskStatus,
  OperationCardTone,
} from '@/components/aichat/operation-cards/types';
import {
  OperationCardActions,
  OperationCardHeader,
  OperationCardShell,
  OperationMetaGrid,
  OperationStatusBadge,
  getToneSoftClassName,
  getToneTextClassName,
} from '@/components/aichat/operation-cards/primitives';

function getHostedTaskTone(status: HostedTaskStatus): OperationCardTone {
  if (status === 'completed') return 'success';
  if (status === 'failed' || status === 'cancelled') return 'destructive';
  if (status === 'paused') return 'warning';
  if (status === 'running') return 'info';
  return 'neutral';
}

function getHostedTaskIcon(status: HostedTaskStatus) {
  if (status === 'running') return Loader2;
  if (status === 'completed') return CheckCircle2;
  if (status === 'failed') return AlertCircle;
  if (status === 'cancelled') return CircleStop;
  if (status === 'paused') return CirclePause;
  return Clock;
}

function clampProgress(value: number) {
  return Math.min(100, Math.max(0, value));
}

function getHostedTaskStatusLabel(
  status: HostedTaskStatus,
  t: ReturnType<typeof useT<'webapp'>>
): string {
  switch (status) {
    case 'running':
      return t('consoleChat.operationCards.hostedTaskStatuses.running');
    case 'paused':
      return t('consoleChat.operationCards.hostedTaskStatuses.paused');
    case 'completed':
      return t('consoleChat.operationCards.hostedTaskStatuses.completed');
    case 'failed':
      return t('consoleChat.operationCards.hostedTaskStatuses.failed');
    case 'cancelled':
      return t('consoleChat.operationCards.hostedTaskStatuses.cancelled');
    case 'queued':
    default:
      return t('consoleChat.operationCards.hostedTaskStatuses.queued');
  }
}

export function HostedTaskCard({
  title,
  description,
  status = 'queued',
  statusLabel,
  eyebrow,
  progress,
  progressLabel,
  currentStep,
  meta,
  actions,
  compact = false,
  className,
}: HostedTaskCardProps) {
  const t = useT('webapp');
  const tone = getHostedTaskTone(status);
  const StatusIcon = getHostedTaskIcon(status);
  const normalizedProgress = typeof progress === 'number' ? clampProgress(progress) : undefined;
  const showTaskProgress =
    Boolean(currentStep || progressLabel) ||
    typeof normalizedProgress === 'number' ||
    status === 'running';

  return (
    <OperationCardShell compact={compact} className={className}>
      <OperationCardHeader
        compact={compact}
        icon={<ServerCog className={cn('size-4', getToneTextClassName(tone))} />}
        title={title ?? t('consoleChat.operationCards.hostedTaskTitle')}
        description={description}
        eyebrow={eyebrow}
        badge={
          <OperationStatusBadge
            label={statusLabel ?? getHostedTaskStatusLabel(status, t)}
            tone={tone}
            loading={status === 'running'}
          />
        }
      />

      {showTaskProgress ? (
        <div className={cn('rounded-md border px-3 py-2.5', getToneSoftClassName(tone))}>
          <div className="flex min-w-0 items-center gap-2">
            <StatusIcon
              className={cn('size-4 shrink-0', getToneTextClassName(tone), {
                'animate-spin': status === 'running',
              })}
            />
            <div className="min-w-0 flex-1">
              {currentStep ? (
                <div className="break-words font-medium text-foreground">{currentStep}</div>
              ) : null}
              {progressLabel ? (
                <div className="mt-0.5 break-words text-xs text-muted-foreground">
                  {progressLabel}
                </div>
              ) : null}
            </div>
            {typeof normalizedProgress === 'number' ? (
              <span className="shrink-0 text-xs font-medium text-muted-foreground">
                {Math.round(normalizedProgress)}%
              </span>
            ) : null}
          </div>
          <div className="mt-2">
            <Progress value={normalizedProgress} className="h-1.5" />
          </div>
        </div>
      ) : null}

      <OperationMetaGrid items={meta} compact={compact} />
      <OperationCardActions actions={actions} compact={compact} />
    </OperationCardShell>
  );
}
