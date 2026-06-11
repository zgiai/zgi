import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type { BuiltinToolsResponse } from './types/tool';

/**
 * ToolService
 * ---------------------------------------------------------------------------
 * Fetches tool providers available in the current organization, including
 * system builtin tools and organization-installed plugin-runner tools.
 * API Reference: GET /console/api/tools/builtin
 */
class ToolService extends BaseService {
  constructor() {
    super({
      endpoint: 'main',
      basePath: '/console/api',
    });
  }

  /**
   * Get available tool providers and tools for the current organization.
   * GET /console/api/tools/builtin
   */
  getBuiltinTools(): Promise<ApiResponseData<BuiltinToolsResponse>> {
    return this.request('get', '/tools/builtin');
  }
}

export const toolService = new ToolService();
export default toolService;
