import type { AIChatConversation, AIChatMessage } from '@/services/types/aichat';
import {
  DEFAULT_AICHAT_MESSAGE_PAGINATION,
  type AIChatControllerState,
  type AIChatPagination,
  type AIChatStreamingMessageState,
} from '@/components/chat/controllers/aichat/types';
import {
  buildChatBranchNavigationByMessageId,
  buildChatMessageById,
  getCurrentChatPathIds,
  materializeChatMessages,
  type ChatMessageTopology,
} from '@/components/chat/utils/message-tree';
import { upsertAIChatMessage } from '@/components/chat/utils/aichat-message';

const EMPTY_AICHAT_MESSAGES: AIChatMessage[] = [];

export function hasRunningMessageState(
  streamingByMessageId: Record<string, AIChatStreamingMessageState>,
  conversationId: string
): boolean {
  return Object.values(streamingByMessageId).some(
    streaming => streaming.conversation_id === conversationId && streaming.status === 'streaming'
  );
}

export function shouldTreatConversationAsRunning(
  state: AIChatControllerState,
  conversationId: string | null | undefined
): boolean {
  if (!conversationId) return false;
  const conversation = state.conversations.find(item => item.id === conversationId);

  return Boolean(
    conversation?.runtime_status === 'streaming' ||
      state.recoveringByConversation[conversationId] ||
      state.stoppingByConversation[conversationId] ||
      hasRunningMessageState(state.streamingByMessageId, conversationId)
  );
}

export function getNextActiveSendingState(
  state: AIChatControllerState,
  conversationId: string | null | undefined,
  isSending: boolean
): boolean {
  return state.activeConversationId === conversationId ? isSending : state.isSending;
}

export function mergeSelectedMessagesWithStreamingState(
  conversation: AIChatConversation,
  incomingMessages: AIChatMessage[],
  current: AIChatControllerState
): AIChatMessage[] {
  const messageId = conversation.active_message_id;
  if (conversation.runtime_status !== 'streaming' || !messageId) {
    return incomingMessages;
  }

  const streamingState = current.streamingByMessageId[messageId];
  const localMessages = current.messagesByConversation[conversation.id] ?? [];
  const localMessage = localMessages.find(message => message.id === messageId);
  if (!localMessage) {
    return incomingMessages;
  }

  const serverMessage = incomingMessages.find(message => message.id === messageId);
  const shouldPreferServer =
    !streamingState?.last_event_id &&
    localMessage.status !== 'streaming' &&
    localMessage.answer.length < (serverMessage?.answer.length ?? 0);
  if (shouldPreferServer) {
    return incomingMessages;
  }

  const preservedMessage: AIChatMessage = {
    ...(serverMessage ?? localMessage),
    answer: localMessage.answer,
    status: 'streaming',
    updated_at: Math.max(serverMessage?.updated_at ?? 0, localMessage.updated_at),
  };

  return upsertAIChatMessage(incomingMessages, preservedMessage);
}

export function isTerminalReplaceableMessageStatus(status: AIChatMessage['status']): boolean {
  return status === 'completed' || status === 'stopped' || status === 'error';
}

export function canReplaceRootMessage(
  conversation: AIChatConversation | null | undefined,
  message: AIChatMessage | null | undefined,
  messages: AIChatMessage[]
): boolean {
  if (!conversation || !message) return false;
  return (
    conversation.runtime_status === 'idle' &&
    conversation.dialogue_count === 1 &&
    !message.parent_id &&
    conversation.current_leaf_message_id === message.id &&
    isTerminalReplaceableMessageStatus(message.status) &&
    messages.length === 1
  );
}

export function selectActiveConversation(state: AIChatControllerState): AIChatConversation | null {
  return state.activeConversationId
    ? state.conversations.find(item => item.id === state.activeConversationId) ?? null
    : null;
}

export function selectActiveMessages(state: AIChatControllerState): AIChatMessage[] {
  return state.activeConversationId
    ? (state.messagesByConversation[state.activeConversationId] ?? EMPTY_AICHAT_MESSAGES)
    : EMPTY_AICHAT_MESSAGES;
}

export function selectActiveMessagePagination(state: AIChatControllerState): AIChatPagination {
  return state.activeConversationId
    ? (state.messagePaginationByConversation[state.activeConversationId] ??
        DEFAULT_AICHAT_MESSAGE_PAGINATION)
    : DEFAULT_AICHAT_MESSAGE_PAGINATION;
}

export function selectIsLoadingOlderMessages(state: AIChatControllerState): boolean {
  return state.activeConversationId
    ? Boolean(state.loadingOlderByConversation[state.activeConversationId])
    : false;
}

export function selectIsRecoveringMessages(state: AIChatControllerState): boolean {
  return state.activeConversationId
    ? Boolean(state.recoveringByConversation[state.activeConversationId])
    : false;
}

export function selectIsStopping(state: AIChatControllerState): boolean {
  return state.activeConversationId
    ? Boolean(state.stoppingByConversation[state.activeConversationId])
    : false;
}

export function selectDisplayMessageIds(
  activeConversation: AIChatConversation | null,
  messageTopology: ChatMessageTopology
): string[] {
  return getCurrentChatPathIds(activeConversation, messageTopology);
}

export function selectDisplayMessages(
  displayMessageIds: string[],
  messages: AIChatMessage[]
): AIChatMessage[] {
  return materializeChatMessages(displayMessageIds, buildChatMessageById(messages));
}

export function selectBranchNavigationByMessageId(
  displayMessageIds: string[],
  messageTopology: ChatMessageTopology
) {
  return buildChatBranchNavigationByMessageId(displayMessageIds, messageTopology);
}
