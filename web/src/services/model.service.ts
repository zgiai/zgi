import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  GetModelsParams,
  GetAvailableModelsParams,
  ModelList,
  ModelItem,
  ModelDetail,
  ToggleModelRequest,
  ToggleModelResponse,
  GetModelParametersParams,
  ParameterRuleItem,
  BatchToggleModelsRequest,
  BatchToggleModelsResponse,
  ToggleProviderModelsRequest,
  CreateCustomModelRequest,
  UpdateCustomModelRequest,
  GetCustomModelsParams,
  CustomModelListResponse,
  DefaultModelUseCase,
  DefaultModelRecord,
  ResolvedDefaultModelList,
  UpsertDefaultModelRequest,
} from './types/model';

// model management temporary service
export class ModelService extends BaseService {
  constructor() {
    super({ endpoint: 'main', basePath: '/console/api/llm' });
  }

  // GET /console/api/llm/models
  getModels(params?: GetModelsParams): Promise<ApiResponseData<ModelList>> {
    return this.request('get', `/models`, undefined, { params });
  }

  // GET /console/api/llm/models/{model_id}
  getModel(modelId: string): Promise<ApiResponseData<ModelDetail>> {
    const encoded = encodeURIComponent(modelId);
    return this.request('get', `/models/${encoded}`);
  }

  // GET /console/api/llm/models/available
  getAvailableModels(params?: GetAvailableModelsParams): Promise<ApiResponseData<ModelList>> {
    return this.request('get', `/models/available`, undefined, { params });
  }

  // GET /console/api/llm/default-models
  getDefaultModels(): Promise<ApiResponseData<ResolvedDefaultModelList>> {
    return this.request('get', '/default-models');
  }

  // PUT /console/api/llm/default-models/{use_case}
  upsertDefaultModel(
    useCase: DefaultModelUseCase,
    data: UpsertDefaultModelRequest
  ): Promise<ApiResponseData<DefaultModelRecord>> {
    const encoded = encodeURIComponent(useCase);
    return this.request('put', `/default-models/${encoded}`, data);
  }

  // DELETE /console/api/llm/default-models/{use_case}
  deleteDefaultModel(useCase: DefaultModelUseCase): Promise<ApiResponseData<null>> {
    const encoded = encodeURIComponent(useCase);
    return this.request('delete', `/default-models/${encoded}`);
  }

  // POST /console/api/llm/providers/{provider}/models/toggle
  toggleModel(
    provider: string,
    data: ToggleModelRequest
  ): Promise<ApiResponseData<ToggleModelResponse>> {
    const encoded = encodeURIComponent(provider);
    return this.request('post', `/providers/${encoded}/models/toggle`, data);
  }

  // GET /console/api/llm/models/parameters
  getModelParameters(
    params: GetModelParametersParams
  ): Promise<ApiResponseData<ParameterRuleItem[]>> {
    return this.request('get', `/models/parameters`, undefined, { params });
  }

  // POST /console/api/llm/models/batch/toggle
  batchToggleModels(
    data: BatchToggleModelsRequest
  ): Promise<ApiResponseData<BatchToggleModelsResponse>> {
    return this.request('post', '/models/batch/toggle', data);
  }

  // POST /console/api/llm/models/provider/toggle
  toggleProviderModels(
    data: ToggleProviderModelsRequest
  ): Promise<ApiResponseData<BatchToggleModelsResponse>> {
    return this.request('post', '/models/provider/toggle', data);
  }

  // POST /console/api/llm/models/custom
  createCustomModel(data: CreateCustomModelRequest): Promise<ApiResponseData<ModelItem>> {
    return this.request('post', '/models/custom', data);
  }

  // PUT /console/api/llm/models/custom/{id}
  updateCustomModel(
    id: string,
    data: UpdateCustomModelRequest
  ): Promise<ApiResponseData<ModelItem>> {
    const encoded = encodeURIComponent(id);
    return this.request('put', `/models/custom/${encoded}`, data);
  }

  // DELETE /console/api/llm/models/custom/{id}
  deleteCustomModel(id: string): Promise<ApiResponseData<{ success: boolean }>> {
    const encoded = encodeURIComponent(id);
    return this.request('delete', `/models/custom/${encoded}`);
  }

  // GET /console/api/llm/models/custom
  getCustomModels(
    params?: GetCustomModelsParams
  ): Promise<ApiResponseData<CustomModelListResponse>> {
    return this.request('get', '/models/custom', undefined, { params });
  }
}

export const modelService = new ModelService();
export default modelService;
