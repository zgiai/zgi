import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  ModelMetaProviderDiffResponse,
  ModelMetaDiffResponse,
  ModelMetaStatusResponse,
  ModelMetaSyncResult,
} from './types/provider';

/**
 * ModelMeta synchronization service
 * Base Path: /console/api/llm/modelmeta
 */
export class ModelMetaService extends BaseService {
  constructor() {
    super({ endpoint: 'main', basePath: '/console/api/llm/modelmeta' });
  }

  /**
   * Retrieves the current upstream/local sync status summary.
   * GET /console/api/llm/modelmeta/status
   */
  getSyncStatus(): Promise<ApiResponseData<ModelMetaStatusResponse>> {
    return this.request('get', '/status');
  }

  /**
   * Retrieves provider-level diff data between upstream and local catalog.
   * GET /console/api/llm/modelmeta/diff/providers
   */
  getProviderDiff(): Promise<ApiResponseData<ModelMetaProviderDiffResponse>> {
    return this.request('get', '/diff/providers');
  }

  /**
   * Synchronizes a specific provider and all its models.
   * POST /console/api/llm/modelmeta/sync-provider-full/:provider
   */
  syncProviderFull(provider: string): Promise<ApiResponseData<ModelMetaSyncResult>> {
    const encoded = encodeURIComponent(provider);
    return this.request('post', `/sync-provider-full/${encoded}`);
  }

  /**
   * Synchronizes models for a specific provider (without updating provider info).
   * POST /console/api/llm/modelmeta/sync/:provider
   */
  syncModels(
    provider: string,
    data?: { models?: string[] }
  ): Promise<ApiResponseData<ModelMetaSyncResult>> {
    const encoded = encodeURIComponent(provider);
    return this.request('post', `/sync/${encoded}`, data);
  }

  /**
   * Compares local models with remote ModelMeta data.
   * GET /console/api/llm/modelmeta/diff/:provider
   */
  getModelDiff(provider: string): Promise<ApiResponseData<ModelMetaDiffResponse>> {
    const encoded = encodeURIComponent(provider);
    return this.request('get', `/diff/${encoded}`);
  }
}

export const modelMetaService = new ModelMetaService();
export default modelMetaService;
