'use client';

import {
  AlertCircle,
  CheckCircle2,
  Circle,
  CircleSlash,
  ClipboardList,
  Loader2,
} from 'lucide-react';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type {
  OperationCardTone,
  OperationPlanCardProps,
  OperationPlanStatus,
  OperationPlanStepStatus,
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

function getPlanTone(status: OperationPlanStatus): OperationCardTone {
  if (status === 'completed') return 'success';
  if (status === 'failed' || status === 'cancelled') return 'destructive';
  if (status === 'running') return 'info';
  return 'neutral';
}

function getStepTone(status: OperationPlanStepStatus): OperationCardTone {
  if (status === 'completed') return 'success';
  if (status === 'failed') return 'destructive';
  if (status === 'running') return 'info';
  return 'neutral';
}

function StepStatusIcon({ status }: { status: OperationPlanStepStatus }) {
  if (status === 'running') return <Loader2 className="size-3.5 animate-spin" />;
  if (status === 'completed') return <CheckCircle2 className="size-3.5" />;
  if (status === 'failed') return <AlertCircle className="size-3.5" />;
  if (status === 'skipped') return <CircleSlash className="size-3.5" />;
  return <Circle className="size-3.5" />;
}

function getPlanStatusLabel(
  status: OperationPlanStatus,
  t: ReturnType<typeof useT<'webapp'>>
): string {
  switch (status) {
    case 'running':
      return t('consoleChat.operationCards.planStatuses.running');
    case 'completed':
      return t('consoleChat.operationCards.planStatuses.completed');
    case 'failed':
      return t('consoleChat.operationCards.planStatuses.failed');
    case 'cancelled':
      return t('consoleChat.operationCards.planStatuses.cancelled');
    case 'pending':
    default:
      return t('consoleChat.operationCards.planStatuses.pending');
  }
}

function getStepStatusLabel(
  status: OperationPlanStepStatus,
  t: ReturnType<typeof useT<'webapp'>>
): string {
  switch (status) {
    case 'running':
      return t('consoleChat.operationCards.stepStatuses.running');
    case 'completed':
      return t('consoleChat.operationCards.stepStatuses.completed');
    case 'failed':
      return t('consoleChat.operationCards.stepStatuses.failed');
    case 'skipped':
      return t('consoleChat.operationCards.stepStatuses.skipped');
    case 'pending':
    default:
      return t('consoleChat.operationCards.stepStatuses.pending');
  }
}

export function OperationPlanCard({
  title,
  description,
  status = 'pending',
  statusLabel,
  eyebrow,
  steps,
  meta,
  actions,
  compact = false,
  className,
}: OperationPlanCardProps) {
  const t = useT('webapp');
  const tone = getPlanTone(status);
  const visibleSteps = steps ?? [];

  return (
    <OperationCardShell compact={compact} className={className}>
      <OperationCardHeader
        compact={compact}
        icon={<ClipboardList className={cn('size-4', getToneTextClassName(tone))} />}
        title={title ?? t('consoleChat.operationCards.planTitle')}
        description={description}
        eyebrow={eyebrow}
        badge={
          <OperationStatusBadge
            label={statusLabel ?? getPlanStatusLabel(status, t)}
            tone={tone}
            loading={status === 'running'}
          />
        }
      />

      {visibleSteps.length > 0 ? (
        <ol className="space-y-2">
          {visibleSteps.map((step, index) => {
            const stepStatus = step.status ?? 'pending';
            const stepTone = getStepTone(stepStatus);

            return (
              <li
                key={step.id}
                className={cn(
                  'grid grid-cols-[auto_minmax(0,1fr)] gap-3 rounded-md border px-3 py-2.5',
                  getToneSoftClassName(stepTone)
                )}
              >
                <div className="flex flex-col items-center gap-1">
                  <span
                    className={cn(
                      'flex size-6 items-center justify-center rounded-full border bg-background',
                      getToneTextClassName(stepTone)
                    )}
                  >
                    <StepStatusIcon status={stepStatus} />
                  </span>
                  <span className="text-[10px] text-muted-foreground">{index + 1}</span>
                </div>
                <div className="min-w-0 space-y-2">
                  <div className="flex min-w-0 flex-wrap items-center gap-2">
                    <div className="min-w-0 flex-1 break-words font-medium text-foreground">
                      {step.title}
                    </div>
                    <OperationStatusBadge
                      label={step.statusLabel ?? getStepStatusLabel(stepStatus, t)}
                      tone={stepTone}
                      loading={stepStatus === 'running'}
                    />
                  </div>
                  {step.description ? (
                    <div className="whitespace-pre-wrap break-words text-xs leading-relaxed text-muted-foreground">
                      {step.description}
                    </div>
                  ) : null}
                  <OperationMetaGrid items={step.meta} compact />
                </div>
              </li>
            );
          })}
        </ol>
      ) : null}

      <OperationMetaGrid items={meta} compact={compact} />
      <OperationCardActions actions={actions} compact={compact} />
    </OperationCardShell>
  );
}
