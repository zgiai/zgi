import { http } from '@/lib/http';
import type { SseMessage } from '@/lib/http';
import {
  SENSITIVE_OUTPUT_BLOCKED_FLAG,
  SENSITIVE_OUTPUT_BLOCKED_TOKEN,
} from '@/utils/model-output-filter';
import {
  createSensitiveWordStreamSession,
  isSensitiveWordFilterEnabled,
} from '@/utils/sensitive-word-filter';
import type { ApiResponseData, SuccessResponse } from './types/common';
import type {
  AIChatChatRequest,
  AIChatConversation,
  AIChatConversationListResponse,
  AIChatCreateConversationRequest,
  AIChatDeleteSkillResponse,
  AIChatImportSkillResponse,
  AIChatMessage,
  AIChatMessageListResponse,
  AIChatRegenerateMessageRequest,
  AIChatSkillConfigResponse,
  AIChatSkillDetailResponse,
  AIChatSkillListResponse,
  AIChatSkillOrganizationConfig,
  AIChatSseEnvelope,
  AIChatStopConversationResponseData,
  AIChatUpdateConversationRequest,
} from './types/aichat';

export interface AIChatStreamCallbacks {
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
  const record = envelope && typeof envelope === 'object' ? (envelope as Record<string, unknown>) : {};
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
    return http.put<AIChatSkillConfigResponse>(`${AICHAT_BASE_PATH}/skills/config`, payload);
  },

  importSkill(file: File) {
    const formData = new FormData();
    formData.append('file', file);
    return http.upload<AIChatImportSkillResponse>(`${AICHAT_BASE_PATH}/skills/import`, formData);
  },

  deleteSkill(id: string) {
    return http.delete<AIChatDeleteSkillResponse>(
      `${AICHAT_BASE_PATH}/skills/${encodeURIComponent(id)}`
    );
  },

  listConversations(params: { page?: number; limit?: number } = {}) {
    return http.get<AIChatConversationListResponse>(`${AICHAT_BASE_PATH}/conversations`, {
      params,
    });
  },

  createConversation(payload: AIChatCreateConversationRequest) {
    return http.post<ApiResponseData<AIChatConversation>>(
      `${AICHAT_BASE_PATH}/conversations`,
      payload
    );
  },

  getConversation(id: string) {
    return http.get<ApiResponseData<AIChatConversation>>(`${AICHAT_BASE_PATH}/conversations/${id}`);
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

  deleteMessage(id: string) {
    return http.delete<ApiResponseData<SuccessResponse>>(`${AICHAT_BASE_PATH}/messages/${id}`);
  },

  streamChat(
    payload: AIChatChatRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    const outputFilter = createAIChatStreamOutputFilter(callbacks);

    return http.sse<AIChatSseEnvelope, AIChatChatRequest>(`${AICHAT_BASE_PATH}/chat`, {
      method: 'POST',
      body: payload,
      abortSignal,
      isTerminalMessage: isAIChatTerminalMessage,
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
        isTerminalMessage: isAIChatTerminalMessage,
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
        isTerminalMessage: isAIChatTerminalMessage,
        onMessage: message => dispatchAIChatSseMessage(message, callbacks, outputFilter),
        onError: error => callbacks.onError?.(error),
        onClose: callbacks.onClose,
      }
    );
  },
};

export type { AIChatMessage };
