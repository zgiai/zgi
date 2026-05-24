'use client';

import { useCallback, useEffect, useMemo, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useStore } from 'zustand';
import type {
  AIChatAgentProgressEventData,
  AIChatConversation,
  AIChatErrorEventData,
  AIChatFileParseEndEventData,
  AIChatFileParseErrorEventData,
  AIChatFileParseStartEventData,
  AIChatMessage,
  AIChatIntermediateAnswerEventData,
  AIChatMessageChunkEventData,
  AIChatMessageEndEventData,
  AIChatMessageFile,
  AIChatMessageRetractEventData,
  AIChatMessageStartEventData,
  AIChatSkillCallEndEventData,
  AIChatSkillCallErrorEventData,
  AIChatSkillCallStartEventData,
  AIChatSkillArtifactCreatedEventData,
  AIChatSkillLoadEndEventData,
  AIChatSkillLoadStartEventData,
  AIChatSkillReferenceReadEventData,
} from '@/services/types/aichat';
import {
  DEFAULT_AICHAT_MESSAGE_PAGINATION,
  DEFAULT_AICHAT_PAGINATION,
  createAIChatControllerStore,
  type AIChatController,
  type AIChatControllerState,
  type AIChatControllerStore,
  type AIChatModelSelection,
  type AIChatRecoveryMode,
} from '@/components/chat/controllers/aichat';
import { aichatTransport } from '@/components/chat/transports/aichat-transport';
import {
  createDraftAIChatConversation,
  createStreamingAIChatMessage,
  isDraftAIChatConversationId,
  replaceAIChatConversation,
  upsertAIChatMessage,
} from '@/components/chat/utils/aichat-message';
import { toAIChatParameters } from '@/components/chat/utils/aichat-parameters';
import {
  buildChatMessageTopology,
  buildChatMessageTopologyKey,
  buildCurrentChatPath,
  findChatBranchLeaf,
  type ChatMessageTopology,
} from '@/components/chat/utils/message-tree';
import {
  canReplaceRootMessage,
  getNextActiveSendingState,
  mergeSelectedMessagesWithStreamingState,
  seedStreamingTimelineFromMessages,
  selectActiveConversation,
  selectActiveMessagePagination,
  selectActiveMessages,
  selectBranchNavigationByMessageId,
  selectDisplayMessageIds,
  selectDisplayMessages,
  selectIsLoadingOlderMessages,
  selectIsRecoveringMessages,
  selectIsStopping,
  shouldTreatConversationAsRunning,
} from '@/components/chat/controllers/aichat/selectors';
import {
  applyMessageChunkState,
  applyMessageRetractState,
  applyAgentProgressState,
  applyIntermediateAnswerState,
  applyFileParseEndState,
  applyFileParseErrorState,
  applyFileParseStartState,
  applyMessageEndState,
  applyMessageStartState,
  applySkillCallEndState,
  applySkillCallErrorState,
  applySkillCallStartState,
  applySkillArtifactCreatedState,
  applySkillLoadEndState,
  applySkillLoadStartState,
  applySkillReferenceReadState,
  applyStreamErrorState,
  mergeAIChatMessages,
  removeStreamingStateByConversation,
} from '@/components/chat/controllers/aichat/state-reducers';
import { useAIChatStreamRuntime } from '@/components/chat/controllers/aichat/stream-runtime';
import { MEMORY_KEYS } from '@/hooks/query-keys';
import { generateClientId } from '@/utils/client-id';

function getErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error || 'Unknown error');
}

function isAbortError(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  return error.name === 'AbortError' || error.message.toLowerCase().includes('abort');
}

const AICHAT_RECOVERY_RETRY_DELAYS = [800, 1600, 3200] as const;
const AICHAT_STREAM_EVENTS_EXPIRED = 'stream events expired';
const USER_MEMORY_SKILL_ID = 'user-memory';
const USER_MEMORY_MUTATION_TOOLS = new Set([
  'add_user_memory',
  'update_user_memory',
  'delete_user_memory',
]);

function createClientDraftId(prefix: string): string {
  return generateClientId(prefix);
}

function removeRunningStreamingStateByConversation(
  streamingByMessageId: AIChatControllerState['streamingByMessageId'],
  conversationId: string
): AIChatControllerState['streamingByMessageId'] {
  const nextStreamingByMessageId = { ...streamingByMessageId };
  Object.values(streamingByMessageId).forEach(streaming => {
    if (streaming.conversation_id !== conversationId) return;
    if (streaming.status === 'streaming' || !streaming.timeline?.length) {
      delete nextStreamingByMessageId[streaming.message_id];
    }
  });
  return nextStreamingByMessageId;
}

function clearStreamingRuntimeMessageMetadata(message: AIChatMessage): AIChatMessage {
  if (!message.metadata) {
    return message;
  }

  const metadata = { ...message.metadata };
  delete metadata.has_trace;
  delete metadata.skill_invocations;
  delete metadata.selected_skill_ids;
  delete metadata.loaded_skill_ids;
  delete metadata.skill_step_count;
  delete metadata.skill_call_count;
  delete metadata.skill_names;
  delete metadata.tool_call_count;
  delete metadata.tool_names;
  delete metadata.generated_file_count;
  delete metadata.generated_files;

  return {
    ...message,
    metadata: Object.keys(metadata).length > 0 ? metadata : undefined,
  };
}

/**
 * @hook useAIChatController
 * @description Dedicated controller for the standalone AIChat console page.
 */
export function useAIChatController(): AIChatController {
  const queryClient = useQueryClient();
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

  const refreshAccountMemoryAfterToolCall = useCallback(
    (payload: AIChatSkillCallEndEventData) => {
      if (
        payload.skill_id !== USER_MEMORY_SKILL_ID ||
        !USER_MEMORY_MUTATION_TOOLS.has(payload.tool_name)
      ) {
        return;
      }
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

  const refreshConversationSilently = useCallback(
    (conversationId: string) => {
      void aichatTransport
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
    [setControllerState]
  );

  const refreshMessagesSilently = useCallback(
    (conversationId: string) => {
      void aichatTransport
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
    [setControllerState]
  );

  const applyMessageStart = useCallback(
    (
      payload: AIChatMessageStartEventData,
      context: {
        query?: string;
        model?: AIChatModelSelection;
        files?: AIChatMessageFile[];
        previousConversationId?: string | null;
        resetAnswer?: boolean;
        mode?: AIChatRecoveryMode;
        moveToTop?: boolean;
      } = {},
      eventId?: string | null
    ) => {
      if (!payload.conversation_id || !payload.message_id) return;

      const mode = resolveMessageStartMode(payload, context);
      const previousConversationId = context.previousConversationId ?? null;
      const shouldRetargetDraftSelection =
        mode === 'active' &&
        previousConversationId !== null &&
        previousConversationId !== payload.conversation_id &&
        isDraftAIChatConversationId(previousConversationId) &&
        stateRef.current.activeConversationId === previousConversationId;
      const shouldRetargetBackgroundDraft =
        mode === 'background' &&
        previousConversationId !== null &&
        previousConversationId !== payload.conversation_id &&
        isDraftAIChatConversationId(previousConversationId) &&
        backgroundConversationIdRef.current === previousConversationId;
      const resolvedContext = {
        ...context,
        mode,
      };

      streamingMessageRef.current = {
        conversationId: payload.conversation_id,
        messageId: payload.message_id,
      };
      setControllerState(current =>
        applyMessageStartState(current, payload, resolvedContext, eventId)
      );
      if (shouldRetargetDraftSelection) {
        migrateLatestSelectionTarget(previousConversationId, payload.conversation_id);
      }
      if (shouldRetargetBackgroundDraft) {
        backgroundConversationIdRef.current = payload.conversation_id;
      }
    },
    [
      backgroundConversationIdRef,
      migrateLatestSelectionTarget,
      resolveMessageStartMode,
      setControllerState,
      streamingMessageRef,
    ]
  );

  const applyMessageChunk = useCallback(
    (payload: AIChatMessageChunkEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      setControllerState(current => applyMessageChunkState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyMessageRetract = useCallback(
    (payload: AIChatMessageRetractEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      setControllerState(current => applyMessageRetractState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyFileParseStart = useCallback(
    (payload: AIChatFileParseStartEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.file_id) return;
      setControllerState(current => applyFileParseStartState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyFileParseEnd = useCallback(
    (payload: AIChatFileParseEndEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.file_id) return;
      setControllerState(current => applyFileParseEndState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyFileParseError = useCallback(
    (payload: AIChatFileParseErrorEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.file_id) return;
      setControllerState(current => applyFileParseErrorState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applySkillCallStart = useCallback(
    (payload: AIChatSkillCallStartEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.skill_id) return;
      setControllerState(current => applySkillCallStartState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applySkillLoadStart = useCallback(
    (payload: AIChatSkillLoadStartEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.skill_id) return;
      setControllerState(current => applySkillLoadStartState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applySkillLoadEnd = useCallback(
    (payload: AIChatSkillLoadEndEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.skill_id) return;
      setControllerState(current => applySkillLoadEndState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applySkillReferenceRead = useCallback(
    (payload: AIChatSkillReferenceReadEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.skill_id) return;
      setControllerState(current => applySkillReferenceReadState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applySkillCallEnd = useCallback(
    (payload: AIChatSkillCallEndEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.skill_id) return;
      setControllerState(current => applySkillCallEndState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applySkillCallError = useCallback(
    (payload: AIChatSkillCallErrorEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.skill_id) return;
      setControllerState(current => applySkillCallErrorState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applySkillArtifactCreated = useCallback(
    (payload: AIChatSkillArtifactCreatedEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) {
        return;
      }

      setControllerState(current => applySkillArtifactCreatedState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyAgentProgress = useCallback(
    (payload: AIChatAgentProgressEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.content) return;
      setControllerState(current => applyAgentProgressState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyIntermediateAnswer = useCallback(
    (payload: AIChatIntermediateAnswerEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      if (!payload.content && payload.done !== true) return;
      setControllerState(current => applyIntermediateAnswerState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyMessageEnd = useCallback(
    (payload: AIChatMessageEndEventData, _eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      if (streamingMessageRef.current?.messageId === payload.message_id) {
        streamingMessageRef.current = null;
      }
      setControllerState(current => applyMessageEndState(current, payload));

      clearRecoveryRetry(payload.conversation_id);
      delete recoveryModeByConversationRef.current[payload.conversation_id];
      if (backgroundConversationIdRef.current === payload.conversation_id) {
        backgroundConversationIdRef.current = null;
      }
      refreshConversationSilently(payload.conversation_id);
      refreshMessagesSilently(payload.conversation_id);
    },
    [
      backgroundConversationIdRef,
      clearRecoveryRetry,
      recoveryModeByConversationRef,
      refreshConversationSilently,
      refreshMessagesSilently,
      setControllerState,
      streamingMessageRef,
    ]
  );

  const applyStreamError = useCallback(
    (
      payload: AIChatErrorEventData,
      _eventId?: string | null,
      fallbackConversationId?: string | null
    ) => {
      const conversationId =
        payload.conversation_id || fallbackConversationId || stateRef.current.activeConversationId;
      setControllerState(current => {
        const previousError = current.error;
        const nextState = applyStreamErrorState(current, payload, conversationId);
        return current.activeConversationId === conversationId
          ? nextState
          : {
              ...nextState,
              error: previousError,
            };
      });

      if (conversationId) {
        clearRecoveryRetry(conversationId);
        delete recoveryModeByConversationRef.current[conversationId];
        if (backgroundConversationIdRef.current === conversationId) {
          backgroundConversationIdRef.current = null;
        }
        refreshConversationSilently(conversationId);
      }
    },
    [
      backgroundConversationIdRef,
      clearRecoveryRetry,
      recoveryModeByConversationRef,
      refreshConversationSilently,
      setControllerState,
    ]
  );

  const refreshList = useCallback(
    async (params: { page?: number; append?: boolean } = {}) => {
      const page = params.page ?? 1;
      const limit = DEFAULT_AICHAT_PAGINATION.limit;
      setControllerState(current => ({ ...current, isLoadingList: true, error: null }));

      try {
        const response = await aichatTransport.listConversations({ page, limit });
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
    [setControllerState]
  );

  const recoverStreamingConversation = useCallback(
    async (
      conversationId: string,
      options: { conversation?: AIChatConversation; mode?: AIChatRecoveryMode } = {}
    ) => {
      if (!conversationId) return;

      const connect = (attempt: number) => {
        const currentState = stateRef.current;
        const requestedMode =
          options.mode ??
          (currentState.activeConversationId === conversationId ? 'active' : 'background');
        const getCurrentMode = (): AIChatRecoveryMode => {
          const storedMode = recoveryModeByConversationRef.current[conversationId] ?? requestedMode;
          return storedMode === 'active' &&
            stateRef.current.activeConversationId === conversationId
            ? 'active'
            : 'background';
        };
        const conversation =
          options.conversation ??
          currentState.conversations.find(item => item.id === conversationId);
        const messageId = conversation?.active_message_id;
        if (conversation?.runtime_status !== 'streaming' || !messageId) {
          closeRecoveryConnection(conversationId);
          return;
        }

        const existingRecovery = recoveryAbortByConversationRef.current[conversationId];
        if (existingRecovery && !existingRecovery.signal.aborted) {
          const nextMode =
            requestedMode === 'active' && stateRef.current.activeConversationId === conversationId
              ? 'active'
              : 'background';
          setRecoveryMode(conversationId, nextMode);
          setControllerState(current => ({
            ...current,
            isSending:
              current.activeConversationId === conversationId
                ? nextMode === 'active'
                : current.isSending,
            recoveringByConversation: {
              ...current.recoveringByConversation,
              [conversationId]: true,
            },
          }));
          return;
        }

        const currentStreaming = currentState.streamingByMessageId[messageId];
        const afterId = currentStreaming?.last_event_id;
        const abortController = new AbortController();
        recoveryAbortByConversationRef.current[conversationId] = abortController;
        setRecoveryMode(
          conversationId,
          requestedMode === 'active' && stateRef.current.activeConversationId === conversationId
            ? 'active'
            : 'background'
        );
        clearRecoveryRetry(conversationId);

        setControllerState(current => {
          const messages = current.messagesByConversation[conversationId] ?? [];
          const existingMessage = messages.find(message => message.id === messageId);
          const previousStreaming = current.streamingByMessageId[messageId];
          const now = Math.floor(Date.now() / 1000);
          const placeholder =
            existingMessage ??
            createStreamingAIChatMessage({
              id: messageId,
              conversationId,
              query: '',
              modelName: 'unknown',
              createdAt: now,
            });
          const preservedAnswer = previousStreaming?.answer || placeholder.answer;
          const shouldDedupeReplay = !afterId && preservedAnswer.length > 0;
          const replayingFromStart = !afterId;
          const replayPlaceholder = replayingFromStart
            ? clearStreamingRuntimeMessageMetadata(placeholder)
            : placeholder;
          const nextMessage: AIChatMessage = {
            ...replayPlaceholder,
            answer: !afterId && !preservedAnswer ? '' : preservedAnswer,
            status: 'streaming',
            updated_at: now,
          };

          return {
            ...current,
            error: null,
            isSending:
              current.activeConversationId === conversationId && requestedMode === 'active'
                ? true
                : current.isSending,
            recoveringByConversation: {
              ...current.recoveringByConversation,
              [conversationId]: true,
            },
            messagesByConversation: {
              ...current.messagesByConversation,
              [conversationId]: upsertAIChatMessage(messages, nextMessage),
            },
            streamingByMessageId: {
              ...current.streamingByMessageId,
              [messageId]: {
                conversation_id: conversationId,
                message_id: messageId,
                answer: nextMessage.answer,
                status: 'streaming',
                timeline: replayingFromStart ? [] : previousStreaming?.timeline ?? [],
                last_event_id: afterId,
                replay_base_answer: shouldDedupeReplay
                  ? preservedAnswer
                  : previousStreaming?.replay_base_answer,
                replay_offset: shouldDedupeReplay ? 0 : previousStreaming?.replay_offset,
                replace: previousStreaming?.replace,
              },
            },
          };
        });

        let reachedTerminalEvent = false;
        let reconnectScheduled = false;
        const scheduleReconnect = () => {
          if (reconnectScheduled || reachedTerminalEvent || abortController.signal.aborted) return;
          if (attempt >= AICHAT_RECOVERY_RETRY_DELAYS.length) {
            setControllerState(current => ({
              ...current,
              error:
                current.activeConversationId === conversationId
                  ? 'AIChat stream recovery failed'
                  : current.error,
              isSending: getNextActiveSendingState(current, conversationId, false),
              recoveringByConversation: {
                ...current.recoveringByConversation,
                [conversationId]: false,
              },
            }));
            return;
          }

          reconnectScheduled = true;
          const timeout = setTimeout(() => {
            delete recoveryRetryTimeoutsRef.current[conversationId];
            delete recoveryAbortByConversationRef.current[conversationId];
            connect(attempt + 1);
          }, AICHAT_RECOVERY_RETRY_DELAYS[attempt]);
          recoveryRetryTimeoutsRef.current[conversationId] = timeout;
        };

        void aichatTransport
          .recoverConversationStream(
            conversationId,
            { messageId, afterId },
            {
              onMessageStart: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applyMessageStart(
                  payload,
                  { resetAnswer: false, mode: getCurrentMode(), moveToTop: false },
                  eventId
                );
              },
              onAgentProgress: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applyAgentProgress(payload, eventId);
              },
              onIntermediateAnswer: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applyIntermediateAnswer(payload, eventId);
              },
              onFileParseStart: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applyFileParseStart(payload, eventId);
              },
              onFileParseEnd: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applyFileParseEnd(payload, eventId);
              },
              onFileParseError: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applyFileParseError(payload, eventId);
              },
              onSkillLoadStart: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applySkillLoadStart(payload, eventId);
              },
              onSkillLoadEnd: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applySkillLoadEnd(payload, eventId);
              },
              onSkillReferenceRead: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applySkillReferenceRead(payload, eventId);
              },
              onSkillCallStart: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applySkillCallStart(payload, eventId);
              },
              onSkillCallEnd: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applySkillCallEnd(payload, eventId);
                refreshAccountMemoryAfterToolCall(payload);
              },
              onSkillCallError: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applySkillCallError(payload, eventId);
              },
              onSkillArtifactCreated: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applySkillArtifactCreated(payload, eventId);
              },
              onMessageChunk: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applyMessageChunk(payload, eventId);
              },
              onMessageRetract: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                applyMessageRetract(payload, eventId);
              },
              onMessageEnd: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                reachedTerminalEvent = true;
                applyMessageEnd(payload, eventId);
              },
              onErrorEvent: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                reachedTerminalEvent = true;
                applyStreamError(payload, eventId, conversationId);
                if (payload.message?.toLowerCase().includes(AICHAT_STREAM_EVENTS_EXPIRED)) {
                  closeRecoveryConnection(conversationId);
                }
              },
              onRequestError: error => {
                if (isAbortError(error)) return;
                scheduleReconnect();
              },
              onClose: () => {
                if (recoveryAbortByConversationRef.current[conversationId] === abortController) {
                  delete recoveryAbortByConversationRef.current[conversationId];
                }
                if (!reachedTerminalEvent && !abortController.signal.aborted) {
                  scheduleReconnect();
                  return;
                }
                setControllerState(current => ({
                  ...current,
                  isSending: getNextActiveSendingState(current, conversationId, false),
                  recoveringByConversation: {
                    ...current.recoveringByConversation,
                    [conversationId]: false,
                  },
                }));
              },
            },
            abortController.signal
          )
          .catch(error => {
            if (isAbortError(error)) return;
            scheduleReconnect();
          });
      };

      connect(0);
    },
    [
      applyAgentProgress,
      applyIntermediateAnswer,
      applyFileParseEnd,
      applyFileParseError,
      applyFileParseStart,
      applyMessageChunk,
      applyMessageRetract,
      applyMessageEnd,
      applyMessageStart,
      applySkillCallEnd,
      applySkillCallError,
      applySkillCallStart,
      applySkillArtifactCreated,
      applySkillLoadEnd,
      applySkillLoadStart,
      applySkillReferenceRead,
      applyStreamError,
      clearRecoveryRetry,
      closeRecoveryConnection,
      recoveryAbortByConversationRef,
      recoveryModeByConversationRef,
      recoveryRetryTimeoutsRef,
      refreshAccountMemoryAfterToolCall,
      setRecoveryMode,
      setControllerState,
    ]
  );

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
          await aichatTransport.getConversation(conversationId);

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
      closeConversationConnection,
      recoverStreamingConversation,
      backgroundConversationIdRef,
      isLatestSelection,
      markSelectionTarget,
      recoveryAbortByConversationRef,
      setBackgroundConversation,
      setRecoveryMode,
      setControllerState,
      streamAbortByConversationRef,
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
    [refreshList, select]
  );

  const startNew = useCallback(() => {
    const currentState = stateRef.current;
    const activeConversationId = stateRef.current.activeConversationId;
    markSelectionTarget(null);
    if (isDraftAIChatConversationId(activeConversationId)) {
      pendingStreamAbortRef.current?.abort();
      pendingStreamAbortRef.current = null;
    } else if (activeConversationId && shouldTreatConversationAsRunning(currentState, activeConversationId)) {
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
    [setControllerState, streamingMessageRef]
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
      const response = await aichatTransport.stopConversation(activeConversationId);
      abortController?.abort();
      closeConversationConnection(activeConversationId);
      if (pendingStreamAbortRef.current === abortController) {
        pendingStreamAbortRef.current = null;
      }
      streamingMessageRef.current = null;
      markConversationStopped(activeConversationId, response.message_id);
      refreshConversationSilently(activeConversationId);
    } catch (error) {
      setControllerState(current => ({
        ...current,
        error: getErrorMessage(error),
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
    streamAbortByConversationRef,
    streamingMessageRef,
  ]);

  const remove = useCallback(
    async (conversationId: string) => {
      if (!conversationId) return;
      try {
        await aichatTransport.removeConversation(conversationId);
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
    [closeConversationConnection, setControllerState]
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
        const conversation = await aichatTransport.updateConversation(conversationId, {
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
    [setControllerState]
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
        const response = await aichatTransport.listMessages(targetConversationId, {
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
    [setControllerState]
  );

  const send = useCallback(
    async ({
      query,
      model,
      files = [],
      parentId: parentIdOverride,
      useMemory = false,
    }: {
      query: string;
      model: AIChatModelSelection;
      files?: AIChatMessageFile[];
      parentId?: string | null;
      useMemory?: boolean;
    }) => {
      const trimmedQuery = query.trim();
      const currentState = stateRef.current;
      const activeConversationId = currentState.activeConversationId;
      const activeConversation = activeConversationId
        ? currentState.conversations.find(item => item.id === activeConversationId) ?? null
        : null;
      const isActiveRecovering = activeConversationId
        ? currentState.recoveringByConversation[activeConversationId]
        : false;
      const isActiveStopping = activeConversationId
        ? currentState.stoppingByConversation[activeConversationId]
        : false;
      if (
        !trimmedQuery ||
        !model.model ||
        currentState.isSending ||
        isActiveRecovering ||
        isActiveStopping ||
        activeConversation?.runtime_status === 'streaming'
      ) {
        return;
      }

      const abortController = new AbortController();
      let streamConversationId = activeConversationId;
      if (activeConversationId) {
        streamAbortByConversationRef.current[activeConversationId]?.abort();
        streamAbortByConversationRef.current[activeConversationId] = abortController;
      } else {
        pendingStreamAbortRef.current?.abort();
        pendingStreamAbortRef.current = abortController;
      }

      const activeMessages = activeConversationId
        ? (currentState.messagesByConversation[activeConversationId] ?? [])
        : [];
      const currentPath = buildCurrentChatPath(activeConversation, activeMessages);
      const parentId =
        parentIdOverride === undefined
          ? currentPath[currentPath.length - 1]?.id
          : parentIdOverride || undefined;
      const draftConversationId = activeConversationId
        ? null
        : createClientDraftId('draft-aichat');
      const draftMessageId = draftConversationId
        ? createClientDraftId('draft-aichat-message')
        : null;
      if (draftConversationId) {
        streamConversationId = draftConversationId;
      }
      markSelectionTarget(draftConversationId ?? activeConversationId);

      setControllerState(current => {
        if (!draftConversationId || !draftMessageId) {
          return { ...current, isSending: true, error: null };
        }

        const draftConversation = {
          ...createDraftAIChatConversation(draftConversationId, trimmedQuery),
          runtime_status: 'streaming' as const,
          active_message_id: draftMessageId,
          current_leaf_message_id: draftMessageId,
          dialogue_count: 1,
        };
        const draftMessage = createStreamingAIChatMessage({
          id: draftMessageId,
          conversationId: draftConversationId,
          query: trimmedQuery,
          modelName: model.model,
          modelProvider: model.provider,
          metadata:
            files.length > 0
              ? {
                  file_count: files.length,
                  files,
                }
              : undefined,
        });

        return {
          ...current,
          activeConversationId: draftConversationId,
          conversations: replaceAIChatConversation(current.conversations, draftConversation),
          messagesByConversation: {
            ...current.messagesByConversation,
            [draftConversationId]: [draftMessage],
          },
          streamingByMessageId: {
            ...current.streamingByMessageId,
            [draftMessageId]: {
              conversation_id: draftConversationId,
              message_id: draftMessageId,
              answer: '',
              status: 'streaming',
              timeline: [],
            },
          },
          isSending: true,
          error: null,
        };
      });

      try {
        await aichatTransport.streamChat(
          {
            conversation_id: activeConversationId ?? undefined,
            parent_id: parentId,
            query: trimmedQuery,
            model: model.model,
            provider: model.provider,
            ...(files.length > 0 ? { file_ids: files.map(file => file.id) } : {}),
            response_mode: 'streaming',
            parameters: toAIChatParameters(model.parameters),
            use_memory: useMemory,
          },
          {
            onMessageStart: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              if (payload.conversation_id) {
                streamConversationId = payload.conversation_id;
                streamAbortByConversationRef.current[payload.conversation_id] = abortController;
                if (pendingStreamAbortRef.current === abortController) {
                  pendingStreamAbortRef.current = null;
                }
              }
              applyMessageStart(
                payload,
                {
                  query: trimmedQuery,
                  model,
                  files,
                  previousConversationId: draftConversationId ?? activeConversationId,
                  mode: 'active',
                },
                eventId
              );
            },
            onAgentProgress: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyAgentProgress(payload, eventId);
            },
            onIntermediateAnswer: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyIntermediateAnswer(payload, eventId);
            },
            onFileParseStart: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyFileParseStart(payload, eventId);
            },
            onFileParseEnd: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyFileParseEnd(payload, eventId);
            },
            onFileParseError: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyFileParseError(payload, eventId);
            },
            onSkillLoadStart: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillLoadStart(payload, eventId);
            },
            onSkillLoadEnd: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillLoadEnd(payload, eventId);
            },
            onSkillReferenceRead: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillReferenceRead(payload, eventId);
            },
            onSkillCallStart: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillCallStart(payload, eventId);
            },
            onSkillCallEnd: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillCallEnd(payload, eventId);
              refreshAccountMemoryAfterToolCall(payload);
            },
            onSkillCallError: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillCallError(payload, eventId);
            },
            onSkillArtifactCreated: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillArtifactCreated(payload, eventId);
            },
            onMessageChunk: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyMessageChunk(payload, eventId);
            },
            onMessageRetract: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyMessageRetract(payload, eventId);
            },
            onMessageEnd: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyMessageEnd(payload, eventId);
            },
            onErrorEvent: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyStreamError(payload, eventId, streamConversationId);
            },
            onRequestError: error => {
              if (isAbortError(error)) return;
              setControllerState(current => {
                const isActiveStream =
                  streamConversationId === null
                    ? current.activeConversationId === null
                    : current.activeConversationId === streamConversationId;
                return {
                  ...current,
                  error: isActiveStream ? error.message : current.error,
                  isSending:
                    streamConversationId === null
                      ? current.activeConversationId === null
                        ? false
                        : current.isSending
                      : getNextActiveSendingState(current, streamConversationId, false),
                };
              });
            },
            onClose: () => {
              if (streamConversationId) {
                if (streamAbortByConversationRef.current[streamConversationId] === abortController) {
                  delete streamAbortByConversationRef.current[streamConversationId];
                }
              }
              if (pendingStreamAbortRef.current === abortController) {
                pendingStreamAbortRef.current = null;
              }
              if (streamingMessageRef.current && !abortController.signal.aborted) {
                streamingMessageRef.current = null;
              }
              if (stateRef.current.isSending && !abortController.signal.aborted) {
                setControllerState(current => ({
                  ...current,
                  isSending:
                    streamConversationId === null
                      ? current.activeConversationId === null
                        ? false
                        : current.isSending
                      : getNextActiveSendingState(current, streamConversationId, false),
                }));
              }
            },
          },
          abortController.signal
        );
      } catch (error) {
        if (!isAbortError(error)) {
          setControllerState(current => {
            const isActiveStream =
              streamConversationId === null
                ? current.activeConversationId === null
                : current.activeConversationId === streamConversationId;
            return {
              ...current,
              error: isActiveStream ? getErrorMessage(error) : current.error,
              isSending:
                streamConversationId === null
                  ? current.activeConversationId === null
                    ? false
                    : current.isSending
                  : getNextActiveSendingState(current, streamConversationId, false),
            };
          });
        }
      }
    },
    [
      applyAgentProgress,
      applyIntermediateAnswer,
      applyFileParseEnd,
      applyFileParseError,
      applyFileParseStart,
      applyMessageChunk,
      applyMessageRetract,
      applyMessageEnd,
      applyMessageStart,
      applySkillCallEnd,
      applySkillCallError,
      applySkillCallStart,
      applySkillArtifactCreated,
      applySkillLoadEnd,
      applySkillLoadStart,
      applySkillReferenceRead,
      applyStreamError,
      markSelectionTarget,
      pendingStreamAbortRef,
      refreshAccountMemoryAfterToolCall,
      setControllerState,
      streamAbortByConversationRef,
      streamingMessageRef,
    ]
  );

  const replaceRootMessage = useCallback(
    async ({
      messageId,
      query,
      model,
    }: {
      messageId: string;
      query?: string;
      model?: AIChatModelSelection;
    }) => {
      const trimmedQuery = query?.trim();
      const currentState = stateRef.current;
      const activeConversationId = currentState.activeConversationId;
      if (!activeConversationId || currentState.isSending) return;

      const activeConversation =
        currentState.conversations.find(item => item.id === activeConversationId) ?? null;
      const messages = currentState.messagesByConversation[activeConversationId] ?? [];
      const source = messages.find(message => message.id === messageId);
      if (!source) return;
      const isActiveRecovering = currentState.recoveringByConversation[activeConversationId];
      const isActiveStopping = currentState.stoppingByConversation[activeConversationId];
      if (
        !canReplaceRootMessage(activeConversation, source, messages) ||
        isActiveRecovering ||
        isActiveStopping
      ) {
        return;
      }
      const sourceMessage = source;
      const sourceConversation = activeConversation;

      const abortController = new AbortController();
      streamAbortByConversationRef.current[activeConversationId]?.abort();
      streamAbortByConversationRef.current[activeConversationId] = abortController;
      markSelectionTarget(activeConversationId);

      let streamStarted = false;
      const restoreOriginalMessage = (errorMessage?: string) => {
        setControllerState(current => {
          const nextStreamingByMessageId = { ...current.streamingByMessageId };
          delete nextStreamingByMessageId[messageId];

          return {
            ...current,
            error:
              errorMessage && current.activeConversationId === activeConversationId
                ? errorMessage
                : current.error,
            isSending: getNextActiveSendingState(current, activeConversationId, false),
            conversations: sourceConversation
              ? current.conversations.map(item =>
                  item.id === activeConversationId ? sourceConversation : item
                )
              : current.conversations,
            messagesByConversation: {
              ...current.messagesByConversation,
              [activeConversationId]: upsertAIChatMessage(
                current.messagesByConversation[activeConversationId] ?? [],
                sourceMessage
              ),
            },
            streamingByMessageId: nextStreamingByMessageId,
          };
        });
      };

      setControllerState(current => {
        const now = Math.floor(Date.now() / 1000);
        const cleanedSourceMessage = clearStreamingRuntimeMessageMetadata(sourceMessage);
        const nextMessage: AIChatMessage = {
          ...cleanedSourceMessage,
          answer: '',
          status: 'streaming',
          error: undefined,
          updated_at: now,
        };

        return {
          ...current,
          error: null,
          isSending: true,
          conversations: current.conversations.map(item =>
            item.id === activeConversationId
              ? {
                  ...item,
                  current_leaf_message_id: messageId,
                  runtime_status: 'streaming',
                  active_message_id: messageId,
                  updated_at: now,
                }
              : item
          ),
          messagesByConversation: {
            ...current.messagesByConversation,
            [activeConversationId]: upsertAIChatMessage(
              current.messagesByConversation[activeConversationId] ?? [],
              nextMessage
            ),
          },
          streamingByMessageId: {
            ...current.streamingByMessageId,
            [messageId]: {
              conversation_id: activeConversationId,
              message_id: messageId,
              answer: '',
              status: 'streaming',
              timeline: [],
              replace: true,
            },
          },
        };
      });

      try {
        await aichatTransport.regenerateMessage(
          messageId,
          {
            query: trimmedQuery || undefined,
            model: model?.model,
            provider: model?.provider,
            parameters: toAIChatParameters(model?.parameters),
          },
          {
            onMessageStart: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              streamStarted = true;
              applyMessageStart(
                {
                  ...payload,
                  replace: payload.replace ?? true,
                },
                {
                  query: trimmedQuery || sourceMessage.query,
                  model,
                  mode: 'active',
                  resetAnswer: true,
                },
                eventId
              );
            },
            onAgentProgress: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyAgentProgress(payload, eventId);
            },
            onIntermediateAnswer: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyIntermediateAnswer(payload, eventId);
            },
            onFileParseStart: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyFileParseStart(payload, eventId);
            },
            onFileParseEnd: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyFileParseEnd(payload, eventId);
            },
            onFileParseError: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyFileParseError(payload, eventId);
            },
            onSkillLoadStart: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillLoadStart(payload, eventId);
            },
            onSkillLoadEnd: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillLoadEnd(payload, eventId);
            },
            onSkillReferenceRead: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillReferenceRead(payload, eventId);
            },
            onSkillCallStart: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillCallStart(payload, eventId);
            },
            onSkillCallEnd: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillCallEnd(payload, eventId);
              refreshAccountMemoryAfterToolCall(payload);
            },
            onSkillCallError: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillCallError(payload, eventId);
            },
            onSkillArtifactCreated: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applySkillArtifactCreated(payload, eventId);
            },
            onMessageChunk: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyMessageChunk(payload, eventId);
            },
            onMessageRetract: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyMessageRetract(payload, eventId);
            },
            onMessageEnd: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyMessageEnd(payload, eventId);
            },
            onErrorEvent: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyStreamError(payload, eventId, activeConversationId);
            },
            onRequestError: error => {
              if (isAbortError(error)) return;
              if (!streamStarted) {
                restoreOriginalMessage(error.message);
                return;
              }
              setControllerState(current => ({
                ...current,
                error:
                  current.activeConversationId === activeConversationId
                    ? error.message
                    : current.error,
                isSending: getNextActiveSendingState(current, activeConversationId, false),
              }));
            },
            onClose: () => {
              if (streamAbortByConversationRef.current[activeConversationId] === abortController) {
                delete streamAbortByConversationRef.current[activeConversationId];
              }
              if (streamingMessageRef.current?.conversationId === activeConversationId) {
                streamingMessageRef.current = null;
              }
              if (!abortController.signal.aborted) {
                setControllerState(current => ({
                  ...current,
                  isSending: getNextActiveSendingState(current, activeConversationId, false),
                }));
              }
            },
          },
          abortController.signal
        );
      } catch (error) {
        if (!isAbortError(error)) {
          if (!streamStarted) {
            restoreOriginalMessage(getErrorMessage(error));
            return;
          }
          setControllerState(current => ({
            ...current,
            error:
              current.activeConversationId === activeConversationId
                ? getErrorMessage(error)
                : current.error,
            isSending: getNextActiveSendingState(current, activeConversationId, false),
          }));
        }
      }
    },
    [
      applyAgentProgress,
      applyIntermediateAnswer,
      applyFileParseEnd,
      applyFileParseError,
      applyFileParseStart,
      applyMessageChunk,
      applyMessageRetract,
      applyMessageEnd,
      applyMessageStart,
      applySkillCallEnd,
      applySkillCallError,
      applySkillCallStart,
      applySkillArtifactCreated,
      applySkillLoadEnd,
      applySkillLoadStart,
      applySkillReferenceRead,
      applyStreamError,
      markSelectionTarget,
      refreshAccountMemoryAfterToolCall,
      setControllerState,
      streamAbortByConversationRef,
      streamingMessageRef,
    ]
  );

  const regenerate = useCallback(
    async (messageId: string, model: AIChatModelSelection) => {
      const activeConversationId = stateRef.current.activeConversationId;
      if (!activeConversationId) return;

      const activeConversation =
        stateRef.current.conversations.find(item => item.id === activeConversationId) ?? null;
      const messages = stateRef.current.messagesByConversation[activeConversationId] ?? [];
      const source = messages.find(message => message.id === messageId);
      if (!source || !source.query.trim()) return;
      if (canReplaceRootMessage(activeConversation, source, messages)) {
        await replaceRootMessage({ messageId, model });
        return;
      }
      if (!source.parent_id) return;

      await send({
        query: source.query,
        model,
        parentId: source.parent_id,
      });
    },
    [replaceRootMessage, send]
  );

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
      void aichatTransport
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
    [setControllerState]
  );

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
  const error = useStore(store, state => state.error);

  const messageTopologyKey = useMemo(() => buildChatMessageTopologyKey(messages), [messages]);

  const messageTopology = useMemo(() => {
    if (topologyRef.current?.key === messageTopologyKey) {
      return topologyRef.current.topology;
    }

    const topology = buildChatMessageTopology(messages);
    topologyRef.current = { key: messageTopologyKey, topology };
    return topology;
  }, [messageTopologyKey, messages]);

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
    store,
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
    error,
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
    stop,
    switchBranch,
  };
}
