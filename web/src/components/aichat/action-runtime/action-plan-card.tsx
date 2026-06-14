'use client';

import { ListChecks, Play } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import type { OperationCardAction, OperationCardMetaItem } from '@/components/aichat/operation-cards';
import {
  OperationCardActions,
  OperationCardHeader,
  OperationCardShell,
  OperationMetaGrid,
  getToneSoftClassName,
} from '@/components/aichat/operation-cards/primitives';
import { cn } from '@/lib/utils';
import type {
  AIChatActionCapability,
  AIChatActionConfirmation,
  AIChatActionPlan,
  AIChatActionRun,
} from '@/types/aichat-action-runtime';
import {
  ActionConfirmationBadge,
  ActionRiskBadge,
  ActionRuntimeConfirmationList,
  ActionRuntimeStepList,
  ActionRuntimeStatusBadge,
  formatActionRuntimeLabel,
  formatActionTimestamp,
  getAggregateConfirmationStatus,
  getRiskTone,
  summarizePermissions,
  type ActionRuntimeStepLike,
} from './utils';

export interface ActionPlanCardProps {
  plan: AIChatActionPlan;
  run?: AIChatActionRun;
  capabilities?: AIChatActionCapability[];
  confirmations?: AIChatActionConfirmation[];
  actions?: OperationCardAction[];
  compact?: boolean;
  className?: string;
  isConfirming?: boolean;
  onStart?: (plan: AIChatActionPlan) => void;
  onConfirm?: (confirmation: AIChatActionConfirmation) => void;
  onReject?: (confirmation: AIChatActionConfirmation) => void;
}

function mergeConfirmations(
  planConfirmations: AIChatActionConfirmation[] | undefined,
  runConfirmations: AIChatActionConfirmation[] | undefined,
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

function mergePlanAndRunSteps(plan: AIChatActionPlan, run?: AIChatActionRun) {
  const runStepsById = new Map((run?.steps ?? []).map(step => [step.id, step]));
  const merged: ActionRuntimeStepLike[] = plan.steps.map(step => {
    const runStep = runStepsById.get(step.id);
    if (!runStep) return step;
    return {
      ...step,
      ...runStep,
      title: runStep.title || step.title,
      description: runStep.description ?? step.description,
      risk: runStep.risk ?? step.risk,
      permissions: runStep.permissions ?? step.permissions,
    };
  });

  const planStepIds = new Set(plan.steps.map(step => step.id));
  const extraRunSteps = (run?.steps ?? []).filter(step => !planStepIds.has(step.id));
  return [...merged, ...extraRunSteps];
}

function buildPlanMetaItems(
  plan: AIChatActionPlan,
  run: AIChatActionRun | undefined,
  confirmations: AIChatActionConfirmation[],
  capabilities: AIChatActionCapability[]
): OperationCardMetaItem[] {
  const completedSteps = (run?.steps ?? plan.steps).filter(
    step => step.status === 'completed' || step.status === 'done'
  ).length;
  const totalSteps = Math.max(plan.steps.length, run?.steps.length ?? 0);
  const risk = getPlanRisk(plan);
  const confirmationStatus = getAggregateConfirmationStatus(
    confirmations,
    Boolean(risk.requires_confirmation || plan.steps.some(step => step.requires_confirmation))
  );
  const permissionSummary = summarizePermissions([
    ...(plan.permissions ?? []),
    ...plan.steps.flatMap(step => step.permissions ?? []),
  ]);

  const items: Array<OperationCardMetaItem | null> = [
    {
      id: 'plan-status',
      label: 'Plan status',
      value: formatActionRuntimeLabel(plan.status),
    },
    {
      id: 'execution-status',
      label: 'Execution',
      value: formatActionRuntimeLabel(run?.status ?? plan.status),
    },
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
    capabilities.length > 0
      ? {
          id: 'capabilities',
          label: 'Capabilities',
          value: String(capabilities.length),
        }
      : null,
    permissionSummary
      ? {
          id: 'permissions',
          label: 'Permissions',
          value: permissionSummary,
        }
      : null,
    plan.updated_at || run?.updated_at
      ? {
          id: 'updated',
          label: 'Updated',
          value: formatActionTimestamp(run?.updated_at ?? plan.updated_at),
        }
      : null,
  ];

  return items.filter((item): item is OperationCardMetaItem => Boolean(item));
}

function getPlanRisk(plan: AIChatActionPlan) {
  return (
    plan.risk ?? {
      level: plan.risk_level,
      requires_confirmation: plan.requires_confirmation,
    }
  );
}

function ActionPlanRiskSummary({ plan }: { plan: AIChatActionPlan }) {
  const risk = getPlanRisk(plan);
  const tone = getRiskTone(risk.level);
  const reasons = risk.reasons ?? [];
  const mitigations = risk.mitigations ?? [];

  if (!risk.summary && reasons.length === 0 && mitigations.length === 0) return null;

  return (
    <div className={cn('rounded-md border px-3 py-2.5 text-xs', getToneSoftClassName(tone))}>
      {risk.summary ? (
        <div className="whitespace-pre-wrap break-words font-medium text-foreground">
          {risk.summary}
        </div>
      ) : null}
      {reasons.length > 0 ? (
        <div className="mt-2 space-y-1 text-muted-foreground">
          {reasons.map(reason => (
            <div key={reason} className="break-words">
              {reason}
            </div>
          ))}
        </div>
      ) : null}
      {mitigations.length > 0 ? (
        <div className="mt-2 space-y-1 text-muted-foreground">
          {mitigations.map(mitigation => (
            <div key={mitigation} className="break-words">
              {mitigation}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function ActionCapabilityChips({ capabilities }: { capabilities: AIChatActionCapability[] }) {
  if (capabilities.length === 0) return null;

  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1.5">
      {capabilities.slice(0, 6).map(capability => (
        <Badge key={capability.id} variant="outline" className="max-w-[220px] gap-1">
          <span className="shrink-0 text-muted-foreground">
            {formatActionRuntimeLabel(capability.kind ?? capability.domain)}
          </span>
          <span className="min-w-0 truncate">{capability.title ?? capability.name}</span>
        </Badge>
      ))}
      {capabilities.length > 6 ? <Badge variant="subtle">+{capabilities.length - 6}</Badge> : null}
    </div>
  );
}

export function ActionPlanCard({
  plan,
  run,
  capabilities: capabilityOverride,
  confirmations: confirmationOverride,
  actions,
  compact = false,
  className,
  isConfirming,
  onStart,
  onConfirm,
  onReject,
}: ActionPlanCardProps) {
  const capabilities = capabilityOverride ?? plan.capabilities ?? [];
  const confirmations = mergeConfirmations(plan.confirmations, run?.confirmations, confirmationOverride);
  const risk = getPlanRisk(plan);
  const confirmationStatus = getAggregateConfirmationStatus(
    confirmations,
    Boolean(risk.requires_confirmation || plan.steps.some(step => step.requires_confirmation))
  );
  const steps = mergePlanAndRunSteps(plan, run);
  const planActions: OperationCardAction[] = [
    onStart
      ? {
          id: 'start-action-plan',
          label: 'Start',
          icon: <Play className="size-3.5" />,
          variant: 'default',
          disabled: plan.status === 'running' || run?.status === 'running',
          onClick: () => onStart(plan),
        }
      : null,
    ...(actions ?? []),
  ].filter((action): action is OperationCardAction => Boolean(action));

  return (
    <OperationCardShell compact={compact} className={className}>
      <OperationCardHeader
        compact={compact}
        icon={<ListChecks className="size-4 text-primary" />}
        eyebrow={`Action Runtime / ${plan.id}`}
        title={plan.title || 'Action plan'}
        description={plan.summary ?? plan.goal}
        badge={
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <ActionRuntimeStatusBadge status={run?.status ?? plan.status} />
            <ActionRiskBadge risk={risk} />
            <ActionConfirmationBadge status={confirmationStatus} />
          </div>
        }
      />

      <OperationMetaGrid
        items={buildPlanMetaItems(plan, run, confirmations, capabilities)}
        compact={compact}
      />

      <ActionCapabilityChips capabilities={capabilities} />
      <ActionPlanRiskSummary plan={plan} />
      <ActionRuntimeStepList
        steps={steps}
        confirmations={confirmations}
        activeStepId={run?.current_step_id ?? run?.progress?.current_step_id}
      />
      <ActionRuntimeConfirmationList
        confirmations={confirmations}
        isConfirming={isConfirming}
        onConfirm={onConfirm}
        onReject={onReject}
      />
      <OperationCardActions actions={planActions} compact={compact} />
    </OperationCardShell>
  );
}
