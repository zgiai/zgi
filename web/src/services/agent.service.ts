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
  AgentRuntimeConfig,
  AgentSkillBindingCandidatesResponse,
  AgentKnowledgeBindingCandidatesResponse,
  AgentDatabaseBindingCandidatesResponse,
  AgentDatabaseTableBindingCandidatesResponse,
  AgentWorkflowBindingCandidatesResponse,
  UpdateAgentRuntimeConfigRequest,
  AgentMemorySlotConfig,
  AgentMemoryValuesResponse,
  UpdateAgentMemoryValueRequest,
  AgentMemoryValue,
  PublishAgentResponse,
  AgentPublishedVersionsResponse,
  RollbackAgentPublishedVersionRequest,
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
  AgentRuntimeSurfaceAuthorizationResponse,
  UpdateAgentRuntimeSurfacesRequest,
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
    return this.request<ApiResponseData<RunnableWebAppsData>>(
      'get',
      '/agents/runnable-webapps',
      undefined,
      {
        params,
        headers: { 'Content-Type': 'application/json' },
      }
    );
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

  getAgentRuntimeSurfaces(
    agentId: string
  ): Promise<ApiResponseData<AgentRuntimeSurfaceAuthorizationResponse>> {
    return this.request('get', `/agents/${agentId}/runtime-surfaces`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  updateAgentRuntimeSurfaces(
    agentId: string,
    data: UpdateAgentRuntimeSurfacesRequest
  ): Promise<ApiResponseData<AgentRuntimeSurfaceAuthorizationResponse>> {
    return this.request('patch', `/agents/${agentId}/runtime-surfaces`, data, {
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
    agentId: string,
    params?: { query?: string; page?: number; limit?: number }
  ): Promise<ApiResponseData<AgentWorkflowBindingCandidatesResponse>> {
    return this.request('get', `/agents/${agentId}/candidates/workflows`, undefined, {
      headers: { 'Content-Type': 'application/json' },
      params,
    });
  }

  getAgentSkillBindingCandidates(
    agentId: string,
    params?: { query?: string; source?: 'system' | 'custom'; page?: number; limit?: number }
  ): Promise<ApiResponseData<AgentSkillBindingCandidatesResponse>> {
    return this.request('get', `/agents/${agentId}/candidates/skills`, undefined, {
      headers: { 'Content-Type': 'application/json' },
      params,
    });
  }

  getAgentKnowledgeBindingCandidates(
    agentId: string,
    params?: { query?: string; page?: number; limit?: number }
  ): Promise<ApiResponseData<AgentKnowledgeBindingCandidatesResponse>> {
    return this.request('get', `/agents/${agentId}/candidates/knowledge`, undefined, {
      headers: { 'Content-Type': 'application/json' },
      params,
    });
  }

  getAgentDatabaseBindingCandidates(
    agentId: string,
    params?: {
      query?: string;
      page?: number;
      limit?: number;
      available_only?: boolean;
      require_write?: boolean;
    }
  ): Promise<ApiResponseData<AgentDatabaseBindingCandidatesResponse>> {
    return this.request('get', `/agents/${agentId}/candidates/databases`, undefined, {
      headers: { 'Content-Type': 'application/json' },
      params,
    });
  }

  getAgentDatabaseTableBindingCandidates(
    agentId: string,
    dataSourceId: string,
    params?: { query?: string; page?: number; limit?: number; include_columns?: boolean }
  ): Promise<ApiResponseData<AgentDatabaseTableBindingCandidatesResponse>> {
    return this.request(
      'get',
      `/agents/${agentId}/candidates/databases/${dataSourceId}/tables`,
      undefined,
      {
        headers: { 'Content-Type': 'application/json' },
        params,
      }
    );
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
