import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  ContentParsePlaygroundCompareResponse,
  ContentParseFileRouteProvidersResponse,
  ContentParsePlaygroundPDFRenderResponse,
  ContentParsePlaygroundParseRequest,
  ContentParsePlaygroundParseResponse,
  ContentParsePlaygroundProviderSummaryResponse,
  ContentParsePlaygroundProvidersResponse,
  ContentParsePlaygroundRunsResponse,
  ContentParsePlaygroundSaveResponse,
} from './types/content-parse';

class ContentParseService extends BaseService {
  async listPlaygroundProviders(): Promise<
    ApiResponseData<ContentParsePlaygroundProvidersResponse>
  > {
    return this.request('get', '/console/api/content-parse/playground/providers');
  }

  async listFileRouteProviders(fileName: string): Promise<
    ApiResponseData<ContentParseFileRouteProvidersResponse>
  > {
    const query = new URLSearchParams({ file_name: fileName }).toString();
    return this.request('get', `/console/api/content-parse/file-route/providers?${query}`);
  }

  async parsePlayground(
    payload: ContentParsePlaygroundParseRequest
  ): Promise<ApiResponseData<ContentParsePlaygroundParseResponse>> {
    const formData = this.buildPlaygroundFormData(payload);

    return this.request('post', '/console/api/content-parse/playground/parse', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
      timeout: 180000,
    });
  }

  async savePlayground(
    payload: ContentParsePlaygroundParseRequest
  ): Promise<ApiResponseData<ContentParsePlaygroundSaveResponse>> {
    const formData = this.buildPlaygroundFormData(payload);

    return this.request('post', '/console/api/content-parse/playground/save', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
      timeout: 180000,
    });
  }

  async listPlaygroundRuns(params?: {
    limit?: number;
    sourceHash?: string;
  }): Promise<ApiResponseData<ContentParsePlaygroundRunsResponse>> {
    const searchParams = new URLSearchParams();
    if (params?.limit) {
      searchParams.set('limit', String(params.limit));
    }
    if (params?.sourceHash) {
      searchParams.set('source_hash', params.sourceHash);
    }
    const query = searchParams.toString();
    return this.request(
      'get',
      `/console/api/content-parse/playground/runs${query ? `?${query}` : ''}`
    );
  }

  async getPlaygroundRun(id: string): Promise<ApiResponseData<ContentParsePlaygroundSaveResponse['run']>> {
    return this.request('get', `/console/api/content-parse/playground/runs/${id}`);
  }

  async getPlaygroundShare(
    token: string
  ): Promise<ApiResponseData<ContentParsePlaygroundSaveResponse['run']>> {
    return this.request('get', `/console/api/content-parse/playground/share/${token}`);
  }

  async comparePlaygroundHash(
    sourceHash: string,
    limit = 50
  ): Promise<ApiResponseData<ContentParsePlaygroundCompareResponse>> {
    return this.request(
      'get',
      `/console/api/content-parse/playground/compare/${sourceHash}?limit=${limit}`
    );
  }

  async getPlaygroundProviderSummary(
    limit = 200
  ): Promise<ApiResponseData<ContentParsePlaygroundProviderSummaryResponse>> {
    return this.request(
      'get',
      `/console/api/content-parse/playground/admin/provider-summary?limit=${limit}`
    );
  }

  private buildPlaygroundFormData(payload: ContentParsePlaygroundParseRequest): FormData {
    const formData = new FormData();
    formData.append('file', payload.file);
    formData.append('provider', payload.provider || 'auto');
    formData.append('profile', payload.profile || 'auto');
    formData.append('intent', payload.intent || 'preview');
    if (payload.fresh) {
      formData.append('fresh', 'true');
    }
    if (payload.ocrEngine && payload.ocrEngine !== 'auto') {
      formData.append('ocr_engine', payload.ocrEngine);
    }
    if (payload.parseResult) {
      formData.append('parse_result_json', JSON.stringify(payload.parseResult));
    }
    return formData;
  }

  async renderPDFPlayground(
    file: File,
    maxPages = 20
  ): Promise<ApiResponseData<ContentParsePlaygroundPDFRenderResponse>> {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('max_pages', String(maxPages));

    return this.request('post', '/console/api/content-parse/playground/pdf-render', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
      timeout: 120000,
    });
  }

  async renderSavedRunSourcePreview(
    runId: string,
    maxPages = 20
  ): Promise<ApiResponseData<ContentParsePlaygroundPDFRenderResponse>> {
    return this.request(
      'get',
      `/console/api/content-parse/playground/runs/${runId}/source-preview?max_pages=${maxPages}`,
      undefined,
      {
        timeout: 120000,
      }
    );
  }

  async renderSharedRunSourcePreview(
    token: string,
    maxPages = 20
  ): Promise<ApiResponseData<ContentParsePlaygroundPDFRenderResponse>> {
    return this.request(
      'get',
      `/console/api/content-parse/playground/share/${token}/source-preview?max_pages=${maxPages}`,
      undefined,
      {
        timeout: 120000,
      }
    );
  }
}

export const contentParseService = new ContentParseService();
export default contentParseService;
