import type { AIChatPlanStep, AIChatPlanUpdatedEventData } from '@/services/types/aichat';
import { type AIChatControllerState } from '@/components/chat/controllers/aichat/types';
import { isStaleAIChatStreamEvent } from './shared';

/** Plans written as the model's checklist; older plans hold runtime phases. */
export const AICHAT_PLAN_CHECKLIST_VERSION = 'operation_plan.v3';

const PLAN_STEP_STATUSES: ReadonlySet<AIChatPlanStep['status']> = new Set([
  'pending',
  'in_progress',
  'completed',
  'skipped',
]);

function normalizePlanSteps(plan: unknown): AIChatPlanStep[] {
  if (!Array.isArray(plan)) return [];
  return plan.flatMap(item => {
    if (!item || typeof item !== 'object') return [];
    const record = item as Record<string, unknown>;
    const step = typeof record.step === 'string' ? record.step.trim() : '';
    const status = record.status as AIChatPlanStep['status'];
    if (!step || !PLAN_STEP_STATUSES.has(status)) return [];
    return [{ id: typeof record.id === 'string' ? record.id : undefined, step, status }];
  });
}

/** Stores the model's latest checklist on its message, where history also keeps it (ADR 0049). */
export function applyPlanUpdatedState(
  current: AIChatControllerState,
  payload: AIChatPlanUpdatedEventData,
  eventId?: string | null
): AIChatControllerState {
  const steps = normalizePlanSteps(payload.plan);
  if (steps.length === 0) return current;
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (isStaleAIChatStreamEvent(eventId, previousStreaming?.last_event_id)) return current;

  const messages = current.messagesByConversation[payload.conversation_id] ?? [];
  let changed = false;
  const nextMessages = messages.map(message => {
    if (message.id !== payload.message_id) return message;
    changed = true;
    return {
      ...message,
      metadata: {
        ...(message.metadata ?? {}),
        operation_plan: {
          ...(message.metadata?.operation_plan ?? {}),
          version: AICHAT_PLAN_CHECKLIST_VERSION,
          phases: steps,
        },
      },
    };
  });
  if (!changed) return current;
  return {
    ...current,
    messagesByConversation: { ...current.messagesByConversation, [payload.conversation_id]: nextMessages },
    streamingByMessageId: previousStreaming
      ? {
          ...current.streamingByMessageId,
          [payload.message_id]: {
            ...previousStreaming,
            last_event_id: eventId ?? previousStreaming.last_event_id,
          },
        }
      : current.streamingByMessageId,
  };
}

/** The checklist to show for a message, or none for a plan from before ADR 0049. */
export function messagePlanChecklist(plan: { phases?: unknown; version?: unknown } | undefined): AIChatPlanStep[] {
  if (!plan || plan.version !== AICHAT_PLAN_CHECKLIST_VERSION) return [];
  return normalizePlanSteps(plan.phases);
}
