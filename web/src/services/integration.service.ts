import { BaseService } from '@/lib/http/services';
import { withBasePath } from '@/lib/config';
import type { ApiResponseData } from '@/services/types/common';
import type {
  AIChatIntegrationPreferenceListResponse,
  AcknowledgeIntegrationOAuthRecoveryRequest,
  AcknowledgeIntegrationOAuthRecoveryResponse,
  CreateIntegrationConnectionRequest,
  GetIntegrationExecutionsParams,
  IntegrationActionPolicyResponse,
  IntegrationCatalogResponse,
  IntegrationConnection,
  IntegrationConnectionDeleteImpact,
  IntegrationConnectionGrant,
  IntegrationConnectionGrantListResponse,
  IntegrationConnectionHealthEventListResponse,
  IntegrationConnectionListResponse,
  IntegrationConnectionTestResult,
  IntegrationExecutionListResponse,
  IntegrationOAuthClientConfig,
  IntegrationOAuthClientConfigImpact,
  IntegrationOAuthFlow,
  IntegrationOAuthFlowStatus,
  IntegrationOAuthFlowStartResponse,
  IntegrationOAuthRecoveryStatus,
  IntegrationProviderCapabilities,
  ReplaceAIChatIntegrationPreferencesRequest,
  SaveIntegrationConnectionGrantRequest,
  StartIntegrationOAuthFlowRequest,
  UpdateIntegrationActionPoliciesRequest,
  UpdateIntegrationConnectionRequest,
  UpdateIntegrationOAuthClientConfigRequest,
} from '@/services/types/integration';

type IntegrationConnectionPageFetcher = (
  page: number,
  limit: number
) => Promise<ApiResponseData<IntegrationConnectionListResponse>>;

async function collectAllIntegrationConnectionPages(
  fetchPage: IntegrationConnectionPageFetcher,
  label: string
): Promise<ApiResponseData<IntegrationConnectionListResponse>> {
  const pageSize = 100;
  const maxPages = 100;
  const items: IntegrationConnection[] = [];
  const seenConnectionIds = new Set<string>();
  let firstResponse: ApiResponseData<IntegrationConnectionListResponse> | null = null;

  for (let page = 1; page <= maxPages; page += 1) {
    const response = await fetchPage(page, pageSize);
    firstResponse ??= response;
    const payload = response.data;
    const pageItems = payload.items ?? payload.data ?? [];
    const previousItemCount = items.length;
    for (const item of pageItems) {
      if (seenConnectionIds.has(item.id)) continue;
      seenConnectionIds.add(item.id);
      items.push(item);
    }

    const hasMore =
      payload.has_more ??
      (typeof payload.total === 'number'
        ? items.length < payload.total
        : pageItems.length === pageSize);
    if (!hasMore) {
      const base = firstResponse ?? response;
      return {
        ...base,
        data: {
          ...base.data,
          data: items,
          items,
          page: 1,
          page_size: items.length,
          total: payload.total ?? items.length,
          has_more: false,
        },
      };
    }
    if (pageItems.length === 0 || items.length === previousItemCount) {
      throw new Error(`${label} pagination did not advance`);
    }
  }

  throw new Error(`${label} exceeded the safe pagination limit`);
}

class IntegrationService extends BaseService {
  constructor() {
    super({ basePath: '/console/api', endpoint: 'main' });
  }

  getCatalog(
    audience: 'account' | 'shared' | 'organization' = 'account'
  ): Promise<ApiResponseData<IntegrationCatalogResponse>> {
    return this.request('get', '/integrations/catalog', undefined, { params: { audience } });
  }

  getProviderCapabilities(
    integrationId: string,
    audience: 'account' | 'organization' = 'account'
  ): Promise<ApiResponseData<IntegrationProviderCapabilities>> {
    return this.request(
      'get',
      `/integrations/providers/${encodeURIComponent(integrationId)}/capabilities`,
      undefined,
      { params: { audience } }
    );
  }

  getConnections(params?: {
    integration_id?: string;
    status?: string;
    page?: number;
    limit?: number;
  }): Promise<ApiResponseData<IntegrationConnectionListResponse>> {
    const { limit, ...rest } = params ?? {};
    return this.request('get', '/integrations/connections', undefined, {
      params: { ...rest, page_size: limit },
    });
  }

  getMyConnections(params?: {
    integration_id?: string;
    page?: number;
    limit?: number;
  }): Promise<ApiResponseData<IntegrationConnectionListResponse>> {
    const { limit, ...rest } = params ?? {};
    return this.request('get', '/integrations/my-connections', undefined, {
      params: { ...rest, page_size: limit },
    });
  }

  getAvailableConnections(params?: {
    integration_id?: string;
    page?: number;
    limit?: number;
  }): Promise<ApiResponseData<IntegrationConnectionListResponse>> {
    const { limit, ...rest } = params ?? {};
    return this.request('get', '/integrations/available-connections', undefined, {
      params: { ...rest, page_size: limit },
    });
  }

  getAllConnections(params?: {
    integration_id?: string;
    status?: string;
  }): Promise<ApiResponseData<IntegrationConnectionListResponse>> {
    return collectAllIntegrationConnectionPages(
      (page, limit) => this.getConnections({ ...params, page, limit }),
      'managed integration connections'
    );
  }

  getAllMyConnections(params?: {
    integration_id?: string;
  }): Promise<ApiResponseData<IntegrationConnectionListResponse>> {
    return collectAllIntegrationConnectionPages(
      (page, limit) => this.getMyConnections({ ...params, page, limit }),
      'personal integration connections'
    );
  }

  /**
   * Loads the complete connection set used by the replace-style AIChat
   * preference API. Returning a truncated page here is unsafe: a subsequent
   * save would otherwise interpret omitted-but-valid selections as removals.
   */
  async getAllAvailableConnections(params?: {
    integration_id?: string;
  }): Promise<ApiResponseData<IntegrationConnectionListResponse>> {
    return collectAllIntegrationConnectionPages(
      (page, limit) => this.getAvailableConnections({ ...params, page, limit }),
      'available integration connections'
    );
  }

  getAIChatPreferences(): Promise<ApiResponseData<AIChatIntegrationPreferenceListResponse>> {
    return this.request('get', '/integrations/aichat/preferences');
  }

  replaceAIChatPreferences(
    data: ReplaceAIChatIntegrationPreferencesRequest
  ): Promise<ApiResponseData<AIChatIntegrationPreferenceListResponse>> {
    return this.request('put', '/integrations/aichat/preferences', data);
  }

  private oauthProxyURL(path: string): string {
    const proxyPath = withBasePath(`/console/api/integrations/oauth${path}`);
    if (typeof window === 'undefined') return proxyPath;
    return new URL(proxyPath, window.location.origin).toString();
  }

  startOAuthFlow(
    data: StartIntegrationOAuthFlowRequest
  ): Promise<ApiResponseData<IntegrationOAuthFlowStartResponse>> {
    // The start response installs an HttpOnly browser-binding cookie that is
    // consumed by the provider callback. Keep the whole OAuth flow on the
    // browser-visible Web origin so a Web-only production domain can proxy
    // it to the private API without weakening the browser binding.
    return this.client.post(this.oauthProxyURL('/flows'), data, {
      withCredentials: true,
    });
  }

  getOAuthFlow(flowId: string): Promise<ApiResponseData<IntegrationOAuthFlow>> {
    return this.client.get(this.oauthProxyURL(`/flows/${encodeURIComponent(flowId)}`), {
      withCredentials: true,
    });
  }

  cancelOAuthFlow(
    flowId: string
  ): Promise<ApiResponseData<{ status: Extract<IntegrationOAuthFlowStatus, 'cancelled'> }>> {
    return this.client.post(
      this.oauthProxyURL(`/flows/${encodeURIComponent(flowId)}/cancel`),
      undefined,
      { withCredentials: true }
    );
  }

  getOAuthClientConfig(
    integrationId: string,
    authMethodId: string
  ): Promise<ApiResponseData<IntegrationOAuthClientConfig>> {
    return this.request(
      'get',
      `/integrations/${encodeURIComponent(integrationId)}/oauth-client-configs/${encodeURIComponent(authMethodId)}`
    );
  }

  getOAuthClientConfigImpact(
    integrationId: string,
    authMethodId: string
  ): Promise<ApiResponseData<IntegrationOAuthClientConfigImpact>> {
    return this.request(
      'get',
      `/integrations/${encodeURIComponent(integrationId)}/oauth-client-configs/${encodeURIComponent(authMethodId)}/impact`
    );
  }

  updateOAuthClientConfig(
    integrationId: string,
    authMethodId: string,
    data: UpdateIntegrationOAuthClientConfigRequest
  ): Promise<ApiResponseData<IntegrationOAuthClientConfig>> {
    return this.request(
      'put',
      `/integrations/${encodeURIComponent(integrationId)}/oauth-client-configs/${encodeURIComponent(authMethodId)}`,
      data
    );
  }

  deleteOAuthClientConfig(
    integrationId: string,
    authMethodId: string
  ): Promise<ApiResponseData<{ deleted: boolean }>> {
    return this.request(
      'delete',
      `/integrations/${encodeURIComponent(integrationId)}/oauth-client-configs/${encodeURIComponent(authMethodId)}`
    );
  }

  getOAuthRecovery(): Promise<ApiResponseData<IntegrationOAuthRecoveryStatus>> {
    return this.request('get', '/integrations/oauth-recovery');
  }

  acknowledgeOAuthRecovery(
    operationRef: string,
    data: AcknowledgeIntegrationOAuthRecoveryRequest
  ): Promise<ApiResponseData<AcknowledgeIntegrationOAuthRecoveryResponse>> {
    return this.request(
      'post',
      `/integrations/oauth-recovery/${encodeURIComponent(operationRef)}/acknowledge`,
      data
    );
  }

  getConnection(id: string): Promise<ApiResponseData<IntegrationConnection>> {
    return this.request('get', `/integrations/connections/${id}`);
  }

  createConnection(
    data: CreateIntegrationConnectionRequest
  ): Promise<ApiResponseData<IntegrationConnection>> {
    return this.request('post', '/integrations/connections', data);
  }

  createMyConnection(
    data: CreateIntegrationConnectionRequest
  ): Promise<ApiResponseData<IntegrationConnection>> {
    return this.request('post', '/integrations/my-connections', data);
  }

  updateConnection(
    id: string,
    data: UpdateIntegrationConnectionRequest
  ): Promise<ApiResponseData<IntegrationConnection>> {
    return this.request('patch', `/integrations/connections/${id}`, data);
  }

  updateMyConnection(
    id: string,
    data: UpdateIntegrationConnectionRequest
  ): Promise<ApiResponseData<IntegrationConnection>> {
    return this.request('patch', `/integrations/my-connections/${id}`, data);
  }

  testConnection(id: string): Promise<ApiResponseData<IntegrationConnectionTestResult>> {
    return this.request('post', `/integrations/connections/${id}/test`);
  }

  testMyConnection(id: string): Promise<ApiResponseData<IntegrationConnectionTestResult>> {
    return this.request('post', `/integrations/my-connections/${id}/test`);
  }

  setDefaultConnection(id: string): Promise<ApiResponseData<IntegrationConnection>> {
    return this.request('post', `/integrations/connections/${id}/default`);
  }

  getDeleteImpact(id: string): Promise<ApiResponseData<IntegrationConnectionDeleteImpact>> {
    return this.request('get', `/integrations/connections/${id}/delete-impact`);
  }

  deleteConnection(id: string): Promise<ApiResponseData<null>> {
    return this.request('delete', `/integrations/connections/${id}`);
  }

  deleteMyConnection(id: string): Promise<ApiResponseData<{ deleted: boolean; id: string }>> {
    return this.request('delete', `/integrations/my-connections/${id}`);
  }

  getConnectionGrants(
    connectionId: string
  ): Promise<ApiResponseData<IntegrationConnectionGrantListResponse>> {
    return this.request('get', `/integrations/connections/${connectionId}/grants`);
  }

  createConnectionGrant(
    connectionId: string,
    data: SaveIntegrationConnectionGrantRequest
  ): Promise<ApiResponseData<IntegrationConnectionGrant>> {
    return this.request('post', `/integrations/connections/${connectionId}/grants`, data);
  }

  updateConnectionGrant(
    connectionId: string,
    grantId: string,
    data: SaveIntegrationConnectionGrantRequest
  ): Promise<ApiResponseData<IntegrationConnectionGrant>> {
    return this.request('put', `/integrations/connections/${connectionId}/grants/${grantId}`, data);
  }

  deleteConnectionGrant(
    connectionId: string,
    grantId: string
  ): Promise<ApiResponseData<{ deleted: boolean; id: string }>> {
    return this.request('delete', `/integrations/connections/${connectionId}/grants/${grantId}`);
  }

  getConnectionHealthEvents(
    connectionId: string,
    params?: { page?: number; limit?: number }
  ): Promise<ApiResponseData<IntegrationConnectionHealthEventListResponse>> {
    const { limit, ...rest } = params ?? {};
    return this.request(
      'get',
      `/integrations/connections/${connectionId}/health-events`,
      undefined,
      { params: { ...rest, page_size: limit } }
    );
  }

  getActionPolicies(
    integrationId: string
  ): Promise<ApiResponseData<IntegrationActionPolicyResponse>> {
    return this.request('get', `/integrations/${integrationId}/action-policies`);
  }

  updateActionPolicies(
    integrationId: string,
    data: UpdateIntegrationActionPoliciesRequest
  ): Promise<ApiResponseData<IntegrationActionPolicyResponse>> {
    return this.request('put', `/integrations/${integrationId}/action-policies`, data);
  }

  getExecutions(
    params?: GetIntegrationExecutionsParams
  ): Promise<ApiResponseData<IntegrationExecutionListResponse>> {
    const { limit, ...rest } = params ?? {};
    return this.request('get', '/integrations/executions', undefined, {
      params: { ...rest, page_size: limit },
    });
  }
}

export const integrationService = new IntegrationService();
