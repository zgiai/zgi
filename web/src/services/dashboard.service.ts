import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  DashboardRecentWork,
  DashboardRecentWorkParams,
  DashboardStats,
} from './types/dashboard';

class DashboardService extends BaseService {
  constructor() {
    super({
      basePath: '/console/api',
      endpoint: 'main',
    });
  }

  /**
   * Get dashboard statistics
   * GET /console/api/dashboard/stats
   */
  getDashboardStats(): Promise<ApiResponseData<DashboardStats>> {
    return this.request('get', '/dashboard/stats', undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Get recently updated console work items.
   * GET /console/api/dashboard/recent-work
   */
  getRecentWork(params?: DashboardRecentWorkParams): Promise<ApiResponseData<DashboardRecentWork>> {
    return this.request('get', '/dashboard/recent-work', undefined, {
      params,
      headers: { 'Content-Type': 'application/json' },
    });
  }
}

export const dashboardService = new DashboardService();
