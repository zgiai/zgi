import { useCallback } from 'react';
import type { MutableRefObject } from 'react';
import {
  DEFAULT_AICHAT_MESSAGE_PAGINATION,
  DEFAULT_AICHAT_PAGINATION,
  type AIChatControllerState,
  type AIChatSetControllerState,
} from '@/components/chat/controllers/aichat';
import { getNextActiveSendingState } from '@/components/chat/controllers/aichat/selectors';
import { mergeAIChatMessages } from '@/components/chat/controllers/aichat/state-reducers';
import type { AIChatRuntimeTransport } from '@/components/chat/transports/aichat-transport';
import { replaceAIChatConversation } from '@/components/chat/utils/aichat-message';
import {
  getErrorMessage,
  removeRunningStreamingStateByConversation,
} from '@/components/chat/runtime/controller/chat-runtime-controller-utils';

interface UseChatRuntimeRefreshersArgs {
  transportRef: MutableRefObject<AIChatRuntimeTransport>;
  setControllerState: AIChatSetControllerState;
}

/**
 * @hook useChatRuntimeRefreshers
 * @description Keeps controller state in sync with runtime conversation/message APIs.
 */
export function useChatRuntimeRefreshers({
  transportRef,
  setControllerState,
}: UseChatRuntimeRefreshersArgs) {
  const refreshConversationSilently = useCallback(
    (conversationId: string) => {
      void transportRef.current
        .refreshConversation(conversationId)
        .then(conversation => {
          setControllerState(current => {
            const nextState: AIChatControllerState = {
              ...current,
              conversations: replaceAIChatConversation(current.conversations, conversation),
            };

            if (conversation.runtime_status === 'streaming' && conversation.active_message_id) {
              return nextState;
            }

            return {
              ...nextState,
              isSending: getNextActiveSendingState(current, conversationId, false),
              streamingByMessageId: removeRunningStreamingStateByConversation(
                current.streamingByMessageId,
                conversationId
              ),
              recoveringByConversation: {
                ...current.recoveringByConversation,
                [conversationId]: false,
              },
              stoppingByConversation: {
                ...current.stoppingByConversation,
                [conversationId]: false,
              },
            };
          });
        })
        .catch(() => undefined);
    },
    [setControllerState, transportRef]
  );

  const refreshMessagesSilently = useCallback(
    (conversationId: string) => {
      void transportRef.current
        .listMessages(conversationId, {
          page: 1,
          limit: DEFAULT_AICHAT_MESSAGE_PAGINATION.limit,
        })
        .then(response => {
          setControllerState(current => ({
            ...current,
            messagesByConversation: {
              ...current.messagesByConversation,
              [conversationId]: mergeAIChatMessages(
                current.messagesByConversation[conversationId] ?? [],
                response.items
              ),
            },
            messagePaginationByConversation: {
              ...current.messagePaginationByConversation,
              [conversationId]: response.pagination,
            },
          }));
        })
        .catch(() => undefined);
    },
    [setControllerState, transportRef]
  );

  const refreshList = useCallback(
    async (params: { page?: number; append?: boolean } = {}) => {
      const page = params.page ?? 1;
      const limit = DEFAULT_AICHAT_PAGINATION.limit;
      setControllerState(current => ({ ...current, isLoadingList: true, error: null }));

      try {
        const response = await transportRef.current.listConversations({ page, limit });
        const incoming = response.items;
        setControllerState(current => {
          const conversations = params.append
            ? [
                ...current.conversations,
                ...incoming.filter(
                  item => !current.conversations.some(existing => existing.id === item.id)
                ),
              ]
            : incoming;

          return {
            ...current,
            conversations,
            pagination: response.pagination,
          };
        });
      } catch (error) {
        setControllerState(current => ({ ...current, error: getErrorMessage(error) }));
      } finally {
        setControllerState(current => ({ ...current, isLoadingList: false }));
      }
    },
    [setControllerState, transportRef]
  );

  return {
    refreshConversationSilently,
    refreshMessagesSilently,
    refreshList,
  };
}
