import type {
  AIChatAgentProgressEventData,
  AIChatMemoryMutationEventData,
  AIChatRuntimeInputAppliedEventData,
  AIChatSkillCallEndEventData,
  AIChatSkillCallErrorEventData,
  AIChatSkillCallStartEventData,
  AIChatSkillActivityStatus,
  AIChatSkillInvocation,
  AIChatSkillLoadEndEventData,
  AIChatSkillLoadStartEventData,
  AIChatSkillReferenceReadEventData,
  AIChatToolGovernanceDecisionEventData,
} from '@/services/types/aichat';
import {
  type AIChatControllerState,
  type AIChatAgenticTimelineItem,
} from '@/components/chat/controllers/aichat/types';
import type { AIChatPresentationPosition } from '@/services/types/aichat';
import {
  findMatchingInvocationIndex,
  hasToolGovernanceDecision,
  invocationIdsConflict,
  isRecordedSkillInvocation,
  isStaleAIChatStreamEvent,
  mergeSkillInvocationByStatus,
  preferCompleteIntermediateAnswerContent,
  removeTransientProgressItems,
  skillInvocationSemanticIdentity,
  upsertMatchingInvocation,
} from './shared';
import { governanceDecisionStatus } from '../status';
import {
  captureAnswerTimelineBoundary,
  presentationPositionFromPayload,
} from '../presentation-order';
import { modelProcessingStateFromEvent } from '../model-processing';
import { applyRuntimeInputApplication } from '@/components/chat/utils/runtime-input-reconcile';
import { sanitizeTimelineForMessage } from '../selectors';
import {
  connectorReceiptFromTimelineItem,
  isPendingConnectorSkillEvent,
  parseConnectorProgressResult,
} from '@/components/connectors/runtime/connector-runtime-source';

function preserveLegacyAnswerBoundary(
  previousStreaming: AIChatControllerState['streamingByMessageId'][string]
): number | undefined {
  return previousStreaming.presentationVersion === 2
    ? previousStreaming.answer_before_timeline_length
    : captureAnswerTimelineBoundary(
        previousStreaming.answer_before_timeline_length,
        previousStreaming.answer
      );
}

function upsertSkillInvocation(
  invocations: AIChatSkillInvocation[],
  incoming: AIChatSkillInvocation
): AIChatSkillInvocation[] {
  const matched = upsertMatchingInvocation(invocations, incoming);
  if (matched) return matched;

  if (incoming.kind === 'intermediate_answer' && incoming.answer_id) {
    return [...invocations, incoming];
  }

  if (incoming.kind === 'guardrail') {
    const index = invocations.findIndex(
      invocation =>
        invocation.kind === 'guardrail' &&
        invocation.skill_id === incoming.skill_id &&
        (invocation.tool_name ?? '') === (incoming.tool_name ?? '') &&
        (invocation.message ?? invocation.error ?? '') ===
          (incoming.message ?? incoming.error ?? '')
    );
    if (index >= 0) {
      const next = invocations.slice();
      next[index] = {
        ...next[index],
        ...incoming,
      };
      return next;
    }
  }

  const next = invocations.slice();
  const incomingKind = incoming.kind ?? 'tool_call';
  const incomingToolName = incoming.tool_name ?? '';
  const incomingPath = incoming.path ?? '';
  const index = [...next].reverse().findIndex(invocation => {
    if (invocationIdsConflict(invocation, incoming)) {
      return false;
    }
    const invocationKind = invocation.kind ?? 'tool_call';
    const sameIdentity =
      invocationKind === incomingKind &&
      invocation.skill_id === incoming.skill_id &&
      (invocation.tool_name ?? '') === incomingToolName &&
      (invocation.path ?? '') === incomingPath;

    return sameIdentity && (invocation.status === 'loading' || invocation.status === 'running');
  });

  if (index < 0) {
    return [...next, incoming];
  }

  const actualIndex = next.length - 1 - index;
  next[actualIndex] = {
    ...next[actualIndex],
    ...incoming,
  };
  return next;
}

function hasSkillInvocationIdentity(invocation: AIChatSkillInvocation): boolean {
  return Boolean(
    invocation.invocation_id || invocation.runtime_id || skillInvocationSemanticIdentity(invocation)
  );
}

function skillInvocationsMatch(
  existing: AIChatSkillInvocation,
  incoming: AIChatSkillInvocation
): boolean {
  if (findMatchingInvocationIndex([existing], incoming) === 0) return true;
  // Records that name no call match by the tool they ran.
  return (
    !hasSkillInvocationIdentity(existing) &&
    !hasSkillInvocationIdentity(incoming) &&
    getSkillInvocationBaseIdentity(existing) === getSkillInvocationBaseIdentity(incoming)
  );
}

function getSkillInvocationBaseIdentity(invocation: AIChatSkillInvocation): string {
  return [
    invocation.kind ?? 'tool_call',
    invocation.skill_id ?? '',
    invocation.tool_name ?? '',
    invocation.path ?? '',
  ].join(':');
}

function upsertMemoryTimelineItem(
  timeline: AIChatAgenticTimelineItem[] | undefined,
  payload: AIChatMemoryMutationEventData,
  eventId: string | null | undefined
): AIChatAgenticTimelineItem[] {
  const baseTimeline = removeTransientProgressItems(timeline);
  const presentationPosition = presentationPositionFromPayload(payload);
  const isAutomaticAgentMemory =
    payload.memory_scope === 'agent' && payload.source_kind === 'automatic';
  if (isAutomaticAgentMemory) {
    const itemId = `memory-auto-${payload.message_id}`;
    const existingIndex = baseTimeline.findIndex(
      item => item.type === 'memory_event' && item.id === itemId
    );
    const existing = existingIndex >= 0 ? baseTimeline[existingIndex] : undefined;
    const existingKeys = existing?.type === 'memory_event' ? (existing.event.keys ?? []) : [];
    const keys = Array.from(
      new Set([...existingKeys, ...(payload.key ? [payload.key] : [])].filter(Boolean))
    );
    const merged: AIChatAgenticTimelineItem = {
      id: itemId,
      type: 'memory_event',
      event: {
        ...(existing?.type === 'memory_event' ? existing.event : {}),
        ...payload,
        keys,
        operation_count: keys.length,
      },
      created_at: payload.created_at ?? existing?.created_at,
      event_id: eventId ?? (existing && 'event_id' in existing ? existing.event_id : null),
      ...presentationPosition,
    };
    if (existingIndex < 0) return [...baseTimeline, merged];
    return baseTimeline.map((item, index) => (index === existingIndex ? merged : item));
  }
  const itemId =
    eventId ??
    `memory-${payload.action}-${payload.entry_id ?? 'entry'}-${payload.created_at ?? Date.now()}`;
  if (baseTimeline.some(item => 'event_id' in item && item.event_id === eventId && eventId)) {
    return baseTimeline;
  }
  return [
    ...baseTimeline,
    {
      id: itemId,
      type: 'memory_event',
      event: payload,
      created_at: payload.created_at,
      event_id: eventId ?? null,
      ...presentationPosition,
    },
  ];
}

function governanceString(value: unknown): string | undefined {
  if (typeof value === 'string' && value.trim()) return value.trim();
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return undefined;
}

function governanceRecord(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  return value as Record<string, unknown>;
}

function governanceAssetOperationAudit(
  payload: AIChatToolGovernanceDecisionEventData
): AIChatToolGovernanceDecisionEventData['asset_operation_audit'] {
  return (
    payload.asset_operation_audit ??
    payload.governance?.asset_operation_audit ??
    (governanceRecord(payload.model_feedback?.asset_operation_audit) as
      | AIChatToolGovernanceDecisionEventData['asset_operation_audit']
      | undefined)
  );
}

function toolGovernanceCorrelationId(
  payload: AIChatToolGovernanceDecisionEventData
): string | undefined {
  const assetOperationAudit = governanceAssetOperationAudit(payload);
  return (
    governanceString(payload.correlation_id) ??
    governanceString(payload.governance?.correlation_id) ??
    governanceString(payload.approval_event?.correlation_id) ??
    governanceString(payload.governance?.approval_event?.correlation_id) ??
    governanceString(assetOperationAudit?.correlation_id)
  );
}

function normalizeToolGovernanceDecisionPayload(
  payload: AIChatToolGovernanceDecisionEventData
): AIChatToolGovernanceDecisionEventData {
  const governance = payload.governance;
  const approvalEvent = payload.approval_event ?? governance?.approval_event;
  const approvalResult = payload.approval_result ?? governance?.approval_result;
  const modelFeedback =
    payload.model_feedback ??
    governance?.model_feedback ??
    (approvalResult?.model_feedback as Record<string, unknown> | undefined);
  const assetOperationAudit = governanceAssetOperationAudit({
    ...payload,
    model_feedback: modelFeedback,
  });
  const approvalStatus =
    payload.approval_status ??
    governance?.approval_status ??
    (approvalResult?.approval_status as AIChatToolGovernanceDecisionEventData['approval_status']);
  const decision = approvalStatus ?? payload.decision ?? governance?.status ?? payload.status;
  const status = approvalStatus ?? payload.status ?? governance?.status ?? payload.decision;
  return {
    ...payload,
    correlation_id:
      payload.correlation_id ?? governance?.correlation_id ?? assetOperationAudit?.correlation_id,
    status,
    decision,
    requires_approval: payload.requires_approval ?? governance?.requires_approval,
    reason: payload.reason ?? governance?.reason,
    risk_level: payload.risk_level ?? governance?.manifest?.risk_level ?? approvalEvent?.risk_level,
    effect: payload.effect ?? governance?.manifest?.effect ?? approvalEvent?.effect,
    asset_type: payload.asset_type ?? governance?.manifest?.asset_type ?? approvalEvent?.asset_type,
    asset_operation_audit: assetOperationAudit,
    approval_status: approvalStatus,
    approval_event: approvalEvent,
    matched_grant: payload.matched_grant ?? governance?.matched_grant,
    approval_result: approvalResult,
    model_feedback: modelFeedback,
    session_grant:
      payload.session_grant ??
      (approvalResult?.session_grant as Record<string, unknown> | undefined),
  };
}

function upsertToolGovernanceTimelineItem(
  timeline: AIChatAgenticTimelineItem[] | undefined,
  payload: AIChatToolGovernanceDecisionEventData,
  eventId: string | null | undefined
): AIChatAgenticTimelineItem[] {
  const baseTimeline = removeTransientProgressItems(timeline);
  const normalizedPayload = normalizeToolGovernanceDecisionPayload(payload);
  const presentationPosition = presentationPositionFromPayload(payload);
  if (baseTimeline.some(item => 'event_id' in item && item.event_id === eventId && eventId)) {
    return baseTimeline;
  }

  const correlationId = toolGovernanceCorrelationId(normalizedPayload);
  const existingIndex = correlationId
    ? baseTimeline.findIndex(
        item =>
          item.type === 'tool_governance_decision' &&
          toolGovernanceCorrelationId(item.event) === correlationId
      )
    : -1;
  if (existingIndex >= 0) {
    const next = baseTimeline.slice();
    const existing = next[existingIndex];
    if (existing.type !== 'tool_governance_decision') return next;
    next[existingIndex] = {
      ...existing,
      event: {
        ...existing.event,
        ...normalizedPayload,
        governance: normalizedPayload.governance
          ? {
              ...(existing.event.governance ?? {}),
              ...normalizedPayload.governance,
            }
          : existing.event.governance,
      },
      created_at: normalizedPayload.created_at ?? existing.created_at,
      event_id: eventId ?? existing.event_id,
      ...presentationPosition,
    };
    return next;
  }

  return [
    ...baseTimeline,
    {
      id:
        eventId ??
        `governance-${correlationId ?? normalizedPayload.skill_id ?? 'tool'}-${normalizedPayload.created_at ?? Date.now()}-${baseTimeline.length}`,
      type: 'tool_governance_decision',
      event: normalizedPayload,
      created_at: normalizedPayload.created_at,
      event_id: eventId ?? null,
      ...presentationPosition,
    },
  ];
}

function toolGovernanceInvocationStatus(
  payload: AIChatToolGovernanceDecisionEventData
): AIChatSkillActivityStatus {
  const decision = governanceDecisionStatus(
    payload.decision ?? payload.status,
    payload.approval_status
  );
  if (decision) return decision;
  // A decision word the client does not know asks nothing of the person
  // unless the event itself says approval is required (ADR 0050).
  return payload.requires_approval === true ? 'needs_approval' : 'advisory';
}

function skillInvocationFromToolGovernanceDecision(
  payload: AIChatToolGovernanceDecisionEventData
): AIChatSkillInvocation {
  const normalizedPayload = normalizeToolGovernanceDecisionPayload(payload);
  const skillId =
    normalizedPayload.skill_id ??
    normalizedPayload.approval_event?.skill_id ??
    normalizedPayload.governance?.manifest?.skill_id ??
    'tool-governance';
  const toolName =
    normalizedPayload.tool_name ??
    normalizedPayload.approval_event?.tool_id ??
    normalizedPayload.governance?.manifest?.tool_id;

  return {
    kind: 'tool_governance',
    // The decision names its call; without the id the stored copy of the
    // same decision would become a second row.
    ...(normalizedPayload.runtime_id ? { runtime_id: normalizedPayload.runtime_id } : {}),
    ...(normalizedPayload.invocation_id ? { invocation_id: normalizedPayload.invocation_id } : {}),
    skill_id: skillId,
    tool_name: toolName,
    status: toolGovernanceInvocationStatus(normalizedPayload),
    duration_ms: normalizedPayload.duration_ms,
    approval_status: normalizedPayload.approval_status,
    governance: {
      ...(normalizedPayload.governance ?? {}),
      ...(normalizedPayload.decision ? { status: normalizedPayload.decision } : {}),
      ...(normalizedPayload.correlation_id
        ? { correlation_id: normalizedPayload.correlation_id }
        : {}),
      ...(normalizedPayload.requires_approval !== undefined
        ? { requires_approval: normalizedPayload.requires_approval }
        : {}),
      ...(normalizedPayload.reason ? { reason: normalizedPayload.reason } : {}),
      ...(normalizedPayload.approval_status
        ? { approval_status: normalizedPayload.approval_status }
        : {}),
      ...(normalizedPayload.approval_event
        ? { approval_event: normalizedPayload.approval_event }
        : {}),
      ...(normalizedPayload.asset_operation_audit
        ? { asset_operation_audit: normalizedPayload.asset_operation_audit }
        : {}),
      ...(normalizedPayload.matched_grant
        ? { matched_grant: normalizedPayload.matched_grant }
        : {}),
      ...(normalizedPayload.approval_result
        ? { approval_result: normalizedPayload.approval_result }
        : {}),
      ...(normalizedPayload.model_feedback
        ? { model_feedback: normalizedPayload.model_feedback }
        : {}),
    },
    asset_operation_audit: normalizedPayload.asset_operation_audit,
    result: normalizedPayload.execution_result ?? null,
    message: normalizedPayload.execution_message,
    created_at: normalizedPayload.created_at,
  };
}

function upsertSkillTimelineItem(
  timeline: AIChatAgenticTimelineItem[] | undefined,
  incoming: AIChatSkillInvocation,
  eventId: string | null | undefined,
  presentationPosition?: AIChatPresentationPosition
): AIChatAgenticTimelineItem[] {
  const baseTimeline = removeTransientProgressItems(timeline);

  if (incoming.kind === 'intermediate_answer') {
    const existingIndex = incoming.answer_id
      ? baseTimeline.findIndex(
          item => item.type === 'intermediate_answer' && item.answer_id === incoming.answer_id
        )
      : -1;

    if (existingIndex >= 0) {
      const next = baseTimeline.slice();
      const existing = next[existingIndex];
      if (existing.type !== 'intermediate_answer') return next;

      next[existingIndex] = {
        ...existing,
        title: incoming.title ?? existing.title,
        content: preferCompleteIntermediateAnswerContent(existing.content, incoming.message),
        status: incoming.status === 'success' ? 'success' : 'streaming',
        created_at: existing.created_at ?? incoming.created_at,
        event_id: eventId ?? existing.event_id,
        ...presentationPosition,
      };
      return next;
    }

    return [
      ...baseTimeline,
      {
        id:
          incoming.answer_id ||
          eventId ||
          `intermediate-${incoming.created_at ?? Date.now()}-${baseTimeline.length}`,
        type: 'intermediate_answer',
        answer_id: incoming.answer_id,
        title: incoming.title,
        content: incoming.message ?? '',
        status: incoming.status === 'success' ? 'success' : 'streaming',
        created_at: incoming.created_at,
        event_id: eventId ?? null,
        ...presentationPosition,
      },
    ];
  }

  const next = baseTimeline.slice();
  const incomingBaseIdentity = getSkillInvocationBaseIdentity(incoming);
  const existingStableIndex = next.findIndex(
    item => item.type === 'skill_event' && skillInvocationsMatch(item.invocation, incoming)
  );
  if (existingStableIndex >= 0) {
    const existing = next[existingStableIndex];
    if (existing.type !== 'skill_event') return next;
    next[existingStableIndex] = {
      ...existing,
      invocation: mergeSkillInvocationByStatus(existing.invocation, incoming),
      created_at: incoming.created_at ?? existing.created_at,
      event_id: eventId ?? existing.event_id,
      ...presentationPosition,
    };
    return next;
  }

  const reverseIndex = [...next].reverse().findIndex(item => {
    if (item.type !== 'skill_event') return false;
    const invocation = item.invocation;
    const status = String(invocation.status ?? '').toLowerCase();
    const isPending =
      status === 'loading' ||
      status === 'running' ||
      status === 'needs_approval' ||
      status === 'waiting_client_action';
    if (!isPending || invocationIdsConflict(invocation, incoming)) return false;
    return (
      skillInvocationsMatch(invocation, incoming) ||
      getSkillInvocationBaseIdentity(invocation) === incomingBaseIdentity
    );
  });

  if (reverseIndex < 0) {
    return [
      ...next,
      {
        id:
          eventId ??
          `skill-${incoming.invocation_id || incoming.runtime_id || incomingBaseIdentity}-${incoming.created_at ?? Date.now()}-${next.length}`,
        type: 'skill_event',
        invocation: incoming,
        created_at: incoming.created_at,
        event_id: eventId ?? null,
        ...presentationPosition,
      },
    ];
  }

  const actualIndex = next.length - 1 - reverseIndex;
  const existing = next[actualIndex];
  if (existing.type !== 'skill_event') return next;

  next[actualIndex] = {
    ...existing,
    invocation: {
      ...existing.invocation,
      ...incoming,
    },
    created_at: incoming.created_at ?? existing.created_at,
    event_id: eventId ?? existing.event_id,
    ...presentationPosition,
  };
  return next;
}

export function updateSkillInvocationMetadata(
  current: AIChatControllerState,
  conversationId: string,
  messageId: string,
  eventId: string | null | undefined,
  invocation: AIChatSkillInvocation,
  presentationPosition?: AIChatPresentationPosition,
  clearModelProcessing = false
): AIChatControllerState {
  if (!isRecordedSkillInvocation(invocation)) {
    return current;
  }
  const previousStreaming = current.streamingByMessageId[messageId];
  if (isStaleAIChatStreamEvent(eventId, previousStreaming?.last_event_id)) {
    return current;
  }
  const messages = current.messagesByConversation[conversationId] ?? [];
  const now = Math.floor(Date.now() / 1000);
  const nextMessages = messages.map(message => {
    if (message.id !== messageId) {
      return message;
    }

    const skillInvocations = upsertSkillInvocation(
      message.metadata?.skill_invocations ?? [],
      invocation
    );

    return {
      ...message,
      metadata: {
        ...(message.metadata ?? {}),
        has_trace: true,
        skill_invocations: skillInvocations,
        selected_skill_ids: Array.from(
          new Set(
            skillInvocations
              .filter(isRecordedSkillInvocation)
              .map(item => item.skill_id)
              .filter(Boolean)
          )
        ),
        loaded_skill_ids: Array.from(
          new Set(
            skillInvocations
              .filter(item => item.kind === 'skill_load' && item.status !== 'error')
              .map(item => item.skill_id)
              .filter(Boolean)
          )
        ),
        skill_step_count: skillInvocations.filter(isRecordedSkillInvocation).length,
        skill_call_count: skillInvocations.filter(isRecordedSkillInvocation).length,
        skill_names: Array.from(
          new Set(
            skillInvocations
              .filter(isRecordedSkillInvocation)
              .map(item => item.skill_id)
              .filter(Boolean)
          )
        ),
        tool_call_count: skillInvocations.filter(item => item.kind === 'tool_call').length,
        tool_names: Array.from(
          new Set(
            skillInvocations
              .filter(item => item.kind === 'tool_call')
              .map(item => item.tool_name)
              .filter((toolName): toolName is string => Boolean(toolName))
          )
        ),
      },
      updated_at: now,
    };
  });
  return {
    ...current,
    messagesByConversation: {
      ...current.messagesByConversation,
      [conversationId]: nextMessages,
    },
    streamingByMessageId: previousStreaming
      ? {
          ...current.streamingByMessageId,
          [messageId]: {
            ...previousStreaming,
            timeline: upsertSkillTimelineItem(
              previousStreaming.timeline,
              invocation,
              eventId,
              presentationPosition
            ),
            answer_before_timeline_length: preserveLegacyAnswerBoundary(previousStreaming),
            modelProcessing: clearModelProcessing ? undefined : previousStreaming.modelProcessing,
            last_event_id: eventId ?? previousStreaming.last_event_id,
          },
        }
      : current.streamingByMessageId,
  };
}

export function applyAgentProgressState(
  current: AIChatControllerState,
  payload: AIChatAgentProgressEventData,
  eventId?: string | null
): AIChatControllerState {
  const content = (payload.content ?? '').trim();
  if ((!content && !payload.phase) || !payload.message_id) {
    return current;
  }

  const existingStreaming = current.streamingByMessageId[payload.message_id];
  const existingMessage = (current.messagesByConversation[payload.conversation_id] ?? []).find(
    message => message.id === payload.message_id
  );
  if (
    !existingStreaming &&
    ['completed', 'error', 'stopped'].includes(existingMessage?.status ?? '')
  ) {
    return current;
  }
  if (isStaleAIChatStreamEvent(eventId, existingStreaming?.last_event_id)) {
    return current;
  }
  const previousStreaming = existingStreaming ?? {
    conversation_id: payload.conversation_id,
    message_id: payload.message_id,
    answer: '',
    status: 'streaming' as const,
    timeline: [],
  };
  if (
    !previousStreaming.conversation_id ||
    previousStreaming.conversation_id !== payload.conversation_id
  ) {
    return current;
  }

  if (payload.phase === 'model_processing') {
    const modelProcessing = modelProcessingStateFromEvent(
      previousStreaming.modelProcessing,
      payload,
      eventId
    );
    if (modelProcessing === previousStreaming.modelProcessing) {
      return current;
    }
    return {
      ...current,
      streamingByMessageId: {
        ...current.streamingByMessageId,
        [payload.message_id]: {
          ...previousStreaming,
          modelProcessing,
          last_event_id: eventId ?? previousStreaming.last_event_id,
        },
      },
    };
  }

  const timeline = previousStreaming.timeline ?? [];
  const clearsModelProcessing =
    payload.phase === 'tool_planning' ||
    payload.phase === 'client_action' ||
    payload.phase === 'client_action_result';
  const hasSameEvent = Boolean(
    eventId && timeline.some(item => 'event_id' in item && item.event_id === eventId)
  );
  if (hasSameEvent) {
    return current;
  }
  if (payload.phase === 'context_compaction' && payload.progress_id) {
    const existingIndex = timeline.findIndex(
      item => item.type === 'progress_text' && item.progress_id === payload.progress_id
    );
    const status = payload.status === 'completed' ? 'completed' : 'running';
    const item: AIChatAgenticTimelineItem = {
      id: existingIndex >= 0 ? timeline[existingIndex].id : payload.progress_id,
      type: 'progress_text',
      content: '',
      phase: payload.phase,
      progress_id: payload.progress_id,
      transient: status !== 'completed',
      status,
      created_at: payload.created_at,
      event_id: eventId ?? null,
    };
    const nextTimeline = [...timeline];
    if (existingIndex >= 0) nextTimeline[existingIndex] = item;
    else nextTimeline.push(item);
    return {
      ...current,
      streamingByMessageId: {
        ...current.streamingByMessageId,
        [payload.message_id]: {
          ...previousStreaming,
          timeline: nextTimeline,
          last_event_id: eventId ?? previousStreaming.last_event_id,
        },
      },
    };
  }
  const connectorResult = parseConnectorProgressResult(payload);
  const receipt =
    connectorResult?.state === 'requires_confirmation'
      ? connectorResult.confirmation.route_receipt
      : payload.progress_id;
  if (connectorResult && receipt) {
    const existingIndex = timeline.findIndex(item => {
      if (item.type !== 'progress_text') return false;
      const result = parseConnectorProgressResult(item);
      return (
        result &&
        (result.state === 'requires_confirmation'
          ? result.confirmation.route_receipt
          : item.progress_id) === receipt
      );
    });
    const previous = existingIndex < 0 ? undefined : timeline[existingIndex];
    const previousResult =
      previous?.type === 'progress_text' ? parseConnectorProgressResult(previous) : null;
    const nextTimeline = [...timeline];
    // A terminal receipt cannot be reopened by a replayed required event.
    if (!previousResult || previousResult.state === 'requires_confirmation') {
      const item: AIChatAgenticTimelineItem = {
        id: previous?.id ?? eventId ?? `connector-${receipt}`,
        type: 'progress_text',
        content: '',
        phase: payload.phase,
        progress_id: receipt,
        result: { ...connectorResult },
        transient: false,
        status: payload.status,
        created_at: previous?.created_at ?? payload.created_at,
        event_id: eventId ?? null,
        ...presentationPositionFromPayload(previous ?? payload),
      };
      if (existingIndex < 0) nextTimeline.push(item);
      else nextTimeline[existingIndex] = item;
    }
    const resolvedTimeline = nextTimeline.filter(item => {
      const itemReceipt = connectorReceiptFromTimelineItem(item);
      return !(itemReceipt === receipt && isPendingConnectorSkillEvent(item));
    });
    return {
      ...current,
      streamingByMessageId: {
        ...current.streamingByMessageId,
        [payload.message_id]: {
          ...previousStreaming,
          timeline: resolvedTimeline,
          last_event_id: eventId ?? previousStreaming.last_event_id,
        },
      },
    };
  }
  const lastItem = timeline[timeline.length - 1];
  const isRepeatedStructuredProgress =
    lastItem?.type === 'progress_text' &&
    payload.phase &&
    lastItem.phase === payload.phase &&
    (lastItem.meta_tool_name ?? '') === (payload.meta_tool_name ?? '') &&
    (lastItem.skill_id ?? '') === (payload.skill_id ?? '') &&
    (lastItem.tool_name ?? '') === (payload.tool_name ?? '') &&
    (lastItem.action_id ?? '') === (payload.action_id ?? '');
  const isRepeatedProgress =
    lastItem?.type === 'progress_text' && lastItem.content.trim() === content;
  if (isRepeatedStructuredProgress || isRepeatedProgress) {
    return {
      ...current,
      streamingByMessageId: {
        ...current.streamingByMessageId,
        [payload.message_id]: {
          ...previousStreaming,
          modelProcessing: clearsModelProcessing ? undefined : previousStreaming.modelProcessing,
          last_event_id: eventId ?? previousStreaming.last_event_id,
        },
      },
    };
  }

  const transient = Boolean(payload.phase && !content);
  const nextBaseTimeline = removeTransientProgressItems(timeline);
  const nextTimeline: AIChatAgenticTimelineItem[] = [
    ...nextBaseTimeline,
    {
      id: eventId ?? `progress-${payload.created_at ?? Date.now()}-${nextBaseTimeline.length}`,
      type: 'progress_text',
      content,
      phase: payload.phase,
      progress_id: payload.progress_id,
      transient,
      meta_tool_name: payload.meta_tool_name,
      skill_id: payload.skill_id,
      tool_name: payload.tool_name,
      action_id: payload.action_id,
      action_type: payload.action_type,
      continuation_policy: payload.continuation_policy,
      blocking: payload.blocking,
      status: payload.status,
      effect: payload.effect,
      asset_type: payload.asset_type,
      assets: payload.assets,
      correlation_id: payload.correlation_id,
      asset_operation_audit: payload.asset_operation_audit,
      result: payload.result,
      arguments_chars: payload.arguments_chars,
      created_at: payload.created_at,
      event_id: eventId ?? null,
      ...presentationPositionFromPayload(payload),
    },
  ];

  return {
    ...current,
    streamingByMessageId: {
      ...current.streamingByMessageId,
      [payload.message_id]: {
        ...previousStreaming,
        timeline: nextTimeline,
        modelProcessing: clearsModelProcessing ? undefined : previousStreaming.modelProcessing,
        answer_before_timeline_length:
          transient || previousStreaming.presentationVersion === 2
            ? previousStreaming.answer_before_timeline_length
            : captureAnswerTimelineBoundary(
                previousStreaming.answer_before_timeline_length,
                previousStreaming.answer
              ),
        last_event_id: eventId ?? previousStreaming.last_event_id,
      },
    },
  };
}

/**
 * Applies a Connector confirmation response returned by the HTTP decision
 * endpoint when no continuation stream is available (the waiting message has
 * already moved the conversation back to idle).
 */
export function applyConnectorRuntimeResolutionState(
  current: AIChatControllerState,
  payload: {
    conversation_id: string;
    message_id: string;
    progress_id: string;
    state: 'cancelled' | 'completed' | 'failed' | 'unknown';
    decision_id?: string;
    error_code?: string;
  }
): AIChatControllerState {
  if (!payload.conversation_id || !payload.message_id || !payload.progress_id) return current;
  return applyAgentProgressState(current, {
    conversation_id: payload.conversation_id,
    message_id: payload.message_id,
    phase: 'connector_confirmation_resolved',
    progress_id: payload.progress_id,
    status: payload.state,
    content: '',
    result: {
      state: payload.state,
      ...(payload.decision_id ? { decision_id: payload.decision_id } : {}),
      ...(payload.error_code ? { error_code: payload.error_code } : {}),
    },
  });
}

export function applyMemoryMutationState(
  current: AIChatControllerState,
  payload: AIChatMemoryMutationEventData,
  eventId?: string | null
): AIChatControllerState {
  if (!payload.conversation_id || !payload.message_id) {
    return current;
  }
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (!previousStreaming) {
    return current;
  }
  if (isStaleAIChatStreamEvent(eventId, previousStreaming.last_event_id)) {
    return current;
  }
  return {
    ...current,
    streamingByMessageId: {
      ...current.streamingByMessageId,
      [payload.message_id]: {
        ...previousStreaming,
        timeline: upsertMemoryTimelineItem(previousStreaming.timeline, payload, eventId),
        answer_before_timeline_length: preserveLegacyAnswerBoundary(previousStreaming),
        last_event_id: eventId ?? previousStreaming.last_event_id,
      },
    },
  };
}

function upsertRuntimeInputTimelineItem(
  timeline: AIChatAgenticTimelineItem[] | undefined,
  payload: AIChatRuntimeInputAppliedEventData,
  eventId?: string | null
): AIChatAgenticTimelineItem[] {
  const nextItem: Extract<AIChatAgenticTimelineItem, { type: 'runtime_input' }> = {
    id: `runtime-input-${payload.input_id}`,
    type: 'runtime_input',
    input_id: payload.input_id,
    content: payload.content,
    checkpoint: payload.checkpoint,
    model_round: payload.model_round,
    created_at: payload.created_at,
    event_id: eventId ?? null,
    ...presentationPositionFromPayload(payload),
  };
  const items = timeline ?? [];
  const index = items.findIndex(
    item => item.type === 'runtime_input' && item.input_id === payload.input_id
  );
  if (index < 0) return [...items, nextItem];
  return items.map((item, itemIndex) =>
    itemIndex === index && item.type === 'runtime_input'
      ? { ...item, ...nextItem, content: nextItem.content || item.content }
      : item
  );
}

export function applyRuntimeInputAppliedState(
  current: AIChatControllerState,
  payload: AIChatRuntimeInputAppliedEventData,
  eventId?: string | null
): AIChatControllerState {
  if (!payload.conversation_id || !payload.message_id || !payload.input_id) return current;
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (previousStreaming && isStaleAIChatStreamEvent(eventId, previousStreaming.last_event_id)) {
    return current;
  }
  const conversationMessages = current.messagesByConversation[payload.conversation_id] ?? [];
  const nextMessages = applyRuntimeInputApplication(conversationMessages, {
    id: payload.input_id,
    client_input_id: payload.client_input_id,
    status: payload.status ?? 'applied_current_run',
    target_message_id: payload.target_message_id ?? payload.message_id,
    consumed_message_id: payload.consumed_message_id,
  });
  if (!previousStreaming) {
    if (nextMessages === conversationMessages) return current;
    return {
      ...current,
      messagesByConversation: {
        ...current.messagesByConversation,
        [payload.conversation_id]: nextMessages,
      },
    };
  }
  return {
    ...current,
    messagesByConversation: {
      ...current.messagesByConversation,
      [payload.conversation_id]: nextMessages,
    },
    streamingByMessageId: {
      ...current.streamingByMessageId,
      [payload.message_id]: {
        ...previousStreaming,
        timeline: upsertRuntimeInputTimelineItem(previousStreaming.timeline, payload, eventId),
        answer_before_timeline_length: preserveLegacyAnswerBoundary(previousStreaming),
        last_event_id: eventId ?? previousStreaming.last_event_id,
      },
    },
  };
}

export function applyToolGovernanceDecisionState(
  current: AIChatControllerState,
  payload: AIChatToolGovernanceDecisionEventData,
  eventId?: string | null
): AIChatControllerState {
  if (!payload.conversation_id || !payload.message_id) {
    return current;
  }
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (isStaleAIChatStreamEvent(eventId, previousStreaming?.last_event_id)) {
    return current;
  }
  const governanceInvocation = skillInvocationFromToolGovernanceDecision(payload);
  const now = Math.floor(Date.now() / 1000);
  const messages = current.messagesByConversation[payload.conversation_id] ?? [];
  const nextMessages = messages.map(message => {
    if (message.id !== payload.message_id) {
      return message;
    }
    const skillInvocations = upsertSkillInvocation(
      message.metadata?.skill_invocations ?? [],
      governanceInvocation
    );
    return {
      ...message,
      metadata: {
        ...(message.metadata ?? {}),
        has_trace: true,
        skill_invocations: skillInvocations,
        selected_skill_ids: Array.from(
          new Set(
            skillInvocations
              .filter(isRecordedSkillInvocation)
              .map(item => item.skill_id)
              .filter(Boolean)
          )
        ),
        loaded_skill_ids: Array.from(
          new Set(
            skillInvocations
              .filter(item => item.kind === 'skill_load' && item.status !== 'error')
              .map(item => item.skill_id)
              .filter(Boolean)
          )
        ),
        skill_step_count: skillInvocations.filter(isRecordedSkillInvocation).length,
        skill_call_count: skillInvocations.filter(isRecordedSkillInvocation).length,
        skill_names: Array.from(
          new Set(
            skillInvocations
              .filter(isRecordedSkillInvocation)
              .map(item => item.skill_id)
              .filter(Boolean)
          )
        ),
        tool_call_count: skillInvocations.filter(item => item.kind === 'tool_call').length,
        tool_names: Array.from(
          new Set(
            skillInvocations
              .filter(item => item.kind === 'tool_call')
              .map(item => item.tool_name)
              .filter((toolName): toolName is string => Boolean(toolName))
          )
        ),
      },
      updated_at: now,
    };
  });
  return {
    ...current,
    messagesByConversation: {
      ...current.messagesByConversation,
      [payload.conversation_id]: nextMessages,
    },
    streamingByMessageId: previousStreaming
      ? {
          ...current.streamingByMessageId,
          [payload.message_id]: {
            ...previousStreaming,
            modelProcessing: undefined,
            timeline: (() => {
              const timeline = upsertToolGovernanceTimelineItem(
                previousStreaming.timeline,
                payload,
                eventId
              );
              const message = nextMessages.find(item => item.id === payload.message_id);
              return message ? sanitizeTimelineForMessage(message, timeline) : timeline;
            })(),
            answer_before_timeline_length: preserveLegacyAnswerBoundary(previousStreaming),
            last_event_id: eventId ?? previousStreaming.last_event_id,
          },
        }
      : current.streamingByMessageId,
  };
}

function toolGovernanceDecisionEventFromSkillCall(
  payload: AIChatSkillCallEndEventData | AIChatSkillCallErrorEventData
): AIChatToolGovernanceDecisionEventData {
  const governance = payload.governance ?? undefined;
  const approvalEvent = governance?.approval_event;
  const approvalResult = governance?.approval_result;
  const assetOperationAudit =
    payload.asset_operation_audit ??
    governance?.asset_operation_audit ??
    (governance?.model_feedback?.asset_operation_audit as
      | AIChatToolGovernanceDecisionEventData['asset_operation_audit']
      | undefined);
  return normalizeToolGovernanceDecisionPayload({
    conversation_id: payload.conversation_id,
    message_id: payload.message_id,
    skill_id: payload.skill_id,
    tool_name: payload.tool_name,
    status: governance?.status ?? payload.status,
    decision: governance?.status ?? payload.status,
    duration_ms: payload.duration_ms,
    created_at: payload.created_at,
    execution_status: payload.status ?? (payload.message ? 'error' : 'success'),
    execution_error:
      'message' in payload && payload.status === 'error' ? payload.message : undefined,
    execution_error_code: payload.error_code,
    execution_message: payload.message,
    execution_duration_ms: payload.duration_ms,
    execution_result: 'result' in payload ? payload.result : undefined,
    governance,
    correlation_id: governance?.correlation_id ?? assetOperationAudit?.correlation_id,
    requires_approval: governance?.requires_approval,
    reason: governance?.reason,
    risk_level: governance?.manifest?.risk_level ?? approvalEvent?.risk_level,
    effect: governance?.manifest?.effect ?? approvalEvent?.effect,
    asset_type: governance?.manifest?.asset_type ?? approvalEvent?.asset_type,
    asset_operation_audit: assetOperationAudit,
    approval_status:
      governance?.approval_status ??
      (approvalResult?.approval_status as AIChatToolGovernanceDecisionEventData['approval_status']),
    approval_event: approvalEvent,
    matched_grant: governance?.matched_grant,
    approval_result: approvalResult,
    model_feedback:
      governance?.model_feedback ??
      (approvalResult?.model_feedback as Record<string, unknown> | undefined),
    session_grant: approvalResult?.session_grant as Record<string, unknown> | undefined,
  });
}

export function applySkillCallStartState(
  current: AIChatControllerState,
  payload: AIChatSkillCallStartEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: payload.kind ?? 'tool_call',
      invocation_id: payload.invocation_id,
      runtime_id: payload.runtime_id,
      skill_id: payload.skill_id,
      tool_name: payload.tool_name,
      status: 'running',
      arguments: payload.arguments_summary ?? payload.arguments,
      created_at: payload.created_at,
    },
    presentationPositionFromPayload(payload),
    true
  );
}

export function applySkillCallEndState(
  current: AIChatControllerState,
  payload: AIChatSkillCallEndEventData,
  eventId?: string | null
): AIChatControllerState {
  if (
    isStaleAIChatStreamEvent(
      eventId,
      current.streamingByMessageId[payload.message_id]?.last_event_id
    )
  ) {
    return current;
  }
  const next = updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: payload.kind ?? 'tool_call',
      runtime_source: payload.runtime_source,
      invocation_id: payload.invocation_id,
      runtime_id: payload.runtime_id,
      action_id: payload.action_id,
      action_type: payload.action_type,
      href: payload.href,
      effect: payload.effect,
      asset_type: payload.asset_type,
      assets: payload.assets,
      correlation_id: payload.correlation_id,
      skill_id: payload.skill_id,
      tool_name: payload.tool_name,
      status: payload.status ?? 'success',
      duration_ms: payload.duration_ms,
      message: payload.message,
      error_code: payload.error_code,
      result: payload.result,
      governance: payload.governance,
      asset_operation_audit: payload.asset_operation_audit,
      created_at: payload.created_at,
    },
    presentationPositionFromPayload(payload),
    true
  );
  if (!hasToolGovernanceDecision(payload.governance)) {
    return next;
  }
  // The decision is part of this same event, so it applies with it, not as a
  // second event: an id derived from this one compares equal to it and would
  // be dropped as stale.
  return applyToolGovernanceDecisionState(next, toolGovernanceDecisionEventFromSkillCall(payload));
}

export function applySkillCallErrorState(
  current: AIChatControllerState,
  payload: AIChatSkillCallErrorEventData,
  eventId?: string | null
): AIChatControllerState {
  if (
    isStaleAIChatStreamEvent(
      eventId,
      current.streamingByMessageId[payload.message_id]?.last_event_id
    )
  ) {
    return current;
  }
  const next = updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: payload.kind ?? (payload.tool_name ? 'tool_call' : 'skill_load'),
      runtime_source: payload.runtime_source,
      invocation_id: payload.invocation_id,
      runtime_id: payload.runtime_id,
      action_id: payload.action_id,
      action_type: payload.action_type,
      href: payload.href,
      effect: payload.effect,
      asset_type: payload.asset_type,
      assets: payload.assets,
      correlation_id: payload.correlation_id,
      skill_id: payload.skill_id,
      tool_name: payload.tool_name ?? '',
      status: payload.status ?? 'error',
      duration_ms: payload.duration_ms,
      message: payload.message,
      error: payload.status === 'blocked' ? undefined : payload.message,
      result: payload.result,
      error_code: payload.error_code,
      governance: payload.governance,
      asset_operation_audit: payload.asset_operation_audit,
      created_at: payload.created_at,
    },
    presentationPositionFromPayload(payload),
    true
  );
  if (!hasToolGovernanceDecision(payload.governance)) {
    return next;
  }
  // The decision is part of this same event, so it applies with it, not as a
  // second event: an id derived from this one compares equal to it and would
  // be dropped as stale.
  return applyToolGovernanceDecisionState(next, toolGovernanceDecisionEventFromSkillCall(payload));
}

export function applySkillLoadStartState(
  current: AIChatControllerState,
  payload: AIChatSkillLoadStartEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: 'skill_load',
      skill_id: payload.skill_id,
      tool_name: '',
      status: 'loading',
      created_at: payload.created_at,
    },
    presentationPositionFromPayload(payload)
  );
}

export function applySkillLoadEndState(
  current: AIChatControllerState,
  payload: AIChatSkillLoadEndEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: 'skill_load',
      skill_id: payload.skill_id,
      tool_name: '',
      status: 'success',
      duration_ms: payload.duration_ms,
      created_at: payload.created_at,
    },
    presentationPositionFromPayload(payload)
  );
}

export function applySkillReferenceReadState(
  current: AIChatControllerState,
  payload: AIChatSkillReferenceReadEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: 'reference_read',
      skill_id: payload.skill_id,
      tool_name: '',
      path: payload.path,
      status: 'success',
      duration_ms: payload.duration_ms,
      created_at: payload.created_at,
    },
    presentationPositionFromPayload(payload)
  );
}
