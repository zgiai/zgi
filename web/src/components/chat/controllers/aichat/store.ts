import { createStore } from 'zustand/vanilla';
import {
  applyFileParseEndState,
  applyFileParseErrorState,
  applyFileParseStartState,
  applyAgentProgressState,
  applyIntermediateAnswerState,
  applyMessageChunkState,
  applyMessageEndState,
  applyMessageStartState,
  applyStreamErrorState,
  mergeAIChatMessages,
} from './state-reducers';
import {
  DEFAULT_AICHAT_PAGINATION,
  type AIChatControllerState,
  type AIChatControllerStore,
  type AIChatSetControllerState,
} from './types';

export const createAIChatInitialState = (): AIChatControllerState => ({
  conversations: [],
  pagination: DEFAULT_AICHAT_PAGINATION,
  activeConversationId: null,
  messagesByConversation: {},
  messagePaginationByConversation: {},
  loadingOlderByConversation: {},
  streamingByMessageId: {},
  recoveringByConversation: {},
  stoppingByConversation: {},
  isLoadingList: false,
  isLoadingMessages: false,
  isSending: false,
  error: null,
});

/**
 * @util createAIChatControllerStore
 * @description Creates the runtime store for AIChat state and stream event reducers.
 */
export function createAIChatControllerStore() {
  return createStore<AIChatControllerStore>()((set, get) => ({
    ...createAIChatInitialState(),
    update: updater => set(current => updater(current)),
    replaceState: nextState =>
      set({
        ...nextState,
        update: get().update,
        replaceState: get().replaceState,
      }),
    applyMessageStart: (payload, context, eventId) =>
      set(current => applyMessageStartState(current, payload, context, eventId)),
    applyMessageChunk: (payload, eventId) =>
      set(current => applyMessageChunkState(current, payload, eventId)),
    applyAgentProgress: (payload, eventId) =>
      set(current => applyAgentProgressState(current, payload, eventId)),
    applyIntermediateAnswer: (payload, eventId) =>
      set(current => applyIntermediateAnswerState(current, payload, eventId)),
    applyFileParseStart: (payload, eventId) =>
      set(current => applyFileParseStartState(current, payload, eventId)),
    applyFileParseEnd: (payload, eventId) =>
      set(current => applyFileParseEndState(current, payload, eventId)),
    applyFileParseError: (payload, eventId) =>
      set(current => applyFileParseErrorState(current, payload, eventId)),
    applyMessageEnd: payload => set(current => applyMessageEndState(current, payload)),
    applyStreamError: (payload, fallbackConversationId) =>
      set(current => applyStreamErrorState(current, payload, fallbackConversationId)),
    mergeMessages: (conversationId, messages) =>
      set(current => ({
        ...current,
        messagesByConversation: {
          ...current.messagesByConversation,
          [conversationId]: mergeAIChatMessages(
            current.messagesByConversation[conversationId] ?? [],
            messages
          ),
        },
      })),
    setActiveConversationId: conversationId =>
      set({
        activeConversationId: conversationId,
      }),
    setConversationRunningState: (conversationId, running, activeMessageId) =>
      set(current => ({
        ...current,
        conversations: current.conversations.map(conversation =>
          conversation.id === conversationId
            ? {
                ...conversation,
                runtime_status: running ? 'streaming' : 'idle',
                active_message_id: running ? activeMessageId : undefined,
              }
            : conversation
        ),
        recoveringByConversation: {
          ...current.recoveringByConversation,
          [conversationId]: running,
        },
      })),
  }));
}

export function createAIChatStoreUpdater(
  store: ReturnType<typeof createAIChatControllerStore>
): AIChatSetControllerState {
  return updater => {
    store.getState().update(updater);
  };
}
