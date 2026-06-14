'use client';

import {
  AlertCircle,
  CheckCircle2,
  Circle,
  CircleSlash,
  ClipboardList,
  Loader2,
} from 'lucide-react';
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

const PLAN_STATUS_FALLBACK_LABEL: Record<OperationPlanStatus, string> = {
  pending: 'Pending',
  running: 'Running',
  completed: 'Completed',
  failed: 'Failed',
  cancelled: 'Cancelled',
};

const STEP_STATUS_FALLBACK_LABEL: Record<OperationPlanStepStatus, string> = {
  pending: 'Pending',
  running: 'Running',
  completed: 'Completed',
  failed: 'Failed',
  skipped: 'Skipped',
};

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

export function OperationPlanCard({
  title = 'Operation plan',
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
  const tone = getPlanTone(status);
  const visibleSteps = steps ?? [];

  return (
    <OperationCardShell compact={compact} className={className}>
      <OperationCardHeader
        compact={compact}
        icon={<ClipboardList className={cn('size-4', getToneTextClassName(tone))} />}
        title={title}
        description={description}
        eyebrow={eyebrow}
        badge={
          <OperationStatusBadge
            label={statusLabel ?? PLAN_STATUS_FALLBACK_LABEL[status]}
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
                      label={step.statusLabel ?? STEP_STATUS_FALLBACK_LABEL[stepStatus]}
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
