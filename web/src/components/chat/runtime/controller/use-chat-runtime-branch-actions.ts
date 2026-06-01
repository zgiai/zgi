import { useCallback } from 'react';
import type { MutableRefObject } from 'react';
import type {
  AIChatControllerStore,
  AIChatSetControllerState,
} from '@/components/chat/controllers/aichat';
import type { AIChatRuntimeTransport } from '@/components/chat/transports/aichat-transport';
import { replaceAIChatConversation } from '@/components/chat/utils/aichat-message';
import { findChatBranchLeaf } from '@/components/chat/utils/message-tree';
import { getErrorMessage } from '@/components/chat/runtime/controller/chat-runtime-controller-utils';

interface UseChatRuntimeBranchActionsArgs {
  stateRef: MutableRefObject<AIChatControllerStore>;
  transportRef: MutableRefObject<AIChatRuntimeTransport>;
  setControllerState: AIChatSetControllerState;
}

export function useChatRuntimeBranchActions({
  stateRef,
  transportRef,
  setControllerState,
}: UseChatRuntimeBranchActionsArgs) {
  const switchBranch = useCallback(
    (messageId: string) => {
      const activeConversationId = stateRef.current.activeConversationId;
      if (!activeConversationId || !messageId || stateRef.current.isSending) return;

      let previousLeafId: string | undefined;
      let nextLeafId: string | undefined;
      setControllerState(current => {
        const messages = current.messagesByConversation[activeConversationId] ?? [];
        const conversation = current.conversations.find(item => item.id === activeConversationId);
        if (!conversation || !messages.some(message => message.id === messageId)) return current;

        const resolvedLeafId = findChatBranchLeaf(messageId, messages);
        if (conversation.current_leaf_message_id === resolvedLeafId) return current;
        previousLeafId = conversation.current_leaf_message_id;
        nextLeafId = resolvedLeafId;

        return {
          ...current,
          conversations: current.conversations.map(item =>
            item.id === activeConversationId
              ? {
                  ...item,
                  current_leaf_message_id: resolvedLeafId,
                }
              : item
          ),
        };
      });

      if (!nextLeafId) return;
      void transportRef.current
        .updateConversation(activeConversationId, {
          current_leaf_message_id: nextLeafId,
        })
        .then(conversation => {
          setControllerState(current => ({
            ...current,
            conversations: replaceAIChatConversation(current.conversations, conversation, {
              moveToTop: false,
            }),
          }));
        })
        .catch(error => {
          setControllerState(current => ({
            ...current,
            error:
              current.activeConversationId === activeConversationId
                ? getErrorMessage(error)
                : current.error,
            conversations: current.conversations.map(item =>
              item.id === activeConversationId && item.current_leaf_message_id === nextLeafId
                ? {
                    ...item,
                    current_leaf_message_id: previousLeafId,
                  }
                : item
            ),
          }));
        });
    },
    [setControllerState, stateRef, transportRef]
  );

  return { switchBranch };
}
