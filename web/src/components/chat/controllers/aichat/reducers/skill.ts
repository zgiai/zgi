import type {
  AIChatAgentProgressEventData,
  AIChatMemoryMutationEventData,
  AIChatSkillCallEndEventData,
  AIChatSkillCallErrorEventData,
  AIChatSkillCallStartEventData,
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
import { removeTransientProgressItems } from './shared';

function upsertSkillInvocation(
  invocations: AIChatSkillInvocation[],
  incoming: AIChatSkillInvocation
): AIChatSkillInvocation[] {
  if (incoming.runtime_id) {
    const index = invocations.findIndex(
      invocation => invocation.runtime_id === incoming.runtime_id
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

  if (incoming.kind === 'intermediate_answer' && incoming.answer_id) {
    const index = invocations.findIndex(
      invocation =>
        invocation.kind === 'intermediate_answer' && invocation.answer_id === incoming.answer_id
    );
    if (index < 0) {
      return [...invocations, incoming];
    }

    const next = invocations.slice();
    next[index] = {
      ...next[index],
      ...incoming,
    };
    return next;
  }

  const next = invocations.slice();
  const incomingKind = incoming.kind ?? 'tool_call';
  const incomingToolName = incoming.tool_name ?? '';
  const incomingPath = incoming.path ?? '';
  const index = [...next].reverse().findIndex(invocation => {
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

function getSkillInvocationIdentity(invocation: AIChatSkillInvocation): string {
  if (invocation.runtime_id) {
    return invocation.runtime_id;
  }
  return [
    invocation.kind ?? 'tool_call',
    invocation.skill_id ?? '',
    invocation.tool_name ?? '',
    invocation.path ?? '',
  ].join(':');
}

function isVisibleSkillInvocation(invocation: AIChatSkillInvocation): boolean {
  return invocation.kind !== 'metadata_exposed' && invocation.kind !== 'memory_planner';
}

function upsertMemoryTimelineItem(
  timeline: AIChatAgenticTimelineItem[] | undefined,
  payload: AIChatMemoryMutationEventData,
  eventId: string | null | undefined
): AIChatAgenticTimelineItem[] {
  const baseTimeline = removeTransientProgressItems(timeline);
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
  return {
    ...payload,
    correlation_id:
      payload.correlation_id ?? governance?.correlation_id ?? assetOperationAudit?.correlation_id,
    decision: payload.decision ?? governance?.status ?? payload.status,
    requires_approval: payload.requires_approval ?? governance?.requires_approval,
    reason: payload.reason ?? governance?.reason,
    risk_level: payload.risk_level ?? governance?.manifest?.risk_level ?? approvalEvent?.risk_level,
    effect: payload.effect ?? governance?.manifest?.effect ?? approvalEvent?.effect,
    asset_type: payload.asset_type ?? governance?.manifest?.asset_type ?? approvalEvent?.asset_type,
    asset_operation_audit: assetOperationAudit,
    approval_status:
      payload.approval_status ??
      governance?.approval_status ??
      (approvalResult?.approval_status as AIChatToolGovernanceDecisionEventData['approval_status']),
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
    },
  ];
}

function upsertSkillTimelineItem(
  timeline: AIChatAgenticTimelineItem[] | undefined,
  incoming: AIChatSkillInvocation,
  eventId: string | null | undefined
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
        content: incoming.message ?? existing.content,
        status: incoming.status === 'success' ? 'success' : 'streaming',
        created_at: existing.created_at ?? incoming.created_at,
        event_id: eventId ?? existing.event_id,
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
      },
    ];
  }

  const next = baseTimeline.slice();
  const incomingIdentity = getSkillInvocationIdentity(incoming);
  const reverseIndex = [...next].reverse().findIndex(item => {
    if (item.type !== 'skill_event') return false;
    const invocation = item.invocation;
    return (
      getSkillInvocationIdentity(invocation) === incomingIdentity &&
      (invocation.status === 'loading' || invocation.status === 'running')
    );
  });

  if (reverseIndex < 0) {
    return [
      ...next,
      {
        id:
          eventId ??
          `skill-${incomingIdentity}-${incoming.created_at ?? Date.now()}-${next.length}`,
        type: 'skill_event',
        invocation: incoming,
        created_at: incoming.created_at,
        event_id: eventId ?? null,
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
  };
  return next;
}

export function updateSkillInvocationMetadata(
  current: AIChatControllerState,
  conversationId: string,
  messageId: string,
  eventId: string | null | undefined,
  invocation: AIChatSkillInvocation
): AIChatControllerState {
  if (!isVisibleSkillInvocation(invocation)) {
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
              .filter(isVisibleSkillInvocation)
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
        skill_step_count: skillInvocations.filter(isVisibleSkillInvocation).length,
        skill_call_count: skillInvocations.filter(isVisibleSkillInvocation).length,
        skill_names: Array.from(
          new Set(
            skillInvocations
              .filter(isVisibleSkillInvocation)
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
  const previousStreaming = current.streamingByMessageId[messageId];

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
            timeline: upsertSkillTimelineItem(previousStreaming.timeline, invocation, eventId),
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

  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (!previousStreaming) {
    return current;
  }

  const timeline = previousStreaming.timeline ?? [];
  const hasSameEvent = Boolean(
    eventId && timeline.some(item => 'event_id' in item && item.event_id === eventId)
  );
  if (hasSameEvent) {
    return current;
  }
  const lastItem = timeline[timeline.length - 1];
  const isRepeatedStructuredProgress =
    lastItem?.type === 'progress_text' &&
    payload.phase &&
    lastItem.phase === payload.phase &&
    (lastItem.meta_tool_name ?? '') === (payload.meta_tool_name ?? '') &&
    (lastItem.skill_id ?? '') === (payload.skill_id ?? '') &&
    (lastItem.tool_name ?? '') === (payload.tool_name ?? '');
  const isRepeatedProgress =
    lastItem?.type === 'progress_text' && lastItem.content.trim() === content;
  if (isRepeatedStructuredProgress || isRepeatedProgress) {
    return {
      ...current,
      streamingByMessageId: {
        ...current.streamingByMessageId,
        [payload.message_id]: {
          ...previousStreaming,
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
      transient,
      meta_tool_name: payload.meta_tool_name,
      skill_id: payload.skill_id,
      tool_name: payload.tool_name,
      arguments_chars: payload.arguments_chars,
      created_at: payload.created_at,
      event_id: eventId ?? null,
    },
  ];

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
  return {
    ...current,
    streamingByMessageId: {
      ...current.streamingByMessageId,
      [payload.message_id]: {
        ...previousStreaming,
        timeline: upsertMemoryTimelineItem(previousStreaming.timeline, payload, eventId),
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
  if (!previousStreaming) {
    return current;
  }
  return {
    ...current,
    streamingByMessageId: {
      ...current.streamingByMessageId,
      [payload.message_id]: {
        ...previousStreaming,
        timeline: upsertToolGovernanceTimelineItem(previousStreaming.timeline, payload, eventId),
        last_event_id: eventId ?? previousStreaming.last_event_id,
      },
    },
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
    execution_error: 'message' in payload && payload.status === 'error' ? payload.message : undefined,
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
      runtime_id: payload.runtime_id,
      skill_id: payload.skill_id,
      tool_name: payload.tool_name,
      status: 'running',
      arguments: payload.arguments_summary ?? payload.arguments,
      created_at: payload.created_at,
    }
  );
}

export function applySkillCallEndState(
  current: AIChatControllerState,
  payload: AIChatSkillCallEndEventData,
  eventId?: string | null
): AIChatControllerState {
  const next = updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: payload.kind ?? 'tool_call',
      runtime_id: payload.runtime_id,
      skill_id: payload.skill_id,
      tool_name: payload.tool_name,
      status: payload.status ?? 'success',
      duration_ms: payload.duration_ms,
      message: payload.message,
      result: payload.result,
      governance: payload.governance,
      created_at: payload.created_at,
    }
  );
  if (!payload.governance) {
    return next;
  }
  return applyToolGovernanceDecisionState(
    next,
    toolGovernanceDecisionEventFromSkillCall(payload),
    eventId ? `${eventId}:governance` : undefined
  );
}

export function applySkillCallErrorState(
  current: AIChatControllerState,
  payload: AIChatSkillCallErrorEventData,
  eventId?: string | null
): AIChatControllerState {
  const next = updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: payload.kind ?? (payload.tool_name ? 'tool_call' : 'skill_load'),
      runtime_id: payload.runtime_id,
      skill_id: payload.skill_id,
      tool_name: payload.tool_name ?? '',
      status: 'error',
      duration_ms: payload.duration_ms,
      message: payload.message,
      error: payload.message,
      governance: payload.governance,
      created_at: payload.created_at,
    }
  );
  if (!payload.governance) {
    return next;
  }
  return applyToolGovernanceDecisionState(
    next,
    toolGovernanceDecisionEventFromSkillCall(payload),
    eventId ? `${eventId}:governance` : undefined
  );
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
    }
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
    }
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
    }
  );
}
