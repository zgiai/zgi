import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  GetProvidersParams,
  ProviderList,
  ProviderDetail,
  ToggleProviderRequest,
  ToggleProviderResponse,
  CreateCustomProviderRequest,
  UpdateCustomProviderRequest,
} from './types/provider';

// provider management temporary service
export class ProviderService extends BaseService {
  constructor() {
    super({ endpoint: 'main', basePath: '/console/api/llm' });
  }

  private normalizeProviderListResponse(
    response: ApiResponseData<ProviderList>
  ): ApiResponseData<ProviderList> {
    return {
      ...response,
      data: {
        ...response.data,
        items: response.data.items ?? [],
      },
    };
  }

  // GET /console/api/llm/providers
  async getProviders(params?: GetProvidersParams): Promise<ApiResponseData<ProviderList>> {
    const response = await this.request<ApiResponseData<ProviderList>>(
      'get',
      '/providers',
      undefined,
      { params }
    );
    return this.normalizeProviderListResponse(response);
  }

  // GET /console/api/llm/providers/{provider}
  getProvider(provider: string): Promise<ApiResponseData<ProviderDetail>> {
    const encoded = encodeURIComponent(provider);
    return this.request('get', `/providers/${encoded}`);
  }

  // POST /console/api/llm/providers/toggle
  toggleProvider(data: ToggleProviderRequest): Promise<ApiResponseData<ToggleProviderResponse>> {
    return this.request('post', '/providers/toggle', data);
  }

  // --- Custom Provider Management ---

  // POST /console/api/llm/providers/custom
  createCustomProvider(
    data: CreateCustomProviderRequest
  ): Promise<ApiResponseData<ProviderDetail>> {
    return this.request('post', '/providers/custom', data);
  }

  // GET /console/api/llm/providers/custom
  async getCustomProviders(params?: GetProvidersParams): Promise<ApiResponseData<ProviderList>> {
    const apiParams: GetProvidersParams & { is_active?: boolean } = { ...(params ?? {}) };
    if (apiParams.is_enabled !== undefined) {
      apiParams.is_active = apiParams.is_enabled;
      delete apiParams.is_enabled;
    }
    const response = await this.request<ApiResponseData<ProviderList>>(
      'get',
      '/providers/custom',
      undefined,
      { params: apiParams }
    );
    return this.normalizeProviderListResponse(response);
  }

  // GET /console/api/llm/providers/custom/{id}
  getCustomProvider(id: string): Promise<ApiResponseData<ProviderDetail>> {
    const encoded = encodeURIComponent(id);
    return this.request('get', `/providers/custom/${encoded}`);
  }

  // PUT /console/api/llm/providers/custom/{id}
  updateCustomProvider(
    id: string,
    data: UpdateCustomProviderRequest
  ): Promise<ApiResponseData<ProviderDetail>> {
    const encoded = encodeURIComponent(id);
    const apiData: UpdateCustomProviderRequest & { is_active?: boolean } = { ...data };
    if (apiData.is_enabled !== undefined) {
      apiData.is_active = apiData.is_enabled;
      delete apiData.is_enabled;
    }
    return this.request('put', `/providers/custom/${encoded}`, apiData);
  }

  // DELETE /console/api/llm/providers/custom/{id}
  deleteCustomProvider(id: string): Promise<ApiResponseData<void>> {
    const encoded = encodeURIComponent(id);
    return this.request('delete', `/providers/custom/${encoded}`);
  }
}

export const providerService = new ProviderService();
export default providerService;
