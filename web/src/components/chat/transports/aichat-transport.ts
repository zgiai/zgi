import { aichatService } from '@/services/aichat.service';
import {
  isSensitiveOutputBlockedValue,
  sanitizeModelOutputValue,
} from '@/utils/model-output-filter';
import type {
  AIChatChatRequest,
  AIChatAgentProgressEventData,
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
  AIChatMemoryMutationEventData,
  AIChatIntermediateAnswerEventData,
  AIChatUserInputRequestedEventData,
  AIChatRegenerateMessageRequest,
  AIChatSkillCallEndEventData,
  AIChatSkillCallErrorEventData,
  AIChatSkillCallStartEventData,
  AIChatSkillArtifactCreatedEventData,
  AIChatSkillLoadEndEventData,
  AIChatSkillLoadStartEventData,
  AIChatSkillReferenceReadEventData,
  AIChatStopConversationResponseData,
  AIChatToolGovernanceDecisionRequest,
  AIChatToolGovernanceDecisionEventData,
  AIChatWorkflowEventData,
  AIChatWorkflowNodeEventData,
  AIChatWorkflowPausedEventData,
} from '@/services/types/aichat';
import {
  DEFAULT_AICHAT_MESSAGE_PAGINATION,
  type AIChatPagination,
} from '@/components/chat/controllers/aichat';

function sanitizeAIChatMessage(message: AIChatMessage): AIChatMessage {
  const sanitizedAnswer = sanitizeModelOutputValue(message.answer);
  if (sanitizedAnswer === message.answer) {
    return message;
  }

  const isSensitiveOutputBlocked = isSensitiveOutputBlockedValue(sanitizedAnswer);
  return {
    ...message,
    answer: typeof sanitizedAnswer === 'string' ? sanitizedAnswer : message.answer,
    metadata: isSensitiveOutputBlocked
      ? {
          ...message.metadata,
          sensitiveOutputBlocked: true,
        }
      : message.metadata,
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

export interface AIChatStreamCallbacks {
  onMessageStart: (payload: AIChatMessageStartEventData, eventId?: string | null) => void;
  onAgentProgress: (payload: AIChatAgentProgressEventData, eventId?: string | null) => void;
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
  onMemoryMutation: (payload: AIChatMemoryMutationEventData, eventId?: string | null) => void;
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
  listConversations(params: { page: number; limit: number }): Promise<AIChatConversationListResult>;
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
  stopConversation(conversationId: string): Promise<AIChatStopConversationResponseData>;
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
    payload: { inputs: { query: string; question_answer_option_id?: string } },
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
}

export function dispatchAIChatStreamEvent(
  event: string,
  data: unknown,
  eventId: string | null | undefined,
  callbacks: AIChatStreamCallbacks
): void {
  switch (event) {
    case 'message_start':
      callbacks.onMessageStart((data ?? {}) as AIChatMessageStartEventData, eventId);
      break;
    case 'agent_progress':
      callbacks.onAgentProgress((data ?? {}) as AIChatAgentProgressEventData, eventId);
      break;
    case 'agent_intermediate_answer':
      callbacks.onIntermediateAnswer((data ?? {}) as AIChatIntermediateAnswerEventData, eventId);
      break;
    case 'user_input_requested':
      callbacks.onUserInputRequested((data ?? {}) as AIChatUserInputRequestedEventData, eventId);
      break;
    case 'file_parse_start':
      callbacks.onFileParseStart((data ?? {}) as AIChatFileParseStartEventData, eventId);
      break;
    case 'file_parse_end':
      callbacks.onFileParseEnd((data ?? {}) as AIChatFileParseEndEventData, eventId);
      break;
    case 'file_parse_error':
      callbacks.onFileParseError((data ?? {}) as AIChatFileParseErrorEventData, eventId);
      break;
    case 'skill_load_start':
      callbacks.onSkillLoadStart((data ?? {}) as AIChatSkillLoadStartEventData, eventId);
      break;
    case 'skill_load_end':
      callbacks.onSkillLoadEnd((data ?? {}) as AIChatSkillLoadEndEventData, eventId);
      break;
    case 'skill_reference_read':
      callbacks.onSkillReferenceRead((data ?? {}) as AIChatSkillReferenceReadEventData, eventId);
      break;
    case 'skill_call_start':
      callbacks.onSkillCallStart((data ?? {}) as AIChatSkillCallStartEventData, eventId);
      break;
    case 'skill_call_end':
      callbacks.onSkillCallEnd((data ?? {}) as AIChatSkillCallEndEventData, eventId);
      break;
    case 'skill_call_error':
      callbacks.onSkillCallError((data ?? {}) as AIChatSkillCallErrorEventData, eventId);
      break;
    case 'skill_artifact_created':
      callbacks.onSkillArtifactCreated(
        (data ?? {}) as AIChatSkillArtifactCreatedEventData,
        eventId
      );
      break;
    case 'tool_governance_decision':
      callbacks.onToolGovernanceDecision?.(
        (data ?? {}) as AIChatToolGovernanceDecisionEventData,
        eventId
      );
      break;
    case 'memory_create':
    case 'memory_update':
    case 'memory_delete':
    case 'memory_clear':
      callbacks.onMemoryMutation((data ?? {}) as AIChatMemoryMutationEventData, eventId);
      break;
    case 'workflow_started':
      callbacks.onWorkflowStarted?.((data ?? {}) as AIChatWorkflowEventData, eventId);
      break;
    case 'node_started':
      callbacks.onWorkflowNodeStarted?.(
        { ...((data ?? {}) as AIChatWorkflowNodeEventData), workflow_event: event },
        eventId
      );
      break;
    case 'node_finished':
      callbacks.onWorkflowNodeFinished?.(
        { ...((data ?? {}) as AIChatWorkflowNodeEventData), workflow_event: event },
        eventId
      );
      break;
    case 'iteration_started':
    case 'iteration_next':
    case 'loop_started':
    case 'loop_next':
      callbacks.onWorkflowNodeStarted?.(
        { ...((data ?? {}) as AIChatWorkflowNodeEventData), workflow_event: event },
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
        { ...((data ?? {}) as AIChatWorkflowNodeEventData), workflow_event: event },
        eventId
      );
      break;
    case 'workflow_paused':
      callbacks.onWorkflowPaused?.((data ?? {}) as AIChatWorkflowPausedEventData, eventId);
      break;
    case 'question_answer_requested':
      callbacks.onWorkflowPaused?.(
        { ...((data ?? {}) as AIChatWorkflowPausedEventData), status: 'pending_question' },
        eventId
      );
      break;
    case 'approval_requested':
      callbacks.onWorkflowApprovalRequested?.(
        (data ?? {}) as AIChatWorkflowPausedEventData,
        eventId
      );
      break;
    case 'approval_result_filled':
      callbacks.onWorkflowNodeFinished?.(
        {
          ...((data ?? {}) as AIChatWorkflowNodeEventData),
          status: 'success',
          node_type: 'approval',
          title: 'Approval submitted',
          outputs: data,
        },
        eventId
      );
      break;
    case 'approval_expired':
      callbacks.onWorkflowPaused?.(
        { ...((data ?? {}) as AIChatWorkflowPausedEventData), status: 'expired' },
        eventId
      );
      callbacks.onWorkflowNodeFinished?.(
        {
          ...((data ?? {}) as AIChatWorkflowNodeEventData),
          status: 'failed',
          node_type: 'approval',
          title: 'Approval expired',
          outputs: data,
        },
        eventId
      );
      break;
    case 'question_answer_submitted':
      callbacks.onWorkflowNodeFinished?.(
        {
          ...((data ?? {}) as AIChatWorkflowNodeEventData),
          status: 'running',
          node_type: 'question-answer',
        },
        eventId
      );
      break;
    case 'workflow_finished':
      callbacks.onWorkflowFinished?.((data ?? {}) as AIChatWorkflowEventData, eventId);
      break;
    case 'workflow_failed':
      callbacks.onWorkflowFailed?.((data ?? {}) as AIChatWorkflowEventData, eventId);
      break;
    case 'workflow_stopped':
      callbacks.onWorkflowFinished?.(
        { ...((data ?? {}) as AIChatWorkflowEventData), status: 'stopped' },
        eventId
      );
      break;
    case 'message':
      callbacks.onMessageChunk((data ?? {}) as AIChatMessageChunkEventData, eventId);
      break;
    case 'message_retract':
      callbacks.onMessageRetract((data ?? {}) as AIChatMessageRetractEventData, eventId);
      break;
    case 'message_end':
      callbacks.onMessageEnd((data ?? {}) as AIChatMessageEndEventData, eventId);
      break;
    case 'error':
      callbacks.onErrorEvent((data ?? {}) as AIChatErrorEventData, eventId);
      break;
    default:
      break;
  }
}

export class AIChatTransport implements AIChatRuntimeTransport {
  async listConversations(params: {
    page: number;
    limit: number;
  }): Promise<AIChatConversationListResult> {
    const response = await aichatService.listConversations(params);

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
      aichatService.getConversation(conversationId),
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
    const response = await aichatService.getConversation(conversationId);
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
        onError: callbacks.onRequestError,
        onClose: callbacks.onClose,
      },
      abortSignal
    );
  }
}

export const aichatTransport = new AIChatTransport();
