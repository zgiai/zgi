import { useCallback } from 'react';
import type { MutableRefObject } from 'react';
import type { AIChatConversation } from '@/services/types/aichat';
import {
  DEFAULT_AICHAT_MESSAGE_PAGINATION,
  type AIChatControllerStore,
  type AIChatRecoveryMode,
  type AIChatSetControllerState,
} from '@/components/chat/controllers/aichat';
import {
  mergeSelectedMessagesWithStreamingState,
  seedStreamingTimelineFromMessages,
  shouldTreatConversationAsRunning,
} from '@/components/chat/controllers/aichat/selectors';
import {
  mergeAIChatMessages,
  removeStreamingStateByConversation,
} from '@/components/chat/controllers/aichat/state-reducers';
import type { AIChatRuntimeTransport } from '@/components/chat/transports/aichat-transport';
import {
  isDraftAIChatConversationId,
  replaceAIChatConversation,
} from '@/components/chat/utils/aichat-message';
import { getErrorMessage } from '@/components/chat/runtime/controller/chat-runtime-controller-utils';

interface UseChatRuntimeConversationActionsArgs {
  stateRef: MutableRefObject<AIChatControllerStore>;
  initializedRef: MutableRefObject<boolean>;
  transportRef: MutableRefObject<AIChatRuntimeTransport>;
  pendingStreamAbortRef: MutableRefObject<AbortController | null>;
  streamAbortByConversationRef: MutableRefObject<Record<string, AbortController>>;
  streamingMessageRef: MutableRefObject<{ conversationId: string; messageId: string } | null>;
  recoveryAbortByConversationRef: MutableRefObject<Record<string, AbortController>>;
  backgroundConversationIdRef: MutableRefObject<string | null>;
  setBackgroundConversation: (conversationId: string | null) => void;
  setRecoveryMode: (conversationId: string, mode: AIChatRecoveryMode) => void;
  closeConversationConnection: (conversationId: string) => void;
  setControllerState: AIChatSetControllerState;
  markSelectionTarget: (conversationId: string | null) => number;
  isLatestSelection: (seq: number, conversationId: string | null) => boolean;
  refreshList: (params?: { page?: number; append?: boolean }) => Promise<void>;
  refreshConversationSilently: (conversationId: string) => void;
  recoverStreamingConversation: (
    conversationId: string,
    options?: { conversation?: AIChatConversation; mode?: AIChatRecoveryMode }
  ) => Promise<void>;
}

export function useChatRuntimeConversationActions({
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
}: UseChatRuntimeConversationActionsArgs) {
  const select = useCallback(
    async (conversationId: string) => {
      if (!conversationId) return;
      const selectionSeq = markSelectionTarget(conversationId);
      const previousState = stateRef.current;
      const previousConversationId = previousState.activeConversationId;
      if (
        previousConversationId &&
        previousConversationId !== conversationId &&
        shouldTreatConversationAsRunning(previousState, previousConversationId)
      ) {
        setBackgroundConversation(previousConversationId);
        if (!isDraftAIChatConversationId(previousConversationId)) {
          const previousRecovery = recoveryAbortByConversationRef.current[previousConversationId];
          const previousStream = streamAbortByConversationRef.current[previousConversationId];
          if (previousRecovery && !previousRecovery.signal.aborted) {
            setRecoveryMode(previousConversationId, 'background');
          } else if (!previousStream || previousStream.signal.aborted) {
            queueMicrotask(() => {
              void recoverStreamingConversation(previousConversationId, { mode: 'background' });
            });
          }
        }
      }
      if (backgroundConversationIdRef.current === conversationId) {
        backgroundConversationIdRef.current = null;
      }
      const hasCachedMessages =
        (previousState.messagesByConversation[conversationId]?.length ?? 0) > 0;
      setControllerState(current => {
        const cachedConversation = current.conversations.find(
          conversation => conversation.id === conversationId
        );
        const cachedMessages = current.messagesByConversation[conversationId] ?? [];
        return {
          ...current,
          activeConversationId: conversationId,
          isLoadingMessages: !hasCachedMessages,
          isSending: shouldTreatConversationAsRunning(current, conversationId),
          streamingByMessageId: cachedConversation
            ? seedStreamingTimelineFromMessages(
                cachedConversation,
                cachedMessages,
                current.streamingByMessageId
              )
            : current.streamingByMessageId,
          error: null,
        };
      });
      try {
        const { conversation, messages, messagePagination } =
          await transportRef.current.getConversation(conversationId);

        const isStreamingConversation =
          conversation.runtime_status === 'streaming' && Boolean(conversation.active_message_id);
        const isCurrentSelection =
          isLatestSelection(selectionSeq, conversationId) &&
          stateRef.current.activeConversationId === conversationId;

        if (!isCurrentSelection) {
          setControllerState(current => ({
            ...current,
            conversations: replaceAIChatConversation(current.conversations, conversation),
            messagesByConversation: {
              ...current.messagesByConversation,
              [conversationId]: isStreamingConversation
                ? mergeSelectedMessagesWithStreamingState(conversation, messages, current)
                : mergeAIChatMessages(
                    current.messagesByConversation[conversationId] ?? [],
                    messages
                  ),
            },
            messagePaginationByConversation: {
              ...current.messagePaginationByConversation,
              [conversationId]: messagePagination,
            },
            streamingByMessageId: isStreamingConversation
              ? seedStreamingTimelineFromMessages(
                  conversation,
                  mergeSelectedMessagesWithStreamingState(conversation, messages, current),
                  current.streamingByMessageId
                )
              : removeStreamingStateByConversation(current.streamingByMessageId, conversationId),
            recoveringByConversation: isStreamingConversation
              ? current.recoveringByConversation
              : {
                  ...current.recoveringByConversation,
                  [conversationId]: false,
                },
            stoppingByConversation: isStreamingConversation
              ? current.stoppingByConversation
              : {
                  ...current.stoppingByConversation,
                  [conversationId]: false,
                },
          }));
          return;
        }

        if (!isStreamingConversation) {
          closeConversationConnection(conversationId);
        }

        setControllerState(current => {
          const nextMessages = mergeSelectedMessagesWithStreamingState(
            conversation,
            messages,
            current
          );
          const nextStreamingByMessageId = isStreamingConversation
            ? seedStreamingTimelineFromMessages(
                conversation,
                nextMessages,
                current.streamingByMessageId
              )
            : removeStreamingStateByConversation(current.streamingByMessageId, conversationId);

          return {
            ...current,
            isSending:
              current.activeConversationId === conversationId
                ? isStreamingConversation
                : current.isSending,
            conversations: replaceAIChatConversation(current.conversations, conversation),
            messagesByConversation: {
              ...current.messagesByConversation,
              [conversationId]: nextMessages,
            },
            messagePaginationByConversation: {
              ...current.messagePaginationByConversation,
              [conversationId]: messagePagination,
            },
            streamingByMessageId: nextStreamingByMessageId,
            recoveringByConversation: {
              ...current.recoveringByConversation,
              [conversationId]: isStreamingConversation
                ? current.recoveringByConversation[conversationId]
                : false,
            },
            stoppingByConversation: {
              ...current.stoppingByConversation,
              [conversationId]: isStreamingConversation
                ? current.stoppingByConversation[conversationId]
                : false,
            },
          };
        });

        if (isStreamingConversation) {
          queueMicrotask(() => {
            const mode =
              isLatestSelection(selectionSeq, conversationId) &&
              stateRef.current.activeConversationId === conversationId
                ? 'active'
                : 'background';
            const activeRecovery = recoveryAbortByConversationRef.current[conversationId];
            const activeStream = streamAbortByConversationRef.current[conversationId];
            if (activeRecovery && !activeRecovery.signal.aborted) {
              void recoverStreamingConversation(conversationId, {
                conversation,
                mode,
              });
              return;
            }
            if (!activeStream || activeStream.signal.aborted) {
              void recoverStreamingConversation(conversationId, {
                conversation,
                mode,
              });
              return;
            }
            setRecoveryMode(conversationId, mode);
            setControllerState(current => ({
              ...current,
              isSending:
                current.activeConversationId === conversationId && mode === 'active'
                  ? true
                  : current.isSending,
            }));
          });
        }
      } catch (error) {
        if (!isLatestSelection(selectionSeq, conversationId)) return;
        setControllerState(current => ({
          ...current,
          error: getErrorMessage(error),
          activeConversationId:
            current.activeConversationId === conversationId ? null : current.activeConversationId,
        }));
      } finally {
        if (isLatestSelection(selectionSeq, conversationId)) {
          setControllerState(current => ({
            ...current,
            isLoadingMessages:
              current.activeConversationId === conversationId ? false : current.isLoadingMessages,
          }));
        }
      }
    },
    [
      backgroundConversationIdRef,
      closeConversationConnection,
      isLatestSelection,
      markSelectionTarget,
      recoverStreamingConversation,
      recoveryAbortByConversationRef,
      setBackgroundConversation,
      setControllerState,
      setRecoveryMode,
      stateRef,
      streamAbortByConversationRef,
      transportRef,
    ]
  );

  const init = useCallback(
    (conversationId?: string | null) => {
      if (initializedRef.current) {
        if (conversationId && conversationId !== stateRef.current.activeConversationId) {
          void select(conversationId);
        }
        return;
      }

      initializedRef.current = true;
      void refreshList().then(() => {
        if (conversationId) {
          void select(conversationId);
        }
      });
    },
    [initializedRef, refreshList, select, stateRef]
  );

  const startNew = useCallback(() => {
    const currentState = stateRef.current;
    const activeConversationId = stateRef.current.activeConversationId;
    markSelectionTarget(null);
    if (isDraftAIChatConversationId(activeConversationId)) {
      pendingStreamAbortRef.current?.abort();
      pendingStreamAbortRef.current = null;
    } else if (
      activeConversationId &&
      shouldTreatConversationAsRunning(currentState, activeConversationId)
    ) {
      setBackgroundConversation(activeConversationId);
      const activeRecovery = recoveryAbortByConversationRef.current[activeConversationId];
      const activeStream = streamAbortByConversationRef.current[activeConversationId];
      if (activeRecovery && !activeRecovery.signal.aborted) {
        setRecoveryMode(activeConversationId, 'background');
      } else if (!activeStream || activeStream.signal.aborted) {
        queueMicrotask(() => {
          void recoverStreamingConversation(activeConversationId, { mode: 'background' });
        });
      }
    } else if (activeConversationId) {
      closeConversationConnection(activeConversationId);
    } else {
      pendingStreamAbortRef.current?.abort();
      pendingStreamAbortRef.current = null;
    }
    streamingMessageRef.current = null;
    setControllerState(current => ({
      ...current,
      conversations: isDraftAIChatConversationId(activeConversationId)
        ? current.conversations.filter(conversation => conversation.id !== activeConversationId)
        : current.conversations,
      messagesByConversation: isDraftAIChatConversationId(activeConversationId)
        ? Object.fromEntries(
            Object.entries(current.messagesByConversation).filter(
              ([conversationId]) => conversationId !== activeConversationId
            )
          )
        : current.messagesByConversation,
      activeConversationId: null,
      isSending: false,
      isLoadingMessages: false,
      error: null,
    }));
  }, [
    closeConversationConnection,
    markSelectionTarget,
    pendingStreamAbortRef,
    recoverStreamingConversation,
    recoveryAbortByConversationRef,
    setBackgroundConversation,
    setControllerState,
    setRecoveryMode,
    stateRef,
    streamAbortByConversationRef,
    streamingMessageRef,
  ]);

  const markConversationStopped = useCallback(
    (conversationId: string, messageId?: string) => {
      const targetMessageId =
        messageId ||
        stateRef.current.conversations.find(conversation => conversation.id === conversationId)
          ?.active_message_id ||
        streamingMessageRef.current?.messageId ||
        Object.values(stateRef.current.streamingByMessageId).find(
          streaming => streaming.conversation_id === conversationId
        )?.message_id;

      setControllerState(current => {
        const messages = current.messagesByConversation[conversationId] ?? [];
        const stoppedAt = Math.floor(Date.now() / 1000);
        const nextStreamingByMessageId = { ...current.streamingByMessageId };
        if (targetMessageId) {
          delete nextStreamingByMessageId[targetMessageId];
        }

        return {
          ...current,
          isSending: current.activeConversationId === conversationId ? false : current.isSending,
          conversations: current.conversations.map(conversation =>
            conversation.id === conversationId
              ? {
                  ...conversation,
                  runtime_status: 'idle' as const,
                  active_message_id: undefined,
                }
              : conversation
          ),
          messagesByConversation: {
            ...current.messagesByConversation,
            [conversationId]: targetMessageId
              ? messages.map(message =>
                  message.id === targetMessageId
                    ? {
                        ...message,
                        status: 'stopped' as const,
                        updated_at: stoppedAt,
                      }
                    : message
                )
              : messages,
          },
          streamingByMessageId: nextStreamingByMessageId,
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
    },
    [setControllerState, stateRef, streamingMessageRef]
  );

  const stop = useCallback(async () => {
    const activeConversationId = stateRef.current.activeConversationId;
    if (!activeConversationId || stateRef.current.stoppingByConversation[activeConversationId]) {
      return;
    }

    const abortController =
      streamAbortByConversationRef.current[activeConversationId] ??
      (streamingMessageRef.current?.conversationId === activeConversationId
        ? pendingStreamAbortRef.current
        : null);
    if (isDraftAIChatConversationId(activeConversationId)) {
      abortController?.abort();
      if (pendingStreamAbortRef.current === abortController) {
        pendingStreamAbortRef.current = null;
      }
      streamingMessageRef.current = null;
      markConversationStopped(activeConversationId);
      return;
    }

    setControllerState(current => ({
      ...current,
      error: null,
      stoppingByConversation: {
        ...current.stoppingByConversation,
        [activeConversationId]: true,
      },
    }));

    try {
      const response = await transportRef.current.stopConversation(activeConversationId);
      abortController?.abort();
      closeConversationConnection(activeConversationId);
      if (pendingStreamAbortRef.current === abortController) {
        pendingStreamAbortRef.current = null;
      }
      streamingMessageRef.current = null;
      markConversationStopped(activeConversationId, response.message_id);
      refreshConversationSilently(activeConversationId);
    } catch (error) {
      console.warn('Failed to stop AIChat conversation', error);
      setControllerState(current => ({
        ...current,
        error: null,
        stoppingByConversation: {
          ...current.stoppingByConversation,
          [activeConversationId]: false,
        },
      }));
    }
  }, [
    closeConversationConnection,
    markConversationStopped,
    pendingStreamAbortRef,
    refreshConversationSilently,
    setControllerState,
    stateRef,
    streamAbortByConversationRef,
    streamingMessageRef,
    transportRef,
  ]);

  const remove = useCallback(
    async (conversationId: string) => {
      if (!conversationId) return;
      try {
        await transportRef.current.removeConversation(conversationId);
        closeConversationConnection(conversationId);
        setControllerState(current => {
          const nextMessages = { ...current.messagesByConversation };
          delete nextMessages[conversationId];
          const nextMessagePagination = { ...current.messagePaginationByConversation };
          const nextLoadingOlder = { ...current.loadingOlderByConversation };
          const nextRecovering = { ...current.recoveringByConversation };
          const nextStopping = { ...current.stoppingByConversation };
          const nextStreamingByMessageId = { ...current.streamingByMessageId };
          delete nextMessagePagination[conversationId];
          delete nextLoadingOlder[conversationId];
          delete nextRecovering[conversationId];
          delete nextStopping[conversationId];
          Object.values(current.streamingByMessageId).forEach(streaming => {
            if (streaming.conversation_id === conversationId) {
              delete nextStreamingByMessageId[streaming.message_id];
            }
          });
          return {
            ...current,
            conversations: current.conversations.filter(item => item.id !== conversationId),
            messagesByConversation: nextMessages,
            messagePaginationByConversation: nextMessagePagination,
            loadingOlderByConversation: nextLoadingOlder,
            recoveringByConversation: nextRecovering,
            stoppingByConversation: nextStopping,
            streamingByMessageId: nextStreamingByMessageId,
            activeConversationId:
              current.activeConversationId === conversationId ? null : current.activeConversationId,
          };
        });
      } catch (error) {
        setControllerState(current => ({ ...current, error: getErrorMessage(error) }));
      }
    },
    [closeConversationConnection, setControllerState, transportRef]
  );

  const rename = useCallback(
    async (conversationId: string, title: string) => {
      const nextTitle = title.trim();
      if (!conversationId || !nextTitle || isDraftAIChatConversationId(conversationId)) return;

      const previousConversation = stateRef.current.conversations.find(
        conversation => conversation.id === conversationId
      );
      if (previousConversation?.title === nextTitle) return;

      setControllerState(current => ({
        ...current,
        error: null,
        conversations: current.conversations.map(conversation =>
          conversation.id === conversationId
            ? {
                ...conversation,
                title: nextTitle,
              }
            : conversation
        ),
      }));

      try {
        const conversation = await transportRef.current.updateConversation(conversationId, {
          title: nextTitle,
        });
        setControllerState(current => ({
          ...current,
          conversations: replaceAIChatConversation(current.conversations, conversation, {
            moveToTop: false,
          }),
        }));
      } catch (error) {
        setControllerState(current => ({
          ...current,
          error: getErrorMessage(error),
          conversations: previousConversation
            ? current.conversations.map(conversation =>
                conversation.id === conversationId ? previousConversation : conversation
              )
            : current.conversations,
        }));
        throw error;
      }
    },
    [setControllerState, stateRef, transportRef]
  );

  const loadOlderMessages = useCallback(
    async (conversationId?: string) => {
      const targetConversationId = conversationId || stateRef.current.activeConversationId;
      if (!targetConversationId) return;

      const currentState = stateRef.current;
      const currentPagination =
        currentState.messagePaginationByConversation[targetConversationId] ??
        DEFAULT_AICHAT_MESSAGE_PAGINATION;

      if (
        !currentPagination.hasMore ||
        currentState.loadingOlderByConversation[targetConversationId]
      ) {
        return;
      }

      setControllerState(current => ({
        ...current,
        error: null,
        loadingOlderByConversation: {
          ...current.loadingOlderByConversation,
          [targetConversationId]: true,
        },
      }));

      try {
        const nextPage = currentPagination.page + 1;
        const response = await transportRef.current.listMessages(targetConversationId, {
          page: nextPage,
          limit: currentPagination.limit || DEFAULT_AICHAT_MESSAGE_PAGINATION.limit,
        });

        setControllerState(current => {
          const existingMessages = current.messagesByConversation[targetConversationId] ?? [];
          const nextLoadingOlder = {
            ...current.loadingOlderByConversation,
            [targetConversationId]: false,
          };

          return {
            ...current,
            messagesByConversation: {
              ...current.messagesByConversation,
              [targetConversationId]: mergeAIChatMessages(existingMessages, response.items),
            },
            messagePaginationByConversation: {
              ...current.messagePaginationByConversation,
              [targetConversationId]: response.pagination,
            },
            loadingOlderByConversation: nextLoadingOlder,
          };
        });
      } catch (error) {
        setControllerState(current => ({
          ...current,
          error: getErrorMessage(error),
          loadingOlderByConversation: {
            ...current.loadingOlderByConversation,
            [targetConversationId]: false,
          },
        }));
      }
    },
    [setControllerState, stateRef, transportRef]
  );

  return {
    init,
    select,
    startNew,
    stop,
    remove,
    rename,
    loadOlderMessages,
  };
}
