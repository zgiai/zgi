import type {
  AIChatMessageMetadata,
  AIChatPresentationItem,
  AIChatPresentationPosition,
  AIChatPresentationProjection,
  AIChatSkillInvocation,
} from '@/services/types/aichat';
import type { AIChatPresentationTextItem } from '@/services/types/aichat';
import type {
  AIChatAgenticTimelineItem,
  AIChatStreamingMessageState,
} from '@/components/chat/controllers/aichat/types';

const PRESENTATION_VERSION = 2;

function isPresentationItem(value: unknown): value is AIChatPresentationItem {
  if (!value || typeof value !== 'object') return false;
  const item = value as Record<string, unknown>;
  return (
    (item.kind === 'text' || item.kind === 'event') &&
    typeof item.presentation_id === 'string' &&
    typeof item.presentation_sequence === 'number'
  );
}

export function presentationProjectionFromMetadata(
  metadata?: AIChatMessageMetadata
): AIChatPresentationProjection | undefined {
  if (metadata?.presentation_version !== PRESENTATION_VERSION) return undefined;
  const raw = metadata.presentation;
  if (!raw || typeof raw !== 'object') return undefined;
  const items = Array.isArray(raw.items) ? raw.items.filter(isPresentationItem) : [];
  return {
    version: PRESENTATION_VERSION,
    last_sequence:
      typeof raw.last_sequence === 'number'
        ? raw.last_sequence
        : items.reduce((maximum, item) => Math.max(maximum, item.presentation_sequence ?? 0), 0),
    items,
  };
}

export function presentationStateFromMetadata(
  metadata?: AIChatMessageMetadata
): Pick<
  AIChatStreamingMessageState,
  'presentationItems' | 'presentationVersion' | 'lastPresentationSequence' | 'segmentsById'
> {
  const projection = presentationProjectionFromMetadata(metadata);
  if (!projection) return {};
  const presentationItems = [...(projection.items ?? [])].sort(
    (left, right) => (left.presentation_sequence ?? 0) - (right.presentation_sequence ?? 0)
  );
  return {
    presentationItems,
    presentationVersion: PRESENTATION_VERSION,
    lastPresentationSequence: projection.last_sequence ?? 0,
    segmentsById: Object.fromEntries(
      presentationItems.filter(item => item.kind === 'text').map(item => [item.segment_id, item])
    ),
  };
}

export function presentationPositionFromPayload(
  payload: unknown
): AIChatPresentationPosition | undefined {
  if (!payload || typeof payload !== 'object') return undefined;
  const value = payload as Record<string, unknown>;
  if (
    value.presentation_version !== PRESENTATION_VERSION ||
    typeof value.presentation_id !== 'string' ||
    typeof value.presentation_sequence !== 'number'
  ) {
    return undefined;
  }
  return {
    presentation_version: PRESENTATION_VERSION,
    presentation_id: value.presentation_id,
    presentation_sequence: value.presentation_sequence,
  };
}

export function upsertPresentationItem(
  items: AIChatPresentationItem[] | undefined,
  incoming: AIChatPresentationItem
): AIChatPresentationItem[] {
  const next = [...(items ?? [])];
  const index = next.findIndex(item => item.presentation_id === incoming.presentation_id);
  if (index >= 0) {
    next[index] = { ...next[index], ...incoming } as AIChatPresentationItem;
  } else {
    next.push(incoming);
  }
  return next.sort(
    (left, right) => (left.presentation_sequence ?? 0) - (right.presentation_sequence ?? 0)
  );
}

export function mergePresentationItems(
  base: AIChatPresentationItem[] | undefined,
  incoming: AIChatPresentationItem[] | undefined
): AIChatPresentationItem[] {
  return (incoming ?? []).reduce(
    (items, item) => upsertPresentationItem(items, item),
    [...(base ?? [])]
  );
}

export function finalPresentationAnswer(
  items: AIChatPresentationItem[] | undefined
): string | undefined {
  const finalItems = (items ?? []).filter(
    (item): item is AIChatPresentationTextItem =>
      item.kind === 'text' && item.content_phase === 'final'
  );
  if (!finalItems.length) return undefined;
  return finalItems.map(item => item.content).join('');
}

export function withPresentationItems(
  metadata: AIChatMessageMetadata | undefined,
  items: AIChatPresentationItem[]
): AIChatMessageMetadata {
  const lastSequence = items.reduce(
    (maximum, item) => Math.max(maximum, item.presentation_sequence ?? 0),
    0
  );
  return {
    ...(metadata ?? {}),
    presentation_version: PRESENTATION_VERSION,
    presentation: {
      version: PRESENTATION_VERSION,
      last_sequence: lastSequence,
      items,
    },
  };
}

export function processTextPresentationItem(
  payload: Record<string, unknown>,
  fallbackContent: string
): AIChatPresentationTextItem | undefined {
  const position = presentationPositionFromPayload(payload);
  const segmentId =
    typeof payload.segment_id === 'string' ? payload.segment_id : position?.presentation_id;
  if (!position?.presentation_id || !segmentId) return undefined;
  const phase =
    payload.content_phase === 'process' || payload.content_phase === 'final'
      ? payload.content_phase
      : 'provisional';
  return {
    ...position,
    kind: 'text',
    segment_id: segmentId,
    content:
      typeof payload.segment_content === 'string' ? payload.segment_content : fallbackContent,
    content_phase: phase,
    created_at_ms: typeof payload.created_at_ms === 'number' ? payload.created_at_ms : Date.now(),
  };
}

export function presentationEventItemFromPayload(
  payload: unknown,
  eventType: string
): AIChatPresentationItem | undefined {
  const position = presentationPositionFromPayload(payload);
  if (!position?.presentation_id) return undefined;
  const value = payload as Record<string, unknown>;
  return {
    ...position,
    kind: 'event',
    event_type: eventType,
    event_ref: typeof value.event_ref === 'string' ? value.event_ref : undefined,
    created_at_ms: typeof value.created_at_ms === 'number' ? value.created_at_ms : Date.now(),
  };
}

function timelineReferences(item: AIChatAgenticTimelineItem): string[] {
  switch (item.type) {
    case 'skill_event': {
      const invocation = item.invocation as AIChatSkillInvocation & {
        request_id?: string;
      };
      return [
        invocation.runtime_id,
        invocation.action_id,
        invocation.correlation_id,
        invocation.request_id,
      ].filter((value): value is string => Boolean(value));
    }
    case 'user_input_request':
      return item.request_id ? [item.request_id] : [];
    case 'user_input_response':
      return item.request_id
        ? [`user_input_response:${item.request_id}`, item.request_id]
        : [];
    case 'tool_governance_decision':
      return [item.event.correlation_id, item.event.request_id, item.event.approval_id].filter(
        (value): value is string => Boolean(value)
      );
    case 'workflow_run':
      return [item.workflowRunId, item.invocationId].filter((value): value is string =>
        Boolean(value)
      );
    default:
      return [];
  }
}

export function optimisticUserInputResponsePresentationPosition(
  metadata: AIChatMessageMetadata | undefined,
  messageId: string,
  requestId: string
): AIChatPresentationPosition | undefined {
  const projection = presentationProjectionFromMetadata(metadata);
  const normalizedMessageId = messageId.trim();
  const normalizedRequestId = requestId.trim();
  if (!projection || !normalizedMessageId || !normalizedRequestId) return undefined;

  const presentationSequence = Math.max(
    projection.last_sequence ?? 0,
    ...(projection.items ?? []).map(item => item.presentation_sequence ?? 0)
  ) + 1;
  return {
    presentation_version: PRESENTATION_VERSION,
    presentation_id: `message:${normalizedMessageId}:event:user_input_response:${normalizedRequestId}`,
    presentation_sequence: presentationSequence,
  };
}

export function withOptimisticUserInputResponsePresentation(
  metadata: AIChatMessageMetadata,
  messageId: string,
  requestId: string
): {
  metadata: AIChatMessageMetadata;
  position?: AIChatPresentationPosition;
} {
  const position = optimisticUserInputResponsePresentationPosition(
    metadata,
    messageId,
    requestId
  );
  if (!position) return { metadata };

  const projection = presentationProjectionFromMetadata(metadata);
  const item: AIChatPresentationItem = {
    ...position,
    kind: 'event',
    event_type: 'user_input_response',
    event_ref: `user_input_response:${requestId.trim()}`,
    created_at_ms: Date.now(),
  };
  return {
    position,
    metadata: {
      ...metadata,
      presentation_version: PRESENTATION_VERSION,
      presentation: {
        version: PRESENTATION_VERSION,
        last_sequence: position.presentation_sequence,
        items: upsertPresentationItem(projection?.items, item),
      },
    },
  };
}

function eventTypeMatchesTimelineItem(eventType: string, item: AIChatAgenticTimelineItem): boolean {
  if (eventType.startsWith('skill_')) return item.type === 'skill_event';
  if (eventType.includes('user_input')) {
    return item.type === 'user_input_request' || item.type === 'user_input_response';
  }
  if (
    eventType.startsWith('workflow_') ||
    eventType.startsWith('approval_') ||
    eventType.startsWith('question_') ||
    eventType.startsWith('node_') ||
    eventType.startsWith('container_')
  ) {
    return item.type === 'workflow_run';
  }
  if (eventType.includes('tool_governance')) {
    return item.type === 'tool_governance_decision';
  }
  return false;
}

export function orderedPresentationTimeline(
  presentationItems: AIChatPresentationItem[] | undefined,
  timeline: AIChatAgenticTimelineItem[]
): AIChatAgenticTimelineItem[] {
  if (!presentationItems?.length) return timeline;
  const eventProjection = presentationItems.filter(
    (item): item is Extract<AIChatPresentationItem, { kind: 'event' }> => item.kind === 'event'
  );
  const matchedProjectionIds = new Set<string>();
  const projectedTimeline = timeline.map(item => {
    if (item.presentation_sequence) return item;
    const match = eventProjection.find(projection => {
      const presentationId = projection.presentation_id;
      if (!presentationId || matchedProjectionIds.has(presentationId)) {
        return false;
      }
      return projection.event_ref
        ? timelineReferences(item).includes(projection.event_ref)
        : eventTypeMatchesTimelineItem(projection.event_type, item);
    });
    if (match?.presentation_id) {
      matchedProjectionIds.add(match.presentation_id);
    }
    return match
      ? {
          ...item,
          presentation_id: match.presentation_id,
          presentation_sequence: match.presentation_sequence,
        }
      : item;
  });
  const ordered: AIChatAgenticTimelineItem[] = projectedTimeline.filter(item =>
    Boolean(item.presentation_sequence)
  );
  presentationItems.forEach(item => {
    if (item.kind === 'text') {
      if (item.content_phase !== 'final' && item.content) {
        ordered.push({
          id: item.presentation_id ?? item.segment_id,
          type: 'process_text',
          segment_id: item.segment_id,
          content: item.content,
          content_phase: item.content_phase === 'provisional' ? 'provisional' : 'process',
          created_at: item.created_at_ms ? Math.floor(item.created_at_ms / 1000) : undefined,
          presentation_id: item.presentation_id,
          presentation_sequence: item.presentation_sequence,
        });
      }
    }
  });
  ordered.sort(
    (left, right) =>
      (left.presentation_sequence ?? Number.MAX_SAFE_INTEGER) -
      (right.presentation_sequence ?? Number.MAX_SAFE_INTEGER)
  );
  projectedTimeline.forEach(item => {
    ordered.push(item);
  });
  return ordered.filter(
    (item, index, items) => items.findIndex(candidate => candidate.id === item.id) === index
  );
}

export function captureAnswerTimelineBoundary(
  currentBoundary: number | undefined,
  answer: string
): number {
  return typeof currentBoundary === 'number' ? currentBoundary : answer.length;
}

export function clearAnswerTimelineBoundaryWithoutDurableTimeline(
  currentBoundary: number | undefined,
  hasDurableTimeline: boolean
): number | undefined {
  return hasDurableTimeline ? currentBoundary : undefined;
}

export function splitAnswerAroundTimeline(
  answer: string,
  boundary: number | undefined
): {
  leadingAnswer: string;
  trailingAnswer: string;
} {
  if (
    typeof boundary !== 'number' ||
    !Number.isInteger(boundary) ||
    boundary < 0 ||
    boundary > answer.length
  ) {
    return {
      leadingAnswer: '',
      trailingAnswer: answer,
    };
  }

  return {
    leadingAnswer: answer.slice(0, boundary),
    trailingAnswer: answer.slice(boundary),
  };
}
