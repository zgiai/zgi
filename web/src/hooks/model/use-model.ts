'use client';

import { useCallback, useEffect, useMemo } from 'react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { InfiniteData } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { modelService } from '@/services/model.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  GetModelsParams,
  ModelList,
  ModelItem,
  ToggleModelRequest,
  ToggleModelResponse,
  ConfigureModelRequest,
  BatchToggleModelsRequest,
  BatchToggleModelsResponse,
  ToggleProviderModelsRequest,
  ModelUseCase,
  AvailableModelUseCase,
  CreateCustomModelRequest,
  UpdateCustomModelRequest,
} from '@/services/types/model';
import { normalizeModel } from '@/utils/model-normalize';
import { useProvider } from '@/hooks/provider/use-provider';
import { NODE_ENV } from '@/lib/config';
import { type ProviderDetail } from '@/services';

import { MODEL_KEYS } from '@/hooks/query-keys';

function isInfiniteModelList(data: unknown): data is InfiniteData<ApiResponseData<ModelList>> {
  return (
    !!data &&
    typeof data === 'object' &&
    'pages' in (data as Record<string, unknown>) &&
    Array.isArray((data as { pages?: unknown }).pages)
  );
}

// Special provider value to fetch all models from all providers
export const ALL_PROVIDERS_MODE = '__all_providers__';

function getErrorDescription(error: unknown): string | undefined {
  if (error instanceof Error) {
    return error.message;
  }

  if (typeof error === 'string') {
    return error;
  }

  if (error && typeof error === 'object' && 'message' in error) {
    const message = (error as { message?: unknown }).message;
    return typeof message === 'string' ? message : undefined;
  }

  return undefined;
}

function showErrorToast(title: string, error: unknown): void {
  const description = getErrorDescription(error);

  if (description) {
    toast.error(title, { description });
    return;
  }

  toast.error(title);
}

// Non-paginated models list for a provider. Does not pass page/limit.
// Pass provider = ALL_PROVIDERS_MODE to fetch all models from all providers.
export function useProviderModelsAll(
  provider?: string,
  options?: { search?: string; limit?: number; is_enabled?: boolean }
): {
  models: ModelItem[];
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const isAllProviders = provider === ALL_PROVIDERS_MODE;
  const t = useT('models');

  const key = useMemo(
    () =>
      MODEL_KEYS.all({
        provider: isAllProviders ? '' : provider || '',
        search: options?.search || '',
        is_enabled: options?.is_enabled,
      }),
    [provider, isAllProviders, options?.search, options?.is_enabled]
  );

  const { data, isLoading, isFetching, error, refetch } = useQuery<ApiResponseData<ModelList>>({
    queryKey: key,
    queryFn: async () => {
      if (!provider) throw new Error('Provider is required');
      // When ALL_PROVIDERS_MODE, don't pass provider param to get all models
      return modelService.getModels({
        provider: isAllProviders ? undefined : provider,
        search: options?.search,
        page_size: options?.limit,
        is_enabled: options?.is_enabled,
      });
    },
    enabled: Boolean(provider),
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  useEffect(() => {
    if (error) {
      showErrorToast(t('messages.loadFailed'), error);
    }
  }, [error, t]);

  return {
    models: (data?.data?.items ?? []).map(normalizeModel),
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch: async () => {
      await refetch();
    },
  };
}

/**
 * Infinite query for all models (all providers) with pagination
 */
export function useAllModelsInfinite(options?: {
  search?: string;
  limit?: number;
  is_enabled?: boolean;
  use_case?: ModelUseCase;
}): {
  models: ModelItem[];
  isLoading: boolean;
  isFetching: boolean;
  hasNextPage: boolean;
  fetchNextPage: () => Promise<void>;
  isFetchingNextPage: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const enabled = options !== undefined;
  const limit = options?.limit ?? 50;
  const search = options?.search;
  const is_enabled = options?.is_enabled;
  const use_case = options?.use_case;

  const key = useMemo(
    () => MODEL_KEYS.allInfinite({ search: search || '', is_enabled, use_case: use_case || '' }),
    [search, is_enabled, use_case]
  );

  const { data, isLoading, isFetching, hasNextPage, fetchNextPage, isFetchingNextPage, refetch } =
    useInfiniteQuery<ApiResponseData<ModelList>>({
      queryKey: key,
      queryFn: async ({ pageParam }) => {
        const page = typeof pageParam === 'number' ? pageParam : 1;
        return modelService.getModels({
          page,
          page_size: limit,
          search,
          is_enabled,
          use_case,
        });
      },
      initialPageParam: 1,
      getNextPageParam: lastPage => {
        const d = lastPage?.data;
        if (!d) return undefined;
        return d.has_more ? d.page + 1 : undefined;
      },
      enabled,
      staleTime: 5 * 60 * 1000,
      gcTime: 30 * 60 * 1000,
      refetchOnWindowFocus: false,
      retry: false,
    });

  const models = useMemo(() => {
    const pages = data?.pages ?? [];
    const items = pages.flatMap(p => p?.data?.items ?? []).map(normalizeModel);
    // Deduplicate by name
    const seen = new Set<string>();
    return items.filter(m => {
      if (seen.has(m.model)) return false;
      seen.add(m.model);
      return true;
    });
  }, [data]);

  const stableFetchNextPage = useCallback(async () => {
    await fetchNextPage();
  }, [fetchNextPage]);

  const stableRefetch = useCallback(async () => {
    await refetch();
  }, [refetch]);

  return {
    models,
    isLoading,
    isFetching,
    hasNextPage: hasNextPage ?? false,
    fetchNextPage: stableFetchNextPage,
    isFetchingNextPage,
    error: null,
    refetch: stableRefetch,
  };
}

/**
 * Non-paginated query for all models across all providers with optional type filter
 */
export function useAllModels(options?: {
  search?: string;
  limit?: number;
  is_enabled?: boolean;
  use_case?: ModelUseCase;
}): {
  models: ModelItem[];
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const t = useT('models');
  const search = options?.search;
  const is_enabled = options?.is_enabled;
  const limit = options?.limit;
  const use_case = options?.use_case;

  const key = useMemo(
    () => MODEL_KEYS.all({ search: search || '', is_enabled, use_case: use_case || '' }),
    [search, is_enabled, use_case]
  );

  const { data, isLoading, isFetching, error, refetch } = useQuery<ApiResponseData<ModelList>>({
    queryKey: key,
    queryFn: async () => modelService.getModels({ search, is_enabled, use_case, page_size: limit }),
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  useEffect(() => {
    if (error) {
      showErrorToast(t('messages.loadFailed'), error);
    }
  }, [error, t]);

  return {
    models: (data?.data?.items ?? []).map(normalizeModel),
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch: async () => {
      await refetch();
    },
  };
}

/** Available models (enabled only) via new API. */
export function useAvailableModels(options?: { use_case?: AvailableModelUseCase }): {
  models: ModelItem[];
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const t = useT('models');
  const use_case = options?.use_case || 'text-chat';

  const key = useMemo(
    () => MODEL_KEYS.available({ use_case: use_case || 'text-chat' }),
    [use_case]
  );

  const { data, isLoading, isFetching, error, refetch } = useQuery<ApiResponseData<ModelList>>({
    queryKey: key,
    queryFn: async () => modelService.getAvailableModels({ use_case }),
    // Non-expiring cache
    staleTime: Number.POSITIVE_INFINITY,
    gcTime: Number.POSITIVE_INFINITY,
    refetchOnWindowFocus: false,
    retry: false,
  });

  const wrappedRefetch = useCallback(async () => {
    const res = await refetch();
    const hasError = Boolean((res as { error?: unknown })?.error);
    if (!hasError) {
      toast.success(t('selector.refreshSuccess'));
    }
  }, [refetch, t]);

  useEffect(() => {
    if (error) {
      showErrorToast(t('messages.loadFailed'), error);
    }
  }, [error, t]);

  return {
    models: (data?.data?.items ?? []).map(normalizeModel),
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch: wrappedRefetch,
  };
}

export interface UseProviderModelsReturn {
  provider?: ProviderDetail;
  models: ModelItem[];
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
}

export function useProviderModels(provider?: string): UseProviderModelsReturn {
  const t = useT('models');
  // Provider detail via provider module
  const detail = useProvider(provider);

  // Models list via new endpoint filtered by provider
  const key = useMemo(() => MODEL_KEYS.list({ provider: provider || '' }), [provider]);
  const { data, isLoading, isFetching, error, refetch } = useQuery<ApiResponseData<ModelList>>({
    queryKey: key,
    queryFn: async () => {
      if (!provider) throw new Error('Provider is required');
      return modelService.getModels({ provider });
    },
    enabled: Boolean(provider),
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  useEffect(() => {
    if (error) {
      showErrorToast(t('messages.loadFailed'), error);
    }
  }, [error, t]);

  return {
    provider: detail.provider,
    models: (data?.data?.items ?? []).map(normalizeModel),
    isLoading: detail.isLoading || isLoading,
    isFetching: detail.isFetching || isFetching,
    error: detail.error || (error ? ((error as { message?: string }).message ?? 'error') : null),
    refetch: async () => {
      await Promise.all([detail.refetch(), refetch()]);
    },
  };
}

export function useToggleModel(): {
  toggleModel: (provider: string, model_name: string, is_enabled: boolean) => Promise<void>;
  isToggling: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('aiProviders');
  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<ToggleModelResponse>,
    unknown,
    ToggleModelRequest & { provider: string },
    {
      previousLists: Array<
        [
          unknown[],
          ApiResponseData<ModelList> | InfiniteData<ApiResponseData<ModelList>> | undefined,
        ]
      >;
    }
  >({
    mutationFn: async ({ provider, model_name, is_enabled }) => {
      return modelService.toggleModel(provider, { model_name, is_enabled });
    },
    onMutate: async ({ provider, model_name, is_enabled }) => {
      await queryClient.cancelQueries({ queryKey: MODEL_KEYS.allRoot });
      const modelsQueries = queryClient.getQueriesData<
        ApiResponseData<ModelList> | InfiniteData<ApiResponseData<ModelList>>
      >({ queryKey: MODEL_KEYS.allRoot });

      const previousLists: Array<
        [
          unknown[],
          ApiResponseData<ModelList> | InfiniteData<ApiResponseData<ModelList>> | undefined,
        ]
      > = modelsQueries.map(([qKey, qData]) => [qKey as unknown[], qData]);

      modelsQueries.forEach(([qKey, qData]) => {
        // Infinite cache
        if (isInfiniteModelList(qData)) {
          const nextPages = qData.pages.map(page => {
            if (!page?.data) return page;
            const nextItems = page.data.items.map(m =>
              m.provider === provider && m.model === model_name ? { ...m, is_enabled } : m
            );
            return {
              ...page,
              data: { ...page.data, items: nextItems },
            } as ApiResponseData<ModelList>;
          });
          queryClient.setQueryData(
            qKey as unknown[],
            {
              ...qData,
              pages: nextPages,
            } as InfiniteData<ApiResponseData<ModelList>>
          );
          return;
        }

        // Paginated cache
        const pageData = qData as ApiResponseData<ModelList> | undefined;
        if (!pageData?.data) return;
        const nextItems = pageData.data.items.map(m =>
          m.provider === provider && m.model === model_name ? { ...m, is_enabled } : m
        );
        queryClient.setQueryData(
          qKey as unknown[],
          {
            ...pageData,
            data: { ...pageData.data, items: nextItems },
          } as ApiResponseData<ModelList>
        );
      });

      return { previousLists };
    },
    onSuccess: () => {
      toast.success(t('modelToggleSuccess'));
    },
    onError: (error, _vars, context) => {
      context?.previousLists.forEach(([qKey, qData]) => {
        queryClient.setQueryData(qKey, qData);
      });
      showErrorToast(t('models.toggleFailed'), error);
    },
    onSettled: () => {
      // Revalidate models queries
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
  });

  const toggleModel = useCallback(
    async (provider: string, model_name: string, is_enabled: boolean) => {
      await mutateAsync({ provider, model_name, is_enabled });
    },
    [mutateAsync]
  );

  return { toggleModel, isToggling: isPending };
}

export function useConfigureModel(): {
  configureModel: (data: ConfigureModelRequest) => Promise<void>;
  isConfiguring: boolean;
} {
  const t = useT('models');
  const queryClient = useQueryClient();
  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<unknown>,
    unknown,
    ConfigureModelRequest
  >({
    mutationFn: async data => modelService.configureModel(data),
    onSuccess: () => {
      toast.success(t('messages.updateSuccess'));
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
    onError: error => {
      showErrorToast(t('messages.updateFailed'), error);
    },
  });

  const configureModel = useCallback(
    async (data: ConfigureModelRequest) => {
      await mutateAsync(data);
    },
    [mutateAsync]
  );

  return { configureModel, isConfiguring: isPending };
}

export function useProviderModelsInfinite(
  provider?: string,
  options?: Partial<GetModelsParams> & { limit?: number; search?: string }
): {
  provider?: ProviderDetail;
  models: ModelItem[];
  total: number;
  isLoading: boolean;
  isFetching: boolean;
  hasNextPage: boolean;
  fetchNextPage: () => Promise<void>;
  isFetchingNextPage: boolean;
  refetch: () => Promise<void>;
} {
  const detail = useProvider(provider);
  const limit = options?.limit ?? 20;
  const input_modalities = options?.input_modalities;
  const output_modalities = options?.output_modalities;
  const use_case = options?.use_case;
  const search = options?.search;

  const key = useMemo(
    () =>
      MODEL_KEYS.list({
        provider: provider || '',
        search: search || '',
        input_modalities: input_modalities || '',
        output_modalities: output_modalities || '',
        use_case: use_case || '',
        limit,
      }),
    [provider, search, input_modalities, output_modalities, use_case, limit]
  );

  const { data, isLoading, isFetching, hasNextPage, fetchNextPage, isFetchingNextPage, refetch } =
    useInfiniteQuery<ApiResponseData<ModelList>>({
      queryKey: key,
      queryFn: async ({ pageParam }) => {
        const page = typeof pageParam === 'number' ? pageParam : 1;
        if (!provider) throw new Error('Provider is required');
        return modelService.getModels({
          provider,
          page,
          page_size: limit,
          search,
          input_modalities,
          output_modalities,
          use_case,
        });
      },
      initialPageParam: 1,
      getNextPageParam: lastPage => {
        const d = lastPage?.data;
        if (!d) return undefined;
        return d.has_more ? d.page + 1 : undefined;
      },
      enabled: Boolean(provider),
      staleTime: 5 * 60 * 1000,
      gcTime: 30 * 60 * 1000,
      refetchOnWindowFocus: false,
      retry: false,
    });

  const models = useMemo(() => {
    const pages = data?.pages ?? [];
    const items = pages.flatMap(p => p?.data?.items ?? []);
    // Deduplicate by ID (backend may return overlapping items between pages)
    const seen = new Map<string, ModelItem>();
    const duplicates: Array<{ duplicate: ModelItem; original: ModelItem }> = [];

    const result = items.map(normalizeModel).filter(m => {
      const existing = seen.get(m.id);
      if (existing) {
        duplicates.push({ duplicate: m, original: existing });
        return false;
      }
      seen.set(m.id, m);
      return true;
    });

    // Log duplicates if any found
    if (NODE_ENV === 'development' && duplicates.length > 0) {
      console.warn(`[useProviderModelsInfinite] Found ${duplicates.length} duplicate model(s)`);
      console.table(
        duplicates.map(d => ({
          'Duplicate ID': d.duplicate.id,
          'Duplicate Name': d.duplicate.model,
          'Original Name': d.original.model,
          'Same Name?': d.duplicate.model === d.original.model,
        }))
      );
    }

    return result;
  }, [data]);

  // Get total from first page
  const total = data?.pages?.[0]?.data?.total ?? 0;

  // Stable callback to avoid infinite scroll effect re-triggering
  const stableFetchNextPage = useCallback(async () => {
    await fetchNextPage();
  }, [fetchNextPage]);

  const stableRefetch = useCallback(async () => {
    await Promise.all([detail.refetch(), refetch()]);
  }, [detail.refetch, refetch]);

  return {
    provider: detail.provider,
    models,
    total,
    isLoading: detail.isLoading || isLoading,
    isFetching: detail.isFetching || isFetching,
    hasNextPage: Boolean(hasNextPage),
    fetchNextPage: stableFetchNextPage,
    isFetchingNextPage,
    refetch: stableRefetch,
  };
}

/**
 * Batch toggle specific models for a provider (optimistic update + toasts)
 */
export function useBatchToggleModels(): {
  toggleBatchModels: (provider: string, modelNames: string[], is_enabled: boolean) => Promise<void>;
  isBatchToggling: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('aiProviders');
  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<BatchToggleModelsResponse>,
    unknown,
    BatchToggleModelsRequest,
    {
      previousLists: Array<
        [
          unknown[],
          ApiResponseData<ModelList> | InfiniteData<ApiResponseData<ModelList>> | undefined,
        ]
      >;
    }
  >({
    mutationFn: async vars => {
      return modelService.batchToggleModels(vars);
    },
    onMutate: async vars => {
      const { provider, models, is_enabled } = vars;
      await queryClient.cancelQueries({ queryKey: MODEL_KEYS.allRoot });
      const modelsQueries = queryClient.getQueriesData<
        ApiResponseData<ModelList> | InfiniteData<ApiResponseData<ModelList>>
      >({ queryKey: MODEL_KEYS.allRoot });

      const previousLists: Array<
        [
          unknown[],
          ApiResponseData<ModelList> | InfiniteData<ApiResponseData<ModelList>> | undefined,
        ]
      > = modelsQueries.map(([qKey, qData]) => [qKey as unknown[], qData]);

      const modelsSet = new Set<string>(models);
      modelsQueries.forEach(([qKey, qData]) => {
        if (isInfiniteModelList(qData)) {
          const nextPages = qData.pages.map(page => {
            if (!page?.data) return page;
            const nextItems = page.data.items.map(m =>
              m.provider === provider && modelsSet.has(m.model) ? { ...m, is_enabled } : m
            );
            return {
              ...page,
              data: { ...page.data, items: nextItems },
            } as ApiResponseData<ModelList>;
          });
          queryClient.setQueryData(
            qKey as unknown[],
            {
              ...qData,
              pages: nextPages,
            } as InfiniteData<ApiResponseData<ModelList>>
          );
          return;
        }

        const pageData = qData as ApiResponseData<ModelList> | undefined;
        if (!pageData?.data) return;
        const nextItems = pageData.data.items.map(m =>
          m.provider === provider && modelsSet.has(m.model) ? { ...m, is_enabled } : m
        );
        queryClient.setQueryData(
          qKey as unknown[],
          {
            ...pageData,
            data: { ...pageData.data, items: nextItems },
          } as ApiResponseData<ModelList>
        );
      });

      return { previousLists };
    },
    onSuccess: () => {
      toast.success(t('models.bulkToggleSuccess'));
    },
    onError: (error, _vars, context) => {
      context?.previousLists.forEach(([qKey, qData]) => {
        queryClient.setQueryData(qKey, qData);
      });
      showErrorToast(t('models.bulkToggleError'), error);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
  });

  const toggleBatchModels = useCallback(
    async (provider: string, modelNames: string[], is_enabled: boolean) => {
      await mutateAsync({ provider, models: modelNames, is_enabled });
    },
    [mutateAsync]
  );

  return { toggleBatchModels, isBatchToggling: isPending };
}

/**
 * Toggle all models of a provider (optimistic update + toasts)
 */
export function useToggleAllProviderModels(): {
  toggleAllModels: (provider: string, is_enabled: boolean) => Promise<void>;
  isTogglingAll: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('aiProviders');
  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<BatchToggleModelsResponse>,
    unknown,
    ToggleProviderModelsRequest,
    {
      previousLists: Array<
        [
          unknown[],
          ApiResponseData<ModelList> | InfiniteData<ApiResponseData<ModelList>> | undefined,
        ]
      >;
    }
  >({
    mutationFn: async vars => {
      return modelService.toggleProviderModels(vars);
    },
    onMutate: async vars => {
      const { provider, is_enabled } = vars;
      await queryClient.cancelQueries({ queryKey: MODEL_KEYS.allRoot });
      const modelsQueries = queryClient.getQueriesData<
        ApiResponseData<ModelList> | InfiniteData<ApiResponseData<ModelList>>
      >({ queryKey: MODEL_KEYS.allRoot });

      const previousLists: Array<
        [
          unknown[],
          ApiResponseData<ModelList> | InfiniteData<ApiResponseData<ModelList>> | undefined,
        ]
      > = modelsQueries.map(([qKey, qData]) => [qKey as unknown[], qData]);

      modelsQueries.forEach(([qKey, qData]) => {
        if (isInfiniteModelList(qData)) {
          const nextPages = qData.pages.map(page => {
            if (!page?.data) return page;
            const nextItems = page.data.items.map(m =>
              m.provider === provider ? { ...m, is_enabled } : m
            );
            return {
              ...page,
              data: { ...page.data, items: nextItems },
            } as ApiResponseData<ModelList>;
          });
          queryClient.setQueryData(
            qKey as unknown[],
            {
              ...qData,
              pages: nextPages,
            } as InfiniteData<ApiResponseData<ModelList>>
          );
          return;
        }

        const pageData = qData as ApiResponseData<ModelList> | undefined;
        if (!pageData?.data) return;
        const nextItems = pageData.data.items.map(m =>
          m.provider === provider ? { ...m, is_enabled } : m
        );
        queryClient.setQueryData(
          qKey as unknown[],
          {
            ...pageData,
            data: { ...pageData.data, items: nextItems },
          } as ApiResponseData<ModelList>
        );
      });

      return { previousLists };
    },
    onSuccess: () => {
      toast.success(t('models.bulkToggleSuccess'));
    },
    onError: (error, _vars, context) => {
      context?.previousLists.forEach(([qKey, qData]) => {
        queryClient.setQueryData(qKey, qData);
      });
      showErrorToast(t('models.bulkToggleError'), error);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
  });

  const toggleAllModels = useCallback(
    async (provider: string, is_enabled: boolean) => {
      await mutateAsync({ provider, is_enabled });
    },
    [mutateAsync]
  );

  return { toggleAllModels, isTogglingAll: isPending };
}

/**
 * Hook to create a custom model
 */
export function useCreateCustomModel(): {
  createCustomModel: (data: CreateCustomModelRequest) => Promise<ModelItem>;
  isCreating: boolean;
} {
  const t = useT('models');
  const queryClient = useQueryClient();
  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<ModelItem>,
    unknown,
    CreateCustomModelRequest
  >({
    mutationFn: async data => modelService.createCustomModel(data),
    onSuccess: () => {
      toast.success(t('messages.createSuccess'));
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
    onError: error => {
      showErrorToast(t('messages.createFailed'), error);
    },
  });

  const createCustomModel = useCallback(
    async (data: CreateCustomModelRequest) => {
      const res = await mutateAsync(data);
      return res.data;
    },
    [mutateAsync]
  );

  return { createCustomModel, isCreating: isPending };
}

/**
 * Hook to update a custom model
 */
export function useUpdateCustomModel(): {
  updateCustomModel: (id: string, data: UpdateCustomModelRequest) => Promise<ModelItem>;
  isUpdating: boolean;
} {
  const t = useT('models');
  const queryClient = useQueryClient();
  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<ModelItem>,
    unknown,
    { id: string; data: UpdateCustomModelRequest }
  >({
    mutationFn: async ({ id, data }) => modelService.updateCustomModel(id, data),
    onSuccess: () => {
      toast.success(t('messages.updateSuccess'));
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
    onError: error => {
      showErrorToast(t('messages.updateFailed'), error);
    },
  });

  const updateCustomModel = useCallback(
    async (id: string, data: UpdateCustomModelRequest) => {
      const res = await mutateAsync({ id, data });
      return res.data;
    },
    [mutateAsync]
  );

  return { updateCustomModel, isUpdating: isPending };
}

/**
 * Hook to delete a custom model
 */
export function useDeleteCustomModel(): {
  deleteCustomModel: (id: string) => Promise<void>;
  isDeleting: boolean;
} {
  const t = useT('models');
  const queryClient = useQueryClient();
  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<{ success: boolean }>,
    unknown,
    string
  >({
    mutationFn: async id => modelService.deleteCustomModel(id),
    onSuccess: () => {
      toast.success(t('messages.deleteSuccess'));
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
    onError: error => {
      showErrorToast(t('messages.deleteFailed'), error);
    },
  });

  const deleteCustomModel = useCallback(
    async (id: string) => {
      await mutateAsync(id);
    },
    [mutateAsync]
  );

  return { deleteCustomModel, isDeleting: isPending };
}
