import { http, webappHttp, type ExtendedRequestConfig, type SseOptions } from '@/lib/http';
import type { ApiResponseData } from '@/services/types/common';
import type {
  AIChatChatRequest,
  AIChatConversation,
  AIChatConversationListResponse,
  AIChatMessage,
  AIChatMessageListResponse,
  AIChatRegenerateMessageRequest,
  AIChatSseEnvelope,
  AIChatStopConversationResponseData,
} from '@/services/types/aichat';
import {
  DEFAULT_AICHAT_MESSAGE_PAGINATION,
  type AIChatPagination,
} from '@/components/chat/controllers/aichat';
import {
  dispatchAIChatStreamEvent,
  type AIChatConversationDetail,
  type AIChatConversationListResult,
  type AIChatMessageListResult,
  type AIChatRuntimeTransport,
  type AIChatStreamCallbacks,
} from '@/components/chat/transports/aichat-transport';

interface RuntimeClient {
  get<T = unknown>(url: string, config?: ExtendedRequestConfig): Promise<T>;
  post<T = unknown>(url: string, data?: unknown, config?: ExtendedRequestConfig): Promise<T>;
  patch<T = unknown>(url: string, data?: unknown, config?: ExtendedRequestConfig): Promise<T>;
  delete<T = unknown>(url: string, config?: ExtendedRequestConfig): Promise<T>;
  sse<TOut = unknown, TBody = unknown>(
    path: string,
    options: SseOptions<TBody, TOut>
  ): Promise<{ close: () => void }>;
}

interface AgentRuntimeTransportOptions {
  runtimeBasePath: string;
  chatPath: string;
  client?: RuntimeClient;
}

function runtimeTerminalMessage(message: { event: string | null; data: unknown }): boolean {
  const record =
    message.data && typeof message.data === 'object'
      ? (message.data as Record<string, unknown>)
      : {};
  const event = typeof record.event === 'string' ? record.event : message.event;
  if (event === 'message_end' || event === 'error') {
    return true;
  }
  if (
    event === 'workflow_started' ||
    event === 'node_started' ||
    event === 'node_finished' ||
    event === 'workflow_paused' ||
    event === 'approval_requested' ||
    event === 'workflow_finished' ||
    event === 'workflow_failed'
  ) {
    return false;
  }
  const data = record.data && typeof record.data === 'object' ? record.data : record;
  return (
    data &&
    typeof data === 'object' &&
    ['completed', 'stopped', 'error'].includes(String((data as Record<string, unknown>).status))
  );
}

function sortMessages(items: AIChatMessage[]): AIChatMessage[] {
  return items.slice().sort((a, b) => a.created_at - b.created_at || a.id.localeCompare(b.id));
}

function paginationFromResponse<T>(
  response: ApiResponseData<{
    page: number;
    limit: number;
    total: number;
    has_more: boolean;
    data: T[];
  }>
): AIChatPagination {
  return {
    page: response.data.page,
    limit: response.data.limit,
    total: response.data.total,
    hasMore: response.data.has_more,
  };
}

export class AgentRuntimeTransport implements AIChatRuntimeTransport {
  private readonly runtimeBasePath: string;
  private readonly chatPath: string;
  private readonly client: RuntimeClient;

  constructor({ runtimeBasePath, chatPath, client = http }: AgentRuntimeTransportOptions) {
    this.runtimeBasePath = runtimeBasePath;
    this.chatPath = chatPath;
    this.client = client;
  }

  async listConversations(params: {
    page: number;
    limit: number;
  }): Promise<AIChatConversationListResult> {
    const response = await this.client.get<AIChatConversationListResponse>(
      `${this.runtimeBasePath}/conversations`,
      { params }
    );
    return {
      items: response.data.data,
      pagination: paginationFromResponse(response),
    };
  }

  async getConversation(conversationId: string): Promise<AIChatConversationDetail> {
    const [conversationResponse, messageList] = await Promise.all([
      this.refreshConversation(conversationId),
      this.listMessages(conversationId, {
        page: 1,
        limit: DEFAULT_AICHAT_MESSAGE_PAGINATION.limit,
      }),
    ]);
    return {
      conversation: conversationResponse,
      messages: messageList.items,
      messagePagination: messageList.pagination,
    };
  }

  async listMessages(
    conversationId: string,
    params: { page: number; limit: number }
  ): Promise<AIChatMessageListResult> {
    const response = await this.client.get<AIChatMessageListResponse>(
      `${this.runtimeBasePath}/conversations/${conversationId}/messages`,
      { params }
    );
    return {
      items: sortMessages(response.data.data),
      pagination: paginationFromResponse(response),
    };
  }

  async refreshConversation(conversationId: string): Promise<AIChatConversation> {
    const response = await this.client.get<ApiResponseData<AIChatConversation>>(
      `${this.runtimeBasePath}/conversations/${conversationId}`
    );
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
    const response = await this.client.patch<ApiResponseData<AIChatConversation>>(
      `${this.runtimeBasePath}/conversations/${conversationId}`,
      payload
    );
    return response.data;
  }

  async removeConversation(conversationId: string): Promise<void> {
    await this.client.delete(`${this.runtimeBasePath}/conversations/${conversationId}`);
  }

  async stopConversation(conversationId: string): Promise<AIChatStopConversationResponseData> {
    const response = await this.client.post<ApiResponseData<AIChatStopConversationResponseData>>(
      `${this.runtimeBasePath}/conversations/${conversationId}/stop`
    );
    return response.data;
  }

  streamChat(
    payload: AIChatChatRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    return this.client.sse<AIChatSseEnvelope, AIChatChatRequest>(this.chatPath, {
      method: 'POST',
      body: payload,
      abortSignal,
      isTerminalMessage: runtimeTerminalMessage,
      onMessage: message =>
        dispatchAIChatStreamEvent(
          String((message.data as AIChatSseEnvelope | undefined)?.event ?? message.event ?? ''),
          (message.data as AIChatSseEnvelope | undefined)?.data ?? message.data,
          message.id,
          callbacks
        ),
      onError: callbacks.onRequestError,
      onClose: callbacks.onClose,
    });
  }

  regenerateMessage(
    messageId: string,
    payload: AIChatRegenerateMessageRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    return this.client.sse<AIChatSseEnvelope, AIChatRegenerateMessageRequest>(
      `${this.runtimeBasePath}/messages/${messageId}/regenerate`,
      {
        method: 'POST',
        body: payload,
        abortSignal,
        isTerminalMessage: runtimeTerminalMessage,
        onMessage: message =>
          dispatchAIChatStreamEvent(
            String((message.data as AIChatSseEnvelope | undefined)?.event ?? message.event ?? ''),
            (message.data as AIChatSseEnvelope | undefined)?.data ?? message.data,
            message.id,
            callbacks
          ),
        onError: callbacks.onRequestError,
        onClose: callbacks.onClose,
      }
    );
  }

  recoverConversationStream(
    conversationId: string,
    params: { messageId: string; afterId?: string },
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    return this.client.sse<AIChatSseEnvelope, never>(
      `${this.runtimeBasePath}/conversations/${conversationId}/events`,
      {
        method: 'GET',
        query: {
          message_id: params.messageId,
          after_id: params.afterId,
        },
        abortSignal,
        isTerminalMessage: runtimeTerminalMessage,
        onMessage: message =>
          dispatchAIChatStreamEvent(
            String((message.data as AIChatSseEnvelope | undefined)?.event ?? message.event ?? ''),
            (message.data as AIChatSseEnvelope | undefined)?.data ?? message.data,
            message.id,
            callbacks
          ),
        onError: callbacks.onRequestError,
        onClose: callbacks.onClose,
      }
    );
  }
}

export function createAgentDraftTransport(agentId: string): AgentRuntimeTransport {
  return new AgentRuntimeTransport({
    runtimeBasePath: `/console/api/agents/${agentId}/runtime`,
    chatPath: `/console/api/agents/${agentId}/chat`,
  });
}

export function createAgentWebAppTransport(webAppId: string): AgentRuntimeTransport {
  return new AgentRuntimeTransport({
    runtimeBasePath: `/console/api/webapps/${webAppId}/runtime`,
    chatPath: `/console/api/webapps/${webAppId}/chat`,
    client: webappHttp,
  });
}
