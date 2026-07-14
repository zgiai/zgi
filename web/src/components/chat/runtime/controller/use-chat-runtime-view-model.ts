import { useMemo } from 'react';
import { useStore } from 'zustand';
import type { MutableRefObject } from 'react';
import type { StoreApi } from 'zustand/vanilla';
import type { AIChatControllerStore } from '@/components/chat/controllers/aichat';
import {
  selectActiveConversation,
  selectActiveMessagePagination,
  selectActiveMessages,
  selectBranchNavigationByMessageId,
  selectDisplayMessageIds,
  selectDisplayMessages,
  selectIsLoadingOlderMessages,
  selectIsRecoveringMessages,
  selectIsStopping,
} from '@/components/chat/controllers/aichat/selectors';
import {
  buildChatMessageTopology,
  buildChatMessageTopologyKey,
  type ChatMessageTopology,
} from '@/components/chat/utils/message-tree';

interface UseChatRuntimeViewModelArgs {
  store: StoreApi<AIChatControllerStore>;
  topologyRef: MutableRefObject<{ key: string; topology: ChatMessageTopology } | null>;
}

export function useChatRuntimeViewModel({ store, topologyRef }: UseChatRuntimeViewModelArgs) {
  const conversations = useStore(store, state => state.conversations);
  const pagination = useStore(store, state => state.pagination);
  const activeConversationId = useStore(store, state => state.activeConversationId);
  const activeConversation = useStore(store, selectActiveConversation);
  const messages = useStore(store, selectActiveMessages);
  const streamingByMessageId = useStore(store, state => state.streamingByMessageId);
  const activeMessagePagination = useStore(store, selectActiveMessagePagination);
  const isLoadingList = useStore(store, state => state.isLoadingList);
  const isLoadingMessages = useStore(store, state => state.isLoadingMessages);
  const isLoadingOlderMessages = useStore(store, selectIsLoadingOlderMessages);
  const isRecoveringMessages = useStore(store, selectIsRecoveringMessages);
  const isStopping = useStore(store, selectIsStopping);
  const isSending = useStore(store, state => state.isSending);
  const connectionState = useStore(
    store,
    state =>
      (state.activeConversationId
        ? state.connectionByConversation[state.activeConversationId]
        : undefined) ?? 'idle'
  );
  const error = useStore(store, state => state.error);

  const messageTopologyKey = useMemo(() => buildChatMessageTopologyKey(messages), [messages]);

  const messageTopology = useMemo(() => {
    if (topologyRef.current?.key === messageTopologyKey) {
      return topologyRef.current.topology;
    }

    const topology = buildChatMessageTopology(messages);
    topologyRef.current = { key: messageTopologyKey, topology };
    return topology;
  }, [messageTopologyKey, messages, topologyRef]);

  const displayMessageIds = useMemo(
    () => selectDisplayMessageIds(activeConversation, messageTopology),
    [activeConversation, messageTopology]
  );

  const displayMessages = useMemo(
    () => selectDisplayMessages(displayMessageIds, messages),
    [displayMessageIds, messages]
  );

  const branchNavigationByMessageId = useMemo(
    () => selectBranchNavigationByMessageId(displayMessageIds, messageTopology),
    [displayMessageIds, messageTopology]
  );

  return {
    conversations,
    pagination,
    activeConversationId,
    activeConversation,
    messages,
    streamingByMessageId,
    displayMessageIds,
    displayMessages,
    branchNavigationByMessageId,
    activeMessagePagination,
    isLoadingList,
    isLoadingMessages,
    isLoadingOlderMessages,
    isRecoveringMessages,
    isStopping,
    isSending,
    connectionState,
    error,
  };
}
