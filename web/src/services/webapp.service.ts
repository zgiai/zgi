import { http, webappHttp } from '@/lib/http';
import type {
  WebAppApiResponseData,
  WebAppWorkflowConfig,
  WebAppRunRequest,
  WebAppRunSseCallbacks,
  WebAppConversationList,
  WebAppConversationDetail,
  WebAppConversationSearchResponse,
  WebAppPrecheckResult,
  WebAppRuntimeCapability,
} from './types/webapp';
import { sanitizeModelOutputValue, wrapModelOutputSseCallbacks } from '@/utils/model-output-filter';
import {
  getWebAppErrorCode,
  WEBAPP_NOT_PUBLISHED_ERROR_CODE,
  WEBAPP_OFFLINE_ERROR_CODE,
} from '@/utils/webapp/errors';

interface WebAppRunBody extends WebAppRunRequest {
  response_mode: 'streaming';
}

function buildWebAppRunBody(payload: WebAppRunRequest): WebAppRunBody {
  return {
    query: payload.query,
    response_mode: 'streaming',
    conversation_id: payload.conversation_id,
    history_window_size: payload.history_window_size,
    files: payload.files,
    inputs: payload.inputs,
  };
}

/**
 * WebAppService - decoupled service for public webapp endpoints.
 * Uses webappHttp with independent Authorization token from localStorage.
 */
export class WebAppService {
  /**
   * Fetch webapp config by workflow version UUID.
   * GET /console/api/workflows/{version_uuid}/config
   */
  static async getConfig(
    versionUuid: string
  ): Promise<WebAppApiResponseData<WebAppWorkflowConfig>> {
    try {
      return await webappHttp.get<WebAppApiResponseData<WebAppWorkflowConfig>>(
        `/console/api/webapps/${versionUuid}/config`
      );
    } catch (error) {
      const code = getWebAppErrorCode(error);
      if (code === WEBAPP_OFFLINE_ERROR_CODE || code === WEBAPP_NOT_PUBLISHED_ERROR_CODE) {
        throw error;
      }
      return webappHttp.get<WebAppApiResponseData<WebAppWorkflowConfig>>(
        `/console/api/workflows/${versionUuid}/config`
      );
    }
  }

  /**
   * Run webapp workflow via SSE POST by version UUID.
   * POST /console/api/workflows/{version_uuid}/run (streaming)
   */
  static ssePostRun(
    versionUuid: string,
    payload: WebAppRunRequest,
    callbacks: WebAppRunSseCallbacks,
    opts?: { abortSignal?: AbortSignal; onClose?: () => void }
  ): Promise<{ close: () => void }> {
    return webappHttp.ssePost(`/console/api/workflows/${versionUuid}/run`, {
      body: buildWebAppRunBody(payload),
      callbacks: wrapModelOutputSseCallbacks(callbacks),
      abortSignal: opts?.abortSignal,
      onClose: opts?.onClose,
    });
  }

  static ssePostAgentChat(
    webAppId: string,
    payload: WebAppRunRequest,
    callbacks: WebAppRunSseCallbacks,
    opts?: { abortSignal?: AbortSignal; onClose?: () => void }
  ): Promise<{ close: () => void }> {
    return webappHttp.ssePost(`/console/api/webapps/${webAppId}/chat`, {
      body: {
        query: payload.query,
        conversation_id: payload.conversation_id,
        response_mode: 'streaming',
        files: payload.files,
      },
      callbacks: wrapModelOutputSseCallbacks(callbacks),
      abortSignal: opts?.abortSignal,
      onClose: opts?.onClose,
    });
  }

  static async getCapability(
    webAppId: string
  ): Promise<WebAppApiResponseData<WebAppRuntimeCapability>> {
    return webappHttp.get<WebAppApiResponseData<WebAppRuntimeCapability>>(
      `/console/api/webapps/${webAppId}/capability`
    );
  }

  static async precheck(
    versionUuid: string,
    payload: WebAppRunRequest
  ): Promise<WebAppApiResponseData<WebAppPrecheckResult>> {
    return webappHttp.post<WebAppApiResponseData<WebAppPrecheckResult>>(
      `/console/api/workflows/${versionUuid}/precheck`,
      buildWebAppRunBody(payload)
    );
  }

  /**
   * List conversations for current webapp token and specific workflow version.
   * GET /console/api/workflows/{version_uuid}/conversations
   */
  static async getConversations(
    versionUuid: string,
    params: { limit?: number; page?: number }
  ): Promise<WebAppApiResponseData<WebAppConversationList>> {
    return webappHttp.get<WebAppApiResponseData<WebAppConversationList>>(
      `/console/api/workflows/${versionUuid}/conversations`,
      { params }
    );
  }

  static async searchConversations(
    versionUuid: string,
    params: { query: string; limit?: number }
  ): Promise<WebAppConversationSearchResponse> {
    return webappHttp.get<WebAppConversationSearchResponse>(
      `/console/api/workflows/${versionUuid}/search`,
      { params }
    );
  }

  /**
   * Get a single conversation with messages for current webapp token.
   * GET /console/api/workflows/{version_uuid}/conversations/{conversation_id}
   */
  static async getConversation(
    versionUuid: string,
    conversationId: string
  ): Promise<WebAppApiResponseData<WebAppConversationDetail>> {
    const response = await webappHttp.get<WebAppApiResponseData<WebAppConversationDetail>>(
      `/console/api/workflows/${versionUuid}/conversations/${conversationId}`
    );
    return {
      ...response,
      data: {
        ...response.data,
        messages: response.data.messages.map(message => ({
          ...message,
          answer: sanitizeModelOutputValue(message.answer) as string,
        })),
      },
    };
  }

  /**
   * Delete a conversation for current webapp token.
   * DELETE /console/api/workflows/{version_uuid}/conversations/{conversation_id}
   */
  static async deleteConversation(
    versionUuid: string,
    conversationId: string
  ): Promise<WebAppApiResponseData<{ result: 'success' | string }>> {
    return webappHttp.delete<WebAppApiResponseData<{ result: 'success' | string }>>(
      `/console/api/workflows/${versionUuid}/conversations/${conversationId}`
    );
  }

  /**
   * Stop a running webapp workflow task
   * POST /console/api/workflows/{version_uuid}/tasks/{task_id}/stop
   */
  static async stopTask(
    versionUuid: string,
    taskId: string
  ): Promise<WebAppApiResponseData<{ result: 'success' | string }>> {
    return webappHttp.post<WebAppApiResponseData<{ result: 'success' | string }>>(
      `/console/api/workflows/${versionUuid}/tasks/${taskId}/stop`
    );
  }

  /**
   * Migrate anonymous webapp conversations into the logged-in account.
   * POST /console/api/workflows/{web_app_id}/migrate-user
   * Body: empty. Headers:
   * - Authorization: Bearer <auth_token> (attached by default http client)
   * - X-User-Account-Id: <local webapp token>
   */
  static async migrateUser(
    localWebAppToken: string,
    webAppId?: string
  ): Promise<WebAppApiResponseData<{ result: 'success' | string }>> {
    const headers: Record<string, string> = {
      'X-User-Account-Id': localWebAppToken,
      'Content-Type': 'application/json',
    };
    const normalizedWebAppId = webAppId?.trim();
    const endpoint = normalizedWebAppId
      ? `/console/api/workflows/${encodeURIComponent(normalizedWebAppId)}/migrate-user`
      : `/console/api/workflows/migrate-user`;
    // Use main-site http client so it carries Authorization and refresh logic
    return http.post<WebAppApiResponseData<{ result: 'success' | string }>>(
      endpoint,
      undefined,
      { headers }
    );
  }
}

export default WebAppService;
