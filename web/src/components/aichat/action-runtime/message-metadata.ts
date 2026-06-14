import type { AIChatMessageMetadata } from '@/services/types/aichat';
import type {
  AIChatActionCapability,
  AIChatActionConfirmation,
  AIChatActionPlan,
  AIChatActionRun,
} from '@/types/aichat-action-runtime';

interface ActionRuntimeMessagePayload {
  plan?: AIChatActionPlan;
  run?: AIChatActionRun;
  capabilities?: AIChatActionCapability[];
  confirmations?: AIChatActionConfirmation[];
}

export interface AIChatActionRuntimeMessagePanel extends ActionRuntimeMessagePayload {
  id: string;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function recordValue(record: Record<string, unknown>, keys: string[]): unknown {
  for (const key of keys) {
    const value = record[key];
    if (value !== undefined) return value;
  }
  return undefined;
}

function recordArrayValue(record: Record<string, unknown>, keys: string[]): unknown[] {
  for (const key of keys) {
    const value = record[key];
    if (Array.isArray(value)) {
      return value;
    }
  }
  return [];
}

function isActionRuntimePayloadRoot(metadata: AIChatMessageMetadata): boolean {
  return (
    metadata.type === 'action_runtime' ||
    metadata.message_type === 'action_runtime' ||
    metadata.kind === 'action_runtime'
  );
}

function getRuntimeRoot(metadata: AIChatMessageMetadata | undefined): Record<string, unknown> | undefined {
  if (!metadata) return undefined;
  const nested = recordValue(metadata, ['action_runtime', 'actionRuntime']);
  if (isRecord(nested)) return nested;
  return isActionRuntimePayloadRoot(metadata) ? metadata : undefined;
}

function isActionPlan(value: unknown): value is AIChatActionPlan {
  return Boolean(
    isRecord(value) &&
      typeof value.id === 'string' &&
      typeof value.status === 'string' &&
      typeof value.title === 'string' &&
      Array.isArray(value.steps)
  );
}

function isActionRun(value: unknown): value is AIChatActionRun {
  return Boolean(
    isRecord(value) &&
      typeof value.id === 'string' &&
      typeof value.status === 'string' &&
      typeof value.title === 'string' &&
      Array.isArray(value.steps)
  );
}

function isActionCapability(value: unknown): value is AIChatActionCapability {
  return isRecord(value) && typeof value.id === 'string' && typeof value.name === 'string';
}

function isActionConfirmation(value: unknown): value is AIChatActionConfirmation {
  return isRecord(value) && typeof value.id === 'string' && typeof value.status === 'string';
}

function getCapabilities(root: Record<string, unknown>): AIChatActionCapability[] | undefined {
  const capabilities = recordArrayValue(root, ['capabilities', 'action_capabilities', 'actionCapabilities'])
    .filter(isActionCapability);
  return capabilities.length > 0 ? capabilities : undefined;
}

function getConfirmations(
  root: Record<string, unknown>
): AIChatActionConfirmation[] | undefined {
  const confirmations = recordArrayValue(root, ['confirmations', 'action_confirmations', 'actionConfirmations'])
    .filter(isActionConfirmation);
  return confirmations.length > 0 ? confirmations : undefined;
}

function firstActionPlan(root: Record<string, unknown>): AIChatActionPlan | undefined {
  const direct = recordValue(root, ['plan', 'action_plan', 'actionPlan']);
  if (isActionPlan(direct)) return direct;
  return recordArrayValue(root, ['plans', 'action_plans', 'actionPlans']).find(isActionPlan);
}

function firstActionRun(root: Record<string, unknown>): AIChatActionRun | undefined {
  const direct = recordValue(root, ['run', 'action_run', 'actionRun']);
  if (isActionRun(direct)) return direct;
  return recordArrayValue(root, ['runs', 'action_runs', 'actionRuns']).find(isActionRun);
}

export function resolveAIChatActionRuntimeMessagePanels(
  metadata: AIChatMessageMetadata | undefined
): AIChatActionRuntimeMessagePanel[] {
  const root = getRuntimeRoot(metadata);
  if (!root) return [];

  const capabilities = getCapabilities(root);
  const confirmations = getConfirmations(root);
  const groupedPanel: ActionRuntimeMessagePayload = {
    plan: firstActionPlan(root),
    run: firstActionRun(root),
    capabilities,
    confirmations,
  };

  const groupedPanels: AIChatActionRuntimeMessagePanel[] =
    groupedPanel.plan || groupedPanel.run
      ? [
          {
            id: [groupedPanel.plan?.id, groupedPanel.run?.id]
              .filter((id): id is string => Boolean(id))
              .join(':'),
            ...groupedPanel,
          },
        ]
      : [];

  const planPanels = recordArrayValue(root, ['plans', 'action_plans', 'actionPlans'])
    .filter(isActionPlan)
    .map(plan => ({
      id: plan.id,
      plan,
      capabilities,
      confirmations,
    }));
  const runPanels = recordArrayValue(root, ['runs', 'action_runs', 'actionRuns'])
    .filter(isActionRun)
    .map(run => ({
      id: run.id,
      run,
      capabilities,
      confirmations,
    }));

  const panelsById = new Map<string, AIChatActionRuntimeMessagePanel>();
  [...groupedPanels, ...planPanels, ...runPanels].forEach(panel => {
    if (panel.id) panelsById.set(panel.id, panel);
  });
  return Array.from(panelsById.values());
}
