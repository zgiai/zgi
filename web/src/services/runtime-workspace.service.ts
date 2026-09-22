import { BaseService } from '@/lib/http';
import type { ApiResponseData } from './types/common';
import type {
  RuntimeAgentCandidateSet,
  RuntimeAppCollection,
  RuntimeCredential,
  RuntimeCredentialCreation,
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
    return response.data;
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
}

export const runtimeWorkspaceService = new RuntimeWorkspaceService();
