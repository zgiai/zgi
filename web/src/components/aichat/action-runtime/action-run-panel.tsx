'use client';

import { Activity, CircleStop, ScrollText } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import type { OperationCardAction, OperationCardMetaItem } from '@/components/aichat/operation-cards';
import {
  OperationCardActions,
  OperationCardHeader,
  OperationCardShell,
  OperationMetaGrid,
} from '@/components/aichat/operation-cards/primitives';
import { cn } from '@/lib/utils';
import type {
  AIChatActionConfirmation,
  AIChatActionLedgerEntry,
  AIChatActionMetadata,
  AIChatActionPlan,
  AIChatActionRisk,
  AIChatActionRun,
} from '@/types/aichat-action-runtime';
import {
  ActionConfirmationBadge,
  ActionRiskBadge,
  ActionRuntimeConfirmationList,
  ActionRuntimeErrorBlock,
  ActionRuntimeStepList,
  ActionRuntimeStatusBadge,
  formatActionRuntimeLabel,
  formatActionTimestamp,
  getAggregateConfirmationStatus,
  getHighestRisk,
  summarizePermissions,
  type ActionRuntimeStepLike,
} from './utils';

export interface ActionRunPanelProps {
  run: AIChatActionRun;
  plan?: AIChatActionPlan;
  confirmations?: AIChatActionConfirmation[];
  actions?: OperationCardAction[];
  compact?: boolean;
  className?: string;
  isConfirming?: boolean;
  onConfirm?: (confirmation: AIChatActionConfirmation) => void;
  onReject?: (confirmation: AIChatActionConfirmation) => void;
  onCancel?: (run: AIChatActionRun) => void;
}

function mergeConfirmations(
  runConfirmations: AIChatActionConfirmation[] | undefined,
  planConfirmations: AIChatActionConfirmation[] | undefined,
  propConfirmations: AIChatActionConfirmation[] | undefined
) {
  const merged = new Map<string, AIChatActionConfirmation>();
  [...(planConfirmations ?? []), ...(runConfirmations ?? []), ...(propConfirmations ?? [])].forEach(
    confirmation => {
      merged.set(confirmation.id, confirmation);
    }
  );
  return Array.from(merged.values());
}

function getRunSteps(run: AIChatActionRun, plan?: AIChatActionPlan): ActionRuntimeStepLike[] {
  if (run.steps.length > 0) return run.steps;
  return plan?.steps ?? [];
}

function clampProgress(value: number) {
  return Math.min(100, Math.max(0, value));
}

function getRunProgressPercent(run: AIChatActionRun, steps: ActionRuntimeStepLike[]) {
  if (typeof run.progress?.percent === 'number') {
    return clampProgress(run.progress.percent);
  }

  const totalSteps = run.progress?.total_steps ?? steps.length;
  if (totalSteps <= 0) return undefined;
  const completedSteps =
    run.progress?.completed_steps ?? steps.filter(step => step.status === 'completed').length;
  return clampProgress((completedSteps / totalSteps) * 100);
}

function getCurrentStepTitle(run: AIChatActionRun, steps: ActionRuntimeStepLike[]) {
  const currentStepId = run.current_step_id ?? run.progress?.current_step_id;
  return (
    run.progress?.current_step_title ??
    steps.find(step => step.id === currentStepId)?.title ??
    undefined
  );
}

function getRunRisk(run: AIChatActionRun, plan: AIChatActionPlan | undefined, steps: ActionRuntimeStepLike[]) {
  return run.risk ?? plan?.risk ?? getHighestRisk(steps.map(step => step.risk));
}

function isTerminalRunStatus(status: AIChatActionRun['status']) {
  return status === 'completed' || status === 'failed' || status === 'canceled' || status === 'cancelled';
}

function buildRunMetaItems(
  run: AIChatActionRun,
  plan: AIChatActionPlan | undefined,
  confirmations: AIChatActionConfirmation[],
  steps: ActionRuntimeStepLike[]
): OperationCardMetaItem[] {
  const confirmationStatus = getAggregateConfirmationStatus(
    confirmations,
    Boolean(
      (run.risk ?? plan?.risk)?.requires_confirmation ||
        steps.some(step => step.requires_confirmation)
    )
  );
  const totalSteps = run.progress?.total_steps ?? steps.length;
  const completedSteps =
    run.progress?.completed_steps ?? steps.filter(step => step.status === 'completed').length;
  const permissionSummary = summarizePermissions([
    ...(run.permissions ?? []),
    ...steps.flatMap(step => step.permissions ?? []),
  ]);

  const items: Array<OperationCardMetaItem | null> = [
    {
      id: 'run-status',
      label: 'Run status',
      value: formatActionRuntimeLabel(run.status),
    },
    plan?.id || run.plan_id
      ? {
          id: 'plan-id',
          label: 'Plan',
          value: plan?.id ?? run.plan_id,
        }
      : null,
    {
      id: 'confirmation-status',
      label: 'Confirmation',
      value:
        confirmationStatus === 'not_required'
          ? 'Not required'
          : formatActionRuntimeLabel(confirmationStatus),
    },
    totalSteps > 0
      ? {
          id: 'step-progress',
          label: 'Steps',
          value: `${completedSteps}/${totalSteps} completed`,
        }
      : null,
    permissionSummary
      ? {
          id: 'permissions',
          label: 'Permissions',
          value: permissionSummary,
        }
      : null,
    run.started_at
      ? {
          id: 'started',
          label: 'Started',
          value: formatActionTimestamp(run.started_at),
        }
      : null,
    run.completed_at || run.updated_at
      ? {
          id: 'updated',
          label: run.completed_at ? 'Completed' : 'Updated',
          value: formatActionTimestamp(run.completed_at ?? run.updated_at),
        }
      : null,
  ];

  return items.filter((item): item is OperationCardMetaItem => Boolean(item));
}

function ActionRunProgress({
  run,
  steps,
}: {
  run: AIChatActionRun;
  steps: ActionRuntimeStepLike[];
}) {
  const progressPercent = getRunProgressPercent(run, steps);
  const currentStepTitle = getCurrentStepTitle(run, steps);
  const shouldShow =
    typeof progressPercent === 'number' ||
    Boolean(currentStepTitle) ||
    run.status === 'queued' ||
    run.status === 'running' ||
    run.status === 'planning' ||
    run.status === 'waiting_confirmation';

  if (!shouldShow) return null;

  return (
    <div className="rounded-md border bg-muted/20 px-3 py-2.5">
      <div className="flex min-w-0 items-center gap-2">
        <Activity className="size-4 shrink-0 text-primary" />
        <div className="min-w-0 flex-1">
          <div className="break-words font-medium text-foreground">
            {currentStepTitle ?? formatActionRuntimeLabel(run.status)}
          </div>
          <div className="mt-0.5 text-xs text-muted-foreground">
            {typeof progressPercent === 'number'
              ? `${Math.round(progressPercent)}% complete`
              : 'Waiting for runtime events'}
          </div>
        </div>
      </div>
      <div className="mt-2">
        <Progress value={progressPercent} className="h-1.5" />
      </div>
    </div>
  );
}

function actionLedgerEntries(value: AIChatActionLedgerEntry[] | AIChatActionMetadata | undefined) {
  if (!value) return [];
  if (Array.isArray(value)) return value;
  const status = typeof value.status === 'string' ? value.status : 'observed';
  return [
    {
      id: 'action-ledger-summary',
      type: status,
      title: 'Ledger summary',
      message: JSON.stringify(value),
      created_at: typeof value.created_at === 'string' || typeof value.created_at === 'number'
        ? value.created_at
        : undefined,
      metadata: value,
    },
  ];
}

function ActionLedgerList({
  entries,
}: {
  entries: AIChatActionLedgerEntry[] | AIChatActionMetadata | undefined;
}) {
  const visibleEntries = actionLedgerEntries(entries).slice(-6).reverse();
  if (visibleEntries.length === 0) return null;

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <ScrollText className="size-3.5" />
        Ledger
      </div>
      <div className="space-y-1.5">
        {visibleEntries.map(entry => (
          <div key={entry.id} className="rounded-md border bg-background/80 px-3 py-2 text-xs">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <Badge variant="subtle" className="max-w-[180px]">
                <span className="truncate">{formatActionRuntimeLabel(entry.type)}</span>
              </Badge>
              {entry.created_at ? (
                <span className="text-[11px] text-muted-foreground">
                  {formatActionTimestamp(entry.created_at)}
                </span>
              ) : null}
            </div>
            {entry.title || entry.message ? (
              <div className="mt-1 whitespace-pre-wrap break-words text-muted-foreground">
                {entry.title ?? entry.message}
              </div>
            ) : null}
          </div>
        ))}
      </div>
    </div>
  );
}

export function ActionRunPanel({
  run,
  plan,
  confirmations: confirmationOverride,
  actions,
  compact = false,
  className,
  isConfirming,
  onConfirm,
  onReject,
  onCancel,
}: ActionRunPanelProps) {
  const steps = getRunSteps(run, plan);
  const confirmations = mergeConfirmations(run.confirmations, plan?.confirmations, confirmationOverride);
  const risk: AIChatActionRisk | undefined = getRunRisk(run, plan, steps);
  const confirmationStatus = getAggregateConfirmationStatus(
    confirmations,
    Boolean(risk?.requires_confirmation || steps.some(step => step.requires_confirmation))
  );
  const runActions: OperationCardAction[] = [
    onCancel
      ? {
          id: 'cancel-action-run',
          label: 'Cancel',
          icon: <CircleStop className="size-3.5" />,
          variant: 'outline',
          disabled: isTerminalRunStatus(run.status),
          onClick: () => onCancel(run),
        }
      : null,
    ...(actions ?? []),
  ].filter((action): action is OperationCardAction => Boolean(action));

  return (
    <OperationCardShell compact={compact} className={cn('max-w-3xl', className)}>
      <OperationCardHeader
        compact={compact}
        icon={<Activity className="size-4 text-primary" />}
        eyebrow={`Action Run / ${run.id}`}
        title={run.title ?? plan?.title ?? 'Action run'}
        description={run.summary ?? plan?.summary ?? plan?.goal}
        badge={
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <ActionRuntimeStatusBadge status={run.status} />
            <ActionRiskBadge risk={risk} />
            <ActionConfirmationBadge status={confirmationStatus} />
          </div>
        }
      />

      <OperationMetaGrid items={buildRunMetaItems(run, plan, confirmations, steps)} compact={compact} />
      <ActionRunProgress run={run} steps={steps} />
      <ActionRuntimeErrorBlock error={run.error} />
      <ActionRuntimeStepList
        steps={steps}
        confirmations={confirmations}
        activeStepId={run.current_step_id ?? run.progress?.current_step_id}
      />
      <ActionRuntimeConfirmationList
        confirmations={confirmations}
        isConfirming={isConfirming}
        onConfirm={onConfirm}
        onReject={onReject}
      />
      <ActionLedgerList entries={run.ledger_entries ?? run.ledger ?? plan?.ledger_entries ?? plan?.ledger} />
      <OperationCardActions actions={runActions} compact={compact} />
    </OperationCardShell>
  );
}
