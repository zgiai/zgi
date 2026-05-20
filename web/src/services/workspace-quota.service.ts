import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from '@/services/types/common';
import type {
  WorkspaceQuota,
  WorkspaceQuotaList,
  GetWorkspaceQuotasParams,
  UpdateWorkspaceQuotaRequest,
} from '@/services/types/workspace-quota';

/**
 * Workspace Quota service for managing workspace-level LLM quotas
 */
class WorkspaceQuotaServiceClass extends BaseService {
  constructor() {
    super({ basePath: '/console/api/llm' });
  }

  /**
   * Get paginated list of workspace quotas
   * GET /console/api/llm/workspace-quotas
   */
  getWorkspaceQuotas(params?: GetWorkspaceQuotasParams): Promise<ApiResponseData<WorkspaceQuotaList>> {
    return this.request('get', '/workspace-quotas', undefined, {
      params,
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Get single workspace quota detail
   * GET /console/api/llm/workspace-quotas/{workspace_id}
   */
  getWorkspaceQuota(workspaceId: string): Promise<ApiResponseData<WorkspaceQuota>> {
    return this.request('get', `/workspace-quotas/${workspaceId}`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Update workspace quota configuration
   * PUT /console/api/llm/workspace-quotas/{workspace_id}
   */
  updateWorkspaceQuota(
    workspaceId: string,
    data: UpdateWorkspaceQuotaRequest
  ): Promise<ApiResponseData<WorkspaceQuota>> {
    return this.request('put', `/workspace-quotas/${workspaceId}`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }
}

// Export singleton instance
export const workspaceQuotaService = new WorkspaceQuotaServiceClass();
