import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type { CreateSetupAdminRequest, SystemSetupStatus } from './types/setup';

/**
 * SetupService
 * Handles system initialization endpoints under `/console/api`.
 * Uses unauthenticated requests (skipAuth) to support first-time setup.
 */
class SetupService extends BaseService {
  constructor() {
    super({ endpoint: 'main', basePath: '/console/api' });
  }

  /**
   * Get current system setup status.
   * GET /console/api/setup
   */
  getStatus(language?: string): Promise<ApiResponseData<SystemSetupStatus>> {
    return this.request('get', `/setup`, undefined, {
      skipAuth: true,
      skipErrorHandling: true,
      params: language ? { language } : undefined,
    });
  }

  /**
   * Create system administrator account.
   * POST /console/api/setup
   */
  createAdmin(
    payload: CreateSetupAdminRequest
  ): Promise<ApiResponseData<{ result: 'success' | string }>> {
    return this.request('post', `/setup`, payload, {
      skipAuth: true,
      skipErrorHandling: true,
    });
  }
}

export const setupService = new SetupService();
export default setupService;
