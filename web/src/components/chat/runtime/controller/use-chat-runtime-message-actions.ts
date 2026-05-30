import { useCallback } from 'react';
import type { MutableRefObject } from 'react';
import type { AIChatMessage, AIChatMessageFile } from '@/services/types/aichat';
import type {
  AIChatControllerStore,
  AIChatModelSelection,
  AIChatSetControllerState,
} from '@/components/chat/controllers/aichat';
import {
  canReplaceRootMessage,
  getNextActiveSendingState,
} from '@/components/chat/controllers/aichat/selectors';
import type { AIChatRuntimeTransport } from '@/components/chat/transports/aichat-transport';
import {
  createDraftAIChatConversation,
  createStreamingAIChatMessage,
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
} from '@/components/chat/runtime/controller/chat-runtime-controller-utils';
import type { ChatRuntimeEventAppliers } from '@/components/chat/runtime/controller/use-chat-runtime-event-appliers';

interface UseChatRuntimeMessageActionsArgs {
  stateRef: MutableRefObject<AIChatControllerStore>;
  transportRef: MutableRefObject<AIChatRuntimeTransport>;
  requireModel: boolean;
  pendingStreamAbortRef: MutableRefObject<AbortController | null>;
  streamAbortByConversationRef: MutableRefObject<Record<string, AbortController>>;
  streamingMessageRef: MutableRefObject<{ conversationId: string; messageId: string } | null>;
  setControllerState: AIChatSetControllerState;
  markSelectionTarget: (conversationId: string | null) => number;
  refreshAccountMemoryAfterMemoryMutation: (
    payload: Parameters<ChatRuntimeEventAppliers['applyMemoryMutation']>[0]
  ) => void;
  eventAppliers: ChatRuntimeEventAppliers;
}

export function useChatRuntimeMessageActions({
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
        await transportRef.current.streamChat(
          {
            conversation_id: activeConversationId ?? undefined,
            parent_id: parentId,
            query: trimmedQuery,
            model: model.model,
            provider: model.provider,
            ...(files.length > 0 ? { file_ids: files.map(file => file.id) } : {}),
            response_mode: 'streaming',
            parameters: toAIChatParameters(model.parameters),
            ...(useMemory ? { use_memory: true } : {}),
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
      applyMemoryMutation,
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
      requireModel,
      refreshAccountMemoryAfterMemoryMutation,
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
        await transportRef.current.regenerateMessage(
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
      applyMemoryMutation,
      applySkillLoadEnd,
      applySkillLoadStart,
      applySkillReferenceRead,
      applyStreamError,
      markSelectionTarget,
      refreshAccountMemoryAfterMemoryMutation,
      setControllerState,
      stateRef,
      streamAbortByConversationRef,
      streamingMessageRef,
      transportRef,
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
        useMemory: Boolean(source.metadata?.use_memory),
      });
    },
    [replaceRootMessage, send, stateRef]
  );

  return {
    send,
    regenerate,
    replaceRootMessage,
  };
}
