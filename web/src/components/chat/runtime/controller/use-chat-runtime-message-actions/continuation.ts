import { useCallback } from 'react';

import type { AIChatConversation } from '@/services/types/aichat';
import {
  getNextActiveSendingState,
  timelineFromAIChatMessage,
} from '@/components/chat/controllers/aichat/selectors';
import type {
  AIChatStreamCallbacks,
  AIChatWorkflowApprovalContinuationPayload,
} from '@/components/chat/transports/aichat-transport';
import { upsertAIChatMessage } from '@/components/chat/utils/aichat-message';
import {
  getErrorMessage,
  isAbortError,
} from '@/components/chat/runtime/controller/chat-runtime-controller-utils';

import type { UseChatRuntimeMessageActionsArgs } from './types';

export function useWorkflowContinuationActions({
  stateRef,
  transportRef,
  streamAbortByConversationRef,
  streamingMessageRef,
  setControllerState,
  markSelectionTarget,
  refreshAccountMemoryAfterMemoryMutation,
  eventAppliers,
}: UseChatRuntimeMessageActionsArgs) {
  const {
    applyMessageStart,
    applyMessageChunk,
    applyMessageRetract,
    applyFileParseStart,
    applyFileParseEnd,
    applyFileParseError,
    applySkillCallStart,
    applySkillLoadStart,
    applySkillLoadEnd,
    applySkillReferenceRead,
    applySkillCallEnd,
    applySkillCallError,
    applySkillArtifactCreated,
    applyMemoryMutation,
    applyAgentProgress,
    applyIntermediateAnswer,
    applyUserInputRequested,
    applyMessageEnd,
    applyStreamError,
  } = eventAppliers;
  const continueWorkflowApproval = useCallback(
    async (
      conversationId: string,
      messageId: string,
      approvalPayload?: AIChatWorkflowApprovalContinuationPayload,
      questionInputs?: { query: string; question_answer_option_id?: string }
    ) => {
      const transport = transportRef.current;
      if (questionInputs) {
        if (!transport.continueWorkflowQuestion) return;
      } else if (!transport.continueWorkflowApproval) {
        return;
      }
      const continueWorkflowQuestionStream = transport.continueWorkflowQuestion?.bind(transport);
      const continueWorkflowApprovalStream = transport.continueWorkflowApproval?.bind(transport);
      const currentState = stateRef.current;
      if (currentState.isSending || currentState.recoveringByConversation[conversationId]) return;
      const conversation =
        currentState.conversations.find(item => item.id === conversationId) ?? null;
      const messages = currentState.messagesByConversation[conversationId] ?? [];
      const sourceMessage = messages.find(message => message.id === messageId);
      if (!conversation || !sourceMessage) return;
      const sourceConversation: AIChatConversation = conversation;
      let streamStarted = false;

      const restoreWorkflowApprovalContinuation = (errorMessage?: string) => {
        setControllerState(current => {
          const nextStreamingByMessageId = { ...current.streamingByMessageId };
          delete nextStreamingByMessageId[messageId];

          return {
            ...current,
            error:
              errorMessage && current.activeConversationId === conversationId
                ? errorMessage
                : current.error,
            isSending: getNextActiveSendingState(current, conversationId, false),
            conversations: current.conversations.map(item =>
              item.id === conversationId ? sourceConversation : item
            ),
            messagesByConversation: {
              ...current.messagesByConversation,
              [conversationId]: upsertAIChatMessage(
                current.messagesByConversation[conversationId] ?? [],
                sourceMessage
              ),
            },
            streamingByMessageId: nextStreamingByMessageId,
          };
        });
      };

      const abortController = new AbortController();
      streamAbortByConversationRef.current[conversationId]?.abort();
      streamAbortByConversationRef.current[conversationId] = abortController;
      markSelectionTarget(conversationId);

      setControllerState(current => {
        const now = Math.floor(Date.now() / 1000);
        return {
          ...current,
          error: null,
          isSending: true,
          conversations: current.conversations.map(item =>
            item.id === conversationId
              ? {
                  ...item,
                  runtime_status: 'streaming',
                  active_message_id: messageId,
                  updated_at: now,
                }
              : item
          ),
          messagesByConversation: {
            ...current.messagesByConversation,
            [conversationId]: upsertAIChatMessage(
              current.messagesByConversation[conversationId] ?? [],
              {
                ...sourceMessage,
                status: 'streaming',
                error: undefined,
                updated_at: now,
              }
            ),
          },
          streamingByMessageId: {
            ...current.streamingByMessageId,
            [messageId]: {
              conversation_id: conversationId,
              message_id: messageId,
              answer: sourceMessage.answer ?? '',
              status: 'streaming',
              timeline: timelineFromAIChatMessage(sourceMessage),
            },
          },
        };
      });

      try {
        const callbacks: AIChatStreamCallbacks = {
          onMessageStart: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applyMessageStart(
              payload,
              {
                mode: 'active',
                moveToTop: false,
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
          onUserInputRequested: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            applyUserInputRequested(payload, eventId);
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
          },
          onSkillCallError: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            applySkillCallError(payload, eventId);
          },
          onSkillArtifactCreated: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            applySkillArtifactCreated(payload, eventId);
          },
          onMemoryMutation: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            applyMemoryMutation(payload, eventId);
            refreshAccountMemoryAfterMemoryMutation(payload);
          },
          onWorkflowStarted: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            eventAppliers.applyWorkflowStarted(payload, eventId);
          },
          onWorkflowNodeStarted: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            eventAppliers.applyWorkflowNodeStarted(payload, eventId);
          },
          onWorkflowNodeFinished: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            eventAppliers.applyWorkflowNodeFinished(payload, eventId);
          },
          onWorkflowPaused: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            eventAppliers.applyWorkflowPaused(payload, eventId);
          },
          onWorkflowApprovalRequested: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            eventAppliers.applyWorkflowApprovalRequested(payload, eventId);
          },
          onWorkflowFinished: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            eventAppliers.applyWorkflowFinished(payload, eventId);
          },
          onWorkflowFailed: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            eventAppliers.applyWorkflowFailed(payload, eventId);
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
            applyStreamError(payload, eventId, conversationId);
          },
          onRequestError: error => {
            if (isAbortError(error)) return;
            if (!streamStarted) {
              restoreWorkflowApprovalContinuation(error.message);
              return;
            }
            setControllerState(current => ({
              ...current,
              error:
                current.activeConversationId === conversationId ? error.message : current.error,
              isSending: getNextActiveSendingState(current, conversationId, false),
            }));
          },
          onClose: () => {
            if (streamAbortByConversationRef.current[conversationId] === abortController) {
              delete streamAbortByConversationRef.current[conversationId];
            }
            if (streamingMessageRef.current?.messageId === messageId) {
              streamingMessageRef.current = null;
            }
            if (!abortController.signal.aborted) {
              if (!streamStarted) {
                restoreWorkflowApprovalContinuation();
                return;
              }
              setControllerState(current => ({
                ...current,
                isSending: getNextActiveSendingState(current, conversationId, false),
              }));
            }
          },
        };
        if (questionInputs) {
          if (!continueWorkflowQuestionStream) return;
          await continueWorkflowQuestionStream(
            conversationId,
            messageId,
            { inputs: questionInputs },
            callbacks,
            abortController.signal
          );
        } else {
          if (!continueWorkflowApprovalStream) return;
          await continueWorkflowApprovalStream(
            conversationId,
            messageId,
            approvalPayload,
            callbacks,
            abortController.signal
          );
        }
      } catch (error) {
        if (!isAbortError(error)) {
          if (!streamStarted) {
            restoreWorkflowApprovalContinuation(getErrorMessage(error));
            return;
          }
          setControllerState(current => ({
            ...current,
            error:
              current.activeConversationId === conversationId
                ? getErrorMessage(error)
                : current.error,
            isSending: getNextActiveSendingState(current, conversationId, false),
          }));
        }
      }
    },
    [
      applyAgentProgress,
      applyFileParseEnd,
      applyFileParseError,
      applyFileParseStart,
      applyIntermediateAnswer,
      applyMemoryMutation,
      applyMessageChunk,
      applyMessageEnd,
      applyMessageRetract,
      applyMessageStart,
      applySkillArtifactCreated,
      applySkillCallEnd,
      applySkillCallError,
      applySkillCallStart,
      applySkillLoadEnd,
      applySkillLoadStart,
      applySkillReferenceRead,
      applyStreamError,
      applyUserInputRequested,
      eventAppliers,
      markSelectionTarget,
      refreshAccountMemoryAfterMemoryMutation,
      setControllerState,
      stateRef,
      streamAbortByConversationRef,
      streamingMessageRef,
      transportRef,
    ]
  );

  const continueWorkflowQuestion = useCallback(
    async (
      conversationId: string,
      messageId: string,
      inputs: { query: string; question_answer_option_id?: string }
    ) => {
      await continueWorkflowApproval(conversationId, messageId, undefined, inputs);
    },
    [continueWorkflowApproval]
  );


  return { continueWorkflowApproval, continueWorkflowQuestion };
}