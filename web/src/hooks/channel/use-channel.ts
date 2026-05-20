'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useT } from '@/i18n';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { channelService } from '@/services/channel.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  GetChannelsParams,
  ChannelsResponse,
  ChannelItem,
  ChannelDetail,
  UpdateChannelRequest,
  CreateChannelRequest,
  BatchTestChannelModelsRequest,
  BatchTestChannelModelsEvent,
  BatchTestModelResult,
  BatchTestCompletedResult,
  UpdateOfficialGroupSettingsRequest,
  PlatformChannelsResponse,
  AdjustChannelWalletRequest,
  AdjustChannelWalletResponse,
} from '@/services/types/channel';
import { CHANNEL_KEYS, MODEL_KEYS } from '@/hooks/query-keys';
import {
  denormalizeAiCreditValue,
  normalizeAdjustChannelWalletResponse,
  normalizeChannelDetail,
  normalizeChannelItem,
} from '@/utils/ai-credits';
import { normalizeToastDescription } from '@/utils/error-notifications';

const channelsKey = (
  params: Required<Pick<GetChannelsParams, 'page_size' | 'page'>> &
    Partial<Pick<GetChannelsParams, 'search' | 'protocol' | 'type' | 'is_active'>>
) =>
  CHANNEL_KEYS.list({
    page_size: params.page_size,
    page: params.page,
    search: params.search ?? undefined,
    protocol: params.protocol ?? undefined,
    type: params.type ?? undefined,
    is_active: params.is_active ?? undefined,
  });

const platformChannelsKey = () => ['channels', 'platform'] as const;

const platformChannelModelsKey = () => ['channels', 'platform', 'models'] as const;

const channelDetailKey = (id: string) => CHANNEL_KEYS.detail(id);

const CHANNEL_LOAD_DUPLICATE_DESCRIPTIONS = ['Failed to load channels', 'Failed to load channel'];

export interface UseChannelsOptions {
  page_size?: number;
  initialPage?: number;
  search?: string;
  protocol?: string;
  type?: 'system' | 'organization';
  is_active?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
}

export interface UseChannelsReturn {
  items: ChannelItem[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
  hasMore: boolean;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  goToPage: (page: number) => void;
  refetch: () => Promise<void>;
}

export function useChannels(options: UseChannelsOptions = {}): UseChannelsReturn {
  const {
    page_size = 20,
    initialPage = 1,
    search,
    protocol,
    type,
    is_active,
    staleTime = 20 * 1000, // 20 seconds for real-time balance
    gcTime = 60 * 1000,
    refetchOnWindowFocus = false,
  } = options;

  const t = useT('channels');
  const [page, setPage] = useState<number>(initialPage);

  // Sync internal page state if initialPage changes (e.g. from URL)
  useEffect(() => {
    if (initialPage !== page) {
      setPage(initialPage);
    }
  }, [initialPage, page]);

  const key = useMemo(
    () => channelsKey({ page_size, page, search, protocol, type, is_active }),
    [page_size, page, search, protocol, type, is_active]
  );

  const { data, isLoading, isFetching, error, refetch } = useQuery<
    ApiResponseData<ChannelsResponse>
  >({
    queryKey: key,
    queryFn: async () => {
      const response = await channelService.getChannels({
        page_size,
        page,
        search,
        protocol,
        type,
        is_active,
      });

      return {
        ...response,
        data: {
          ...response.data,
          data: response.data.data.map(normalizeChannelItem),
        },
      };
    },
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    retry: false,
  });

  useEffect(() => {
    if (!error) return;

    const title = t('messages.loadFailed');
    toast.error(title, {
      description: normalizeToastDescription(title, (error as { message?: string }).message, {
        duplicateDescriptions: CHANNEL_LOAD_DUPLICATE_DESCRIPTIONS,
      }),
    });
  }, [error, t]);

  const resp = data?.data;
  const items = resp?.data ?? [];
  const processedItems: ChannelItem[] = items.map(ch => ({
    ...ch,
    provider: ch.channel_provider || ch.provider || 'unknown',
  }));

  return {
    items: processedItems,
    total: resp?.total ?? 0,
    page,
    page_size,
    total_pages: Math.ceil((resp?.total ?? 0) / (resp?.page_size ?? page_size)),
    hasMore:
      processedItems.length > 0 && (resp?.total ?? 0) > page * (resp?.page_size ?? page_size),
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    goToPage: setPage,
    refetch: async () => {
      await refetch();
    },
  };
}

export function usePlatformChannels() {
  return useQuery({
    queryKey: platformChannelsKey(),
    queryFn: async () => {
      const resp = await channelService.getPlatformChannels();
      const data = resp.data;
      if (data) {
        resp.data = {
          ...data,
        };
      }
      return resp;
    },
    staleTime: 5 * 60 * 1000,
  });
}

export function usePlatformChannelModels(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: platformChannelModelsKey(),
    queryFn: async () => {
      return channelService.getPlatformChannelModels();
    },
    staleTime: 30 * 60 * 1000, // Models change less frequently
    enabled: options?.enabled,
  });
}

export function useChannel(id?: string): {
  channel: ChannelDetail | undefined;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const t = useT('channels');
  const key = useMemo(() => channelDetailKey(id || ''), [id]);
  const { data, isLoading, isFetching, error, refetch } = useQuery<ApiResponseData<ChannelDetail>>({
    queryKey: key,
    queryFn: async () => {
      if (!id) throw new Error('Channel id is required');
      const response = await channelService.getChannelDetail(id);
      return {
        ...response,
        data: normalizeChannelDetail(response.data),
      };
    },
    enabled: Boolean(id),
    staleTime: 20 * 1000, // 20 seconds for real-time balance
    gcTime: 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  useEffect(() => {
    if (!error) return;

    const title = t('messages.loadFailed');
    toast.error(title, {
      description: normalizeToastDescription(title, (error as { message?: string }).message, {
        duplicateDescriptions: CHANNEL_LOAD_DUPLICATE_DESCRIPTIONS,
      }),
    });
  }, [error, t]);

  return {
    channel: data?.data,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch: async () => {
      await refetch();
    },
  };
}

export function useUpdateChannel(): {
  updateChannel: (id: string, data: UpdateChannelRequest) => Promise<void>;
  isUpdating: boolean;
} {
  const t = useT('channels');
  const queryClient = useQueryClient();
  const { mutateAsync, isPending } = useMutation({
    mutationFn: async ({ id, data }: { id: string; data: UpdateChannelRequest }) => {
      const response = await channelService.updateChannel(id, data);
      return {
        ...response,
        data: normalizeChannelDetail(response.data),
      };
    },
    onMutate: async ({ id, data }) => {
      // Optimistic update for list and detail
      await queryClient.cancelQueries({ queryKey: CHANNEL_KEYS.all });
      const channelsQueries = queryClient.getQueriesData<ApiResponseData<ChannelsResponse>>({
        queryKey: CHANNEL_KEYS.all,
      });

      const previousLists = channelsQueries.map(([qKey, qData]) => [qKey, qData]) as Array<
        [unknown, ApiResponseData<ChannelsResponse> | undefined]
      >;

      channelsQueries.forEach(([qKey, qData]) => {
        if (!qData?.data) return;
        const resp = qData.data;
        const nextList = Array.isArray(resp.data)
          ? resp.data.map(ch => (ch.id === id ? ({ ...ch, ...data } as ChannelItem) : ch))
          : resp.data;
        queryClient.setQueryData(qKey, {
          ...qData,
          data: { ...resp, data: nextList },
        } as ApiResponseData<ChannelsResponse>);
      });

      const dKey = channelDetailKey(id);
      const prevDetail = queryClient.getQueryData<ApiResponseData<ChannelDetail>>(dKey);
      if (prevDetail?.data) {
        queryClient.setQueryData(dKey, {
          ...prevDetail,
          data: { ...prevDetail.data, ...data },
        } as ApiResponseData<ChannelDetail>);
      }

      return { previousLists, prevDetail } as const;
    },
    onSuccess: res => {
      toast.success(t('messages.updateSuccess'), {
        description: res?.message,
      });
      queryClient.invalidateQueries({ queryKey: CHANNEL_KEYS.all });
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
    onError: (error, { id }, context) => {
      if (context) {
        const { previousLists, prevDetail } = context as unknown as {
          previousLists: Array<[unknown, ApiResponseData<ChannelsResponse> | undefined]>;
          prevDetail?: ApiResponseData<ChannelDetail>;
        };
        previousLists.forEach(([qKey, qData]) => {
          queryClient.setQueryData(qKey as unknown[], qData);
        });
        if (prevDetail) queryClient.setQueryData(channelDetailKey(id), prevDetail);
      }
      const title = t('messages.updateFailed');
      toast.error(title, {
        description: normalizeToastDescription(title, (error as { message?: string }).message, {
          duplicateDescriptions: ['Failed to update channel'],
        }),
      });
    },
  });

  const updateChannel = useCallback(
    async (id: string, data: UpdateChannelRequest) => {
      await mutateAsync({ id, data });
    },
    [mutateAsync]
  );

  return { updateChannel, isUpdating: isPending };
}

export function useUpdateOfficialChannelSettings(): {
  updateOfficialSettings: (data: UpdateOfficialGroupSettingsRequest) => Promise<void>;
  isUpdating: boolean;
} {
  const t = useT('channels');
  const queryClient = useQueryClient();
  const { mutateAsync, isPending } = useMutation({
    mutationFn: async (data: UpdateOfficialGroupSettingsRequest) => {
      return channelService.updateOfficialGroupSettings(data);
    },
    onMutate: async data => {
      await queryClient.cancelQueries({ queryKey: platformChannelsKey() });
      const prevPlatform =
        queryClient.getQueryData<ApiResponseData<PlatformChannelsResponse>>(platformChannelsKey());

      if (prevPlatform?.data) {
        queryClient.setQueryData(platformChannelsKey(), {
          ...prevPlatform,
          data: {
            ...prevPlatform.data,
            ...data,
          },
        });
      }

      return { prevPlatform } as const;
    },
    onSuccess: res => {
      toast.success(t('messages.updateSuccess'), {
        description: res?.message,
      });
      queryClient.invalidateQueries({ queryKey: platformChannelsKey() });
      // Also invalidate channel list as it might show the official channel status
      queryClient.invalidateQueries({ queryKey: CHANNEL_KEYS.all });
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
    onError: (_error, _variables, context) => {
      if (context?.prevPlatform) {
        queryClient.setQueryData(platformChannelsKey(), context.prevPlatform);
      }
      toast.error(t('messages.updateFailed'));
    },
  });

  const updateOfficialSettings = useCallback(
    async (data: UpdateOfficialGroupSettingsRequest) => {
      await mutateAsync(data);
    },
    [mutateAsync]
  );

  return { updateOfficialSettings, isUpdating: isPending };
}

export function useDeleteChannel(): {
  deleteChannel: (id: string) => Promise<void>;
  isDeleting: boolean;
} {
  const t = useT('channels');
  const queryClient = useQueryClient();
  const { mutateAsync, isPending } = useMutation({
    mutationFn: async ({ id }: { id: string }) => channelService.deleteChannel(id),
    onMutate: async ({ id }) => {
      await queryClient.cancelQueries({ queryKey: CHANNEL_KEYS.all });
      const channelsQueries = queryClient.getQueriesData<ApiResponseData<ChannelsResponse>>({
        queryKey: CHANNEL_KEYS.all,
      });

      const previousLists = channelsQueries.map(([qKey, qData]) => [qKey, qData]) as Array<
        [unknown, ApiResponseData<ChannelsResponse> | undefined]
      >;

      channelsQueries.forEach(([qKey, qData]) => {
        if (!qData?.data) return;
        const resp = qData.data;
        const nextList = Array.isArray(resp.data)
          ? resp.data.filter(ch => ch.id !== id)
          : resp.data;
        queryClient.setQueryData(qKey, {
          ...qData,
          data: { ...resp, data: nextList },
        } as ApiResponseData<ChannelsResponse>);
      });

      const dKey = channelDetailKey(id);
      const prevDetail = queryClient.getQueryData<ApiResponseData<ChannelDetail>>(dKey);
      queryClient.removeQueries({ queryKey: dKey });

      return { previousLists, prevDetail } as const;
    },
    onSuccess: res => {
      toast.success(t('messages.deleteSuccess'), {
        description: res?.message,
      });
      queryClient.invalidateQueries({ queryKey: CHANNEL_KEYS.all });
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
    onError: (error, { id }, context) => {
      if (context) {
        const { previousLists, prevDetail } = context as unknown as {
          previousLists: Array<[unknown, ApiResponseData<ChannelsResponse> | undefined]>;
          prevDetail?: ApiResponseData<ChannelDetail>;
        };
        previousLists.forEach(([qKey, qData]) => {
          queryClient.setQueryData(qKey as unknown[], qData);
        });
        if (prevDetail) queryClient.setQueryData(channelDetailKey(id), prevDetail);
      }
      const title = t('messages.deleteFailed');
      toast.error(title, {
        description: normalizeToastDescription(title, (error as { message?: string }).message, {
          duplicateDescriptions: ['Failed to delete channel'],
        }),
      });
    },
  });

  const deleteChannel = useCallback(
    async (id: string) => {
      await mutateAsync({ id });
    },
    [mutateAsync]
  );

  return { deleteChannel, isDeleting: isPending };
}

export function useCreateChannel(): {
  createChannel: (data: CreateChannelRequest) => Promise<void>;
  isCreating: boolean;
} {
  const t = useT('channels');
  const queryClient = useQueryClient();
  const { mutateAsync, isPending } = useMutation({
    mutationFn: async (data: CreateChannelRequest) => {
      const payload: CreateChannelRequest = {
        ...data,
        initial_funds: denormalizeAiCreditValue(data.initial_funds) ?? undefined,
      };
      const response = await channelService.createChannel(payload);
      return {
        ...response,
        data: normalizeChannelDetail(response.data),
      };
    },
    onMutate: async data => {
      await queryClient.cancelQueries({ queryKey: CHANNEL_KEYS.all });
      const channelsQueries = queryClient.getQueriesData<ApiResponseData<ChannelsResponse>>({
        queryKey: CHANNEL_KEYS.all,
      });

      const previousLists = channelsQueries.map(([qKey, qData]) => [qKey, qData]) as Array<
        [unknown, ApiResponseData<ChannelsResponse> | undefined]
      >;

      channelsQueries.forEach(([qKey, qData]) => {
        if (!qData?.data) return;
        const optimistic: ChannelItem = {
          id: `temp-${Date.now()}`,
          is_official: false,
          name: data.name,
          provider: data.channel_provider,
          channel_provider: data.channel_provider,
          supported_protocols: [],
          models: data.models ?? [],
          priority: data.priority ?? 100,
          weight: data.weight ?? 100,
          is_enabled: true,
          api_base_url: data.api_base_url ?? '',
          api_key_masked: '',
          remaining_funds: data.initial_funds ?? 0,
          created_at: Date.now() / 1000,
          updated_at: Date.now() / 1000,
        };

        const resp = qData.data;
        const nextList = Array.isArray(resp.data) ? [optimistic, ...resp.data] : [optimistic];
        queryClient.setQueryData(qKey, {
          ...qData,
          data: { ...resp, data: nextList },
        } as ApiResponseData<ChannelsResponse>);
      });

      return { previousLists } as const;
    },
    onSuccess: res => {
      toast.success(t('messages.createSuccess'), {
        description: res?.message,
      });
      queryClient.invalidateQueries({ queryKey: CHANNEL_KEYS.all });
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
    onError: (error, _data, context) => {
      if (context) {
        const { previousLists } = context as unknown as {
          previousLists: Array<[unknown, ApiResponseData<ChannelsResponse> | undefined]>;
        };
        previousLists.forEach(([qKey, qData]) => {
          queryClient.setQueryData(qKey as unknown[], qData);
        });
      }
      const title = t('messages.createFailed');
      toast.error(title, {
        description: normalizeToastDescription(title, (error as { message?: string }).message, {
          duplicateDescriptions: ['Failed to create channel'],
        }),
      });
    },
  });

  const createChannel = useCallback(
    async (data: CreateChannelRequest) => {
      await mutateAsync(data);
    },
    [mutateAsync]
  );

  return { createChannel, isCreating: isPending };
}

/**
 * Hook for batch testing channel models via SSE
 */
export interface UseBatchTestChannelModelsOptions {
  onResult?: (result: BatchTestModelResult) => void;
  onComplete?: (result: BatchTestCompletedResult) => void;
  onError?: (error: Error) => void;
}

export interface UseBatchTestChannelModelsReturn {
  batchTest: (id: string, request: BatchTestChannelModelsRequest) => void;
  abort: () => void;
  isRunning: boolean;
  results: BatchTestModelResult[];
  completedResult: BatchTestCompletedResult | null;
}

export function useBatchTestChannelModels(
  options?: UseBatchTestChannelModelsOptions
): UseBatchTestChannelModelsReturn {
  const t = useT('channels');
  const [isRunning, setIsRunning] = useState(false);
  const [results, setResults] = useState<BatchTestModelResult[]>([]);
  const [completedResult, setCompletedResult] = useState<BatchTestCompletedResult | null>(null);
  const [abortController, setAbortController] = useState<AbortController | null>(null);

  const abort = useCallback(() => {
    abortController?.abort();
    setAbortController(null);
    setIsRunning(false);
  }, [abortController]);

  const batchTest = useCallback(
    (id: string, request: BatchTestChannelModelsRequest) => {
      // Abort any existing test
      abortController?.abort();

      // Reset state
      setResults([]);
      setCompletedResult(null);
      setIsRunning(true);

      const controller = new AbortController();
      setAbortController(controller);

      channelService.batchTestChannelModels(id, request, {
        onMessage: (event: BatchTestChannelModelsEvent) => {
          if ('completed' in event && event.completed) {
            setCompletedResult(event);
            setIsRunning(false);
            options?.onComplete?.(event);
          } else {
            const result = event as BatchTestModelResult;
            setResults(prev => [...prev, result]);
            options?.onResult?.(result);
          }
        },
        onError: (error: Error) => {
          setIsRunning(false);
          const title = t('connectivityTest.toast.error');
          toast.error(title, {
            description: normalizeToastDescription(title, error.message),
          });
          options?.onError?.(error);
        },
        abortSignal: controller.signal,
      });
    },
    [abortController, options, t]
  );

  return {
    batchTest,
    abort,
    isRunning,
    results,
    completedResult,
  };
}

/**
 * Hook for adjusting private channel wallet balance
 */
export function useAdjustChannelWallet(): {
  adjustWallet: (
    channelId: string,
    data: AdjustChannelWalletRequest
  ) => Promise<AdjustChannelWalletResponse | undefined>;
  isAdjusting: boolean;
} {
  const t = useT('channels');
  const queryClient = useQueryClient();

  const { mutateAsync, isPending } = useMutation({
    mutationFn: async ({
      channelId,
      data,
    }: {
      channelId: string;
      data: AdjustChannelWalletRequest;
    }) => {
      const payload: AdjustChannelWalletRequest = {
        ...data,
        amount: denormalizeAiCreditValue(data.amount) ?? 0,
      };
      const response = await channelService.adjustChannelWallet(channelId, payload);
      return {
        ...response,
        data: normalizeAdjustChannelWalletResponse(response.data),
      };
    },
    onSuccess: (res, { channelId }) => {
      toast.success(t('walletAdjust.success'));
      // Invalidate channel list and detail to refresh balance
      queryClient.invalidateQueries({ queryKey: CHANNEL_KEYS.all });
      queryClient.invalidateQueries({ queryKey: CHANNEL_KEYS.detail(channelId) });
    },
    onError: (error: Error & { response?: { status?: number } }) => {
      // Handle 403 permission error specifically
      if (error.response?.status === 403) {
        toast.error(t('walletAdjust.permissionDenied'));
      } else {
        const title = t('walletAdjust.error');
        toast.error(title, {
          description: normalizeToastDescription(title, error.message),
        });
      }
    },
  });

  const adjustWallet = useCallback(
    async (channelId: string, data: AdjustChannelWalletRequest) => {
      const result = await mutateAsync({ channelId, data });
      return result?.data;
    },
    [mutateAsync]
  );

  return { adjustWallet, isAdjusting: isPending };
}
