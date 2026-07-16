import { useCallback } from 'react';
import type { MutableRefObject } from 'react';
import type {
  AIChatControllerStore,
  AIChatSetControllerState,
} from '@/components/chat/controllers/aichat';
import type { AIChatConversation, AIChatMessage } from '@/services/types/aichat';
import type { AIChatRuntimeTransport } from '@/components/chat/transports/aichat-transport';
import { replaceAIChatConversation } from '@/components/chat/utils/aichat-message';
import { buildChatMessageTopology } from '@/components/chat/utils/message-tree';
import { getErrorMessage } from '@/components/chat/runtime/controller/chat-runtime-controller-utils';

interface UseChatRuntimeBranchActionsArgs {
  stateRef: MutableRefObject<AIChatControllerStore>;
  transportRef: MutableRefObject<AIChatRuntimeTransport>;
  setControllerState: AIChatSetControllerState;
}

function isPersistableChatLeaf(
  conversation: AIChatConversation,
  message: AIChatMessage | undefined
) {
  if (!message) return false;
  if (
    message.status === 'completed' ||
    message.status === 'stopped' ||
    message.status === 'error' ||
    message.status === 'waiting_approval' ||
    message.status === 'waiting_question' ||
    message.status === 'waiting_client_action'
  ) {
    return true;
  }
  return (
    message.status === 'streaming' &&
    conversation.runtime_status === 'streaming' &&
    conversation.active_message_id === message.id
  );
}

function findPersistableChatBranchLeaf(
  messageId: string,
  messages: AIChatMessage[],
  conversation: AIChatConversation
) {
  const byId = new Map(messages.map(message => [message.id, message]));
  const start = byId.get(messageId);
  if (!start || !isPersistableChatLeaf(conversation, start)) return null;

  const topology = buildChatMessageTopology(messages);
  let cursor = start;
  let lastPersistable = start;
  const visited = new Set<string>();

  while (!visited.has(cursor.id)) {
    visited.add(cursor.id);
    const children = topology.childrenIdsByParent.get(cursor.id) ?? [];
    if (children.length === 0) return lastPersistable.id;

    const next = byId.get(children[children.length - 1]);
    if (!next || !isPersistableChatLeaf(conversation, next)) return lastPersistable.id;
    cursor = next;
    lastPersistable = next;
  }

  return lastPersistable.id;
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

        const resolvedLeafId = findPersistableChatBranchLeaf(messageId, messages, conversation);
        if (!resolvedLeafId) {
          return current;
        }
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
          setControllerState(current => {
            const currentConversation = current.conversations.find(
              item => item.id === activeConversationId
            );
            if (!currentConversation) return current;

            const safeConversation =
              currentConversation.current_leaf_message_id === nextLeafId
                ? conversation
                : {
                    ...conversation,
                    current_leaf_message_id: currentConversation.current_leaf_message_id,
                    runtime_status: currentConversation.runtime_status,
                    active_message_id: currentConversation.active_message_id,
                  };

            return {
              ...current,
              conversations: replaceAIChatConversation(current.conversations, safeConversation, {
                moveToTop: false,
              }),
            };
          });
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
