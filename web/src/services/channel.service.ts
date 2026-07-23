import { BaseService } from '@/lib/http/services';
import { http } from '@/lib/http';
import type { ApiResponseData } from './types/common';
import type {
  GetChannelsParams,
  ChannelsResponse,
  ChannelDetail,
  UpdateChannelRequest,
  CreateChannelRequest,
  DraftTestChannelModelRequest,
  ChannelModelTestResult,
  DiscoverDraftChannelModelsRequest,
  DiscoverDraftChannelModelsResponse,
  BatchTestChannelModelsRequest,
  BatchTestChannelModelsEvent,
  PlatformChannelsResponse,
  PlatformChannelModelsResponse,
  AdjustChannelWalletRequest,
  AdjustChannelWalletResponse,
  UpstreamState,
  UpdateUpstreamStateSettingsRequest,
} from './types/channel';

// channel management temporary service
export class ChannelService extends BaseService {
  constructor() {
    super({ endpoint: 'main', basePath: '/console/api' });
  }

  // GET /console/api/llm/channels
  getChannels(params?: GetChannelsParams): Promise<ApiResponseData<ChannelsResponse>> {
    const { page_size, ...rest } = params || {};
    return this.request('get', '/llm/channels', undefined, {
      params: { ...rest, page_size },
    });
  }

  // GET /console/api/llm/channels/platform (§3.1.1)
  getPlatformChannels(): Promise<ApiResponseData<PlatformChannelsResponse>> {
    return this.request('get', '/llm/channels/platform');
  }

  // GET /console/api/llm/channels/platform/models
  getPlatformChannelModels(): Promise<ApiResponseData<PlatformChannelModelsResponse>> {
    return this.request('get', '/llm/channels/platform/models');
  }

  // GET /console/api/llm/channels/{id}
  getChannelDetail(id: string): Promise<ApiResponseData<ChannelDetail>> {
    const encoded = encodeURIComponent(id);
    return this.request('get', `/llm/channels/${encoded}`);
  }

  // PUT /console/api/llm/channels/{id}
  updateChannel(id: string, data: UpdateChannelRequest): Promise<ApiResponseData<ChannelDetail>> {
    const encoded = encodeURIComponent(id);
    return this.request('put', `/llm/channels/${encoded}`, data);
  }

  // PUT /console/api/llm/channels/platform (§3.3)
  // New dedicated endpoint for updating official channel settings
  updateOfficialGroupSettings(data: {
    priority?: number;
    weight?: number;
    is_enabled?: boolean;
  }): Promise<ApiResponseData<ChannelDetail>> {
    return this.request('put', '/llm/channels/platform', data);
  }

  // DELETE /console/api/llm/channels/{id}
  deleteChannel(id: string): Promise<ApiResponseData<unknown>> {
    const encoded = encodeURIComponent(id);
    return this.request('delete', `/llm/channels/${encoded}`);
  }

  // POST /console/api/llm/channels
  createChannel(data: CreateChannelRequest): Promise<ApiResponseData<ChannelDetail>> {
    return this.request('post', '/llm/channels', data);
  }

  // POST /console/api/llm/channels/draft/test/model - test a single model before creating a channel
  testDraftChannelModel(
    data: DraftTestChannelModelRequest
  ): Promise<ApiResponseData<ChannelModelTestResult>> {
    return this.request('post', '/llm/channels/draft/test/model', data);
  }

  // POST /console/api/llm/channels/draft/discover-models - list upstream models before creating a channel
  discoverDraftChannelModels(
    data: DiscoverDraftChannelModelsRequest
  ): Promise<ApiResponseData<DiscoverDraftChannelModelsResponse>> {
    return this.request('post', '/llm/channels/draft/discover-models', data);
  }

  // POST /console/api/llm/channels/{id}/test/batch - batch test multiple models (SSE POST)
  batchTestChannelModels(
    id: string,
    data: BatchTestChannelModelsRequest,
    options: {
      onMessage: (event: BatchTestChannelModelsEvent) => void;
      onError?: (error: Error) => void;
      abortSignal?: AbortSignal;
    }
  ): Promise<{ close: () => void }> {
    const encoded = encodeURIComponent(id);
    return http.ssePost<BatchTestChannelModelsRequest>(
      `/console/api/llm/channels/${encoded}/test/batch`,
      {
        body: data,
        abortSignal: options.abortSignal,
        onError: options.onError,
        isTerminalMessage: message => {
          const payload = message.data;
          return (
            typeof payload === 'object' &&
            payload !== null &&
            (payload as { completed?: unknown }).completed === true
          );
        },
        callbacks: {
          onMessage: payload => {
            options.onMessage(payload as unknown as BatchTestChannelModelsEvent);
          },
        },
      }
    );
  }

  // POST /console/api/llm/channels/{channel_id}/wallet/adjust - adjust private channel wallet balance
  adjustChannelWallet(
    channelId: string,
    data: AdjustChannelWalletRequest
  ): Promise<ApiResponseData<AdjustChannelWalletResponse>> {
    const encoded = encodeURIComponent(channelId);
    return this.request('post', `/llm/channels/${encoded}/wallet/adjust`, data);
  }

  checkUpstreamState(channelId: string): Promise<ApiResponseData<UpstreamState>> {
    const encoded = encodeURIComponent(channelId);
    return this.request('post', `/llm/channels/${encoded}/upstream-state/check`);
  }

  retryUpstreamState(channelId: string): Promise<ApiResponseData<UpstreamState>> {
    const encoded = encodeURIComponent(channelId);
    return this.request('post', `/llm/channels/${encoded}/upstream-state/retry`);
  }

  updateUpstreamStateSettings(
    channelId: string,
    data: UpdateUpstreamStateSettingsRequest
  ): Promise<ApiResponseData<UpstreamState>> {
    const encoded = encodeURIComponent(channelId);
    return this.request('put', `/llm/channels/${encoded}/upstream-state/settings`, data);
  }
}

export const channelService = new ChannelService();
export default channelService;
