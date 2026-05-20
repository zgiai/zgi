import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from '@/services/types/common';
import type {
  ApiKeyList,
  ApiKeyDetail,
  GetApiKeysParams,
  CreateApiKeyRequest,
  CreateApiKeyResponse,
  UpdateApiKeyRequest,
  ApiKeyValidateResult,
} from '@/services/types/apikey';

/**
 * API Key service for LLM API key management
 */
class ApiKeyServiceClass extends BaseService {
  constructor() {
    super({ basePath: '/console/api/llm' });
  }

  /**
   * Get list of API keys
   * GET /console/api/llm/api-keys
   */
  getApiKeys(params?: GetApiKeysParams): Promise<ApiResponseData<ApiKeyList>> {
    return this.request('get', '/api-keys', undefined, {
      params,
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Get API key detail by ID
   * GET /console/api/llm/api-keys/{id}
   */
  getApiKey(id: string): Promise<ApiResponseData<ApiKeyDetail>> {
    return this.request('get', `/api-keys/${id}`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Create new API keys
   * POST /console/api/llm/api-keys
   */
  createApiKey(data: CreateApiKeyRequest): Promise<ApiResponseData<CreateApiKeyResponse>> {
    return this.request('post', '/api-keys', data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Update API key by ID
   * PUT /console/api/llm/api-keys/{id}
   */
  updateApiKey(id: string, data: UpdateApiKeyRequest): Promise<ApiResponseData<ApiKeyDetail>> {
    return this.request('put', `/api-keys/${id}`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Delete API key by ID
   * DELETE /console/api/llm/api-keys/{id}
   */
  deleteApiKey(id: string): Promise<ApiResponseData<null>> {
    return this.request('delete', `/api-keys/${id}`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Validate API key
   * POST /console/api/llm/api-keys/validate
   */
  validateApiKey(key: string): Promise<ApiResponseData<ApiKeyValidateResult>> {
    return this.request(
      'post',
      '/api-keys/validate',
      { key },
      {
        headers: { 'Content-Type': 'application/json' },
      }
    );
  }
}

// Export singleton instance
export const apiKeyService = new ApiKeyServiceClass();
