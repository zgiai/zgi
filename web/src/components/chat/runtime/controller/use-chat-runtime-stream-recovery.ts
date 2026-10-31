import { useCallback } from 'react';
import type { MutableRefObject } from 'react';
import type {
  AIChatConversation,
  AIChatMessage,
  AIChatMemoryMutationEventData,
} from '@/services/types/aichat';
import type {
  AIChatControllerStore,
  AIChatRecoveryMode,
  AIChatSetControllerState,
} from '@/components/chat/controllers/aichat';
import {
  getNextActiveSendingState,
  mergeRuntimeTimelineWithMessageTimeline,
  sanitizeTimelineForMessage,
  timelineFromAIChatMessage,
} from '@/components/chat/controllers/aichat/selectors';
import type { AIChatRuntimeTransport } from '@/components/chat/transports/aichat-transport';
import {
  createStreamingAIChatMessage,
  replaceAIChatConversation,
  upsertAIChatMessage,
} from '@/components/chat/utils/aichat-message';
import {
  AICHAT_DISCONNECTED_STATUS_POLL_LIMIT,
  AICHAT_DISCONNECTED_STATUS_POLL_MS,
  AICHAT_RECOVERY_RETRY_DELAYS,
  AICHAT_STREAM_EVENTS_EXPIRED,
  clearStreamingRuntimeMessageMetadata,
  isAbortError,
  isRecoverableStreamTransportError,
} from '@/components/chat/runtime/controller/chat-runtime-controller-utils';
import type { ChatRuntimeEventAppliers } from '@/components/chat/runtime/controller/use-chat-runtime-event-appliers';
import { createRuntimeEventCallbacks } from '@/components/chat/runtime/controller/runtime-event-callbacks';

interface UseChatRuntimeStreamRecoveryArgs {
  stateRef: MutableRefObject<AIChatControllerStore>;
  transportRef: MutableRefObject<AIChatRuntimeTransport>;
  recoveryAbortByConversationRef: MutableRefObject<Record<string, AbortController>>;
  recoveryModeByConversationRef: MutableRefObject<Record<string, AIChatRecoveryMode>>;
  recoveryRetryTimeoutsRef: MutableRefObject<Record<string, ReturnType<typeof setTimeout>>>;
  setRecoveryMode: (conversationId: string, mode: AIChatRecoveryMode) => void;
  clearRecoveryRetry: (conversationId: string) => void;
  closeRecoveryConnection: (conversationId: string) => void;
  setControllerState: AIChatSetControllerState;
  refreshMessagesSilently: (conversationId: string) => void;
  refreshAccountMemoryAfterMemoryMutation: (payload: AIChatMemoryMutationEventData) => void;
  eventAppliers: ChatRuntimeEventAppliers;
}

export function useChatRuntimeStreamRecovery({
  stateRef,
  transportRef,
  recoveryAbortByConversationRef,
  recoveryModeByConversationRef,
  recoveryRetryTimeoutsRef,
  setRecoveryMode,
  clearRecoveryRetry,
  closeRecoveryConnection,
  setControllerState,
  refreshMessagesSilently,
  refreshAccountMemoryAfterMemoryMutation,
  eventAppliers,
}: UseChatRuntimeStreamRecoveryArgs) {
  return useCallback(
    async (
      conversationId: string,
      options: {
        conversation?: AIChatConversation;
        mode?: AIChatRecoveryMode;
        refresh?: boolean;
      } = {}
    ) => {
      if (!conversationId) return;
      // A server-side continuation (for example after a connector decision)
      // changes the conversation outside this client, so the cached runtime
      // status cannot decide whether a stream is running.
      if (options.refresh && !options.conversation) {
        try {
          const conversation = await transportRef.current.refreshConversation(conversationId);
          setControllerState(current => ({
            ...current,
            conversations: replaceAIChatConversation(current.conversations, conversation),
          }));
          if (conversation.runtime_status !== 'streaming' || !conversation.active_message_id) {
            refreshMessagesSilently(conversationId);
            return;
          }
          options = { ...options, conversation };
        } catch {
          // Fall back to the cached conversation; a later refresh still recovers.
        }
      }

      // Checks a disconnected conversation until the server settles its turn,
      // then loads the settled messages. Reconnecting or leaving the
      // conversation clears the timer with the other recovery retries.
      const watchUntilSettled = (poll: number) => {
        if (poll >= AICHAT_DISCONNECTED_STATUS_POLL_LIMIT) return;
        recoveryRetryTimeoutsRef.current[conversationId] = setTimeout(() => {
          delete recoveryRetryTimeoutsRef.current[conversationId];
          void transportRef.current
            .refreshConversation(conversationId)
            .then(conversation => {
              if (conversation.runtime_status === 'streaming' && conversation.active_message_id) {
                watchUntilSettled(poll + 1);
                return;
              }
              setControllerState(current => ({
                ...current,
                conversations: replaceAIChatConversation(current.conversations, conversation),
                isSending: getNextActiveSendingState(current, conversationId, false),
                connectionByConversation: {
                  ...current.connectionByConversation,
                  [conversationId]: 'idle',
                },
              }));
              refreshMessagesSilently(conversationId);
            })
            .catch(() => watchUntilSettled(poll + 1));
        }, AICHAT_DISCONNECTED_STATUS_POLL_MS);
      };

      const connect = (attempt: number) => {
        const currentState = stateRef.current;
        const requestedMode =
          options.mode ??
          (currentState.activeConversationId === conversationId ? 'active' : 'background');
        const getCurrentMode = (): AIChatRecoveryMode => {
          const storedMode = recoveryModeByConversationRef.current[conversationId] ?? requestedMode;
          return storedMode === 'active' && stateRef.current.activeConversationId === conversationId
            ? 'active'
            : 'background';
        };
        const conversation =
          (attempt === 0 ? options.conversation : undefined) ??
          currentState.conversations.find(item => item.id === conversationId);
        const messageId = conversation?.active_message_id;
        if (conversation?.runtime_status !== 'streaming' || !messageId) {
          closeRecoveryConnection(conversationId);
          setControllerState(current => ({
            ...current,
            isSending: getNextActiveSendingState(current, conversationId, false),
            connectionByConversation: {
              ...current.connectionByConversation,
              [conversationId]: 'idle',
            },
          }));
          return;
        }

        setControllerState(current => ({
          ...current,
          connectionByConversation: {
            ...current.connectionByConversation,
            [conversationId]: 'reconnecting',
          },
        }));

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
          const historicalTimeline = existingMessage
            ? timelineFromAIChatMessage(existingMessage)
            : [];
          const preservedTimeline = sanitizeTimelineForMessage(
            replayPlaceholder,
            mergeRuntimeTimelineWithMessageTimeline(
              historicalTimeline,
              previousStreaming?.timeline
            )
          );
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
                // Replaying the stream must not discard the durable timeline
                // just because the replay starts from the beginning. Keep
                // historical events, then let incoming events merge into this
                // clean snapshot; only generic task-governance placeholders
                // are removed by the sanitizer above.
                timeline: preservedTimeline,
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
        const scheduleReconnect = (finalize = false) => {
          if (reconnectScheduled || reachedTerminalEvent || abortController.signal.aborted) return;
          if (finalize || attempt >= AICHAT_RECOVERY_RETRY_DELAYS.length) {
            reconnectScheduled = true;
            abortController.abort();
            if (recoveryAbortByConversationRef.current[conversationId] === abortController) {
              delete recoveryAbortByConversationRef.current[conversationId];
            }
            void transportRef.current
              .refreshConversation(conversationId)
              .then(authoritativeConversation => {
                const stillRunning =
                  authoritativeConversation.runtime_status === 'streaming' &&
                  Boolean(authoritativeConversation.active_message_id);
                setControllerState(current => ({
                  ...current,
                  conversations: current.conversations.map(item =>
                    item.id === conversationId ? authoritativeConversation : item
                  ),
                  isSending: getNextActiveSendingState(current, conversationId, stillRunning),
                  recoveringByConversation: {
                    ...current.recoveringByConversation,
                    [conversationId]: false,
                  },
                  connectionByConversation: {
                    ...current.connectionByConversation,
                    [conversationId]: stillRunning ? 'disconnected' : 'idle',
                  },
                }));
                if (!stillRunning) {
                  refreshMessagesSilently(conversationId);
                } else {
                  watchUntilSettled(0);
                }
              })
              .catch(() => {
                setControllerState(current => ({
                  ...current,
                  recoveringByConversation: {
                    ...current.recoveringByConversation,
                    [conversationId]: false,
                  },
                  connectionByConversation: {
                    ...current.connectionByConversation,
                    [conversationId]: 'disconnected',
                  },
                }));
                // The server may be down; keep checking until it settles
                // the turn.
                watchUntilSettled(0);
              });
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

        void transportRef.current
          .recoverConversationStream(
            conversationId,
            { messageId, afterId },
            {
              onOpen: () => {
                if (abortController.signal.aborted) return;
                setControllerState(current => ({
                  ...current,
                  connectionByConversation: {
                    ...current.connectionByConversation,
                    [conversationId]: 'connected',
                  },
                }));
              },
              onMessageStart: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applyMessageStart(
                  payload,
                  { resetAnswer: false, mode: getCurrentMode(), moveToTop: false },
                  eventId
                );
              },
              ...createRuntimeEventCallbacks({
                appliers: eventAppliers,
                refreshAccountMemoryAfterMemoryMutation,
                isAborted: () => abortController.signal.aborted,
              }),
              onMessageEnd: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                reachedTerminalEvent = true;
                eventAppliers.applyMessageEnd(payload, eventId);
                setControllerState(current => ({
                  ...current,
                  connectionByConversation: {
                    ...current.connectionByConversation,
                    [conversationId]: 'idle',
                  },
                }));
              },
              onErrorEvent: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                reachedTerminalEvent = true;
                eventAppliers.applyStreamError(payload, eventId, conversationId);
                setControllerState(current => ({
                  ...current,
                  connectionByConversation: {
                    ...current.connectionByConversation,
                    [conversationId]: 'idle',
                  },
                }));
                if (payload.message?.toLowerCase().includes(AICHAT_STREAM_EVENTS_EXPIRED)) {
                  closeRecoveryConnection(conversationId);
                }
              },
              onRequestError: error => {
                if (isAbortError(error)) return;
                scheduleReconnect(!isRecoverableStreamTransportError(error));
              },
              onClose: () => {
                if (recoveryAbortByConversationRef.current[conversationId] === abortController) {
                  delete recoveryAbortByConversationRef.current[conversationId];
                }
                if (!reachedTerminalEvent && !abortController.signal.aborted) {
                  scheduleReconnect();
                  return;
                }
                if (reconnectScheduled && !reachedTerminalEvent) return;
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
      clearRecoveryRetry,
      closeRecoveryConnection,
      eventAppliers,
      recoveryAbortByConversationRef,
      recoveryModeByConversationRef,
      recoveryRetryTimeoutsRef,
      refreshAccountMemoryAfterMemoryMutation,
      refreshMessagesSilently,
      setControllerState,
      setRecoveryMode,
      stateRef,
      transportRef,
    ]
  );
}
