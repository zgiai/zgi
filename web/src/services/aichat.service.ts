import { http, SSE_IDLE_TIMEOUT_MS } from '@/lib/http';
import type { SseMessage } from '@/lib/http';
import {
  SENSITIVE_OUTPUT_BLOCKED_FLAG,
  SENSITIVE_OUTPUT_BLOCKED_TOKEN,
} from '@/utils/model-output-filter';
import {
  createSensitiveWordStreamSession,
  isSensitiveWordFilterEnabled,
} from '@/utils/sensitive-word-filter';
import type {
  AgentBindingMutationConfirmation,
  AgentResourceBoundImpact,
  ApiResponseData,
  SuccessResponse,
} from './types/common';
import type {
  AIChatAssetOperationAuditListResponse,
  AIChatChatRequest,
  AIChatCancelImportSkillPreviewResponse,
  AIChatClientActionResultRequest,
  AIChatConversation,
  AIChatConversationType,
  AIChatConversationListResponse,
  AIChatConfirmImportSkillRequest,
  AIChatCreateConversationRequest,
  AIChatDeleteSkillResponse,
  AIChatImportSkillPreviewResponse,
  AIChatMessage,
  AIChatMessageListResponse,
  AIChatRegenerateMessageRequest,
  AIChatRuntimeSurface,
  AIChatUserInputContinuationRequest,
  AIChatSearchResponse,
  AIChatSkillConfigResponse,
  AIChatSkillConfigUpdateResponse,
  AIChatSkillDetailResponse,
  AIChatSkillListResponse,
  AIChatSkillOrganizationConfig,
  AIChatSkillPreference,
  AIChatSkillPreferenceResponse,
  AIChatSseEnvelope,
  AIChatStopConversationResponseData,
  AIChatToolGovernanceDecisionRequest,
  AIChatToolGovernanceDecisionResponse,
  AIChatUpdateConversationRequest,
} from './types/aichat';

export interface AIChatStreamCallbacks {
  onOpen?: () => void;
  onEvent: (event: string, data: unknown, eventId?: string | null) => void;
  onError?: (error: Error) => void;
  onClose?: () => void;
}

const AICHAT_BASE_PATH = '/console/api/aichat';

function dispatchAIChatSseMessage(
  message: SseMessage<AIChatSseEnvelope | string>,
  callbacks: AIChatStreamCallbacks,
  filter?: AIChatStreamOutputFilter
): void {
  const envelope =
    typeof message.data === 'string'
      ? (safeJsonParse(message.data) as AIChatSseEnvelope)
      : message.data;

  if (!envelope || typeof envelope !== 'object') {
    return;
  }

  const event = typeof envelope.event === 'string' ? envelope.event : message.event || '';
  if (!event) return;
  if (filter) {
    filter.dispatch(event, envelope.data, message.id);
    return;
  }
  callbacks.onEvent(event, envelope.data, message.id);
}

function safeJsonParse(value: string): unknown {
  try {
    return JSON.parse(value);
  } catch {
    return {};
  }
}

function isAIChatTerminalMessage(message: SseMessage<unknown>): boolean {
  const envelope =
    typeof message.data === 'string'
      ? (safeJsonParse(message.data) as AIChatSseEnvelope)
      : message.data;
  const record =
    envelope && typeof envelope === 'object' ? (envelope as Record<string, unknown>) : {};
  const event =
    (typeof record.event === 'string' && record.event) ||
    (typeof message.event === 'string' ? message.event : '');

  return event === 'message_end' || event === 'done' || event === 'error';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function getAIChatChunkText(data: unknown): string {
  if (!isRecord(data)) {
    return '';
  }
  return typeof data.answer === 'string' ? data.answer : '';
}

function getStringField(data: unknown, key: string): string | undefined {
  if (!isRecord(data)) {
    return undefined;
  }
  return typeof data[key] === 'string' ? data[key] : undefined;
}

interface AIChatStreamOutputFilter {
  dispatch: (event: string, data: unknown, eventId?: string | null) => void;
}

function createAIChatStreamOutputFilter(
  callbacks: AIChatStreamCallbacks
): AIChatStreamOutputFilter | undefined {
  if (!isSensitiveWordFilterEnabled()) {
    return undefined;
  }

  let session = createSensitiveWordStreamSession({ chunkSize: 50, lookbehindSize: 50 });
  let blocked = false;
  let activeConversationId: string | undefined;
  let activeMessageId: string | undefined;

  const dispatchBlockedChunk = (data: unknown, eventId?: string | null): void => {
    blocked = true;
    callbacks.onEvent(
      'message',
      {
        ...(isRecord(data) ? data : {}),
        conversation_id: getStringField(data, 'conversation_id') ?? activeConversationId,
        message_id: getStringField(data, 'message_id') ?? activeMessageId,
        answer: SENSITIVE_OUTPUT_BLOCKED_TOKEN,
        [SENSITIVE_OUTPUT_BLOCKED_FLAG]: true,
      },
      eventId
    );
  };

  return {
    dispatch(event, data, eventId) {
      if (event === 'message_start') {
        blocked = false;
        session = createSensitiveWordStreamSession({ chunkSize: 50, lookbehindSize: 50 });
        activeConversationId = getStringField(data, 'conversation_id');
        activeMessageId = getStringField(data, 'message_id');
        callbacks.onEvent(event, data, eventId);
        return;
      }

      if (event === 'message') {
        if (blocked) {
          return;
        }

        const text = getAIChatChunkText(data);
        if (text && session.append(text).matched) {
          dispatchBlockedChunk(data, eventId);
          return;
        }

        callbacks.onEvent(event, data, eventId);
        return;
      }

      if (event === 'message_end') {
        if (!blocked && session.finish().matched) {
          dispatchBlockedChunk(data, eventId);
        }
        callbacks.onEvent(event, data, eventId);
        return;
      }

      callbacks.onEvent(event, data, eventId);
    },
  };
}

/**
 * AIChat service for the standalone console chat module.
 */
export const aichatService = {
  listSkills() {
    return http.get<AIChatSkillListResponse>(`${AICHAT_BASE_PATH}/skills`);
  },

  getSkill(id: string) {
    return http.get<AIChatSkillDetailResponse>(
      `${AICHAT_BASE_PATH}/skills/${encodeURIComponent(id)}`
    );
  },

  getSkillConfig() {
    return http.get<AIChatSkillConfigResponse>(`${AICHAT_BASE_PATH}/skills/config`);
  },

  updateSkillConfig(payload: AIChatSkillOrganizationConfig) {
    return http.put<AIChatSkillConfigUpdateResponse>(`${AICHAT_BASE_PATH}/skills/config`, payload);
  },

  getSkillPreference() {
    return http.get<AIChatSkillPreferenceResponse>(`${AICHAT_BASE_PATH}/skill-preferences/me`);
  },

  updateSkillPreference(payload: AIChatSkillPreference) {
    return http.put<AIChatSkillPreferenceResponse>(
      `${AICHAT_BASE_PATH}/skill-preferences/me`,
      payload
    );
  },

  previewImportSkill(file: File) {
    const formData = new FormData();
    formData.append('file', file);
    return http.upload<AIChatImportSkillPreviewResponse>(
      `${AICHAT_BASE_PATH}/skills/import/preview`,
      formData
    );
  },

  confirmImportSkill(payload: AIChatConfirmImportSkillRequest) {
    return http.post<AIChatSkillDetailResponse>(
      `${AICHAT_BASE_PATH}/skills/import/confirm`,
      payload
    );
  },

  cancelImportSkillPreview(importId: string) {
    return http.delete<AIChatCancelImportSkillPreviewResponse>(
      `${AICHAT_BASE_PATH}/skills/import/preview/${encodeURIComponent(importId)}`
    );
  },

  deleteSkill(id: string, confirmation?: AgentBindingMutationConfirmation) {
    return http.delete<AIChatDeleteSkillResponse>(
      `${AICHAT_BASE_PATH}/skills/${encodeURIComponent(id)}`,
      { params: confirmation }
    );
  },

  previewSkillDeleteImpact(id: string) {
    return http.get<ApiResponseData<AgentResourceBoundImpact | null>>(
      `${AICHAT_BASE_PATH}/skills/${encodeURIComponent(id)}/delete-impact`
    );
  },

  listConversations(
    params: {
      page?: number;
      limit?: number;
      surface?: AIChatRuntimeSurface;
      conversation_type?: AIChatConversationType;
    } = {}
  ) {
    return http.get<AIChatConversationListResponse>(`${AICHAT_BASE_PATH}/conversations`, {
      params,
    });
  },

  search(
    query: string,
    limit = 20,
    params: { surface?: AIChatRuntimeSurface; conversation_type?: AIChatConversationType } = {}
  ) {
    return http.get<AIChatSearchResponse>(`${AICHAT_BASE_PATH}/search`, {
      params: { query, limit, ...params },
    });
  },

  createConversation(payload: AIChatCreateConversationRequest) {
    return http.post<ApiResponseData<AIChatConversation>>(
      `${AICHAT_BASE_PATH}/conversations`,
      payload
    );
  },

  getConversation(id: string, conversationType?: AIChatConversationType) {
    return http.get<ApiResponseData<AIChatConversation>>(`${AICHAT_BASE_PATH}/conversations/${id}`, {
      params: { conversation_type: conversationType },
    });
  },

  updateConversation(id: string, payload: AIChatUpdateConversationRequest) {
    return http.patch<ApiResponseData<AIChatConversation>>(
      `${AICHAT_BASE_PATH}/conversations/${id}`,
      payload
    );
  },

  deleteConversation(id: string) {
    return http.delete<ApiResponseData<SuccessResponse>>(`${AICHAT_BASE_PATH}/conversations/${id}`);
  },

  stopConversation(id: string) {
    return http.post<ApiResponseData<AIChatStopConversationResponseData>>(
      `${AICHAT_BASE_PATH}/conversations/${id}/stop`
    );
  },

  listMessages(id: string, params: { page?: number; limit?: number } = {}) {
    return http.get<AIChatMessageListResponse>(`${AICHAT_BASE_PATH}/conversations/${id}/messages`, {
      params,
    });
  },

  listAssetOperationAudits(id: string, params: { page?: number; limit?: number } = {}) {
    return http.get<AIChatAssetOperationAuditListResponse>(
      `${AICHAT_BASE_PATH}/conversations/${encodeURIComponent(id)}/asset-operation-audits`,
      {
        params,
      }
    );
  },

  deleteMessage(id: string) {
    return http.delete<ApiResponseData<SuccessResponse>>(`${AICHAT_BASE_PATH}/messages/${id}`);
  },

  submitToolGovernanceDecision(
    conversationId: string,
    messageId: string,
    correlationId: string,
    payload: AIChatToolGovernanceDecisionRequest
  ) {
    return http.post<ApiResponseData<AIChatToolGovernanceDecisionResponse>>(
      `${AICHAT_BASE_PATH}/conversations/${encodeURIComponent(
        conversationId
      )}/messages/${encodeURIComponent(messageId)}/tool-governance/${encodeURIComponent(
        correlationId
      )}`,
      payload
    );
  },

  continueToolGovernanceDecision(
    conversationId: string,
    messageId: string,
    correlationId: string,
    payload: AIChatToolGovernanceDecisionRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    const outputFilter = createAIChatStreamOutputFilter(callbacks);

    return http.sse<AIChatSseEnvelope, AIChatToolGovernanceDecisionRequest>(
      `${AICHAT_BASE_PATH}/conversations/${encodeURIComponent(
        conversationId
      )}/messages/${encodeURIComponent(messageId)}/tool-governance/${encodeURIComponent(
        correlationId
      )}/continue`,
      {
        method: 'POST',
        body: payload,
        abortSignal,
        idleTimeoutMs: SSE_IDLE_TIMEOUT_MS,
        skipErrorHandling: true,
        isTerminalMessage: isAIChatTerminalMessage,
        onOpen: callbacks.onOpen,
        onMessage: message => dispatchAIChatSseMessage(message, callbacks, outputFilter),
        onError: error => callbacks.onError?.(error),
        onClose: callbacks.onClose,
      }
    );
  },

  continueClientAction(
    conversationId: string,
    messageId: string,
    actionId: string,
    payload: AIChatClientActionResultRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    const outputFilter = createAIChatStreamOutputFilter(callbacks);

    return http.sse<AIChatSseEnvelope, AIChatClientActionResultRequest>(
      `${AICHAT_BASE_PATH}/conversations/${encodeURIComponent(
        conversationId
      )}/messages/${encodeURIComponent(messageId)}/client-actions/${encodeURIComponent(
        actionId
      )}/continue`,
      {
        method: 'POST',
        body: payload,
        abortSignal,
        idleTimeoutMs: SSE_IDLE_TIMEOUT_MS,
        skipErrorHandling: true,
        isTerminalMessage: isAIChatTerminalMessage,
        onOpen: callbacks.onOpen,
        onMessage: message => dispatchAIChatSseMessage(message, callbacks, outputFilter),
        onError: error => callbacks.onError?.(error),
        onClose: callbacks.onClose,
      }
    );
  },

  continueUserInput(
    conversationId: string,
    messageId: string,
    requestId: string,
    payload: AIChatUserInputContinuationRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    const outputFilter = createAIChatStreamOutputFilter(callbacks);

    return http.sse<AIChatSseEnvelope, AIChatUserInputContinuationRequest>(
      `${AICHAT_BASE_PATH}/conversations/${encodeURIComponent(
        conversationId
      )}/messages/${encodeURIComponent(messageId)}/user-input/${encodeURIComponent(
        requestId
      )}/continue`,
      {
        method: 'POST',
        body: payload,
        abortSignal,
        idleTimeoutMs: SSE_IDLE_TIMEOUT_MS,
        skipErrorHandling: true,
        isTerminalMessage: isAIChatTerminalMessage,
        onOpen: callbacks.onOpen,
        onMessage: message => dispatchAIChatSseMessage(message, callbacks, outputFilter),
        onError: error => callbacks.onError?.(error),
        onClose: callbacks.onClose,
      }
    );
  },

  streamChat(
    payload: AIChatChatRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    const outputFilter = createAIChatStreamOutputFilter(callbacks);
    const surface = payload.surface ?? 'work_chat';
    const endpoint =
      surface === 'contextual_sidebar'
        ? `${AICHAT_BASE_PATH}/contextual/chat`
        : surface === 'work_chat'
          ? `${AICHAT_BASE_PATH}/work-chat/chat`
          : `${AICHAT_BASE_PATH}/chat`;
    const body = { ...payload };
    if (surface === 'contextual_sidebar' || surface === 'work_chat') {
      delete body.surface;
    }

    return http.sse<AIChatSseEnvelope, typeof body>(endpoint, {
      method: 'POST',
      body,
      abortSignal,
      idleTimeoutMs: SSE_IDLE_TIMEOUT_MS,
      skipErrorHandling: true,
      isTerminalMessage: isAIChatTerminalMessage,
      onOpen: callbacks.onOpen,
      onMessage: message => dispatchAIChatSseMessage(message, callbacks, outputFilter),
      onError: error => callbacks.onError?.(error),
      onClose: callbacks.onClose,
    });
  },

  regenerateMessage(
    messageId: string,
    payload: AIChatRegenerateMessageRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    const outputFilter = createAIChatStreamOutputFilter(callbacks);

    return http.sse<AIChatSseEnvelope, AIChatRegenerateMessageRequest>(
      `${AICHAT_BASE_PATH}/messages/${messageId}/regenerate`,
      {
        method: 'POST',
        body: payload,
        abortSignal,
        idleTimeoutMs: SSE_IDLE_TIMEOUT_MS,
        skipErrorHandling: true,
        isTerminalMessage: isAIChatTerminalMessage,
        onOpen: callbacks.onOpen,
        onMessage: message => dispatchAIChatSseMessage(message, callbacks, outputFilter),
        onError: error => callbacks.onError?.(error),
        onClose: callbacks.onClose,
      }
    );
  },

  recoverConversationStream(
    conversationId: string,
    params: { message_id: string; after_id?: string },
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    const outputFilter = createAIChatStreamOutputFilter(callbacks);

    return http.sse<AIChatSseEnvelope, never>(
      `${AICHAT_BASE_PATH}/conversations/${conversationId}/events`,
      {
        method: 'GET',
        query: params,
        abortSignal,
        idleTimeoutMs: SSE_IDLE_TIMEOUT_MS,
        skipErrorHandling: true,
        isTerminalMessage: isAIChatTerminalMessage,
        onOpen: callbacks.onOpen,
        onMessage: message => dispatchAIChatSseMessage(message, callbacks, outputFilter),
        onError: error => callbacks.onError?.(error),
        onClose: callbacks.onClose,
      }
    );
  },
};

export type { AIChatMessage };
