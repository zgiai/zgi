'use client';

import { useCallback, useEffect, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type {
  AIChatMemoryMutationEventData,
  AIChatMessageStartEventData,
} from '@/services/types/aichat';
import {
  createAIChatControllerStore,
  type AIChatController,
  type AIChatControllerState,
  type AIChatControllerStore,
  type AIChatRecoveryMode,
} from '@/components/chat/controllers/aichat';
import {
  aichatTransport,
  type AIChatRuntimeTransport,
} from '@/components/chat/transports/aichat-transport';
import { isDraftAIChatConversationId } from '@/components/chat/utils/aichat-message';
import type { ChatMessageTopology } from '@/components/chat/utils/message-tree';
import { useAIChatStreamRuntime } from '@/components/chat/controllers/aichat/stream-runtime';
import { useChatRuntimeEventAppliers } from '@/components/chat/runtime/controller/use-chat-runtime-event-appliers';
import { useChatRuntimeRefreshers } from '@/components/chat/runtime/controller/use-chat-runtime-refreshers';
import { useChatRuntimeBranchActions } from '@/components/chat/runtime/controller/use-chat-runtime-branch-actions';
import { useChatRuntimeConversationActions } from '@/components/chat/runtime/controller/use-chat-runtime-conversation-actions';
import { useChatRuntimeMessageActions } from '@/components/chat/runtime/controller/use-chat-runtime-message-actions';
import { useChatRuntimeStreamRecovery } from '@/components/chat/runtime/controller/use-chat-runtime-stream-recovery';
import { useChatRuntimeViewModel } from '@/components/chat/runtime/controller/use-chat-runtime-view-model';
import { MEMORY_KEYS } from '@/hooks/query-keys';

/**
 * @hook useChatRuntimeController
 * @description Shared controller for ChatRuntime callers such as AIChat and AGENT.
 */
export function useChatRuntimeController(options?: {
  transport?: AIChatRuntimeTransport;
  requireModel?: boolean;
}): AIChatController {
  const queryClient = useQueryClient();
  const requireModel = options?.requireModel ?? true;
  const transportRef = useRef<AIChatRuntimeTransport>(options?.transport ?? aichatTransport);
  useEffect(() => {
    if (options?.transport) {
      transportRef.current = options.transport;
    }
  }, [options?.transport]);
  const storeRef = useRef<ReturnType<typeof createAIChatControllerStore> | null>(null);
  if (!storeRef.current) {
    storeRef.current = createAIChatControllerStore();
  }
  const store = storeRef.current;
  const stateRef = useRef<AIChatControllerStore>(store.getState());
  const topologyRef = useRef<{ key: string; topology: ChatMessageTopology } | null>(null);
  const initializedRef = useRef(false);
  const selectionSeqRef = useRef(0);
  const latestSelectionRef = useRef<{ seq: number; conversationId: string | null }>({
    seq: 0,
    conversationId: null,
  });

  const setControllerState = useCallback(
    (updater: (current: AIChatControllerState) => AIChatControllerState) => {
      store.getState().update(updater);
      stateRef.current = store.getState();
    },
    [store]
  );

  useEffect(() => {
    stateRef.current = store.getState();
    return store.subscribe(nextState => {
      stateRef.current = nextState;
    });
  }, [store]);

  const {
    pendingStreamAbortRef,
    streamAbortByConversationRef,
    streamingMessageRef,
    recoveryAbortByConversationRef,
    recoveryRetryTimeoutsRef,
    recoveryModeByConversationRef,
    backgroundConversationIdRef,
    clearRecoveryRetry,
    closeRecoveryConnection,
    closeConversationConnection,
    setBackgroundConversation,
    setRecoveryMode,
  } = useAIChatStreamRuntime(setControllerState);

  const refreshAccountMemoryAfterMemoryMutation = useCallback(
    (_payload: AIChatMemoryMutationEventData) => {
      void queryClient.invalidateQueries({ queryKey: MEMORY_KEYS.me() });
    },
    [queryClient]
  );

  const markSelectionTarget = useCallback((conversationId: string | null) => {
    const nextSeq = selectionSeqRef.current + 1;
    selectionSeqRef.current = nextSeq;
    latestSelectionRef.current = {
      seq: nextSeq,
      conversationId,
    };
    return nextSeq;
  }, []);

  const isLatestSelection = useCallback((seq: number, conversationId: string | null) => {
    const latest = latestSelectionRef.current;
    return latest.seq === seq && latest.conversationId === conversationId;
  }, []);

  const migrateLatestSelectionTarget = useCallback((from: string | null, to: string) => {
    if (latestSelectionRef.current.conversationId !== from) return;
    latestSelectionRef.current = {
      ...latestSelectionRef.current,
      conversationId: to,
    };
  }, []);

  const resolveMessageStartMode = useCallback(
    (
      payload: AIChatMessageStartEventData,
      context: {
        previousConversationId?: string | null;
        mode?: AIChatRecoveryMode;
      }
    ): AIChatRecoveryMode => {
      const requestedMode = context.mode ?? 'active';
      if (requestedMode !== 'active') return requestedMode;

      const currentActiveConversationId = stateRef.current.activeConversationId;
      if (currentActiveConversationId === payload.conversation_id) {
        return 'active';
      }

      const previousConversationId = context.previousConversationId;
      if (
        previousConversationId &&
        isDraftAIChatConversationId(previousConversationId) &&
        currentActiveConversationId === previousConversationId
      ) {
        return 'active';
      }

      return 'background';
    },
    []
  );

  const { refreshConversationSilently, refreshMessagesSilently, refreshList } =
    useChatRuntimeRefreshers({
      transportRef,
      setControllerState,
    });

  const eventAppliers = useChatRuntimeEventAppliers({
    stateRef,
    backgroundConversationIdRef,
    streamingMessageRef,
    recoveryModeByConversationRef,
    setControllerState,
    resolveMessageStartMode,
    migrateLatestSelectionTarget,
    clearRecoveryRetry,
    refreshConversationSilently,
    refreshMessagesSilently,
  });

  const recoverStreamingConversation = useChatRuntimeStreamRecovery({
    stateRef,
    transportRef,
    recoveryAbortByConversationRef,
    recoveryModeByConversationRef,
    recoveryRetryTimeoutsRef,
    setRecoveryMode,
    clearRecoveryRetry,
    closeRecoveryConnection,
    setControllerState,
    refreshAccountMemoryAfterMemoryMutation,
    eventAppliers,
  });
  const { init, select, startNew, stop, remove, rename, loadOlderMessages } =
    useChatRuntimeConversationActions({
      stateRef,
      initializedRef,
      transportRef,
      pendingStreamAbortRef,
      streamAbortByConversationRef,
      streamingMessageRef,
      recoveryAbortByConversationRef,
      backgroundConversationIdRef,
      setBackgroundConversation,
      setRecoveryMode,
      closeConversationConnection,
      setControllerState,
      markSelectionTarget,
      isLatestSelection,
      refreshList,
      refreshConversationSilently,
      recoverStreamingConversation,
    });
  const {
    send,
    regenerate,
    replaceRootMessage,
    continueWorkflowApproval,
    continueWorkflowQuestion,
  } = useChatRuntimeMessageActions({
    stateRef,
    transportRef,
    requireModel,
    pendingStreamAbortRef,
    streamAbortByConversationRef,
    streamingMessageRef,
    setControllerState,
    markSelectionTarget,
    refreshAccountMemoryAfterMemoryMutation,
    eventAppliers,
  });

  const { switchBranch } = useChatRuntimeBranchActions({
    stateRef,
    transportRef,
    setControllerState,
  });
  const search = useCallback((query: string, limit: number) => {
    return transportRef.current.searchConversations?.(query, limit) ?? Promise.resolve([]);
  }, []);
  const viewModel = useChatRuntimeViewModel({ store, topologyRef });
  return {
    store,
    conversations: viewModel.conversations,
    pagination: viewModel.pagination,
    activeConversationId: viewModel.activeConversationId,
    activeConversation: viewModel.activeConversation,
    messages: viewModel.messages,
    streamingByMessageId: viewModel.streamingByMessageId,
    displayMessageIds: viewModel.displayMessageIds,
    displayMessages: viewModel.displayMessages,
    branchNavigationByMessageId: viewModel.branchNavigationByMessageId,
    activeMessagePagination: viewModel.activeMessagePagination,
    isLoadingList: viewModel.isLoadingList,
    isLoadingMessages: viewModel.isLoadingMessages,
    isLoadingOlderMessages: viewModel.isLoadingOlderMessages,
    isRecoveringMessages: viewModel.isRecoveringMessages,
    isStopping: viewModel.isStopping,
    isSending: viewModel.isSending,
    error: viewModel.error,
    init,
    refreshList,
    select,
    startNew,
    remove,
    rename,
    loadOlderMessages,
    recoverStreamingConversation,
    send,
    regenerate,
    replaceRootMessage,
    continueWorkflowApproval,
    continueWorkflowQuestion,
    stop,
    switchBranch,
    search,
  };
}

export const useAIChatController = useChatRuntimeController;
