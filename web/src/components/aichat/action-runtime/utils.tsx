'use client';

import type { ReactNode } from 'react';
import {
  AlertCircle,
  AlertTriangle,
  Check,
  CheckCircle2,
  Circle,
  CircleDashed,
  CircleSlash,
  Clock,
  Loader2,
  ShieldAlert,
  ShieldCheck,
  ShieldQuestion,
  X,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  OperationMetaGrid,
  OperationStatusBadge,
  getToneBadgeVariant,
  getToneSoftClassName,
  getToneTextClassName,
} from '@/components/aichat/operation-cards/primitives';
import type { OperationCardMetaItem, OperationCardTone } from '@/components/aichat/operation-cards';
import { cn } from '@/lib/utils';
import type {
  AIChatActionConfirmation,
  AIChatActionConfirmationStatus,
  AIChatActionPermission,
  AIChatActionPlanStep,
  AIChatActionRisk,
  AIChatActionRiskLevel,
  AIChatActionRunStep,
  AIChatActionRunStatus,
  AIChatActionStepStatus,
} from '@/types/aichat-action-runtime';

export type ActionRuntimeStepLike = AIChatActionPlanStep &
  Partial<
    Pick<
      AIChatActionRunStep,
      'started_at' | 'completed_at' | 'duration_ms' | 'progress' | 'error' | 'result'
    >
  >;

const RISK_RANK: Record<string, number> = {
  none: 0,
  low: 1,
  medium: 2,
  high: 3,
  critical: 4,
};

export function formatActionRuntimeLabel(value: string | undefined, fallback = 'Unknown') {
  if (!value) return fallback;
  return value
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/\b\w/g, character => character.toUpperCase());
}

export function formatActionTimestamp(value: string | number | undefined): string | undefined {
  if (!value) return undefined;
  const date = typeof value === 'number' ? new Date(value * 1000) : new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function formatActionDuration(durationMs: number | undefined): string | undefined {
  if (typeof durationMs !== 'number' || durationMs < 0) return undefined;
  if (durationMs < 1000) return `${Math.round(durationMs)}ms`;
  const seconds = durationMs / 1000;
  if (seconds < 60) return `${seconds.toFixed(seconds < 10 ? 1 : 0)}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = Math.round(seconds % 60);
  return remainingSeconds > 0 ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
}

export function getRiskLevel(
  risk: AIChatActionRisk | undefined,
  fallback: AIChatActionRiskLevel = 'none'
): AIChatActionRiskLevel {
  return risk?.level ?? fallback;
}

export function getHighestRisk(risks: Array<AIChatActionRisk | undefined>) {
  return risks
    .filter((risk): risk is AIChatActionRisk => Boolean(risk))
    .sort((left, right) => (RISK_RANK[right.level] ?? 0) - (RISK_RANK[left.level] ?? 0))[0];
}

export function getRiskTone(level: AIChatActionRiskLevel | undefined): OperationCardTone {
  if (level === 'critical' || level === 'high') return 'destructive';
  if (level === 'medium') return 'warning';
  if (level === 'low') return 'info';
  return 'neutral';
}

export function getRuntimeStatusTone(status: string | undefined): OperationCardTone {
  if (status === 'completed' || status === 'confirmed' || status === 'done') return 'success';
  if (status === 'failed' || status === 'canceled' || status === 'cancelled' || status === 'rejected') {
    return 'destructive';
  }
  if (status === 'blocked' || status === 'expired' || status === 'waiting_confirmation' || status === 'needs_confirmation') {
    return 'warning';
  }
  if (status === 'running' || status === 'planning' || status === 'queued' || status === 'planned') return 'info';
  return 'neutral';
}

export function isRuntimeStatusLoading(status: string | undefined) {
  return status === 'running' || status === 'planning' || status === 'queued';
}

export function getConfirmationTone(status: AIChatActionConfirmationStatus): OperationCardTone {
  if (status === 'confirmed') return 'success';
  if (status === 'rejected' || status === 'expired' || status === 'canceled' || status === 'cancelled') {
    return 'destructive';
  }
  if (status === 'pending') return 'warning';
  return 'neutral';
}

export function getStepStatusIcon(status: AIChatActionStepStatus | undefined): ReactNode {
  if (status === 'running') return <Loader2 className="size-3.5 animate-spin" />;
  if (status === 'completed' || status === 'done') return <CheckCircle2 className="size-3.5" />;
  if (status === 'failed' || status === 'blocked') return <AlertCircle className="size-3.5" />;
  if (status === 'skipped' || status === 'cancelled') return <CircleSlash className="size-3.5" />;
  if (status === 'waiting_confirmation') return <Clock className="size-3.5" />;
  if (status === 'pending') return <Circle className="size-3.5" />;
  return <CircleDashed className="size-3.5" />;
}

export function getConfirmationStatusForStep(
  step: Pick<
    AIChatActionPlanStep,
    'requires_confirmation' | 'confirmation_id' | 'confirmation_status'
  >,
  confirmation?: AIChatActionConfirmation
): AIChatActionConfirmationStatus {
  if (step.confirmation_status) return step.confirmation_status;
  if (confirmation?.status) return confirmation.status;
  if (step.requires_confirmation || step.confirmation_id) return 'pending';
  return 'not_required';
}

export function getAggregateConfirmationStatus(
  confirmations: AIChatActionConfirmation[],
  requiresConfirmation = false
): AIChatActionConfirmationStatus {
  if (confirmations.some(confirmation => confirmation.status === 'pending')) return 'pending';
  if (confirmations.some(confirmation => confirmation.status === 'rejected')) return 'rejected';
  if (confirmations.some(confirmation => confirmation.status === 'expired')) return 'expired';
  if (confirmations.some(confirmation => confirmation.status === 'cancelled')) return 'cancelled';
  if (confirmations.some(confirmation => confirmation.status === 'confirmed')) return 'confirmed';
  return requiresConfirmation ? 'pending' : 'not_required';
}

export function findStepConfirmation(
  step: Pick<AIChatActionPlanStep, 'id' | 'confirmation_id'>,
  confirmations: AIChatActionConfirmation[]
) {
  return confirmations.find(
    confirmation => confirmation.id === step.confirmation_id || confirmation.step_id === step.id
  );
}

export function summarizePermissions(permissions: AIChatActionPermission[] | undefined) {
  const visible = permissions ?? [];
  if (visible.length === 0) return undefined;
  const required = visible.filter(permission => permission.required !== false).length;
  const granted = visible.filter(permission => permission.granted).length;
  if (required === 0) return `${visible.length} optional`;
  if (granted > 0) return `${granted}/${required} granted`;
  return `${required} required`;
}

function formatDelegation(step: ActionRuntimeStepLike) {
  const delegation = step.delegated_to;
  if (!delegation) return undefined;
  return delegation.title ?? delegation.target_id ?? formatActionRuntimeLabel(delegation.target);
}

function formatTarget(step: ActionRuntimeStepLike) {
  const target = step.target;
  if (!target) return undefined;
  return target.title ?? `${target.resource_type}:${target.resource_id}`;
}

function buildStepMeta(step: ActionRuntimeStepLike): OperationCardMetaItem[] {
  const items: Array<OperationCardMetaItem | null> = [
    step.capability_id
      ? {
          id: `${step.id}:capability`,
          label: 'Capability',
          value: step.capability_id,
        }
      : null,
    formatDelegation(step)
      ? {
          id: `${step.id}:delegation`,
          label: 'Delegated to',
          value: formatDelegation(step),
        }
      : null,
    formatTarget(step)
      ? {
          id: `${step.id}:target`,
          label: 'Target',
          value: formatTarget(step),
        }
      : null,
    summarizePermissions(step.permissions)
      ? {
          id: `${step.id}:permissions`,
          label: 'Permissions',
          value: summarizePermissions(step.permissions),
        }
      : null,
    formatActionDuration(step.duration_ms)
      ? {
          id: `${step.id}:duration`,
          label: 'Duration',
          value: formatActionDuration(step.duration_ms),
        }
      : null,
  ];

  return items.filter((item): item is OperationCardMetaItem => Boolean(item));
}

export function ActionRiskBadge({
  risk,
  level,
  className,
}: {
  risk?: AIChatActionRisk;
  level?: AIChatActionRiskLevel;
  className?: string;
}) {
  const resolvedLevel = risk?.level ?? level ?? 'none';
  const tone = getRiskTone(resolvedLevel);
  const Icon =
    resolvedLevel === 'critical' || resolvedLevel === 'high'
      ? ShieldAlert
      : resolvedLevel === 'low'
        ? ShieldCheck
        : ShieldQuestion;

  return (
    <Badge variant={getToneBadgeVariant(tone)} className={cn('max-w-full', className)}>
      <Icon className="size-3 shrink-0" />
      <span className="min-w-0 truncate">Risk: {formatActionRuntimeLabel(resolvedLevel)}</span>
    </Badge>
  );
}

export function ActionRuntimeStatusBadge({
  status,
  label,
  className,
}: {
  status?: AIChatActionRunStatus | AIChatActionStepStatus | string;
  label?: string;
  className?: string;
}) {
  const tone = getRuntimeStatusTone(status);
  return (
    <span className={className}>
      <OperationStatusBadge
        label={label ?? formatActionRuntimeLabel(status, 'Unknown')}
        tone={tone}
        loading={isRuntimeStatusLoading(status)}
      />
    </span>
  );
}

export function ActionConfirmationBadge({
  status,
  className,
}: {
  status: AIChatActionConfirmationStatus;
  className?: string;
}) {
  return (
    <span className={className}>
      <OperationStatusBadge
        label={
          status === 'not_required'
            ? 'No confirmation'
            : formatActionRuntimeLabel(status, 'Confirmation')
        }
        tone={getConfirmationTone(status)}
      />
    </span>
  );
}

export function ActionRuntimeErrorBlock({
  error,
  className,
}: {
  error?: string | { code?: string; message: string; details?: string };
  className?: string;
}) {
  const normalized =
    typeof error === 'string'
      ? { message: error }
      : error;
  if (!normalized?.message) return null;

  return (
    <div
      className={cn(
        'rounded-md border px-3 py-2.5 text-xs leading-relaxed',
        getToneSoftClassName('destructive'),
        className
      )}
    >
      <div className="flex min-w-0 items-start gap-2">
        <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-destructive" />
        <div className="min-w-0 flex-1">
          <div className="break-words font-medium text-foreground">
            {normalized.code ? `${normalized.code}: ${normalized.message}` : normalized.message}
          </div>
          {normalized.details ? (
            <div className="mt-1 whitespace-pre-wrap break-words text-muted-foreground">
              {normalized.details}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}

export function ActionRuntimeStepList({
  steps,
  confirmations = [],
  activeStepId,
  emptyLabel = 'No steps yet.',
}: {
  steps: ActionRuntimeStepLike[];
  confirmations?: AIChatActionConfirmation[];
  activeStepId?: string;
  emptyLabel?: string;
}) {
  if (steps.length === 0) {
    return <div className="rounded-md border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">{emptyLabel}</div>;
  }

  return (
    <ol className="space-y-2">
      {steps.map((step, index) => {
        const stepStatus = step.status ?? 'pending';
        const statusTone = getRuntimeStatusTone(stepStatus);
        const confirmation = findStepConfirmation(step, confirmations);
        const confirmationStatus = getConfirmationStatusForStep(step, confirmation);
        const isActive = activeStepId === step.id;
        const stepMeta = buildStepMeta(step);

        return (
          <li
            key={step.id}
            className={cn(
              'grid grid-cols-[auto_minmax(0,1fr)] gap-3 rounded-md border px-3 py-2.5',
              getToneSoftClassName(statusTone),
              isActive && 'ring-1 ring-primary/30'
            )}
          >
            <div className="flex flex-col items-center gap-1">
              <span
                className={cn(
                  'flex size-6 items-center justify-center rounded-full border bg-background',
                  getToneTextClassName(statusTone)
                )}
              >
                {getStepStatusIcon(stepStatus)}
              </span>
              <span className="text-[10px] text-muted-foreground">{index + 1}</span>
            </div>
            <div className="min-w-0 space-y-2">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <div className="min-w-0 flex-1 break-words font-medium text-foreground">
                  {step.title}
                </div>
                <ActionRuntimeStatusBadge status={stepStatus} />
                <ActionRiskBadge risk={step.risk} />
                <ActionConfirmationBadge status={confirmationStatus} />
              </div>
              {step.description ? (
                <div className="whitespace-pre-wrap break-words text-xs leading-relaxed text-muted-foreground">
                  {step.description}
                </div>
              ) : null}
              <OperationMetaGrid items={stepMeta} compact />
              <ActionRuntimeErrorBlock error={step.error} />
            </div>
          </li>
        );
      })}
    </ol>
  );
}

export function ActionRuntimeConfirmationList({
  confirmations,
  isConfirming,
  onConfirm,
  onReject,
}: {
  confirmations: AIChatActionConfirmation[];
  isConfirming?: boolean;
  onConfirm?: (confirmation: AIChatActionConfirmation) => void;
  onReject?: (confirmation: AIChatActionConfirmation) => void;
}) {
  if (confirmations.length === 0) return null;

  return (
    <div className="space-y-2">
      {confirmations.map(confirmation => {
        const isPending = confirmation.status === 'pending';
        const canResolve = isPending && (onConfirm || onReject);

        return (
          <div key={confirmation.id} className="rounded-md border bg-background/80 px-3 py-2.5">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <div className="min-w-0 flex-1 break-words font-medium text-foreground">
                {confirmation.title ?? 'Confirmation required'}
              </div>
              <ActionConfirmationBadge status={confirmation.status} />
              {confirmation.risk ? <ActionRiskBadge risk={confirmation.risk} /> : null}
            </div>
            {confirmation.summary || confirmation.prompt ? (
              <div className="mt-2 whitespace-pre-wrap break-words text-xs leading-relaxed text-muted-foreground">
                {confirmation.summary ?? confirmation.prompt}
              </div>
            ) : null}
            {confirmation.expires_at ? (
              <div className="mt-2 text-[11px] text-muted-foreground">
                Expires {formatActionTimestamp(confirmation.expires_at)}
              </div>
            ) : null}
            {canResolve ? (
              <div className="mt-3 flex flex-wrap justify-end gap-2">
                {onReject ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="xs"
                    disabled={isConfirming}
                    onClick={() => onReject(confirmation)}
                  >
                    <X className="size-3.5" />
                    Reject
                  </Button>
                ) : null}
                {onConfirm ? (
                  <Button
                    type="button"
                    variant="default"
                    size="xs"
                    loading={isConfirming}
                    onClick={() => onConfirm(confirmation)}
                  >
                    <Check className="size-3.5" />
                    Confirm
                  </Button>
                ) : null}
              </div>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}
