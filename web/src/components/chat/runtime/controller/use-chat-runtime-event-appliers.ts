import { useCallback, useMemo } from 'react';
import type { MutableRefObject } from 'react';
import type {
  AIChatAgentProgressEventData,
  AIChatClientActionRequiredEventData,
  AIChatClientActionResultEventData,
  AIChatErrorEventData,
  AIChatFileParseEndEventData,
  AIChatFileParseErrorEventData,
  AIChatFileParseStartEventData,
  AIChatIntermediateAnswerEventData,
  AIChatMessageChunkEventData,
  AIChatMessageEndEventData,
  AIChatMessageRetractEventData,
  AIChatMessageStartEventData,
  AIChatMemoryMutationEventData,
  AIChatSkillArtifactCreatedEventData,
  AIChatSkillCallEndEventData,
  AIChatSkillCallErrorEventData,
  AIChatSkillCallStartEventData,
  AIChatSkillLoadEndEventData,
  AIChatSkillLoadStartEventData,
  AIChatSkillReferenceReadEventData,
  AIChatToolGovernanceDecisionEventData,
  AIChatUserInputRequestedEventData,
  AIChatWorkflowEventData,
  AIChatWorkflowNodeEventData,
  AIChatWorkflowPausedEventData,
} from '@/services/types/aichat';
import type {
  AIChatControllerState,
  AIChatControllerStore,
  AIChatMessageStartContext,
  AIChatRecoveryMode,
  AIChatSetControllerState,
} from '@/components/chat/controllers/aichat';
import {
  applyAgentProgressState,
  applyFileParseEndState,
  applyFileParseErrorState,
  applyFileParseStartState,
  applyIntermediateAnswerState,
  applyUserInputRequestedState,
  applyMessageChunkState,
  applyMessageEndState,
  applyMemoryMutationState,
  applyMessageRetractState,
  applyMessageStartState,
  applySkillArtifactCreatedState,
  applySkillCallEndState,
  applySkillCallErrorState,
  applySkillCallStartState,
  applySkillLoadEndState,
  applySkillLoadStartState,
  applySkillReferenceReadState,
  applyStreamErrorState,
  applyToolGovernanceDecisionState,
  applyWorkflowApprovalRequestedState,
  applyWorkflowFailedState,
  applyWorkflowFinishedState,
  applyWorkflowNodeFinishedState,
  applyWorkflowNodeStartedState,
  applyWorkflowPausedState,
  applyWorkflowStartedState,
} from '@/components/chat/controllers/aichat/state-reducers';
import { debugAIChatTimeline } from '@/components/chat/controllers/aichat/debug';
import { isDraftAIChatConversationId } from '@/components/chat/utils/aichat-message';

function runtimeDebugRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

interface UseAIChatEventAppliersArgs {
  stateRef: MutableRefObject<AIChatControllerStore>;
  backgroundConversationIdRef: MutableRefObject<string | null>;
  streamingMessageRef: MutableRefObject<{ conversationId: string; messageId: string } | null>;
  recoveryModeByConversationRef: MutableRefObject<Record<string, AIChatRecoveryMode>>;
  setControllerState: AIChatSetControllerState;
  resolveMessageStartMode: (
    payload: AIChatMessageStartEventData,
    context: {
      previousConversationId?: string | null;
      mode?: AIChatRecoveryMode;
    }
  ) => AIChatRecoveryMode;
  migrateLatestSelectionTarget: (from: string | null, to: string) => void;
  clearRecoveryRetry: (conversationId: string) => void;
  refreshConversationSilently: (conversationId: string) => void;
  refreshMessagesSilently: (conversationId: string) => void;
}

function shouldRefreshConversationAfterMessageEnd(
  current: AIChatControllerStore,
  payload: AIChatMessageEndEventData
): boolean {
  const conversation = current.conversations.find(item => item.id === payload.conversation_id);
  if (!conversation) return true;

  const status = String(payload.status ?? '').toLowerCase();
  if (status === 'completed') {
    return true;
  }
  if (
    status === 'waiting_approval' ||
    status === 'waiting_client_action' ||
    status === 'waiting_question' ||
    status === 'stopped' ||
    status === 'error' ||
    status === 'failed'
  ) {
    return true;
  }

  return false;
}

function shouldRefreshMessagesAfterMessageEnd(
  current: AIChatControllerStore,
  payload: AIChatMessageEndEventData
): boolean {
  const messages = current.messagesByConversation[payload.conversation_id] ?? [];
  return !messages.some(message => message.id === payload.message_id);
}

function clientActionProgressPayload(
  payload: AIChatClientActionRequiredEventData | AIChatClientActionResultEventData,
  phase: AIChatAgentProgressEventData['phase']
): AIChatAgentProgressEventData | null {
  if (!payload.conversation_id || !payload.message_id || !payload.action_id) {
    return null;
  }
  return {
    conversation_id: payload.conversation_id,
    message_id: payload.message_id,
    phase,
    skill_id: payload.skill_id,
    tool_name: payload.tool_name,
    action_id: payload.action_id,
    action_type: payload.action_type,
    status: payload.status,
    effect: payload.effect,
    asset_type: payload.asset_type,
    assets: payload.assets,
    correlation_id:
      typeof payload.correlation_id === 'string' ? payload.correlation_id : undefined,
    asset_operation_audit: payload.asset_operation_audit,
    result: clientActionProgressResult(payload),
    created_at: payload.created_at,
  };
}

function clientActionProgressResult(
  payload: AIChatClientActionRequiredEventData | AIChatClientActionResultEventData
): Record<string, unknown> | null | undefined {
  const result =
    payload.result && typeof payload.result === 'object' && !Array.isArray(payload.result)
      ? { ...payload.result }
      : {};
  const mergeIfPresent = (key: string, value: unknown) => {
    if (value !== undefined && value !== null && result[key] === undefined) {
      result[key] = value;
    }
  };

  mergeIfPresent('action_type', payload.action_type);
  mergeIfPresent('action_id', payload.action_id);
  mergeIfPresent('correlation_id', payload.correlation_id);
  mergeIfPresent('href', payload.href);
  mergeIfPresent('effect', payload.effect);
  mergeIfPresent('asset_type', payload.asset_type);
  mergeIfPresent('assets', payload.assets);
  mergeIfPresent('asset_operation_audit', payload.asset_operation_audit);

  return Object.keys(result).length > 0 ? result : payload.result;
}

/**
 * @hook useChatRuntimeEventAppliers
 * @description Maps ChatRuntime SSE events into controller state mutations.
 */
export function useChatRuntimeEventAppliers({
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
}: UseAIChatEventAppliersArgs) {
  const applyMessageStart = useCallback(
    (
      payload: AIChatMessageStartEventData,
      context: AIChatMessageStartContext = {},
      eventId?: string | null
    ) => {
      if (!payload.conversation_id || !payload.message_id) return;
      debugAIChatTimeline('event:message_start', {
        eventId,
        conversation_id: payload.conversation_id,
        message_id: payload.message_id,
        replace: payload.replace,
        status: 'streaming',
      });

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
      stateRef,
      streamingMessageRef,
    ]
  );

  const applyMessageChunk = useCallback(
    (payload: AIChatMessageChunkEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      debugAIChatTimeline('event:message_chunk', {
        eventId,
        conversation_id: payload.conversation_id,
        message_id: payload.message_id,
        answer_len: (payload.answer ?? '').length,
      });
      setControllerState(current => applyMessageChunkState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyMessageRetract = useCallback(
    (payload: AIChatMessageRetractEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      debugAIChatTimeline('event:message_retract', {
        eventId,
        conversation_id: payload.conversation_id,
        message_id: payload.message_id,
        content_len: (payload.content ?? '').length,
        length: payload.length,
      });
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
      const payloadRecord = runtimeDebugRecord(payload);
      const dynamicPayload = payload as unknown as Record<string, unknown>;
      debugAIChatTimeline('event:skill_call_start', {
        eventId,
        conversation_id: payload.conversation_id,
        message_id: payload.message_id,
        runtime_id: payload.runtime_id,
        kind: payload.kind,
        skill_id: payload.skill_id,
        tool_name: payload.tool_name,
        action_type: dynamicPayload.action_type,
        action_id: dynamicPayload.action_id,
        created_at: payload.created_at,
        created_at_ms: payloadRecord.created_at_ms,
      });
      setControllerState(current => applySkillCallStartState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applySkillLoadStart = useCallback(
    (payload: AIChatSkillLoadStartEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.skill_id) return;
      const payloadRecord = runtimeDebugRecord(payload);
      debugAIChatTimeline('event:skill_load_start', {
        eventId,
        conversation_id: payload.conversation_id,
        message_id: payload.message_id,
        runtime_id: payloadRecord.runtime_id,
        skill_id: payload.skill_id,
        created_at: payload.created_at,
        created_at_ms: payloadRecord.created_at_ms,
      });
      setControllerState(current => applySkillLoadStartState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applySkillLoadEnd = useCallback(
    (payload: AIChatSkillLoadEndEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.skill_id) return;
      const payloadRecord = runtimeDebugRecord(payload);
      debugAIChatTimeline('event:skill_load_end', {
        eventId,
        conversation_id: payload.conversation_id,
        message_id: payload.message_id,
        runtime_id: payloadRecord.runtime_id,
        skill_id: payload.skill_id,
        status: payload.status,
        created_at: payload.created_at,
        created_at_ms: payloadRecord.created_at_ms,
      });
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
      const payloadRecord = runtimeDebugRecord(payload);
      debugAIChatTimeline('event:skill_call_end', {
        eventId,
        conversation_id: payload.conversation_id,
        message_id: payload.message_id,
        runtime_id: payload.runtime_id,
        kind: payload.kind,
        skill_id: payload.skill_id,
        tool_name: payload.tool_name,
        action_type: payload.action_type,
        action_id: payload.action_id,
        status: payload.status,
        created_at: payload.created_at,
        created_at_ms: payloadRecord.created_at_ms,
      });
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
      if (!payload.conversation_id || !payload.message_id) return;
      if (!payload.content && !payload.phase) return;
      const payloadRecord = runtimeDebugRecord(payload);
      debugAIChatTimeline('event:agent_progress', {
        eventId,
        conversation_id: payload.conversation_id,
        message_id: payload.message_id,
        phase: payload.phase,
        content_len: (payload.content ?? '').length,
        content_preview: (payload.content ?? '').slice(0, 80),
        skill_id: payload.skill_id,
        tool_name: payload.tool_name,
        created_at: payload.created_at,
        created_at_ms: payloadRecord.created_at_ms,
      });
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

  const applyUserInputRequested = useCallback(
    (payload: AIChatUserInputRequestedEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.questions?.length) return;
      setControllerState(current => applyUserInputRequestedState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyMemoryMutation = useCallback(
    (payload: AIChatMemoryMutationEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id || !payload.action) return;
      setControllerState(current => applyMemoryMutationState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyToolGovernanceDecision = useCallback(
    (payload: AIChatToolGovernanceDecisionEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      debugAIChatTimeline('event:tool_governance_decision', {
        eventId,
        conversation_id: payload.conversation_id,
        message_id: payload.message_id,
        runtime_id: payload.runtime_id,
        skill_id: payload.skill_id,
        tool_name: payload.tool_name,
        status: payload.status,
        approval_status: payload.approval_status,
        correlation_id: payload.correlation_id,
        created_at: payload.created_at,
        created_at_ms: payload.created_at_ms,
      });
      setControllerState(current => applyToolGovernanceDecisionState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyClientActionRequired = useCallback(
    (payload: AIChatClientActionRequiredEventData, eventId?: string | null) => {
      debugAIChatTimeline('event:client_action_required', {
        eventId,
        conversation_id: payload.conversation_id,
        message_id: payload.message_id,
        action_id: payload.action_id,
        action_type: payload.action_type,
        skill_id: payload.skill_id,
        tool_name: payload.tool_name,
        status: payload.status,
      });
      const progressPayload = clientActionProgressPayload(payload, 'client_action');
      if (!progressPayload) return;
      setControllerState(current => applyAgentProgressState(current, progressPayload, eventId));
    },
    [setControllerState]
  );

  const applyClientActionResult = useCallback(
    (payload: AIChatClientActionResultEventData, eventId?: string | null) => {
      debugAIChatTimeline('event:client_action_result', {
        eventId,
        conversation_id: payload.conversation_id,
        message_id: payload.message_id,
        action_id: payload.action_id,
        action_type: payload.action_type,
        skill_id: payload.skill_id,
        tool_name: payload.tool_name,
        status: payload.status,
      });
      const progressPayload = clientActionProgressPayload(payload, 'client_action_result');
      if (!progressPayload) return;
      setControllerState(current => applyAgentProgressState(current, progressPayload, eventId));
    },
    [setControllerState]
  );

  const applyWorkflowStarted = useCallback(
    (payload: AIChatWorkflowEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      setControllerState(current => applyWorkflowStartedState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyWorkflowNodeStarted = useCallback(
    (payload: AIChatWorkflowNodeEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      setControllerState(current => applyWorkflowNodeStartedState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyWorkflowNodeFinished = useCallback(
    (payload: AIChatWorkflowNodeEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      setControllerState(current => applyWorkflowNodeFinishedState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyWorkflowPaused = useCallback(
    (payload: AIChatWorkflowPausedEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      setControllerState(current => applyWorkflowPausedState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyWorkflowApprovalRequested = useCallback(
    (payload: AIChatWorkflowPausedEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      setControllerState(current => applyWorkflowApprovalRequestedState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyWorkflowFinished = useCallback(
    (payload: AIChatWorkflowEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      setControllerState(current => applyWorkflowFinishedState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyWorkflowFailed = useCallback(
    (payload: AIChatWorkflowEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      setControllerState(current => applyWorkflowFailedState(current, payload, eventId));
    },
    [setControllerState]
  );

  const applyMessageEnd = useCallback(
    (payload: AIChatMessageEndEventData, eventId?: string | null) => {
      if (!payload.conversation_id || !payload.message_id) return;
      const shouldRefreshConversation = shouldRefreshConversationAfterMessageEnd(
        stateRef.current,
        payload
      );
      const shouldRefreshMessages = shouldRefreshMessagesAfterMessageEnd(stateRef.current, payload);
      if (streamingMessageRef.current?.messageId === payload.message_id) {
        streamingMessageRef.current = null;
      }
      setControllerState(current => applyMessageEndState(current, payload, eventId));

      clearRecoveryRetry(payload.conversation_id);
      delete recoveryModeByConversationRef.current[payload.conversation_id];
      if (backgroundConversationIdRef.current === payload.conversation_id) {
        backgroundConversationIdRef.current = null;
      }
      if (shouldRefreshConversation) {
        refreshConversationSilently(payload.conversation_id);
      }
      if (shouldRefreshMessages) {
        refreshMessagesSilently(payload.conversation_id);
      }
    },
    [
      backgroundConversationIdRef,
      clearRecoveryRetry,
      recoveryModeByConversationRef,
      refreshConversationSilently,
      refreshMessagesSilently,
      setControllerState,
      stateRef,
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
      setControllerState((current: AIChatControllerState) => {
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
      stateRef,
    ]
  );

  return useMemo(
    () => ({
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
      applyWorkflowStarted,
      applyWorkflowNodeStarted,
      applyWorkflowNodeFinished,
      applyWorkflowPaused,
      applyWorkflowApprovalRequested,
      applyWorkflowFinished,
      applyWorkflowFailed,
      applyAgentProgress,
      applyIntermediateAnswer,
      applyUserInputRequested,
      applyMessageEnd,
      applyStreamError,
    }),
    [
      applyAgentProgress,
      applyClientActionRequired,
      applyClientActionResult,
      applyFileParseEnd,
      applyFileParseError,
      applyFileParseStart,
      applyIntermediateAnswer,
      applyUserInputRequested,
      applyMemoryMutation,
      applyWorkflowApprovalRequested,
      applyWorkflowFailed,
      applyWorkflowFinished,
      applyWorkflowNodeFinished,
      applyWorkflowNodeStarted,
      applyWorkflowPaused,
      applyWorkflowStarted,
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
      applyToolGovernanceDecision,
    ]
  );
}

export type ChatRuntimeEventAppliers = ReturnType<typeof useChatRuntimeEventAppliers>;
