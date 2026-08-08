import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  GetModelUsageParams,
  ModelUsageData,
  GetWorkspaceQuotaParams,
  WorkspaceQuotaData,
  GetInvocationLogParams,
  InvocationLogData,
  InvocationContentSettings,
  InvocationContentDetail,
} from './types/statistics';

/**
 * StatisticsService
 * Handles LLM usage statistics endpoints under `/console/api/llm/statistics`.
 */
class StatisticsService extends BaseService {
  constructor() {
    super({ endpoint: 'main', basePath: '/console/api/llm/statistics' });
  }

  /**
   * Get model usage statistics for the current organization.
   * GET /console/api/llm/statistics/model-usage
   */
  getModelUsage(params: GetModelUsageParams): Promise<ApiResponseData<ModelUsageData>> {
    return this.request('get', '/model-usage', undefined, {
      params,
    });
  }

  getInvocationLog(params: GetInvocationLogParams): Promise<ApiResponseData<InvocationLogData>> {
    return this.request('get', '/invocations', undefined, { params });
  }

  getInvocationContentSettings(): Promise<ApiResponseData<InvocationContentSettings>> {
    return this.request('get', '/invocation-content/settings');
  }

  updateInvocationContentSettings(
    enabled: boolean
  ): Promise<ApiResponseData<InvocationContentSettings>> {
    return this.request('put', '/invocation-content/settings', { enabled });
  }

  getInvocationContent(
    invocationId: string
  ): Promise<ApiResponseData<InvocationContentDetail>> {
    return this.request('get', `/invocations/${encodeURIComponent(invocationId)}/content`);
  }

  /**
   * Get workspace quota statistics.
   * GET /console/api/llm/statistics/workspace-quota
   */
  getWorkspaceQuota(
    params?: GetWorkspaceQuotaParams
  ): Promise<ApiResponseData<WorkspaceQuotaData>> {
    return this.request('get', '/workspace-quota', undefined, {
      params,
    });
  }
}

export const statisticsService = new StatisticsService();
export default statisticsService;
