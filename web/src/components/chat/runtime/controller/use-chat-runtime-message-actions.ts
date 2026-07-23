import { useCallback } from 'react';
import type {
  AIChatMessage,
  AIChatMessageFile,
  AIChatRuntimeSurface,
} from '@/services/types/aichat';
import type { AIChatModelSelection } from '@/components/chat/controllers/aichat';
import {
  canReplaceRootMessage,
  getNextActiveSendingState,
} from '@/components/chat/controllers/aichat/selectors';
import { mergeAIChatMessages } from '@/components/chat/controllers/aichat/state-reducers';
import {
  createDraftAIChatConversation,
  createStreamingAIChatMessage,
  isDraftAIChatConversationId,
  replaceAIChatConversation,
  upsertAIChatMessage,
} from '@/components/chat/utils/aichat-message';
import { toAIChatParameters } from '@/components/chat/utils/aichat-parameters';
import { buildCurrentChatPath } from '@/components/chat/utils/message-tree';
import {
  clearStreamingRuntimeMessageMetadata,
  createClientDraftId,
  getErrorMessage,
  isAbortError,
  isRecoverableStreamTransportError,
} from '@/components/chat/runtime/controller/chat-runtime-controller-utils';
import { useWorkflowContinuationActions } from './use-chat-runtime-message-actions/continuation';
import type { UseChatRuntimeMessageActionsArgs } from './use-chat-runtime-message-actions/types';

export function useChatRuntimeMessageActions({
  stateRef,
  transportRef,
  requireModel,
  pendingStreamAbortRef,
  streamAbortByConversationRef,
  streamingMessageRef,
  setControllerState,
  markSelectionTarget,
  isLatestSelection,
  refreshConversationSilently,
  refreshMessagesSilently,
  refreshAccountMemoryAfterMemoryMutation,
  recoverStreamingConversation,
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
  const send = useCallback(
    async ({
      query,
      model,
      files = [],
      parentId: parentIdOverride,
      useMemory = false,
      forceAdvanceLeaf = false,
      runtimeSurface,
      operationContext,
    }: {
      query: string;
      model: AIChatModelSelection;
      files?: AIChatMessageFile[];
      parentId?: string | null;
      useMemory?: boolean;
      forceAdvanceLeaf?: boolean;
      runtimeSurface?: AIChatRuntimeSurface;
      operationContext?: unknown;
    }) => {
      const trimmedQuery = query.trim();
      const currentState = stateRef.current;
      const activeConversationId = currentState.activeConversationId;
      const activeConversation = activeConversationId
        ? (currentState.conversations.find(item => item.id === activeConversationId) ?? null)
        : null;
      const isActiveRecovering = activeConversationId
        ? currentState.recoveringByConversation[activeConversationId]
        : false;
      const isActiveStopping = activeConversationId
        ? currentState.stoppingByConversation[activeConversationId]
        : false;
      if (
        !trimmedQuery ||
        (requireModel && !model.model) ||
        currentState.isSending ||
        isActiveRecovering ||
        isActiveStopping ||
        activeConversation?.runtime_status === 'streaming'
      ) {
        return;
      }

      const abortController = new AbortController();
      let streamConversationId = activeConversationId;
      let reachedTerminalEvent = false;
      let recoveryRequested = false;
      const requestStreamRecovery = () => {
        if (recoveryRequested) return true;
        if (reachedTerminalEvent || abortController.signal.aborted || !streamConversationId) {
          return false;
        }
        recoveryRequested = true;
        const conversationId = streamConversationId;
        setControllerState(current => ({
          ...current,
          error: null,
          isSending: getNextActiveSendingState(current, conversationId, true),
          connectionByConversation: {
            ...current.connectionByConversation,
            [conversationId]: 'reconnecting',
          },
        }));
        void recoverStreamingConversation(conversationId, { mode: 'active' });
        return true;
      };
      let forceAdvanceLeafConversationId: string | null = null;
      let forceAdvanceLeafMessageId: string | null = null;
      let forceAdvanceLeafPersisted = false;
      const persistForcedLeaf = (conversationId?: string, messageId?: string) => {
        if (!forceAdvanceLeaf || forceAdvanceLeafPersisted || !conversationId || !messageId) {
          return;
        }
        forceAdvanceLeafPersisted = true;
        void transportRef.current
          .updateConversation(conversationId, {
            current_leaf_message_id: messageId,
          })
          .then(conversation => {
            setControllerState(current => {
              const currentConversation = current.conversations.find(
                item => item.id === conversationId
              );
              if (!currentConversation) return current;

              const safeConversation =
                currentConversation.current_leaf_message_id === messageId
                  ? {
                      ...conversation,
                      runtime_status: currentConversation.runtime_status,
                      active_message_id: currentConversation.active_message_id,
                    }
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
            console.warn('Failed to persist forced AIChat leaf', {
              conversationId,
              messageId,
              error: getErrorMessage(error),
            });
          });
      };
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
      const draftConversationId = activeConversationId ? null : createClientDraftId('draft-aichat');
      const draftMessageId = draftConversationId
        ? createClientDraftId('draft-aichat-message')
        : null;
      if (draftConversationId) {
        streamConversationId = draftConversationId;
      }
      const selectionSeq = markSelectionTarget(draftConversationId ?? activeConversationId);

      const recoverDetachedDraftConversation = async (conversationId?: string | null) => {
        if (!draftConversationId || !conversationId) return;
        if (!isLatestSelection(selectionSeq, conversationId)) return;

        const currentState = stateRef.current;
        const activeId = currentState.activeConversationId;
        const hasVisibleConversation =
          activeId === conversationId &&
          (currentState.messagesByConversation[conversationId]?.length ?? 0) > 0;
        if (hasVisibleConversation) return;
        if (activeId && activeId !== conversationId && !isDraftAIChatConversationId(activeId)) {
          return;
        }

        try {
          const { conversation, messages, messagePagination } =
            await transportRef.current.getConversation(conversationId);
          if (!isLatestSelection(selectionSeq, conversationId)) return;

          setControllerState(current => {
            const currentActiveId = current.activeConversationId;
            if (
              currentActiveId &&
              currentActiveId !== conversationId &&
              !isDraftAIChatConversationId(currentActiveId)
            ) {
              return current;
            }

            const nextMessagesByConversation = { ...current.messagesByConversation };
            delete nextMessagesByConversation[draftConversationId];
            nextMessagesByConversation[conversationId] = mergeAIChatMessages(
              current.messagesByConversation[conversationId] ?? [],
              messages
            );

            const nextMessagePaginationByConversation = {
              ...current.messagePaginationByConversation,
              [conversationId]: messagePagination,
            };
            delete nextMessagePaginationByConversation[draftConversationId];

            const nextLoadingOlderByConversation = { ...current.loadingOlderByConversation };
            delete nextLoadingOlderByConversation[draftConversationId];
            const nextRecoveringByConversation = { ...current.recoveringByConversation };
            delete nextRecoveringByConversation[draftConversationId];
            const nextStoppingByConversation = { ...current.stoppingByConversation };
            delete nextStoppingByConversation[draftConversationId];

            return {
              ...current,
              activeConversationId: conversationId,
              conversations: replaceAIChatConversation(
                current.conversations.filter(item => item.id !== draftConversationId),
                conversation
              ),
              messagesByConversation: nextMessagesByConversation,
              messagePaginationByConversation: nextMessagePaginationByConversation,
              loadingOlderByConversation: nextLoadingOlderByConversation,
              recoveringByConversation: {
                ...nextRecoveringByConversation,
                [conversationId]: false,
              },
              stoppingByConversation: {
                ...nextStoppingByConversation,
                [conversationId]: false,
              },
              isLoadingMessages: false,
              isSending: false,
              error: null,
            };
          });
        } catch (error) {
          console.warn('Failed to recover detached AIChat conversation after send', {
            conversationId,
            error: getErrorMessage(error),
          });
        }
      };

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
        await transportRef.current.streamChat(
          {
            conversation_id: activeConversationId ?? undefined,
            parent_id: parentId,
            query: trimmedQuery,
            surface: runtimeSurface,
            model: model.model,
            provider: model.provider,
            ...(files.length > 0 ? { file_ids: files.map(file => file.id) } : {}),
            response_mode: 'streaming',
            parameters: toAIChatParameters(model.parameters),
            operation_context: operationContext,
            ...(useMemory ? { use_memory: true } : {}),
          },
          {
            onOpen: () => {
              if (abortController.signal.aborted || !streamConversationId) return;
              const conversationId = streamConversationId;
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
              if (payload.conversation_id) {
                streamConversationId = payload.conversation_id;
                streamAbortByConversationRef.current[payload.conversation_id] = abortController;
                if (pendingStreamAbortRef.current === abortController) {
                  pendingStreamAbortRef.current = null;
                }
                setControllerState(current => ({
                  ...current,
                  connectionByConversation: {
                    ...current.connectionByConversation,
                    [payload.conversation_id]: 'connected',
                  },
                }));
              }
              if (forceAdvanceLeaf && payload.conversation_id && payload.message_id) {
                forceAdvanceLeafConversationId = payload.conversation_id;
                forceAdvanceLeafMessageId = payload.message_id;
              }
              applyMessageStart(
                payload,
                {
                  query: trimmedQuery,
                  model,
                  files,
                  previousConversationId: draftConversationId ?? activeConversationId,
                  forceAdvanceLeaf,
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
            onToolGovernanceDecision: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyToolGovernanceDecision(payload, eventId);
            },
            onClientActionRequired: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyClientActionRequired(payload, eventId);
            },
            onClientActionResult: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyClientActionResult(payload, eventId);
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
              reachedTerminalEvent = true;
              applyMessageEnd(payload, eventId);
              persistForcedLeaf(
                payload.conversation_id || forceAdvanceLeafConversationId || undefined,
                payload.message_id || forceAdvanceLeafMessageId || undefined
              );
              void recoverDetachedDraftConversation(
                payload.conversation_id || forceAdvanceLeafConversationId || streamConversationId
              );
              const conversationId = payload.conversation_id || streamConversationId;
              if (conversationId) {
                setControllerState(current => ({
                  ...current,
                  connectionByConversation: {
                    ...current.connectionByConversation,
                    [conversationId]: 'idle',
                  },
                }));
              }
            },
            onErrorEvent: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              reachedTerminalEvent = true;
              const errorConversationId =
                payload.conversation_id || forceAdvanceLeafConversationId || streamConversationId;
              const errorMessageId = payload.message_id || forceAdvanceLeafMessageId;
              applyStreamError(payload, eventId, errorConversationId);
              persistForcedLeaf(errorConversationId || undefined, errorMessageId || undefined);
              void recoverDetachedDraftConversation(errorConversationId);
              if (errorConversationId) {
                setControllerState(current => ({
                  ...current,
                  connectionByConversation: {
                    ...current.connectionByConversation,
                    [errorConversationId]: 'idle',
                  },
                }));
              }
            },
            onRequestError: error => {
              if (isAbortError(error)) return;
              if (isRecoverableStreamTransportError(error) && requestStreamRecovery()) return;
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
                if (
                  streamAbortByConversationRef.current[streamConversationId] === abortController
                ) {
                  delete streamAbortByConversationRef.current[streamConversationId];
                }
              }
              if (pendingStreamAbortRef.current === abortController) {
                pendingStreamAbortRef.current = null;
              }
              if (streamingMessageRef.current && !abortController.signal.aborted) {
                streamingMessageRef.current = null;
              }
              if (!reachedTerminalEvent && requestStreamRecovery()) return;
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
          if (isRecoverableStreamTransportError(error) && requestStreamRecovery()) return;
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
      applyUserInputRequested,
      applyFileParseEnd,
      applyFileParseError,
      applyFileParseStart,
      applyMessageChunk,
      applyMessageRetract,
      applyMessageEnd,
      applyMessageStart,
      applyMemoryMutation,
      applySkillCallEnd,
      applySkillCallError,
      applySkillCallStart,
      applySkillArtifactCreated,
      applyToolGovernanceDecision,
      applyClientActionRequired,
      applyClientActionResult,
      applySkillLoadEnd,
      applySkillLoadStart,
      applySkillReferenceRead,
      applyStreamError,
      eventAppliers,
      isLatestSelection,
      markSelectionTarget,
      pendingStreamAbortRef,
      requireModel,
      refreshAccountMemoryAfterMemoryMutation,
      recoverStreamingConversation,
      setControllerState,
      stateRef,
      streamAbortByConversationRef,
      streamingMessageRef,
      transportRef,
    ]
  );

  const replaceRootMessage = useCallback(
    async ({
      messageId,
      query,
      model,
      operationContext,
    }: {
      messageId: string;
      query?: string;
      model?: AIChatModelSelection;
      operationContext?: unknown;
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
      let reachedTerminalEvent = false;
      let recoveryRequested = false;
      const requestStreamRecovery = () => {
        if (recoveryRequested) return true;
        if (reachedTerminalEvent || abortController.signal.aborted) {
          return false;
        }
        recoveryRequested = true;
        setControllerState(current => ({
          ...current,
          error: null,
          isSending: getNextActiveSendingState(current, activeConversationId, true),
          connectionByConversation: {
            ...current.connectionByConversation,
            [activeConversationId]: 'reconnecting',
          },
        }));
        void recoverStreamingConversation(activeConversationId, { mode: 'active' });
        return true;
      };
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
        const replacementMetadata = cleanedSourceMessage.metadata
          ? { ...cleanedSourceMessage.metadata }
          : undefined;
        if (replacementMetadata) {
          delete replacementMetadata.user_input_responses;
        }
        const nextMessage: AIChatMessage = {
          ...cleanedSourceMessage,
          answer: '',
          status: 'streaming',
          error: undefined,
          metadata: replacementMetadata,
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
        await transportRef.current.regenerateMessage(
          messageId,
          {
            query: trimmedQuery || undefined,
            operation_context: operationContext,
            model: model?.model,
            provider: model?.provider,
            parameters: toAIChatParameters(model?.parameters),
          },
          {
            onOpen: () => {
              if (abortController.signal.aborted) return;
              setControllerState(current => ({
                ...current,
                connectionByConversation: {
                  ...current.connectionByConversation,
                  [activeConversationId]: 'connected',
                },
              }));
            },
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
            onToolGovernanceDecision: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyToolGovernanceDecision(payload, eventId);
            },
            onClientActionRequired: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyClientActionRequired(payload, eventId);
            },
            onClientActionResult: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              applyClientActionResult(payload, eventId);
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
              reachedTerminalEvent = true;
              applyMessageEnd(payload, eventId);
              setControllerState(current => ({
                ...current,
                connectionByConversation: {
                  ...current.connectionByConversation,
                  [activeConversationId]: 'idle',
                },
              }));
            },
            onErrorEvent: (payload, eventId) => {
              if (abortController.signal.aborted) return;
              reachedTerminalEvent = true;
              applyStreamError(payload, eventId, activeConversationId);
              setControllerState(current => ({
                ...current,
                connectionByConversation: {
                  ...current.connectionByConversation,
                  [activeConversationId]: 'idle',
                },
              }));
            },
            onRequestError: error => {
              if (isAbortError(error)) return;
              if (isRecoverableStreamTransportError(error) && requestStreamRecovery()) return;
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
                if (!reachedTerminalEvent && requestStreamRecovery()) return;
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
          if (isRecoverableStreamTransportError(error) && requestStreamRecovery()) return;
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
      applyUserInputRequested,
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
      applyToolGovernanceDecision,
      applyClientActionRequired,
      applyClientActionResult,
      applyMemoryMutation,
      applySkillLoadEnd,
      applySkillLoadStart,
      applySkillReferenceRead,
      applyStreamError,
      eventAppliers,
      markSelectionTarget,
      refreshAccountMemoryAfterMemoryMutation,
      recoverStreamingConversation,
      setControllerState,
      stateRef,
      streamAbortByConversationRef,
      streamingMessageRef,
      transportRef,
    ]
  );

  const {
    continueWorkflowApproval,
    continueWorkflowQuestion,
    continueToolGovernanceDecision,
    continueClientAction,
    continueUserInput,
  } = useWorkflowContinuationActions({
    stateRef,
    transportRef,
    requireModel,
    pendingStreamAbortRef,
    streamAbortByConversationRef,
    streamingMessageRef,
    setControllerState,
    markSelectionTarget,
    isLatestSelection,
    refreshConversationSilently,
    refreshMessagesSilently,
    refreshAccountMemoryAfterMemoryMutation,
    recoverStreamingConversation,
    eventAppliers,
  });
  const regenerate = useCallback(
    async (
      messageId: string,
      model: AIChatModelSelection,
      options?: { operationContext?: unknown; runtimeSurface?: AIChatRuntimeSurface }
    ) => {
      const activeConversationId = stateRef.current.activeConversationId;
      if (!activeConversationId) return;

      const activeConversation =
        stateRef.current.conversations.find(item => item.id === activeConversationId) ?? null;
      const messages = stateRef.current.messagesByConversation[activeConversationId] ?? [];
      const source = messages.find(message => message.id === messageId);
      if (!source || !source.query.trim()) return;
      if (canReplaceRootMessage(activeConversation, source, messages)) {
        await replaceRootMessage({
          messageId,
          model,
          operationContext: options?.operationContext,
        });
        return;
      }
      if (!source.parent_id) return;

      await send({
        query: source.query,
        model,
        parentId: source.parent_id,
        useMemory: Boolean(source.metadata?.use_memory),
        forceAdvanceLeaf: source.status === 'error' || source.status === 'stopped',
        runtimeSurface: options?.runtimeSurface,
        operationContext: options?.operationContext,
      });
    },
    [replaceRootMessage, send, stateRef]
  );

  return {
    send,
    regenerate,
    replaceRootMessage,
    continueWorkflowApproval,
    continueWorkflowQuestion,
    continueToolGovernanceDecision,
    continueClientAction,
    continueUserInput,
  };
}
