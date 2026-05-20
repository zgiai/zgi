import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type { BuiltinToolsResponse } from './types/tool';

/**
 * ToolService
 * ---------------------------------------------------------------------------
 * Fetches builtin tool providers and their tools for the current workspace.
 * API Reference: GET /console/api/workspaces/current/tools/builtin
 */
class ToolService extends BaseService {
  constructor() {
    super({
      endpoint: 'main',
      basePath: '/console/api',
    });
  }

  /**
   * Get builtin tool providers and tools for the current workspace
   * GET /console/api/workspaces/current/tools/builtin
   */
  getBuiltinTools(): Promise<ApiResponseData<BuiltinToolsResponse>> {
    return this.request('get', '/tools/builtin');
  }
}

export const toolService = new ToolService();
export default toolService;
