import { BaseService } from '@/lib/http';
import type { ApiResponseData } from './types/common';
import type {
  RuntimeEndUserDigitalHuman,
  RuntimeEndUserExportJob,
  RuntimeAgentCandidateSet,
  RuntimeAppCollection,
  RuntimeCredentialCreation,
  RuntimeEndUser,
  RuntimeCreditGrant,
  RuntimeCreditGrantResult,
  RuntimeAppListParams,
  RuntimeCandidateParams,
  RuntimeCreditLedger,
  RuntimeCredentialList,
  RuntimeManagedAppPage,
  RuntimePage,
  RuntimeEndUserBatchResult,
  RuntimeEndUserEvent,
  RuntimeEndUserList,
  RuntimeEndUserListParams,
  RuntimeEndUserPatch,
  RuntimeEndUserSummary,
  RuntimeUsageSeries,
  RuntimeWebhook,
  RuntimeWebhookDelivery,
  RuntimeWebhookInput,
  RuntimeWebhookSaved,
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

  /** A page of the bound apps, with usage and caps. */
  async listManagedApps(
    workspaceId: string,
    query: RuntimeAppListParams = {}
  ): Promise<RuntimeManagedAppPage> {
    const params = new URLSearchParams();
    if (query.search) params.set('search', query.search);
    if (query.type) params.set('type', query.type);
    if (query.state) params.set('state', query.state);
    params.set('page', String(query.page ?? 1));
    params.set('limit', String(query.limit ?? 20));
    const response = await this.request<ApiResponseData<RuntimeManagedAppPage>>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/apps?${params.toString()}`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  /** A page of the apps that may be added, matched on the server. */
  async listAppCandidatesPage(
    workspaceId: string,
    query: RuntimeCandidateParams,
    page: number
  ): Promise<RuntimeAgentCandidateSet> {
    const params = new URLSearchParams({
      scope: query.scope,
      page: String(page),
      limit: '30',
      exclude_bound: 'true',
    });
    if (query.sourceWorkspaceId) params.set('source_workspace_id', query.sourceWorkspaceId);
    if (query.search) params.set('search', query.search);
    if (query.type) params.set('type', query.type);
    const response = await this.request<ApiResponseData<RuntimeAgentCandidateSet>>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/agent-candidates?${params.toString()}`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  /** Adds apps to, or removes them from, the current selection. */
  async changeApps(
    workspaceId: string,
    change:
      | { expected_revision: number; apps: Array<{ type: string; id: string }> }
      | { expected_revision: number; app_ids: string[] }
  ): Promise<RuntimeAppCollection> {
    const action = 'apps' in change ? 'add' : 'remove';
    const response = await this.request<ApiResponseData<RuntimeAppCollection>>(
      'post',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/apps/${action}`,
      change,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  /** Active keys only, or every key with status 'all'. */
  async listCredentials(
    workspaceId: string,
    status: 'active' | 'all' = 'active'
  ): Promise<RuntimeCredentialList> {
    const response = await this.request<ApiResponseData<RuntimeCredentialList>>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/credentials${status === 'active' ? '?status=active' : ''}`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data ?? { items: [], inactive: 0, limit: 50 };
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
    return (
      response.data ?? { scope, cross_workspace_enabled: false, source_workspaces: [], items: [] }
    );
  }

  async syncAgents(
    workspaceId: string,
    data: RuntimeWorkspaceAgentSelectionInput
  ): Promise<RuntimeAppCollection> {
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

  async listEndUserCredits(
    workspaceId: string,
    subjectId: string,
    page = 1
  ): Promise<RuntimeCreditLedger> {
    const response = await this.request<ApiResponseData<RuntimeCreditLedger>>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}/credits?limit=20&page=${page}`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  async grantEndUserCredits(
    workspaceId: string,
    subjectId: string,
    data: RuntimeCreditGrant
  ): Promise<RuntimeCreditGrantResult> {
    const response = await this.request<ApiResponseData<RuntimeCreditGrantResult>>(
      'post',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}/credits`,
      data,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  private webhookPath(workspaceId: string, suffix = '') {
    return `/runtime-workspaces/${encodeURIComponent(workspaceId)}/webhook${suffix}`;
  }

  async getWebhook(workspaceId: string): Promise<RuntimeWebhook> {
    const response = await this.request<ApiResponseData<RuntimeWebhook>>(
      'get',
      this.webhookPath(workspaceId),
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  async saveWebhook(workspaceId: string, data: RuntimeWebhookInput): Promise<RuntimeWebhookSaved> {
    const response = await this.request<ApiResponseData<RuntimeWebhookSaved>>(
      'put',
      this.webhookPath(workspaceId),
      data,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  async deleteWebhook(workspaceId: string): Promise<void> {
    await this.request<ApiResponseData<unknown>>(
      'delete',
      this.webhookPath(workspaceId),
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
  }

  async rotateWebhookSecret(workspaceId: string): Promise<string> {
    const response = await this.request<ApiResponseData<{ secret: string }>>(
      'post',
      this.webhookPath(workspaceId, '/rotate-secret'),
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data.secret;
  }

  async testWebhook(workspaceId: string): Promise<RuntimeWebhookDelivery> {
    const response = await this.request<ApiResponseData<RuntimeWebhookDelivery>>(
      'post',
      this.webhookPath(workspaceId, '/test'),
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  async listWebhookDeliveries(
    workspaceId: string,
    page = 1
  ): Promise<RuntimePage<RuntimeWebhookDelivery>> {
    const response = await this.request<ApiResponseData<RuntimePage<RuntimeWebhookDelivery>>>(
      'get',
      this.webhookPath(workspaceId, `/deliveries?limit=20&page=${page}`),
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data ?? { items: [], page, limit: 20, has_more: false };
  }

  async retryWebhookDelivery(workspaceId: string, deliveryId: string): Promise<void> {
    await this.request<ApiResponseData<unknown>>(
      'post',
      this.webhookPath(workspaceId, `/deliveries/${encodeURIComponent(deliveryId)}/retry`),
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
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

  async listEndUserDigitalHumans(
    workspaceId: string,
    subjectId: string
  ): Promise<RuntimeEndUserDigitalHuman[]> {
    const response = await this.request<ApiResponseData<{ items: RuntimeEndUserDigitalHuman[] }>>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}/digital-humans`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data?.items ?? [];
  }

  /** Removes one of a DigitalHuman's notes on the end user (ADR 0015). */
  async deleteEndUserNote(
    workspaceId: string,
    subjectId: string,
    digitalHumanId: string,
    key: string,
    expectedRevision: number
  ): Promise<void> {
    await this.request<ApiResponseData<unknown>>(
      'delete',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}/digital-humans/${encodeURIComponent(digitalHumanId)}/notes/${encodeURIComponent(key)}?expected_revision=${expectedRevision}`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
  }

  /** Makes a note the DigitalHuman's own memory, internal only (ADR 0015). */
  async promoteEndUserNote(
    workspaceId: string,
    subjectId: string,
    digitalHumanId: string,
    key: string,
    expectedRevision: number
  ): Promise<void> {
    await this.request<ApiResponseData<unknown>>(
      'post',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}/digital-humans/${encodeURIComponent(digitalHumanId)}/notes/${encodeURIComponent(key)}/promote?expected_revision=${expectedRevision}`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
  }

  /** Queues an export, or returns the one already queued or running. */
  async requestEndUserExport(
    workspaceId: string,
    subjectId: string
  ): Promise<RuntimeEndUserExportJob> {
    const response = await this.request<ApiResponseData<RuntimeEndUserExportJob>>(
      'post',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}/export`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  async getEndUserExport(
    workspaceId: string,
    subjectId: string,
    exportId: string
  ): Promise<RuntimeEndUserExportJob> {
    const response = await this.request<ApiResponseData<RuntimeEndUserExportJob>>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}/exports/${encodeURIComponent(exportId)}`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data;
  }

  async downloadEndUserExport(
    workspaceId: string,
    subjectId: string,
    exportId: string
  ): Promise<Blob> {
    return this.request<Blob>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}/exports/${encodeURIComponent(exportId)}/download`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId), responseType: 'blob' }
    );
  }

  async eraseEndUser(workspaceId: string, subjectId: string): Promise<void> {
    await this.request<ApiResponseData<unknown>>(
      'delete',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
  }

  async listEndUserEvents(
    workspaceId: string,
    subjectId: string,
    page = 1
  ): Promise<RuntimePage<RuntimeEndUserEvent>> {
    const response = await this.request<ApiResponseData<RuntimePage<RuntimeEndUserEvent>>>(
      'get',
      `/runtime-workspaces/${encodeURIComponent(workspaceId)}/end-users/${encodeURIComponent(subjectId)}/events?limit=30&page=${page}`,
      undefined,
      { headers: this.workspaceHeaders(workspaceId) }
    );
    return response.data ?? { items: [], page, limit: 30, has_more: false };
  }
}

export const runtimeWorkspaceService = new RuntimeWorkspaceService();
