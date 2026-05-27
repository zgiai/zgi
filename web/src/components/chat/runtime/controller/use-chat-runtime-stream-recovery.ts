import { useCallback } from 'react';
import type { MutableRefObject } from 'react';
import type {
  AIChatConversation,
  AIChatMessage,
  AIChatSkillCallEndEventData,
} from '@/services/types/aichat';
import type {
  AIChatControllerStore,
  AIChatRecoveryMode,
  AIChatSetControllerState,
} from '@/components/chat/controllers/aichat';
import { getNextActiveSendingState } from '@/components/chat/controllers/aichat/selectors';
import type { AIChatRuntimeTransport } from '@/components/chat/transports/aichat-transport';
import {
  createStreamingAIChatMessage,
  upsertAIChatMessage,
} from '@/components/chat/utils/aichat-message';
import {
  AICHAT_RECOVERY_RETRY_DELAYS,
  AICHAT_STREAM_EVENTS_EXPIRED,
  clearStreamingRuntimeMessageMetadata,
  isAbortError,
} from '@/components/chat/runtime/controller/chat-runtime-controller-utils';
import type { ChatRuntimeEventAppliers } from '@/components/chat/runtime/controller/use-chat-runtime-event-appliers';

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
  refreshAccountMemoryAfterToolCall: (payload: AIChatSkillCallEndEventData) => void;
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
  refreshAccountMemoryAfterToolCall,
  eventAppliers,
}: UseChatRuntimeStreamRecoveryArgs) {
  return useCallback(
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
          return storedMode === 'active' && stateRef.current.activeConversationId === conversationId
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
                timeline: replayingFromStart ? [] : (previousStreaming?.timeline ?? []),
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

        void transportRef.current
          .recoverConversationStream(
            conversationId,
            { messageId, afterId },
            {
              onMessageStart: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applyMessageStart(
                  payload,
                  { resetAnswer: false, mode: getCurrentMode(), moveToTop: false },
                  eventId
                );
              },
              onAgentProgress: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applyAgentProgress(payload, eventId);
              },
              onIntermediateAnswer: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applyIntermediateAnswer(payload, eventId);
              },
              onFileParseStart: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applyFileParseStart(payload, eventId);
              },
              onFileParseEnd: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applyFileParseEnd(payload, eventId);
              },
              onFileParseError: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applyFileParseError(payload, eventId);
              },
              onSkillLoadStart: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applySkillLoadStart(payload, eventId);
              },
              onSkillLoadEnd: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applySkillLoadEnd(payload, eventId);
              },
              onSkillReferenceRead: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applySkillReferenceRead(payload, eventId);
              },
              onSkillCallStart: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applySkillCallStart(payload, eventId);
              },
              onSkillCallEnd: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applySkillCallEnd(payload, eventId);
                refreshAccountMemoryAfterToolCall(payload);
              },
              onSkillCallError: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applySkillCallError(payload, eventId);
              },
              onSkillArtifactCreated: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applySkillArtifactCreated(payload, eventId);
              },
              onMessageChunk: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applyMessageChunk(payload, eventId);
              },
              onMessageRetract: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                eventAppliers.applyMessageRetract(payload, eventId);
              },
              onMessageEnd: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                reachedTerminalEvent = true;
                eventAppliers.applyMessageEnd(payload, eventId);
              },
              onErrorEvent: (payload, eventId) => {
                if (abortController.signal.aborted) return;
                reachedTerminalEvent = true;
                eventAppliers.applyStreamError(payload, eventId, conversationId);
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
      clearRecoveryRetry,
      closeRecoveryConnection,
      eventAppliers,
      recoveryAbortByConversationRef,
      recoveryModeByConversationRef,
      recoveryRetryTimeoutsRef,
      refreshAccountMemoryAfterToolCall,
      setControllerState,
      setRecoveryMode,
      stateRef,
      transportRef,
    ]
  );
}
