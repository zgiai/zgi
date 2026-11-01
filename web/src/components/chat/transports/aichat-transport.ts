import { aichatService } from '@/services/aichat.service';
import {
  isSensitiveOutputBlockedValue,
  sanitizeModelOutputValue,
} from '@/utils/model-output-filter';
import type {
  ExecutionFileReader,
  AIChatChatRequest,
  AIChatAgentProgressEventData,
  AIChatPlanUpdatedEventData,
  AIChatClientActionRequiredEventData,
  AIChatClientActionResultEventData,
  AIChatClientActionResultRequest,
  AIChatConversation,
  AIChatErrorEventData,
  AIChatFileParseEndEventData,
  AIChatFileParseErrorEventData,
  AIChatFileParseStartEventData,
  AIChatMessage,
  AIChatMessageChunkEventData,
  AIChatMessageEndEventData,
  AIChatMessageRetractEventData,
  AIChatMessageStartEventData,
  AIChatTokenUsageUpdatedEventData,
  AIChatTokenBudgetRequestedEventData,
  AIChatMemoryMutationEventData,
  AIChatIntermediateAnswerEventData,
  AIChatUserInputRequestedEventData,
  AIChatRegenerateMessageRequest,
  AIChatRuntimeSurface,
  AIChatRuntimeInputAppliedEventData,
  AIChatSearchResult,
  AIChatSkillCallEndEventData,
  AIChatSkillCallErrorEventData,
  AIChatSkillCallStartEventData,
  AIChatSkillArtifactCreatedEventData,
  AIChatSkillLoadEndEventData,
  AIChatSkillLoadStartEventData,
  AIChatSkillReferenceReadEventData,
  AIChatStopConversationResponseData,
  AIChatSubmitRuntimeInputRequest,
  AIChatSubmitRuntimeInputResponseData,
  AIChatToolGovernanceDecisionRequest,
  AIChatUserInputContinuationRequest,
  AIChatToolGovernanceDecisionEventData,
  AIChatWorkflowEventData,
  AIChatWorkflowNodeEventData,
  AIChatWorkflowPausedEventData,
  AIChatWorkflowQuestionAnswerInputs,
  AIChatWorkflowRunNodeMetadata,
} from '@/services/types/aichat';
import type { ConversationSearchResult } from '@/components/chat/controllers/types';
import {
  DEFAULT_AICHAT_MESSAGE_PAGINATION,
  type AIChatPagination,
} from '@/components/chat/controllers/aichat';
import { decodeStreamEventPayload } from './stream-event-decoder';

function sanitizeWorkflowNodeOutputs(
  node: AIChatWorkflowRunNodeMetadata
): AIChatWorkflowRunNodeMetadata {
  return {
    ...node,
    outputs: sanitizeModelOutputValue(node.outputs),
    iteration_outputs: sanitizeModelOutputValue(node.iteration_outputs),
    loop_outputs: sanitizeModelOutputValue(node.loop_outputs),
    iteration_rounds: node.iteration_rounds?.map(round => ({
      ...round,
      nodes: round.nodes?.map(sanitizeWorkflowNodeOutputs),
    })),
    loop_rounds: node.loop_rounds?.map(round => ({
      ...round,
      nodes: round.nodes?.map(sanitizeWorkflowNodeOutputs),
    })),
  };
}

export function sanitizeAIChatMessage(message: AIChatMessage): AIChatMessage {
  const metadata = message.metadata;
  const sanitizedMetadata = metadata
    ? {
        ...metadata,
        presentation: metadata.presentation
          ? {
              ...metadata.presentation,
              items: metadata.presentation.items?.map(item =>
                item.kind === 'text'
                  ? { ...item, content: sanitizeModelOutputValue(item.content) as string }
                  : item
              ),
            }
          : metadata.presentation,
        skill_invocations: metadata.skill_invocations?.map(invocation => ({
          ...invocation,
          result: sanitizeModelOutputValue(invocation.result) as typeof invocation.result,
          ...(invocation.kind === 'intermediate_answer'
            ? {
                title: sanitizeModelOutputValue(invocation.title) as typeof invocation.title,
                message: sanitizeModelOutputValue(invocation.message) as typeof invocation.message,
              }
            : {}),
        })),
        workflow_runs: metadata.workflow_runs?.map(run => ({
          ...run,
          outputs: sanitizeModelOutputValue(run.outputs),
          nodes: run.nodes?.map(sanitizeWorkflowNodeOutputs),
        })),
      }
    : metadata;
  const sanitizedMessage: AIChatMessage = {
    ...message,
    answer: sanitizeModelOutputValue(message.answer) as string,
    metadata: sanitizedMetadata,
  };
  const presentationBlocked = sanitizedMetadata?.presentation?.items?.some(
    item => item.kind === 'text' && isSensitiveOutputBlockedValue(item.content)
  );
  const isSensitiveOutputBlocked =
    isSensitiveOutputBlockedValue(sanitizedMessage.answer) || presentationBlocked === true;
  return {
    ...sanitizedMessage,
    metadata: isSensitiveOutputBlocked
      ? {
          ...sanitizedMessage.metadata,
          sensitiveOutputBlocked: true,
        }
      : sanitizedMessage.metadata,
  };
}

export interface AIChatConversationDetail {
  conversation: AIChatConversation;
  messages: AIChatMessage[];
  messagePagination: AIChatPagination;
}

export interface AIChatConversationListResult {
  items: AIChatConversation[];
  pagination: AIChatPagination;
}

export interface AIChatMessageListResult {
  items: AIChatMessage[];
  pagination: AIChatPagination;
}

export interface AIChatStreamOpenMetadata {
  conversation_id?: string;
  message_id?: string;
}

export interface AIChatStreamCallbacks {
  onOpen?: (metadata?: AIChatStreamOpenMetadata) => void;
  onMessageStart: (payload: AIChatMessageStartEventData, eventId?: string | null) => void;
  onAgentProgress: (payload: AIChatAgentProgressEventData, eventId?: string | null) => void;
  onPlanUpdated?: (payload: AIChatPlanUpdatedEventData, eventId?: string | null) => void;
  onIntermediateAnswer: (
    payload: AIChatIntermediateAnswerEventData,
    eventId?: string | null
  ) => void;
  onUserInputRequested: (
    payload: AIChatUserInputRequestedEventData,
    eventId?: string | null
  ) => void;
  onFileParseStart: (payload: AIChatFileParseStartEventData, eventId?: string | null) => void;
  onFileParseEnd: (payload: AIChatFileParseEndEventData, eventId?: string | null) => void;
  onFileParseError: (payload: AIChatFileParseErrorEventData, eventId?: string | null) => void;
  onSkillLoadStart: (payload: AIChatSkillLoadStartEventData, eventId?: string | null) => void;
  onSkillLoadEnd: (payload: AIChatSkillLoadEndEventData, eventId?: string | null) => void;
  onSkillReferenceRead: (
    payload: AIChatSkillReferenceReadEventData,
    eventId?: string | null
  ) => void;
  onSkillCallStart: (payload: AIChatSkillCallStartEventData, eventId?: string | null) => void;
  onSkillCallEnd: (payload: AIChatSkillCallEndEventData, eventId?: string | null) => void;
  onSkillCallError: (payload: AIChatSkillCallErrorEventData, eventId?: string | null) => void;
  onSkillArtifactCreated: (
    payload: AIChatSkillArtifactCreatedEventData,
    eventId?: string | null
  ) => void;
  onToolGovernanceDecision?: (
    payload: AIChatToolGovernanceDecisionEventData,
    eventId?: string | null
  ) => void;
  onClientActionRequired?: (
    payload: AIChatClientActionRequiredEventData,
    eventId?: string | null
  ) => void;
  onClientActionResult?: (
    payload: AIChatClientActionResultEventData,
    eventId?: string | null
  ) => void;
  onMemoryMutation: (payload: AIChatMemoryMutationEventData, eventId?: string | null) => void;
  onRuntimeInputApplied?: (
    payload: AIChatRuntimeInputAppliedEventData,
    eventId?: string | null
  ) => void;
  onWorkflowStarted?: (payload: AIChatWorkflowEventData, eventId?: string | null) => void;
  onWorkflowNodeStarted?: (payload: AIChatWorkflowNodeEventData, eventId?: string | null) => void;
  onWorkflowNodeFinished?: (payload: AIChatWorkflowNodeEventData, eventId?: string | null) => void;
  onWorkflowPaused?: (payload: AIChatWorkflowPausedEventData, eventId?: string | null) => void;
  onWorkflowApprovalRequested?: (
    payload: AIChatWorkflowPausedEventData,
    eventId?: string | null
  ) => void;
  onWorkflowFinished?: (payload: AIChatWorkflowEventData, eventId?: string | null) => void;
  onWorkflowFailed?: (payload: AIChatWorkflowEventData, eventId?: string | null) => void;
  onMessageChunk: (payload: AIChatMessageChunkEventData, eventId?: string | null) => void;
  onMessageRetract: (payload: AIChatMessageRetractEventData, eventId?: string | null) => void;
  onMessageEnd: (payload: AIChatMessageEndEventData, eventId?: string | null) => void;
  onTokenUsageUpdated?: (
    payload: AIChatTokenUsageUpdatedEventData,
    eventId?: string | null
  ) => void;
  onTokenBudgetRequested?: (
    payload: AIChatTokenBudgetRequestedEventData,
    eventId?: string | null
  ) => void;
  onErrorEvent: (payload: AIChatErrorEventData, eventId?: string | null) => void;
  onRequestError: (error: Error) => void;
  onClose: () => void;
}

export interface AIChatWorkflowApprovalContinuationPayload {
  approvalToken: string;
  inputs: Record<string, unknown>;
  action: string;
}

export interface AIChatRuntimeTransport {
  readExecutionFile?: ExecutionFileReader;
  readonly supportsRuntimeInput?: boolean;
  listConversations(params: {
    page: number;
    limit: number;
    surface?: AIChatRuntimeSurface;
  }): Promise<AIChatConversationListResult>;
  getConversation(conversationId: string): Promise<AIChatConversationDetail>;
  listMessages(
    conversationId: string,
    params: { page: number; limit: number }
  ): Promise<AIChatMessageListResult>;
  refreshConversation(conversationId: string): Promise<AIChatConversation>;
  updateConversation(
    conversationId: string,
    payload: {
      title?: AIChatConversation['title'];
      status?: AIChatConversation['status'];
      current_leaf_message_id?: string;
    }
  ): Promise<AIChatConversation>;
  removeConversation(conversationId: string): Promise<void>;
  searchConversations?(
    query: string,
    limit: number,
    options?: { surface?: AIChatRuntimeSurface }
  ): Promise<ConversationSearchResult[]>;
  stopConversation(conversationId: string): Promise<AIChatStopConversationResponseData>;
  submitRuntimeInput?(
    conversationId: string,
    messageId: string,
    payload: AIChatSubmitRuntimeInputRequest
  ): Promise<AIChatSubmitRuntimeInputResponseData>;
  streamChat(
    payload: AIChatChatRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ): Promise<{ close: () => void }>;
  regenerateMessage(
    messageId: string,
    payload: AIChatRegenerateMessageRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ): Promise<{ close: () => void }>;
  recoverConversationStream(
    conversationId: string,
    params: { messageId: string; afterId?: string },
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ): Promise<{ close: () => void }>;
  continueWorkflowApproval?(
    conversationId: string,
    messageId: string,
    payload: AIChatWorkflowApprovalContinuationPayload | undefined,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ): Promise<{ close: () => void }>;
  continueWorkflowQuestion?(
    conversationId: string,
    messageId: string,
    payload: { inputs: AIChatWorkflowQuestionAnswerInputs },
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ): Promise<{ close: () => void }>;
  continueToolGovernanceDecision?(
    conversationId: string,
    messageId: string,
    correlationId: string,
    payload: AIChatToolGovernanceDecisionRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ): Promise<{ close: () => void }>;
  continueClientAction?(
    conversationId: string,
    messageId: string,
    actionId: string,
    payload: AIChatClientActionResultRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ): Promise<{ close: () => void }>;
  continueUserInput?(
    conversationId: string,
    messageId: string,
    requestId: string,
    payload: AIChatUserInputContinuationRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ): Promise<{ close: () => void }>;
  /** Resumes a turn whose server process stopped in the middle of it. */
  resumeInterruptedTurn?(
    conversationId: string,
    messageId: string,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ): Promise<{ close: () => void }>;
}

export function mapAIChatSearchResult(item: AIChatSearchResult): ConversationSearchResult {
  return {
    type: item.type,
    conversationId: item.conversation_id,
    conversationTitle: item.conversation_title,
    messageId: item.message_id,
    snippet: item.snippet,
    updatedAt: item.updated_at * 1000,
  };
}

export function dispatchAIChatStreamEvent(
  event: string,
  data: unknown,
  eventId: string | null | undefined,
  callbacks: AIChatStreamCallbacks
): void {
  // One check per event, before any handler (ADR 0050): a payload that is not
  // an object, or a message event without its ids, is dropped here.
  const decoded = decodeStreamEventPayload(event, data);
  if (!decoded) return;
  // Each case reads the decoded payload as its event's shape.
  const payload: unknown = decoded;
  switch (event) {
    case 'message_start':
      callbacks.onMessageStart(payload as AIChatMessageStartEventData, eventId);
      break;
    case 'agent_progress':
      callbacks.onAgentProgress(payload as AIChatAgentProgressEventData, eventId);
      break;
    case 'plan_updated':
      callbacks.onPlanUpdated?.(payload as AIChatPlanUpdatedEventData, eventId);
      break;
    case 'agent_intermediate_answer':
      callbacks.onIntermediateAnswer(payload as AIChatIntermediateAnswerEventData, eventId);
      break;
    case 'user_input_requested':
      callbacks.onUserInputRequested(payload as AIChatUserInputRequestedEventData, eventId);
      break;
    case 'file_parse_start':
      callbacks.onFileParseStart(payload as AIChatFileParseStartEventData, eventId);
      break;
    case 'file_parse_end':
      callbacks.onFileParseEnd(payload as AIChatFileParseEndEventData, eventId);
      break;
    case 'file_parse_error':
      callbacks.onFileParseError(payload as AIChatFileParseErrorEventData, eventId);
      break;
    case 'skill_load_start':
      callbacks.onSkillLoadStart(payload as AIChatSkillLoadStartEventData, eventId);
      break;
    case 'skill_load_end':
      callbacks.onSkillLoadEnd(payload as AIChatSkillLoadEndEventData, eventId);
      break;
    case 'skill_reference_read':
      callbacks.onSkillReferenceRead(payload as AIChatSkillReferenceReadEventData, eventId);
      break;
    case 'skill_call_start':
      callbacks.onSkillCallStart(payload as AIChatSkillCallStartEventData, eventId);
      break;
    case 'skill_call_end':
      callbacks.onSkillCallEnd(payload as AIChatSkillCallEndEventData, eventId);
      break;
    case 'skill_call_error':
      callbacks.onSkillCallError(payload as AIChatSkillCallErrorEventData, eventId);
      break;
    case 'skill_artifact_created':
      callbacks.onSkillArtifactCreated(
        payload as AIChatSkillArtifactCreatedEventData,
        eventId
      );
      break;
    case 'tool_governance_decision':
      callbacks.onToolGovernanceDecision?.(
        payload as AIChatToolGovernanceDecisionEventData,
        eventId
      );
      break;
    case 'client_action_required':
      callbacks.onClientActionRequired?.(
        payload as AIChatClientActionRequiredEventData,
        eventId
      );
      break;
    case 'client_action_result':
      callbacks.onClientActionResult?.(payload as AIChatClientActionResultEventData, eventId);
      break;
    case 'connector_confirmation_required':
    case 'connector_confirmation_resolved': {
      const envelope = decoded;
      const nestedResult =
        envelope.result && typeof envelope.result === 'object' && !Array.isArray(envelope.result)
          ? (envelope.result as Record<string, unknown>)
          : envelope.execution_result &&
              typeof envelope.execution_result === 'object' &&
              !Array.isArray(envelope.execution_result)
            ? (envelope.execution_result as Record<string, unknown>)
            : null;
      const result = nestedResult ?? {
        state: envelope.state,
        ...(envelope.decision_id !== undefined ? { decision_id: envelope.decision_id } : {}),
        ...(envelope.error_code !== undefined ? { error_code: envelope.error_code } : {}),
        ...(envelope.confirmation !== undefined ? { confirmation: envelope.confirmation } : {}),
      };
      callbacks.onAgentProgress(
        {
          conversation_id: String(envelope.conversation_id),
          message_id: String(envelope.message_id),
          phase: event,
          ...(typeof envelope.route_receipt === 'string' && envelope.route_receipt
            ? { progress_id: envelope.route_receipt }
            : {}),
          status:
            event === 'connector_confirmation_required'
              ? 'waiting_approval'
              : String(result.state ?? 'unknown'),
          result,
          created_at: typeof envelope.created_at === 'number' ? envelope.created_at : undefined,
        },
        eventId
      );
      break;
    }
    case 'memory_create':
    case 'memory_update':
    case 'memory_delete':
    case 'memory_clear':
      callbacks.onMemoryMutation(payload as AIChatMemoryMutationEventData, eventId);
      break;
    case 'runtime_input_applied':
      callbacks.onRuntimeInputApplied?.(
        payload as AIChatRuntimeInputAppliedEventData,
        eventId
      );
      break;
    case 'workflow_started':
      callbacks.onWorkflowStarted?.(payload as AIChatWorkflowEventData, eventId);
      break;
    case 'workflow_resumed':
      callbacks.onWorkflowStarted?.(
        { ...(payload as AIChatWorkflowEventData), workflow_event: event },
        eventId
      );
      break;
    case 'node_started':
      callbacks.onWorkflowNodeStarted?.(
        { ...(payload as AIChatWorkflowNodeEventData), workflow_event: event },
        eventId
      );
      break;
    case 'node_finished':
      callbacks.onWorkflowNodeFinished?.(
        { ...(payload as AIChatWorkflowNodeEventData), workflow_event: event },
        eventId
      );
      break;
    case 'iteration_started':
    case 'iteration_next':
    case 'loop_started':
    case 'loop_next':
      callbacks.onWorkflowNodeStarted?.(
        { ...(payload as AIChatWorkflowNodeEventData), workflow_event: event },
        eventId
      );
      break;
    case 'iteration_completed':
    case 'iteration_succeeded':
    case 'iteration_failed':
    case 'loop_completed':
    case 'loop_succeeded':
    case 'loop_failed':
      callbacks.onWorkflowNodeFinished?.(
        { ...(payload as AIChatWorkflowNodeEventData), workflow_event: event },
        eventId
      );
      break;
    case 'workflow_paused':
      callbacks.onWorkflowPaused?.(payload as AIChatWorkflowPausedEventData, eventId);
      break;
    case 'question_answer_requested':
      callbacks.onWorkflowPaused?.(
        { ...(payload as AIChatWorkflowPausedEventData), status: 'pending_question' },
        eventId
      );
      break;
    case 'approval_requested':
      callbacks.onWorkflowApprovalRequested?.(
        payload as AIChatWorkflowPausedEventData,
        eventId
      );
      break;
    case 'approval_result_filled':
      callbacks.onWorkflowNodeFinished?.(
        {
          ...(payload as AIChatWorkflowNodeEventData),
          workflow_event: event,
          status: 'success',
          node_type: 'approval',
          title: 'Approval submitted',
          outputs: payload,
        },
        eventId
      );
      break;
    case 'approval_expired':
      // One event, one transition: the run and its approval node expire in
      // the same update, so the event's sequence is taken once.
      callbacks.onWorkflowPaused?.(
        {
          ...(payload as AIChatWorkflowPausedEventData),
          workflow_event: event,
          status: 'expired',
        },
        eventId
      );
      break;
    case 'question_answer_submitted':
      callbacks.onWorkflowNodeFinished?.(
        {
          ...(payload as AIChatWorkflowNodeEventData),
          workflow_event: event,
          status: 'running',
          node_type: 'question-answer',
        },
        eventId
      );
      break;
    case 'workflow_finished':
      callbacks.onWorkflowFinished?.(payload as AIChatWorkflowEventData, eventId);
      break;
    case 'workflow_failed':
      callbacks.onWorkflowFailed?.(payload as AIChatWorkflowEventData, eventId);
      break;
    case 'workflow_stopped':
      callbacks.onWorkflowFinished?.(
        { ...(payload as AIChatWorkflowEventData), status: 'stopped' },
        eventId
      );
      break;
    case 'message':
      callbacks.onMessageChunk(payload as AIChatMessageChunkEventData, eventId);
      break;
    case 'message_retract':
      callbacks.onMessageRetract(payload as AIChatMessageRetractEventData, eventId);
      break;
    case 'message_end':
      callbacks.onMessageEnd(payload as AIChatMessageEndEventData, eventId);
      break;
    case 'token_usage_updated':
      callbacks.onTokenUsageUpdated?.(
        payload as AIChatTokenUsageUpdatedEventData,
        eventId
      );
      break;
    case 'token_budget_requested':
      callbacks.onTokenBudgetRequested?.(
        payload as AIChatTokenBudgetRequestedEventData,
        eventId
      );
      break;
    case 'error':
      callbacks.onErrorEvent(payload as AIChatErrorEventData, eventId);
      break;
    default:
      break;
  }
}

export class AIChatTransport implements AIChatRuntimeTransport {
  async listConversations(params: {
    page: number;
    limit: number;
    surface?: AIChatRuntimeSurface;
  }): Promise<AIChatConversationListResult> {
    const response = await aichatService.listConversations({
      ...params,
      conversation_type: 'chat',
    });

    return {
      items: response.data.data,
      pagination: {
        page: response.data.page,
        limit: response.data.limit,
        total: response.data.total,
        hasMore: response.data.has_more,
      },
    };
  }

  async getConversation(conversationId: string): Promise<AIChatConversationDetail> {
    const [conversationResponse, messageList] = await Promise.all([
      aichatService.getConversation(conversationId, 'chat'),
      this.listMessages(conversationId, {
        page: 1,
        limit: DEFAULT_AICHAT_MESSAGE_PAGINATION.limit,
      }),
    ]);

    return {
      conversation: conversationResponse.data,
      messages: messageList.items,
      messagePagination: messageList.pagination,
    };
  }

  async listMessages(
    conversationId: string,
    params: { page: number; limit: number }
  ): Promise<AIChatMessageListResult> {
    const response = await aichatService.listMessages(conversationId, params);

    return {
      items: response.data.data
        .slice()
        .map(sanitizeAIChatMessage)
        .sort((a, b) => a.created_at - b.created_at || a.id.localeCompare(b.id)),
      pagination: {
        page: response.data.page,
        limit: response.data.limit,
        total: response.data.total,
        hasMore: response.data.has_more,
      },
    };
  }

  async refreshConversation(conversationId: string): Promise<AIChatConversation> {
    const response = await aichatService.getConversation(conversationId, 'chat');
    return response.data;
  }

  async updateConversation(
    conversationId: string,
    payload: {
      title?: string;
      status?: AIChatConversation['status'];
      current_leaf_message_id?: string;
    }
  ): Promise<AIChatConversation> {
    const response = await aichatService.updateConversation(conversationId, payload);
    return response.data;
  }

  async removeConversation(conversationId: string): Promise<void> {
    await aichatService.deleteConversation(conversationId);
  }

  async searchConversations(
    query: string,
    limit: number,
    options?: { surface?: AIChatRuntimeSurface }
  ): Promise<ConversationSearchResult[]> {
    const response = await aichatService.search(query, limit, {
      surface: options?.surface,
      conversation_type: 'chat',
    });
    return (response.data ?? []).map(mapAIChatSearchResult);
  }

  async stopConversation(conversationId: string): Promise<AIChatStopConversationResponseData> {
    const response = await aichatService.stopConversation(conversationId);
    return response.data;
  }

  streamChat(
    payload: AIChatChatRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    return aichatService.streamChat(
      payload,
      {
        onEvent: (event, data, eventId) => {
          dispatchAIChatStreamEvent(event, data, eventId, callbacks);
        },
        onOpen: () => callbacks.onOpen?.(),
        onError: callbacks.onRequestError,
        onClose: callbacks.onClose,
      },
      abortSignal
    );
  }

  regenerateMessage(
    messageId: string,
    payload: AIChatRegenerateMessageRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    return aichatService.regenerateMessage(
      messageId,
      payload,
      {
        onEvent: (event, data, eventId) => {
          dispatchAIChatStreamEvent(event, data, eventId, callbacks);
        },
        onOpen: () => callbacks.onOpen?.(),
        onError: callbacks.onRequestError,
        onClose: callbacks.onClose,
      },
      abortSignal
    );
  }

  recoverConversationStream(
    conversationId: string,
    params: { messageId: string; afterId?: string },
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    return aichatService.recoverConversationStream(
      conversationId,
      {
        message_id: params.messageId,
        after_id: params.afterId,
      },
      {
        onEvent: (event, data, eventId) => {
          dispatchAIChatStreamEvent(event, data, eventId, callbacks);
        },
        onOpen: () => callbacks.onOpen?.(),
        onError: callbacks.onRequestError,
        onClose: callbacks.onClose,
      },
      abortSignal
    );
  }

  continueToolGovernanceDecision(
    conversationId: string,
    messageId: string,
    correlationId: string,
    payload: AIChatToolGovernanceDecisionRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    return aichatService.continueToolGovernanceDecision(
      conversationId,
      messageId,
      correlationId,
      payload,
      {
        onEvent: (event, data, eventId) => {
          dispatchAIChatStreamEvent(event, data, eventId, callbacks);
        },
        onOpen: () => callbacks.onOpen?.(),
        onError: callbacks.onRequestError,
        onClose: callbacks.onClose,
      },
      abortSignal
    );
  }

  continueClientAction(
    conversationId: string,
    messageId: string,
    actionId: string,
    payload: AIChatClientActionResultRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    return aichatService.continueClientAction(
      conversationId,
      messageId,
      actionId,
      payload,
      {
        onEvent: (event, data, eventId) => {
          dispatchAIChatStreamEvent(event, data, eventId, callbacks);
        },
        onOpen: () => callbacks.onOpen?.(),
        onError: callbacks.onRequestError,
        onClose: callbacks.onClose,
      },
      abortSignal
    );
  }

  continueUserInput(
    conversationId: string,
    messageId: string,
    requestId: string,
    payload: AIChatUserInputContinuationRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    return aichatService.continueUserInput(
      conversationId,
      messageId,
      requestId,
      payload,
      {
        onEvent: (event, data, eventId) => {
          dispatchAIChatStreamEvent(event, data, eventId, callbacks);
        },
        onOpen: () => callbacks.onOpen?.(),
        onError: callbacks.onRequestError,
        onClose: callbacks.onClose,
      },
      abortSignal
    );
  }

  resumeInterruptedTurn(
    conversationId: string,
    messageId: string,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    return aichatService.resumeInterruptedTurn(
      conversationId,
      messageId,
      {
        onEvent: (event, data, eventId) => {
          dispatchAIChatStreamEvent(event, data, eventId, callbacks);
        },
        onOpen: () => callbacks.onOpen?.(),
        onError: callbacks.onRequestError,
        onClose: callbacks.onClose,
      },
      abortSignal
    );
  }
}

export const aichatTransport = new AIChatTransport();
