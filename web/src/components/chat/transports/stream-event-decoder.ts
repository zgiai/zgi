import { debugAIChatTimeline } from '@/components/chat/controllers/aichat/debug';

/**
 * Stream events whose change belongs to one message. Without both the
 * conversation and the message id nothing can apply them, so they are
 * rejected here instead of in every handler (ADR 0050).
 *
 * `message_start` and `message_end` fall back to the stream's own
 * conversation, `error` may concern the whole request, and the client action
 * events are also read by the page that performs the action, so those are
 * decoded without the ids.
 */
const MESSAGE_SCOPED_EVENTS: ReadonlySet<string> = new Set([
  'agent_progress',
  'plan_updated',
  'agent_intermediate_answer',
  'user_input_requested',
  'file_parse_start',
  'file_parse_end',
  'file_parse_error',
  'skill_load_start',
  'skill_load_end',
  'skill_reference_read',
  'skill_call_start',
  'skill_call_end',
  'skill_call_error',
  'skill_artifact_created',
  'tool_governance_decision',
  'connector_confirmation_required',
  'connector_confirmation_resolved',
  'memory_create',
  'memory_update',
  'memory_delete',
  'memory_clear',
  'runtime_input_applied',
  'workflow_started',
  'workflow_resumed',
  'node_started',
  'node_finished',
  'iteration_started',
  'iteration_next',
  'iteration_completed',
  'iteration_succeeded',
  'iteration_failed',
  'loop_started',
  'loop_next',
  'loop_completed',
  'loop_succeeded',
  'loop_failed',
  'workflow_paused',
  'question_answer_requested',
  'approval_requested',
  'approval_result_filled',
  'approval_expired',
  'question_answer_submitted',
  'workflow_finished',
  'workflow_failed',
  'workflow_stopped',
  'message',
  'message_retract',
  'token_usage_updated',
  'token_budget_requested',
]);

export type StreamEventPayload = Record<string, unknown>;

function isPlainObject(value: unknown): value is StreamEventPayload {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function hasId(payload: StreamEventPayload, key: string): boolean {
  const value = payload[key];
  return typeof value === 'string' && value.trim() !== '';
}

/**
 * Checks one stream event's payload before any handler sees it. It returns
 * the payload unchanged, or `null` when the event cannot be applied; the
 * reason is recorded in the timeline debug log.
 */
export function decodeStreamEventPayload(event: string, data: unknown): StreamEventPayload | null {
  const scoped = MESSAGE_SCOPED_EVENTS.has(event);
  if (data === null || data === undefined) {
    if (!scoped) return {};
    debugAIChatTimeline('stream:rejected', { event, reason: 'no payload' });
    return null;
  }
  if (!isPlainObject(data)) {
    debugAIChatTimeline('stream:rejected', { event, reason: 'payload is not an object' });
    return null;
  }
  if (scoped && (!hasId(data, 'conversation_id') || !hasId(data, 'message_id'))) {
    debugAIChatTimeline('stream:rejected', {
      event,
      reason: 'missing conversation_id or message_id',
    });
    return null;
  }
  return data;
}

/** Whether the decoder knows the event as one scoped to a message. */
export function isMessageScopedStreamEvent(event: string): boolean {
  return MESSAGE_SCOPED_EVENTS.has(event);
}
