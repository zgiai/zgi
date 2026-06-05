import { BaseService } from '@/lib/http/services';
import { wrapModelOutputSseCallbacks } from '@/utils/model-output-filter';
import { http, webappHttp } from '@/lib/http';
import type {
  Agent,
  AgentList,
  AgentDetail,
  AgentCreateResponse,
  CreateAgentRequest,
  UpdateAgentRequest,
  UpdateWebAppStatusRequest,
  UpdateWebAppStatusResponse,
  AgentRuntimeConfig,
  AgentWorkflowBindingCandidatesResponse,
  UpdateAgentRuntimeConfigRequest,
  AgentMemorySlotConfig,
  AgentMemoryValuesResponse,
  UpdateAgentMemoryValueRequest,
  AgentMemoryValue,
  PublishAgentResponse,
  AgentPublishedVersionsResponse,
  RollbackAgentPublishedVersionRequest,
  AgentChatRequest,
  AgentChatSseEnvelope,
  AgentChatStreamCallbacks,
  AgentListParams,
  AgentApiKey,
  AgentApiKeyList,
  CreateAgentApiKeyRequest,
  UpdateAgentApiKeyRequest,
  AgentApiKeyCreateResponse,
  RunnableWebAppsData,
  RunnableWebAppsParams,
  GenerateAgentSuggestedQuestionsRequest,
  GenerateAgentSuggestedQuestionsResponse,
} from './types/agent';
import type { WebAppRunRequest, WebAppRunSseCallbacks } from './types/webapp';
import type { ApiResponseData } from './types/common';

/**
 * AgentService
 * ---------------------------------------------------------------------------
 * Handles all agent related APIs.
 * All methods return the unified `ApiResponseData<T>` structure **without**
 * stripping the `data` wrapper so that callers can decide how to consume it.
 * ---------------------------------------------------------------------------
 * API Reference: Agent module APIs
 */
class AgentService extends BaseService {
  constructor() {
    super({
      basePath: '/console/api',
      endpoint: 'main',
    });
  }

  /**
   * Get all agents
   * GET /console/api/agents
   */
  getAgents(params?: AgentListParams): Promise<ApiResponseData<AgentList>> {
    return this.request('get', '/agents', undefined, {
      params,
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Get runnable webapp list for current account.
   * GET /api/v1/agents/runnable-webapps
   */
  getRunnableWebApps(
    params?: RunnableWebAppsParams
  ): Promise<ApiResponseData<RunnableWebAppsData>> {
    return this.request<ApiResponseData<RunnableWebAppsData>>('get', '/agents/runnable-webapps', {
      params,
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Create a new agent
   * POST /console/api/agents
   */
  createAgent(data: CreateAgentRequest): Promise<ApiResponseData<AgentCreateResponse>> {
    return this.request('post', '/agents', data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Get agent detail by ID
   * GET /console/api/agents/{agent_id}
   */
  getAgent(agentId: string): Promise<ApiResponseData<AgentDetail>> {
    return this.request('get', `/agents/${agentId}`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Update agent by ID
   * PUT /console/api/agents/{agent_id}
   */
  updateAgent(agentId: string, data: UpdateAgentRequest): Promise<ApiResponseData<Agent>> {
    return this.request('put', `/agents/${agentId}`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Delete agent by ID
   * DELETE /console/api/agents/{agent_id}
   */
  deleteAgent(agentId: string): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request('delete', `/agents/${agentId}`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Update web app online/offline status.
   * PATCH /console/api/agents/{agent_id}/webapp/status
   */
  updateWebAppStatus(
    agentId: string,
    data: UpdateWebAppStatusRequest
  ): Promise<ApiResponseData<UpdateWebAppStatusResponse>> {
    return this.request('patch', `/agents/${agentId}/webapp/status`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  getAgentConfig(agentId: string): Promise<ApiResponseData<AgentRuntimeConfig>> {
    return this.request('get', `/agents/${agentId}/config`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  updateAgentConfig(
    agentId: string,
    data: UpdateAgentRuntimeConfigRequest
  ): Promise<ApiResponseData<AgentRuntimeConfig>> {
    return this.request('put', `/agents/${agentId}/config`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  getAgentWorkflowBindingCandidates(
    agentId: string
  ): Promise<ApiResponseData<AgentWorkflowBindingCandidatesResponse>> {
    return this.request('get', `/agents/${agentId}/workflow-bindings/candidates`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  getAgentMemorySlots(
    agentId: string
  ): Promise<ApiResponseData<{ slots: AgentMemorySlotConfig[] }>> {
    return this.request('get', `/agents/${agentId}/memory/slots`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  updateAgentMemorySlots(
    agentId: string,
    slots: AgentMemorySlotConfig[]
  ): Promise<ApiResponseData<{ slots: AgentMemorySlotConfig[] }>> {
    return this.request(
      'put',
      `/agents/${agentId}/memory/slots`,
      { slots },
      {
        headers: { 'Content-Type': 'application/json' },
      }
    );
  }

  getAgentMemoryValues(agentId: string): Promise<ApiResponseData<AgentMemoryValuesResponse>> {
    return this.request('get', `/agents/${agentId}/memory/values`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  updateAgentMemoryValue(
    agentId: string,
    data: UpdateAgentMemoryValueRequest
  ): Promise<ApiResponseData<AgentMemoryValue>> {
    return this.request('put', `/agents/${agentId}/memory/values`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  clearAgentMemoryValue(
    agentId: string,
    params: { key: string }
  ): Promise<ApiResponseData<AgentMemoryValue>> {
    return this.request(
      'delete',
      `/agents/${agentId}/memory/values/${encodeURIComponent(params.key)}`,
      undefined,
      {
        headers: { 'Content-Type': 'application/json' },
      }
    );
  }

  publishAgent(agentId: string): Promise<ApiResponseData<PublishAgentResponse>> {
    return this.request('post', `/agents/${agentId}/publish`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  getPublishedVersions(agentId: string): Promise<ApiResponseData<AgentPublishedVersionsResponse>> {
    return this.request('get', `/agents/${agentId}/published-versions`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  rollbackPublishedVersion(
    agentId: string,
    data: RollbackAgentPublishedVersionRequest
  ): Promise<ApiResponseData<AgentRuntimeConfig>> {
    return this.request('post', `/agents/${agentId}/published-versions/rollback`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  generateSuggestedQuestions(
    agentId: string,
    data: GenerateAgentSuggestedQuestionsRequest
  ): Promise<ApiResponseData<GenerateAgentSuggestedQuestionsResponse>> {
    return this.request('post', `/agents/${agentId}/suggested-questions/generate`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  streamAgentChat(
    agentId: string,
    payload: AgentChatRequest,
    callbacks: AgentChatStreamCallbacks,
    abortSignal?: AbortSignal
  ): Promise<{ close: () => void }> {
    return http.sse<AgentChatSseEnvelope, AgentChatRequest>(`/console/api/agents/${agentId}/chat`, {
      method: 'POST',
      body: {
        ...payload,
        response_mode: payload.response_mode ?? 'streaming',
      },
      abortSignal,
      isTerminalMessage: message => {
        const data = message.data as AgentChatSseEnvelope | undefined;
        const event = data?.event ?? message.event;
        return event === 'message_end' || event === 'error';
      },
      onMessage: message => {
        const envelope = message.data;
        const event = envelope.event ?? message.event ?? '';
        const data = envelope.data ?? {};
        if (event === 'message_start') callbacks.onMessageStart?.(data);
        else if (event === 'message') callbacks.onMessage?.(data);
        else if (event === 'message_end') callbacks.onMessageEnd?.(data);
        else if (event === 'error') callbacks.onError?.(data);
      },
      onError: error => callbacks.onError?.(error),
      onClose: callbacks.onClose,
    });
  }

  /**
   * Create API Key for an agent
   * POST /console/api/agents/{agentId}/api-keys
   */
  createAgentApiKey(
    agentId: string,
    data: CreateAgentApiKeyRequest
  ): Promise<ApiResponseData<AgentApiKeyCreateResponse>> {
    return this.request('post', `/agents/${agentId}/api-keys`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Get all API Keys of an agent
   * GET /console/api/agents/{agentId}/api-keys
   */
  getAgentApiKeys(agentId: string): Promise<ApiResponseData<AgentApiKeyList>> {
    return this.request('get', `/agents/${agentId}/api-keys`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Update specific API Key
   * PUT /console/api/agents/{agentId}/api-keys/{keyId}
   */
  updateAgentApiKey(
    agentId: string,
    keyId: string,
    data: UpdateAgentApiKeyRequest
  ): Promise<ApiResponseData<AgentApiKey>> {
    return this.request('put', `/agents/${agentId}/api-keys/${keyId}`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Delete specific API Key
   * DELETE /console/api/agents/{agentId}/api-keys/{keyId}
   */
  deleteAgentApiKey(
    agentId: string,
    keyId: string
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request('delete', `/agents/${agentId}/api-keys/${keyId}`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Run agent advanced-chat workflow via SSE POST
   * POST /console/api/agents/{agent_id}/advanced-chat/workflows/run (streaming)
   */
  sseAdvancedChatRun(
    agentId: string,
    payload: WebAppRunRequest,
    callbacks: WebAppRunSseCallbacks,
    opts?: { abortSignal?: AbortSignal }
  ): Promise<{ close: () => void }> {
    return webappHttp.ssePost(`/console/api/agents/${agentId}/advanced-chat/workflows/run`, {
      body: {
        query: payload.query,
        response_mode: 'streaming',
        conversation_id: payload.conversation_id,
        history_window_size: payload.history_window_size,
        files: payload.files,
        inputs: payload.inputs,
      },
      callbacks: wrapModelOutputSseCallbacks(callbacks),
      abortSignal: opts?.abortSignal,
    });
  }
}

export const agentService = new AgentService();
export default agentService;
