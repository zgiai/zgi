import { BaseService } from '@/lib/http/services';
import { wrapModelOutputSseCallbacks } from '@/utils/model-output-filter';
import { webappHttp } from '@/lib/http';
import type {
  Agent,
  AgentList,
  AgentDetail,
  AgentCreateResponse,
  CreateAgentRequest,
  UpdateAgentRequest,
  UpdateWebAppStatusRequest,
  UpdateWebAppStatusResponse,
  AgentListParams,
  AgentApiKey,
  AgentApiKeyList,
  CreateAgentApiKeyRequest,
  UpdateAgentApiKeyRequest,
  AgentApiKeyCreateResponse,
  RunnableWebAppsData,
  RunnableWebAppsParams,
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
