import { useCallback } from 'react';

import type {
  AIChatClientActionResultRequest,
  AIChatConversation,
  AIChatMessage,
  AIChatToolGovernanceDecisionRequest,
  AIChatUserInputContinuationRequest,
} from '@/services/types/aichat';
import {
  getNextActiveSendingState,
  mergeRuntimeTimelineWithMessageTimeline,
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
  isContinuationLikelyStartedError,
} from '@/components/chat/runtime/controller/chat-runtime-controller-utils';

import type { UseChatRuntimeMessageActionsArgs } from './types';

export function useWorkflowContinuationActions({
  stateRef,
  transportRef,
  streamAbortByConversationRef,
  streamingMessageRef,
  setControllerState,
  markSelectionTarget,
  refreshConversationSilently,
  refreshMessagesSilently,
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
    applyToolGovernanceDecision,
    applyClientActionRequired,
    applyClientActionResult,
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
      questionInputs?: { query: string; question_answer_option_id?: string },
      toolGovernanceDecision?: {
        correlationId: string;
        payload: AIChatToolGovernanceDecisionRequest;
      },
      clientActionResult?: {
        actionId: string;
        payload: AIChatClientActionResultRequest;
      },
      userInputContinuation?: {
        requestId: string;
        payload: AIChatUserInputContinuationRequest;
      }
    ) => {
      const transport = transportRef.current;
      if (userInputContinuation) {
        if (!transport.continueUserInput) {
          throw new Error('User input continuation is unavailable.');
        }
      } else if (clientActionResult) {
        if (!transport.continueClientAction) {
          throw new Error('Client action continuation is unavailable.');
        }
      } else if (toolGovernanceDecision) {
        if (!transport.continueToolGovernanceDecision) {
          throw new Error('Tool governance continuation is unavailable.');
        }
      } else if (questionInputs) {
        if (!transport.continueWorkflowQuestion) return;
      } else if (!transport.continueWorkflowApproval) {
        return;
      }
      const continueWorkflowQuestionStream = transport.continueWorkflowQuestion?.bind(transport);
      const continueWorkflowApprovalStream = transport.continueWorkflowApproval?.bind(transport);
      const continueToolGovernanceDecisionStream =
        transport.continueToolGovernanceDecision?.bind(transport);
      const continueClientActionStream = transport.continueClientAction?.bind(transport);
      const continueUserInputStream = transport.continueUserInput?.bind(transport);
      const currentState = stateRef.current;
      const conversation =
        currentState.conversations.find(item => item.id === conversationId) ?? null;
      const messages = currentState.messagesByConversation[conversationId] ?? [];
      const persistedSourceMessage = messages.find(message => message.id === messageId);
      const previousStreaming = currentState.streamingByMessageId[messageId];
      const fallbackSourceMessage: AIChatMessage | null =
        toolGovernanceDecision || clientActionResult || userInputContinuation
          ? {
              id: messageId,
              conversation_id: conversationId,
              query: '',
              answer: previousStreaming?.answer ?? '',
              status: clientActionResult
                ? 'waiting_client_action'
                : userInputContinuation
                  ? 'waiting_question'
                  : 'waiting_approval',
              model_name: '',
              created_at: Math.floor(Date.now() / 1000),
              updated_at: Math.floor(Date.now() / 1000),
            }
          : null;
      const sourceMessage = persistedSourceMessage ?? fallbackSourceMessage;
      const sourceTimeline = persistedSourceMessage
        ? mergeRuntimeTimelineWithMessageTimeline(
            timelineFromAIChatMessage(persistedSourceMessage),
            previousStreaming?.timeline
          )
        : (previousStreaming?.timeline ?? []);
      const streamingStatus = previousStreaming?.status;
      const waitingForContinuation =
        Boolean(toolGovernanceDecision) ||
        Boolean(clientActionResult) ||
        Boolean(userInputContinuation) ||
        sourceMessage?.status === 'waiting_approval' ||
        sourceMessage?.status === 'waiting_client_action' ||
        sourceMessage?.status === 'waiting_question' ||
        streamingStatus === 'waiting_approval' ||
        streamingStatus === 'waiting_client_action' ||
        streamingStatus === 'waiting_question';
      if (
        (currentState.isSending && !waitingForContinuation) ||
        (currentState.recoveringByConversation[conversationId] &&
          !toolGovernanceDecision &&
          !clientActionResult &&
          !userInputContinuation)
      ) {
        return;
      }
      if (!conversation || !sourceMessage) return;
      const alreadyContinuingMessage =
        (currentState.isSending || conversation.runtime_status === 'streaming') &&
        (previousStreaming?.status === 'streaming' || conversation.active_message_id === messageId);
      if (alreadyContinuingMessage) return;
      const sourceConversation: AIChatConversation = conversation;
      let streamStarted = false;
      let streamEnded = false;
      let startError: Error | null = null;
      const shouldSyncContinuationOnClose =
        Boolean(toolGovernanceDecision) ||
        Boolean(clientActionResult) ||
        Boolean(userInputContinuation);
      const syncContinuationState = () => {
        if (!shouldSyncContinuationOnClose) return;
        refreshConversationSilently(conversationId);
        refreshMessagesSilently(conversationId);
      };

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

      const continuationMetadata = sourceMessage.metadata
        ? { ...sourceMessage.metadata }
        : undefined;
      if (userInputContinuation && continuationMetadata) {
        delete continuationMetadata.user_input_request;
      }

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
                metadata: continuationMetadata,
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
              timeline: sourceTimeline,
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
            streamStarted = true;
            applyAgentProgress(payload, eventId);
          },
          onIntermediateAnswer: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applyIntermediateAnswer(payload, eventId);
          },
          onUserInputRequested: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applyUserInputRequested(payload, eventId);
          },
          onFileParseStart: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applyFileParseStart(payload, eventId);
          },
          onFileParseEnd: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applyFileParseEnd(payload, eventId);
          },
          onFileParseError: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applyFileParseError(payload, eventId);
          },
          onSkillLoadStart: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applySkillLoadStart(payload, eventId);
          },
          onSkillLoadEnd: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applySkillLoadEnd(payload, eventId);
          },
          onSkillReferenceRead: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applySkillReferenceRead(payload, eventId);
          },
          onSkillCallStart: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applySkillCallStart(payload, eventId);
          },
          onSkillCallEnd: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applySkillCallEnd(payload, eventId);
          },
          onSkillCallError: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applySkillCallError(payload, eventId);
          },
          onSkillArtifactCreated: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applySkillArtifactCreated(payload, eventId);
          },
          onToolGovernanceDecision: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applyToolGovernanceDecision(payload, eventId);
          },
          onClientActionRequired: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applyClientActionRequired(payload, eventId);
          },
          onClientActionResult: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applyClientActionResult(payload, eventId);
          },
          onMemoryMutation: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applyMemoryMutation(payload, eventId);
            refreshAccountMemoryAfterMemoryMutation(payload);
          },
          onWorkflowStarted: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            eventAppliers.applyWorkflowStarted(payload, eventId);
          },
          onWorkflowNodeStarted: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            eventAppliers.applyWorkflowNodeStarted(payload, eventId);
          },
          onWorkflowNodeFinished: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            eventAppliers.applyWorkflowNodeFinished(payload, eventId);
          },
          onWorkflowPaused: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            eventAppliers.applyWorkflowPaused(payload, eventId);
          },
          onWorkflowApprovalRequested: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            eventAppliers.applyWorkflowApprovalRequested(payload, eventId);
          },
          onWorkflowFinished: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            eventAppliers.applyWorkflowFinished(payload, eventId);
          },
          onWorkflowFailed: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            eventAppliers.applyWorkflowFailed(payload, eventId);
          },
          onMessageChunk: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applyMessageChunk(payload, eventId);
          },
          onMessageRetract: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applyMessageRetract(payload, eventId);
          },
          onMessageEnd: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            streamEnded = true;
            applyMessageEnd(payload, eventId);
          },
          onErrorEvent: (payload, eventId) => {
            if (abortController.signal.aborted) return;
            streamStarted = true;
            applyStreamError(payload, eventId, conversationId);
          },
          onRequestError: error => {
            if (isAbortError(error)) return;
            if (!streamStarted) {
              const suppressError =
                (toolGovernanceDecision || clientActionResult || userInputContinuation) &&
                isContinuationLikelyStartedError(error);
              if (!suppressError) {
                startError = error;
              }
              restoreWorkflowApprovalContinuation(suppressError ? undefined : error.message);
              if (suppressError) {
                syncContinuationState();
              }
              return;
            }
            const suppressError =
              (toolGovernanceDecision || clientActionResult || userInputContinuation) &&
              isContinuationLikelyStartedError(error);
            if (suppressError) {
              syncContinuationState();
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
                syncContinuationState();
                return;
              }
              setControllerState(current => ({
                ...current,
                isSending: getNextActiveSendingState(current, conversationId, false),
              }));
              if (!streamEnded) {
                syncContinuationState();
              }
            }
          },
        };
        if (userInputContinuation) {
          if (!continueUserInputStream) return;
          await continueUserInputStream(
            conversationId,
            messageId,
            userInputContinuation.requestId,
            userInputContinuation.payload,
            callbacks,
            abortController.signal
          );
          if (startError) throw startError;
        } else if (clientActionResult) {
          if (!continueClientActionStream) return;
          await continueClientActionStream(
            conversationId,
            messageId,
            clientActionResult.actionId,
            clientActionResult.payload,
            callbacks,
            abortController.signal
          );
          if (startError) throw startError;
        } else if (toolGovernanceDecision) {
          if (!continueToolGovernanceDecisionStream) return;
          await continueToolGovernanceDecisionStream(
            conversationId,
            messageId,
            toolGovernanceDecision.correlationId,
            toolGovernanceDecision.payload,
            callbacks,
            abortController.signal
          );
          if (startError) throw startError;
        } else if (questionInputs) {
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
            const errorMessage = getErrorMessage(error);
            const suppressError =
              (toolGovernanceDecision || clientActionResult || userInputContinuation) &&
              isContinuationLikelyStartedError(error);
            restoreWorkflowApprovalContinuation(suppressError ? undefined : errorMessage);
            if (suppressError) {
              syncContinuationState();
              return;
            }
            if (clientActionResult || toolGovernanceDecision || userInputContinuation) {
              throw error instanceof Error ? error : new Error(errorMessage);
            }
            return;
          }
          if (
            (toolGovernanceDecision || clientActionResult || userInputContinuation) &&
            isContinuationLikelyStartedError(error)
          ) {
            syncContinuationState();
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
      applyToolGovernanceDecision,
      applyClientActionRequired,
      applyClientActionResult,
      applyStreamError,
      applyUserInputRequested,
      eventAppliers,
      markSelectionTarget,
      refreshAccountMemoryAfterMemoryMutation,
      refreshConversationSilently,
      refreshMessagesSilently,
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

  const continueToolGovernanceDecision = useCallback(
    async (
      conversationId: string,
      messageId: string,
      correlationId: string,
      payload: AIChatToolGovernanceDecisionRequest
    ) => {
      await continueWorkflowApproval(conversationId, messageId, undefined, undefined, {
        correlationId,
        payload,
      });
    },
    [continueWorkflowApproval]
  );

  const continueClientAction = useCallback(
    async (
      conversationId: string,
      messageId: string,
      actionId: string,
      payload: AIChatClientActionResultRequest
    ) => {
      await continueWorkflowApproval(conversationId, messageId, undefined, undefined, undefined, {
        actionId,
        payload,
      });
    },
    [continueWorkflowApproval]
  );

  const continueUserInput = useCallback(
    async (
      conversationId: string,
      messageId: string,
      requestId: string,
      payload: AIChatUserInputContinuationRequest
    ) => {
      await continueWorkflowApproval(
        conversationId,
        messageId,
        undefined,
        undefined,
        undefined,
        undefined,
        { requestId, payload }
      );
    },
    [continueWorkflowApproval]
  );

  return {
    continueWorkflowApproval,
    continueWorkflowQuestion,
    continueToolGovernanceDecision,
    continueClientAction,
    continueUserInput,
  };
}
