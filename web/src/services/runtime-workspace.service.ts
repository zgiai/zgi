import { BaseService } from '@/lib/http';
import type { ApiResponseData } from './types/common';
import type {
  RuntimeAgentCandidateSet,
  RuntimeAppCollection,
  RuntimeCredential,
  RuntimeCredentialCreation,
  RuntimeEndUser,
  RuntimeEndUserBatchResult,
  RuntimeEndUserEvent,
  RuntimeEndUserList,
  RuntimeEndUserListParams,
  RuntimeEndUserPatch,
  RuntimeEndUserSummary,
  RuntimeUsageSeries,
  RuntimeWorkspace,
  RuntimeWorkspaceAgentSelectionInput,
  RuntimeWorkspaceCredentialInput,
  RuntimeWorkspaceStatus,
} from './types/runtime-workspace';

class RuntimeWorkspaceService extends BaseService {
  constructor() {
    super({ endpoint: 'main', basePath: '/console/api' });
  }

  private workspaceHeaders(workspaceId: string) {
    return {
      'Content-Type': 'application/json',
      'X-Workspace-ID': workspaceId,
    };
  }

  async getCurrent(workspaceId: string): Promise<RuntimeWorkspace | null> {
    const response = await this.request<ApiResponseData<RuntimeWorkspace | null>>(
      'get',
      '/runtime-workspaces',
      undefined,
      { headers: this.workspaceHeaders(workspaceId), skipErrorHandling: true }
    );
    // The API omits `data` when the runtime workspace has not been created
    // yet (`response.Success(c, nil)` plus `omitempty`). React Query treats
    // an undefined query result as an error, while the page needs null to
    // render the first-time enablement flow.
    return response.data ?? null;
  }

  async create(workspaceId: string): Promise<RuntimeWorkspace> {
    const response = await this.request<ApiResponseData<RuntimeWorkspace>>(
      'post',
      '/runtime-workspaces',
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  async update(
    workspaceId: string,
    data: { status?: RuntimeWorkspaceStatus; policy_json?: Record<string, unknown> }
  ): Promise<RuntimeWorkspace> {
    const response = await this.request<ApiResponseData<RuntimeWorkspace>>(
      'patch',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}`,
      data,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  async listCredentials(workspaceId: string): Promise<RuntimeCredential[]> {
    const response = await this.request<
      ApiResponseData<{ items: RuntimeCredential[] }>
    >(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/credentials`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data?.items ?? [];
  }

  async createCredential(
    workspaceId: string,
    data: RuntimeWorkspaceCredentialInput
  ): Promise<RuntimeCredentialCreation> {
    const { idempotency_key: idempotencyKey, ...payload } = data;
    const response = await this.request<ApiResponseData<RuntimeCredentialCreation>>(
      'post',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/credentials`,
      payload,
      {
        headers: {
          ...this.workspaceHeaders(workspaceId),
          ...(idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {}),
        },
      }
    );
    return response.data;
  }

  async revokeCredential(workspaceId: string, credentialId: string): Promise<void> {
    await this.request<ApiResponseData<{ result: 'success' }>>(
      'post',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/credentials/${encodeURIComponent(credentialId)}/revoke`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
  }

  async listApps(workspaceId: string): Promise<RuntimeAppCollection> {
    const response = await this.request<ApiResponseData<RuntimeAppCollection>>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/agents`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data ?? { revision: 0, items: [] };
  }

  async listAgentCandidates(
    workspaceId: string,
    scope: 'current' | 'authorized' = 'current',
    sourceWorkspaceId?: string
  ): Promise<RuntimeAgentCandidateSet> {
    const params = new URLSearchParams({ scope });
    if (sourceWorkspaceId) params.set('source_workspace_id', sourceWorkspaceId);
    const response = await this.request<ApiResponseData<RuntimeAgentCandidateSet>>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/agent-candidates?${params.toString()}`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data ?? { scope, cross_workspace_enabled: false, source_workspaces: [], items: [] };
  }

  async syncAgents(workspaceId: string, data: RuntimeWorkspaceAgentSelectionInput): Promise<RuntimeAppCollection> {
    const response = await this.request<ApiResponseData<RuntimeAppCollection>>(
      'put',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/agents`,
      data,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  async listEndUsers(
    workspaceId: string,
    query: RuntimeEndUserListParams = {}
  ): Promise<RuntimeEndUserList> {
    const params = new URLSearchParams();
    if (query.search) params.set('search', query.search);
    if (query.status) params.set('status', query.status);
    if (query.quota_state) params.set('quota_state', query.quota_state);
    if (query.plan) params.set('plan', query.plan);
    if (query.sort) params.set('sort', query.sort);
    if (query.order) params.set('order', query.order);
    if (query.page) params.set('page', String(query.page));
    if (query.limit) params.set('limit', String(query.limit));
    const response = await this.request<ApiResponseData<RuntimeEndUserList>>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users?${params.toString()}`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data ?? { items: [], page: 1, limit: 50, total: 0, has_more: false };
  }

  async updateEndUser(
    workspaceId: string,
    subjectId: string,
    data: RuntimeEndUserPatch
  ): Promise<RuntimeEndUser> {
    const response = await this.request<ApiResponseData<RuntimeEndUser>>(
      'patch',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}`,
      data,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  async batchUpdateEndUsers(
    workspaceId: string,
    subjectIds: string[],
    data: RuntimeEndUserPatch
  ): Promise<RuntimeEndUserBatchResult> {
    const response = await this.request<ApiResponseData<RuntimeEndUserBatchResult>>(
      'post',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/batch`,
      { ...data, subject_ids: subjectIds },
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  async getEndUserSummary(workspaceId: string): Promise<RuntimeEndUserSummary> {
    const response = await this.request<ApiResponseData<RuntimeEndUserSummary>>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/summary`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  /** Daily end-user usage of the workspace, or of one end user. */
  async getEndUserUsageSeries(
    workspaceId: string,
    options: { subjectId?: string; days?: number; timezone?: string } = {}
  ): Promise<RuntimeUsageSeries> {
    const params = new URLSearchParams();
    if (options.days) params.set('days', String(options.days));
    if (options.timezone) params.set('tz', options.timezone);
    const path = options.subjectId
      ? `/end-users/${encodeURIComponent(options.subjectId)}/usage-series`
      : '/end-users/usage-series';
    const response = await this.request<ApiResponseData<RuntimeUsageSeries>>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}${path}?${params.toString()}`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data ?? { days: [], apps: [] };
  }

  async exportEndUser(workspaceId: string, subjectId: string): Promise<Record<string, unknown>> {
    const response = await this.request<ApiResponseData<Record<string, unknown>>>(
      'post',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}/export`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  async eraseEndUser(workspaceId: string, subjectId: string): Promise<void> {
    await this.request<ApiResponseData<unknown>>(
      'delete',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
  }

  async listEndUserEvents(workspaceId: string, subjectId: string): Promise<RuntimeEndUserEvent[]> {
    const response = await this.request<ApiResponseData<{ items: RuntimeEndUserEvent[] }>>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}/events`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data?.items ?? [];
  }
}

export const runtimeWorkspaceService = new RuntimeWorkspaceService();
