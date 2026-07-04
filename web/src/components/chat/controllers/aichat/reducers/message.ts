import type {
  AIChatConversation,
  AIChatErrorEventData,
  AIChatIntermediateAnswerEventData,
  AIChatMessage,
  AIChatMessageMetadata,
  AIChatMessageChunkEventData,
  AIChatMessageEndEventData,
  AIChatMessageRetractEventData,
  AIChatMessageStartEventData,
  AIChatSkillInvocation,
  AIChatUserInputRequestedEventData
} from '@/services/types/aichat';
import {
  SENSITIVE_OUTPUT_BLOCKED_FLAG,
  SENSITIVE_OUTPUT_BLOCKED_TOKEN,
  isSensitiveOutputBlockedValue
} from '@/utils/model-output-filter';
import {
  DEFAULT_AICHAT_MESSAGE_PAGINATION,
  type AIChatControllerState,
  type AIChatAgenticTimelineItem,
  type AIChatMessageStartContext,
  type AIChatStreamingMessageState
} from '@/components/chat/controllers/aichat/types';
import {
  createDraftAIChatConversation,
  createStreamingAIChatMessage,
  normalizeAIChatStatus,
  replaceAIChatConversation,
  upsertAIChatMessage
} from '@/components/chat/utils/aichat-message';
import { getNextActiveSendingState } from '../selectors';
import {
  createAIChatFileMetadata,
  mergeMessageMetadata,
  clearRuntimeMessageMetadata,
  isStaleAIChatStreamEvent,
  removeTransientProgressItems
} from './shared';
import { updateSkillInvocationMetadata } from './skill';

export function mergeAIChatMessages(
  currentMessages: AIChatMessage[],
  incomingMessages: AIChatMessage[]
): AIChatMessage[] {
  const byId = new Map<string, AIChatMessage>();
  currentMessages.forEach(message => byId.set(message.id, message));
  incomingMessages.forEach(message => {
    const existing = byId.get(message.id);
    byId.set(message.id, existing ? mergeAIChatMessage(existing, message) : message);
  });

  return Array.from(byId.values()).sort(
    (a, b) => a.created_at - b.created_at || a.id.localeCompare(b.id)
  );
}

function mergeAIChatMessage(existing: AIChatMessage, incoming: AIChatMessage): AIChatMessage {
  const metadata = mergeTerminalMessageMetadata(
    mergeMessageMetadata(existing.metadata, incoming.metadata),
    incoming.status
  );
  return {
    ...existing,
    ...incoming,
    metadata,
  };
}

function mergeTerminalMessageMetadata(
  metadata: AIChatMessageMetadata | undefined,
  status: AIChatMessage['status']
): AIChatMessageMetadata | undefined {
  if (!metadata) return undefined;
  if (
    status === 'waiting_approval' ||
    status === 'waiting_client_action' ||
    status === 'waiting_question' ||
    status === 'pending' ||
    status === 'streaming'
  ) {
    return metadata;
  }

  const next = { ...metadata };
  delete next.user_input_request;
  return next;
}

export function removeStreamingStateByConversation(
  streamingByMessageId: Record<string, AIChatStreamingMessageState>,
  conversationId: string
): Record<string, AIChatStreamingMessageState> {
  const nextStreamingByMessageId = { ...streamingByMessageId };
  Object.values(streamingByMessageId).forEach(streaming => {
    if (streaming.conversation_id === conversationId) {
      if (
        streaming.timeline?.length &&
        (streaming.status === 'waiting_approval' ||
          streaming.status === 'waiting_client_action' ||
          streaming.status === 'waiting_question')
      ) {
        return;
      }
      delete nextStreamingByMessageId[streaming.message_id];
    }
  });
  return nextStreamingByMessageId;
}

function shouldAdvanceConversationLeaf(
  conversation: AIChatConversation | undefined,
  messageId: string,
  parentId?: string | null
) {
  if (!conversation?.current_leaf_message_id) return true;
  if (conversation.current_leaf_message_id === messageId) return true;
  return Boolean(parentId && conversation.current_leaf_message_id === parentId);
}

export function applyIntermediateAnswerState(
  current: AIChatControllerState,
  payload: AIChatIntermediateAnswerEventData,
  eventId?: string | null
): AIChatControllerState {
  const content = payload.content ?? '';
  if ((!content && payload.done !== true) || !payload.conversation_id || !payload.message_id) {
    return current;
  }
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  const answerId =
    payload.answer_id ||
    eventId ||
    `intermediate-${payload.created_at ?? Date.now()}-${payload.index ?? 0}`;
  const previousItem = previousStreaming?.timeline?.find(
    (item): item is Extract<AIChatAgenticTimelineItem, { type: 'intermediate_answer' }> =>
      item.type === 'intermediate_answer' && item.answer_id === answerId
  );
  const nextContent =
    payload.done === true && !content
      ? (previousItem?.content ?? '')
      : payload.delta
        ? `${previousItem?.content ?? ''}${content}`
        : content;
  if (!nextContent && payload.done === true) {
    return current;
  }

  return updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: 'intermediate_answer',
      answer_id: answerId,
      skill_id: '',
      title: payload.title,
      status: payload.done === false ? 'running' : 'success',
      message: nextContent,
      created_at: payload.created_at,
    }
  );
}

export function applyUserInputRequestedState(
  current: AIChatControllerState,
  payload: AIChatUserInputRequestedEventData,
  eventId?: string | null
): AIChatControllerState {
  const questions = (payload.questions ?? []).filter(question => question.question?.trim());
  if (!payload.conversation_id || !payload.message_id || questions.length === 0) {
    return current;
  }
  const request = {
    request_id: payload.request_id,
    source: payload.source,
    workflow_run_id: payload.workflow_run_id,
    node_id: payload.node_id,
    conversation_id: payload.conversation_id,
    message_id: payload.message_id,
    round: payload.round,
    questions: questions.map(question => ({
      ...question,
      question: question.question.trim(),
      options: question.options?.filter(option => option.label?.trim()),
    })),
    created_at: payload.created_at,
  };
  const messages = current.messagesByConversation[payload.conversation_id] ?? [];
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (isStaleAIChatStreamEvent(eventId, previousStreaming?.last_event_id)) {
    return current;
  }
  const nextMessages = messages.map(message =>
    message.id === payload.message_id
      ? {
          ...message,
          metadata: {
            ...(message.metadata ?? {}),
            user_input_request: request,
          },
          updated_at: Math.floor(Date.now() / 1000),
        }
      : message
  );
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
            last_event_id: eventId ?? previousStreaming.last_event_id,
          },
        }
      : current.streamingByMessageId,
  };
}

export function applyMessageStartState(
  current: AIChatControllerState,
  payload: AIChatMessageStartEventData,
  context: AIChatMessageStartContext = {},
  eventId?: string | null
): AIChatControllerState {
  const mode = context.mode ?? 'active';
  const conversation = createDraftAIChatConversation(payload.conversation_id, payload.title || '');
  conversation.current_leaf_message_id = payload.message_id;
  conversation.runtime_status = 'streaming';
  conversation.active_message_id = payload.message_id;
  const createdAt = payload.created_at ?? Math.floor(Date.now() / 1000);
  conversation.created_at = createdAt;
  conversation.updated_at = createdAt;

  const messages =
    current.messagesByConversation[payload.conversation_id] ??
    (context.previousConversationId
      ? (current.messagesByConversation[context.previousConversationId] ?? [])
      : []);
  const existingMessage = messages.find(message => message.id === payload.message_id);
  const existingConversation =
    current.conversations.find(item => item.id === payload.conversation_id) ?? conversation;
  const shouldAdvanceLeaf = shouldAdvanceConversationLeaf(
    existingConversation,
    payload.message_id,
    payload.parent_id ?? existingMessage?.parent_id
  );
  const nextConversation: AIChatConversation = {
    ...existingConversation,
    title: payload.title || existingConversation.title,
    current_leaf_message_id: (context.forceAdvanceLeaf || shouldAdvanceLeaf)
      ? payload.message_id
      : existingConversation.current_leaf_message_id,
    runtime_status: 'streaming',
    active_message_id: payload.message_id,
    updated_at: createdAt,
  };
  const shouldMigrateDraftConversation = Boolean(
    context.previousConversationId && context.previousConversationId !== payload.conversation_id
  );
  const baseConversations = shouldMigrateDraftConversation
    ? current.conversations.filter(item => item.id !== context.previousConversationId)
    : current.conversations;
  const isReplace = payload.replace === true || context.resetAnswer === true;
  const createdMessage = createStreamingAIChatMessage({
    id: payload.message_id,
    conversationId: payload.conversation_id,
    parentId: payload.parent_id ?? existingMessage?.parent_id,
    query: context.query ?? existingMessage?.query ?? '',
    modelName: payload.model || context.model?.model || existingMessage?.model_name || 'unknown',
    modelProvider: context.model?.provider ?? existingMessage?.model_provider,
    createdAt: payload.created_at ?? existingMessage?.created_at,
    metadata: createAIChatFileMetadata(context.files),
  });
  const message: AIChatMessage = existingMessage
    ? {
        ...existingMessage,
        ...createdMessage,
        answer: isReplace ? '' : existingMessage.answer,
        created_at: existingMessage.created_at,
        error: undefined,
        metadata: isReplace
          ? clearRuntimeMessageMetadata(
              mergeMessageMetadata(existingMessage.metadata, createdMessage.metadata)
            )
          : mergeMessageMetadata(existingMessage.metadata, createdMessage.metadata),
        updated_at: createdAt,
      }
    : createdMessage;
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (isStaleAIChatStreamEvent(eventId, previousStreaming?.last_event_id)) {
    return current;
  }
  const migratedMessages =
    shouldMigrateDraftConversation && context.previousConversationId
      ? messages.filter(message => message.conversation_id !== context.previousConversationId)
      : messages;
  const nextMessagesByConversation = {
    ...current.messagesByConversation,
    [payload.conversation_id]: upsertAIChatMessage(migratedMessages, message),
  };
  const nextMessagePaginationByConversation = {
    ...current.messagePaginationByConversation,
  };
  const nextLoadingOlderByConversation = {
    ...current.loadingOlderByConversation,
  };
  const nextRecoveringByConversation = {
    ...current.recoveringByConversation,
  };
  const nextStoppingByConversation = {
    ...current.stoppingByConversation,
  };
  const nextStreamingByMessageId = {
    ...current.streamingByMessageId,
  };
  Object.values(current.streamingByMessageId).forEach(streaming => {
    if (
      streaming.conversation_id === payload.conversation_id &&
      streaming.message_id !== payload.message_id &&
      (streaming.status === 'completed' ||
        streaming.status === 'stopped' ||
        streaming.status === 'error')
    ) {
      delete nextStreamingByMessageId[streaming.message_id];
    }
  });
  if (shouldMigrateDraftConversation && context.previousConversationId) {
    delete nextMessagesByConversation[context.previousConversationId];
    delete nextMessagePaginationByConversation[context.previousConversationId];
    delete nextLoadingOlderByConversation[context.previousConversationId];
    delete nextRecoveringByConversation[context.previousConversationId];
    delete nextStoppingByConversation[context.previousConversationId];
    Object.values(current.streamingByMessageId).forEach(streaming => {
      if (streaming.conversation_id === context.previousConversationId) {
        delete nextStreamingByMessageId[streaming.message_id];
      }
    });
  }
  nextStreamingByMessageId[payload.message_id] = {
    conversation_id: payload.conversation_id,
    message_id: payload.message_id,
    answer: message.answer,
    status: 'streaming',
    timeline: isReplace ? [] : (previousStreaming?.timeline ?? []),
    last_event_id: eventId ?? (isReplace ? undefined : previousStreaming?.last_event_id),
    replay_base_answer: isReplace ? undefined : previousStreaming?.replay_base_answer,
    replay_offset: isReplace ? undefined : previousStreaming?.replay_offset,
    replace: isReplace || previousStreaming?.replace,
    sensitiveOutputBlocked: isReplace ? undefined : previousStreaming?.sensitiveOutputBlocked,
  };

  return {
    ...current,
    activeConversationId:
      mode === 'active' ? payload.conversation_id : current.activeConversationId,
    conversations: replaceAIChatConversation(baseConversations, nextConversation, {
      moveToTop: context.moveToTop ?? true,
    }),
    messagesByConversation: nextMessagesByConversation,
    messagePaginationByConversation: {
      ...nextMessagePaginationByConversation,
      [payload.conversation_id]: {
        ...(nextMessagePaginationByConversation[payload.conversation_id] ??
          DEFAULT_AICHAT_MESSAGE_PAGINATION),
        total: existingMessage
          ? (nextMessagePaginationByConversation[payload.conversation_id]?.total ??
            migratedMessages.length)
          : (nextMessagePaginationByConversation[payload.conversation_id]?.total ??
              migratedMessages.length) + 1,
      },
    },
    loadingOlderByConversation: nextLoadingOlderByConversation,
    recoveringByConversation: nextRecoveringByConversation,
    stoppingByConversation: nextStoppingByConversation,
    streamingByMessageId: nextStreamingByMessageId,
  };
}

function resolveReplayChunk(
  streaming: AIChatStreamingMessageState | undefined,
  answerChunk: string
): {
  appendChunk: string;
  replayBaseAnswer?: string;
  replayOffset?: number;
} {
  const replayBaseAnswer = streaming?.replay_base_answer;
  const replayOffset = streaming?.replay_offset ?? 0;
  if (!replayBaseAnswer || replayOffset >= replayBaseAnswer.length || !answerChunk) {
    return {
      appendChunk: answerChunk,
      replayBaseAnswer,
      replayOffset: streaming?.replay_offset,
    };
  }

  const remainingBase = replayBaseAnswer.slice(replayOffset);
  const maxOverlap = Math.min(answerChunk.length, remainingBase.length);
  let overlap = 0;
  while (overlap < maxOverlap && answerChunk[overlap] === remainingBase[overlap]) {
    overlap += 1;
  }

  if (overlap === 0) {
    return { appendChunk: answerChunk };
  }

  const nextReplayOffset = replayOffset + overlap;
  return {
    appendChunk: answerChunk.slice(overlap),
    replayBaseAnswer: nextReplayOffset >= replayBaseAnswer.length ? undefined : replayBaseAnswer,
    replayOffset: nextReplayOffset >= replayBaseAnswer.length ? undefined : nextReplayOffset,
  };
}

export function applyMessageChunkState(
  current: AIChatControllerState,
  payload: AIChatMessageChunkEventData,
  eventId?: string | null
): AIChatControllerState {
  const isSensitiveBlocked =
    isSensitiveOutputBlockedValue(payload.answer) ||
    (payload as unknown as Record<string, unknown>)[SENSITIVE_OUTPUT_BLOCKED_FLAG] === true;
  const answerChunk = isSensitiveBlocked ? SENSITIVE_OUTPUT_BLOCKED_TOKEN : payload.answer || '';
  const messages = current.messagesByConversation[payload.conversation_id] ?? [];
  const existingMessage = messages.find(message => message.id === payload.message_id);
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (isStaleAIChatStreamEvent(eventId, previousStreaming?.last_event_id)) {
    return current;
  }
  const { appendChunk, replayBaseAnswer, replayOffset } = isSensitiveBlocked
    ? {
        appendChunk: answerChunk,
        replayBaseAnswer: undefined,
        replayOffset: undefined,
      }
    : resolveReplayChunk(previousStreaming, answerChunk);
  const now = Math.floor(Date.now() / 1000);
  const nextMessage = existingMessage
    ? {
        ...existingMessage,
        answer: isSensitiveBlocked ? answerChunk : `${existingMessage.answer}${appendChunk}`,
        status: 'streaming' as const,
        metadata: isSensitiveBlocked
          ? {
              ...existingMessage.metadata,
              sensitiveOutputBlocked: true,
            }
          : existingMessage.metadata,
        updated_at: now,
      }
    : createStreamingAIChatMessage({
        id: payload.message_id,
        conversationId: payload.conversation_id,
        query: '',
        modelName: 'unknown',
        createdAt: now,
      });
  if (!existingMessage) {
    nextMessage.answer = appendChunk;
    if (isSensitiveBlocked) {
      nextMessage.metadata = {
        ...nextMessage.metadata,
        sensitiveOutputBlocked: true,
      };
    }
  }
  const nextStreamingAnswer = isSensitiveBlocked
    ? answerChunk
    : `${previousStreaming?.answer ?? existingMessage?.answer ?? ''}${appendChunk}`;
  let conversationChanged = false;
  const nextConversations = current.conversations.map(conversation => {
    if (conversation.id !== payload.conversation_id) return conversation;
    if (
      conversation.runtime_status === 'streaming' &&
      conversation.active_message_id === payload.message_id
    ) {
      return conversation;
    }

    conversationChanged = true;
    return {
      ...conversation,
      runtime_status: 'streaming' as const,
      active_message_id: payload.message_id,
    };
  });
  const conversations = conversationChanged ? nextConversations : current.conversations;

  return {
    ...current,
    conversations,
    messagesByConversation: {
      ...current.messagesByConversation,
      [payload.conversation_id]: upsertAIChatMessage(messages, nextMessage),
    },
    streamingByMessageId: {
      ...current.streamingByMessageId,
      [payload.message_id]: {
        conversation_id: payload.conversation_id,
        message_id: payload.message_id,
        answer: nextStreamingAnswer,
        status: 'streaming',
        timeline: removeTransientProgressItems(previousStreaming?.timeline),
        last_event_id: eventId ?? previousStreaming?.last_event_id,
        replay_base_answer: replayBaseAnswer,
        replay_offset: replayOffset,
        replace: previousStreaming?.replace,
        sensitiveOutputBlocked: isSensitiveBlocked || previousStreaming?.sensitiveOutputBlocked,
      },
    },
  };
}

function removeRetractedSuffix(answer: string, content: string, length?: number): string {
  if (!answer) {
    return answer;
  }
  if (content && answer.endsWith(content)) {
    return answer.slice(0, -content.length);
  }
  const safeLength =
    typeof length === 'number' && Number.isFinite(length) && length > 0
      ? Math.min(Math.floor(length), answer.length)
      : 0;
  if (!content && safeLength > 0) {
    return answer.slice(0, -safeLength);
  }
  return answer;
}

function skillInvocationsFromRuntimeTimeline(
  timeline: AIChatAgenticTimelineItem[] | undefined
): AIChatSkillInvocation[] {
  return (timeline ?? []).flatMap(item => {
    if (item.type === 'skill_event') {
      return [item.invocation];
    }
    if (item.type === 'intermediate_answer' && item.content.trim()) {
      return [
        {
          kind: 'intermediate_answer',
          skill_id: '',
          answer_id: item.answer_id,
          title: item.title,
          status: item.status === 'success' ? 'success' : 'running',
          message: item.content,
          created_at: item.created_at,
        } satisfies AIChatSkillInvocation,
      ];
    }
    return [];
  });
}

function mergeRuntimeTimelineMetadata(
  messageMetadata: AIChatMessageMetadata | undefined,
  payloadMetadata: AIChatMessageMetadata | undefined,
  runtimeTimeline: AIChatAgenticTimelineItem[] | undefined
): AIChatMessageMetadata | undefined {
  const runtimeSkillInvocations = skillInvocationsFromRuntimeTimeline(runtimeTimeline);
  if (runtimeSkillInvocations.length === 0) {
    return mergeMessageMetadata(messageMetadata, payloadMetadata);
  }
  return mergeMessageMetadata(
    mergeMessageMetadata(messageMetadata, { skill_invocations: runtimeSkillInvocations }),
    payloadMetadata
  );
}

export function applyMessageRetractState(
  current: AIChatControllerState,
  payload: AIChatMessageRetractEventData,
  eventId?: string | null
): AIChatControllerState {
  const content = payload.content ?? '';
  if (!payload.conversation_id || !payload.message_id) {
    return current;
  }

  const messages = current.messagesByConversation[payload.conversation_id] ?? [];
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (isStaleAIChatStreamEvent(eventId, previousStreaming?.last_event_id)) {
    return current;
  }
  const nextMessages = messages.map(message =>
    message.id === payload.message_id
      ? {
          ...message,
          answer: removeRetractedSuffix(message.answer, content, payload.length),
        }
      : message
  );

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
            answer: removeRetractedSuffix(previousStreaming.answer, content, payload.length),
            last_event_id: eventId ?? previousStreaming.last_event_id,
          },
        }
      : current.streamingByMessageId,
  };
}

export function applyMessageEndState(
  current: AIChatControllerState,
  payload: AIChatMessageEndEventData,
  eventId?: string | null
): AIChatControllerState {
  const endedAt = Math.floor(Date.now() / 1000);
  const messages = current.messagesByConversation[payload.conversation_id] ?? [];
  const endedMessage = messages.find(message => message.id === payload.message_id);
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (isStaleAIChatStreamEvent(eventId, previousStreaming?.last_event_id)) {
    return current;
  }
  const nextTimeline = removeTransientProgressItems(previousStreaming?.timeline);
  const nextMessages = messages.map(message =>
    message.id === payload.message_id
      ? {
          ...message,
          status: normalizeAIChatStatus(payload.status),
          metadata:
            message.metadata?.sensitiveOutputBlocked === true
              ? {
                  ...mergeRuntimeTimelineMetadata(message.metadata, payload.metadata, nextTimeline),
                  sensitiveOutputBlocked: true,
                }
              : mergeRuntimeTimelineMetadata(message.metadata, payload.metadata, nextTimeline),
          updated_at: endedAt,
        }
      : message
  );
  const nextStreamingByMessageId = { ...current.streamingByMessageId };
  const terminalStatus = normalizeAIChatStatus(payload.status);
  if (
    previousStreaming &&
    nextTimeline.length &&
    // Keep the final runtime timeline until the next turn so live rendering and
    // hydrated history reconcile through the same timeline merge path.
    (terminalStatus === 'waiting_approval' ||
      terminalStatus === 'waiting_client_action' ||
      terminalStatus === 'waiting_question' ||
      terminalStatus === 'completed' ||
      terminalStatus === 'error' ||
      terminalStatus === 'stopped')
  ) {
    nextStreamingByMessageId[payload.message_id] = {
      ...previousStreaming,
      timeline: nextTimeline,
      status: terminalStatus,
      last_event_id: eventId ?? previousStreaming.last_event_id,
    };
  } else {
    delete nextStreamingByMessageId[payload.message_id];
  }

  return {
    ...current,
    conversations: current.conversations.map(conversation =>
      conversation.id === payload.conversation_id
        ? {
            ...conversation,
            runtime_status: 'idle' as const,
            active_message_id: undefined,
            current_leaf_message_id: shouldAdvanceConversationLeaf(
              conversation,
              payload.message_id,
              endedMessage?.parent_id
            )
              ? payload.message_id
              : conversation.current_leaf_message_id,
            dialogue_count:
              endedMessage && !endedMessage.parent_id ? 1 : conversation.dialogue_count,
            updated_at: endedAt,
          }
        : conversation
    ),
    messagesByConversation: {
      ...current.messagesByConversation,
      [payload.conversation_id]: nextMessages,
    },
    streamingByMessageId: nextStreamingByMessageId,
    recoveringByConversation: {
      ...current.recoveringByConversation,
      [payload.conversation_id]: false,
    },
    isSending: getNextActiveSendingState(current, payload.conversation_id, false),
  };
}

export function applyStreamErrorState(
  current: AIChatControllerState,
  payload: AIChatErrorEventData,
  fallbackConversationId: string | null
): AIChatControllerState {
  const conversationId = payload.conversation_id || fallbackConversationId;
  const messageId = payload.message_id;
  const message = payload.message || 'AIChat stream error';
  const errorMetadata =
    payload.code || payload.params
      ? {
          error_code: payload.code,
          error_params: payload.params,
        }
      : undefined;
  const messages = conversationId ? (current.messagesByConversation[conversationId] ?? []) : [];
  const erroredMessage = messageId ? messages.find(item => item.id === messageId) : undefined;
  const previousStreaming = messageId ? current.streamingByMessageId[messageId] : undefined;
  const preservedTimeline = removeTransientProgressItems(previousStreaming?.timeline);
  const nextStreamingByMessageId = { ...current.streamingByMessageId };
  if (messageId) {
    delete nextStreamingByMessageId[messageId];
  }

  return {
    ...current,
    error: message,
    isSending: getNextActiveSendingState(current, conversationId, false),
    conversations: conversationId
      ? current.conversations.map(conversation =>
          conversation.id === conversationId
            ? {
                ...conversation,
                runtime_status: 'idle' as const,
                active_message_id: undefined,
                current_leaf_message_id:
                  messageId &&
                  shouldAdvanceConversationLeaf(conversation, messageId, erroredMessage?.parent_id)
                    ? messageId
                    : conversation.current_leaf_message_id,
                dialogue_count:
                  messageId && erroredMessage && !erroredMessage.parent_id
                    ? 1
                    : conversation.dialogue_count,
              }
            : conversation
        )
      : current.conversations,
    messagesByConversation: conversationId
      ? {
          ...current.messagesByConversation,
          [conversationId]:
            conversationId && messageId
              ? messages.map(item =>
                  item.id === messageId
                    ? {
                        ...item,
                        status: 'error' as const,
                        error: message,
                        metadata: mergeRuntimeTimelineMetadata(
                          item.metadata,
                          errorMetadata,
                          preservedTimeline
                        ),
                        updated_at: Math.floor(Date.now() / 1000),
                      }
                    : item
                )
              : messages,
        }
      : current.messagesByConversation,
    streamingByMessageId: nextStreamingByMessageId,
    recoveringByConversation: conversationId
      ? {
          ...current.recoveringByConversation,
          [conversationId]: false,
        }
      : current.recoveringByConversation,
  };
}
