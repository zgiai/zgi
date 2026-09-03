import { parseConnectorRuntimeExecutionResult } from './connector-runtime-model';

function record(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

/** Only a host's explicit server event is authority; provider/model JSON is not. */
export function parseConnectorProgressResult(item: { phase?: string; result?: unknown }) {
  const result = parseConnectorRuntimeExecutionResult(item.result);
  if (item.phase === 'connector_confirmation_required') {
    return result?.state === 'requires_confirmation' ? result : null;
  }
  if (
    item.phase === 'connector_confirmation_resolved' &&
    result &&
    ['completed', 'failed', 'cancelled', 'unknown'].includes(result.state)
  ) {
    return result;
  }
  return null;
}

export function parseConnectorNodeResult(metadata: unknown, value: unknown) {
  const tool = record(record(metadata)?.tool_info);
  if (
    tool?.provider_type !== 'connector' ||
    typeof tool.provider_id !== 'string' ||
    typeof tool.tool_name !== 'string'
  ) {
    return null;
  }
  return parseConnectorRuntimeExecutionResult(value);
}

export function connectorChatEvents(
  metadata: unknown,
  timeline: readonly unknown[],
  conversationID: string,
  messageID: string
) {
  const events = record(metadata)?.connector_confirmation_events;
  const rows = new Map<
    string,
    {
      receipt: string;
      result: NonNullable<ReturnType<typeof parseConnectorRuntimeExecutionResult>>;
      live: boolean;
    }
  >();
  if (Array.isArray(events)) {
    for (const value of events) {
      const event = record(value);
      if (
        !event ||
        event.conversation_id !== conversationID ||
        event.message_id !== messageID ||
        typeof event.route_receipt !== 'string'
      ) {
        continue;
      }
      const result = parseConnectorProgressResult({
        phase: String(event.event_type),
        result: event.result ?? {
          state: event.state,
          ...(event.decision_id !== undefined ? { decision_id: event.decision_id } : {}),
          ...(event.error_code !== undefined ? { error_code: event.error_code } : {}),
          ...(event.confirmation !== undefined ? { confirmation: event.confirmation } : {}),
        },
      });
      if (
        result?.state === 'requires_confirmation' &&
        result.confirmation.route_receipt !== event.route_receipt
      ) {
        continue;
      }
      const previous = rows.get(event.route_receipt);
      if (
        result &&
        !(
          previous &&
          previous.result.state !== 'requires_confirmation' &&
          result.state === 'requires_confirmation'
        )
      ) {
        rows.set(event.route_receipt, { receipt: event.route_receipt, result, live: false });
      }
    }
  }
  for (const value of timeline) {
    const event = record(value);
    if (!event || event.type !== 'progress_text') continue;
    const result = parseConnectorProgressResult({
      phase: String(event.phase),
      result: event.result,
    });
    const receipt =
      result?.state === 'requires_confirmation'
        ? result.confirmation.route_receipt
        : event.progress_id;
    if (!result || typeof receipt !== 'string' || !receipt) continue;
    const existing = rows.get(receipt);
    // A replayed pending event never replaces durable terminal evidence.
    if (existing && existing.result.state !== 'requires_confirmation') continue;
    rows.set(receipt, { receipt, result, live: true });
  }
  return [...rows.values()];
}

/** Do not suppress an unrelated approval on a message that also has Connector history. */
export function connectorOwnsWaitingState(metadata: unknown, timeline: readonly unknown[]) {
  const data = record(metadata);
  if (
    ['tool_governance_continuation', 'agent_workflow_continuation'].some(
      key => record(data?.[key])?.status === 'waiting_approval'
    )
  ) {
    return false;
  }
  const candidates: unknown[] = Array.isArray(data?.tool_governance_decisions)
    ? [...data.tool_governance_decisions]
    : [];
  for (const value of timeline) {
    const item = record(value);
    if (item?.type === 'tool_governance_decision') candidates.push(item.event);
    if (item?.type === 'workflow_run' && record(item.run)?.status === 'waiting_approval') {
      return false;
    }
  }
  const latest = new Map<unknown, Record<string, unknown>>();
  for (const value of candidates) {
    const event = record(value);
    if (event) latest.set(event.correlation_id ?? event, event);
  }
  return ![...latest.values()].some(event => {
    const governance = record(event.governance);
    const approval = event.approval_status ?? governance?.approval_status;
    const status = event.decision ?? event.status ?? governance?.status;
    if (
      event.execution_result ||
      ['approved', 'rejected', 'cancelled'].includes(String(approval)) ||
      ['allowed', 'denied', 'completed', 'failed'].includes(String(status))
    ) {
      return false;
    }
    return (
      approval === 'pending' ||
      status === 'needs_approval' ||
      governance?.requires_approval === true
    );
  });
}

/** Durable terminal evidence replaces only its own replayed progress row. */
export function withoutSupersededConnectorProgress<T>(
  timeline: readonly T[],
  events: ReturnType<typeof connectorChatEvents>
): T[] {
  const restored = new Set(events.filter(event => !event.live).map(event => event.receipt));
  return timeline.filter(value => {
    const item = record(value);
    if (item?.type !== 'progress_text') return true;
    const result = parseConnectorProgressResult({ phase: String(item.phase), result: item.result });
    const receipt =
      result?.state === 'requires_confirmation'
        ? result.confirmation.route_receipt
        : item.progress_id;
    return !result || typeof receipt !== 'string' || !restored.has(receipt);
  });
}
