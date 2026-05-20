'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { providerService } from '@/services/provider.service';
import type {
  ProviderList,
  ProviderItem,
  ToggleProviderRequest,
  ProviderDetail,
  CreateCustomProviderRequest,
  UpdateCustomProviderRequest,
} from '@/services/types/provider';
import type { ApiResponseData } from '@/services/types/common';

import { PROVIDER_KEYS, MODEL_KEYS } from '@/hooks/query-keys';

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

export interface UseProvidersOptions {
  is_enabled?: boolean;
  limit?: number;
  initialPage?: number;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
}

export interface UseProvidersReturn {
  items: ProviderItem[];
  total: number;
  page: number;
  limit: number;
  hasMore: boolean;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  goToPage: (page: number) => void;
  refetch: () => Promise<void>;
}

export function useProviders(options: UseProvidersOptions = {}): UseProvidersReturn {
  const {
    is_enabled,
    limit = 100,
    initialPage = 1,
    staleTime = 5 * 60 * 1000,
    gcTime = 30 * 60 * 1000,
    refetchOnWindowFocus = false,
  } = options;
  const [page, setPage] = useState<number>(initialPage);
  const t = useT('aiProviders');

  const key = useMemo(
    () => PROVIDER_KEYS.list({ limit, page, is_enabled }),
    [limit, page, is_enabled]
  );

  const { data, isLoading, isFetching, error, refetch } = useQuery<ApiResponseData<ProviderList>>({
    queryKey: key,
    queryFn: async () => providerService.getProviders({ limit, page, is_enabled }),
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    retry: false,
  });

  useEffect(() => {
    if (!error) return;

    showErrorToast(t('loadError'), error);
  }, [error, t]);

  const list = data?.data;

  return {
    items: list?.items ?? [],
    total: list?.total ?? 0,
    page: list?.page ?? page,
    limit: list?.limit ?? limit,
    hasMore: list?.has_more ?? false,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    goToPage: setPage,
    refetch: async () => {
      await refetch();
    },
  };
}

export function useCustomProviders(options: UseProvidersOptions = {}): UseProvidersReturn {
  const {
    is_enabled,
    limit = 100,
    initialPage = 1,
    staleTime = 5 * 60 * 1000,
    gcTime = 30 * 60 * 1000,
    refetchOnWindowFocus = false,
  } = options;
  const [page, setPage] = useState<number>(initialPage);
  const t = useT('aiProviders');

  const key = useMemo(
    () => ['CUSTOM_PROVIDER', limit, page, is_enabled],
    [limit, page, is_enabled]
  );

  const { data, isLoading, isFetching, error, refetch } = useQuery<ApiResponseData<ProviderList>>({
    queryKey: key,
    queryFn: async () => providerService.getCustomProviders({ limit, page, is_enabled }),
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    retry: false,
  });

  useEffect(() => {
    if (!error) return;

    showErrorToast(t('custom.messages.loadFailed'), error);
  }, [error, t]);

  const list = data?.data;

  return {
    items: list?.items ?? [],
    total: list?.total ?? 0,
    page: list?.page ?? page,
    limit: list?.limit ?? limit,
    hasMore: list?.has_more ?? false,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    goToPage: setPage,
    refetch: async () => {
      await refetch();
    },
  };
}

export function useProvider(provider?: string): {
  provider: ProviderDetail | undefined;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const t = useT('aiProviders');
  const key = useMemo(() => PROVIDER_KEYS.detail(provider || ''), [provider]);
  const { data, isLoading, isFetching, error, refetch } = useQuery<ApiResponseData<ProviderDetail>>(
    {
      queryKey: key,
      queryFn: async () => {
        if (!provider) throw new Error('Provider is required');
        return providerService.getProvider(provider);
      },
      enabled: Boolean(provider),
      staleTime: 5 * 60 * 1000,
      gcTime: 30 * 60 * 1000,
      refetchOnWindowFocus: false,
      retry: false,
    }
  );

  useEffect(() => {
    if (!error) return;

    showErrorToast(t('loadProviderError'), error);
  }, [error, t]);

  return {
    provider: data?.data,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch: async () => {
      await refetch();
    },
  };
}

export function useToggleProvider(): {
  toggleProvider: (provider: string, is_enabled: boolean) => Promise<void>;
  isToggling: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('aiProviders');
  const { mutateAsync, isPending } = useMutation({
    mutationFn: async ({ provider, is_enabled }: ToggleProviderRequest) => {
      return providerService.toggleProvider({ provider, is_enabled });
    },
    onMutate: async ({ provider, is_enabled }) => {
      // Optimistically toggle in list AND detail
      await queryClient.cancelQueries({ queryKey: PROVIDER_KEYS.all });

      const providersQueries = queryClient.getQueriesData<ApiResponseData<ProviderList>>({
        queryKey: PROVIDER_KEYS.all,
      });

      const previousProviders = providersQueries.map(([qKey, qData]) => [qKey, qData]) as Array<
        [unknown, ApiResponseData<ProviderList> | undefined]
      >;

      providersQueries.forEach(([qKey, qData]) => {
        if (!qData?.data) return;
        const nextItems = qData.data.items.map(it =>
          it.provider === provider ? { ...it, is_enabled } : it
        );
        queryClient.setQueryData(qKey, {
          ...qData,
          data: { ...qData.data, items: nextItems },
        } as ApiResponseData<ProviderList>);
      });

      const detailKey = PROVIDER_KEYS.detail(provider);
      const detail = queryClient.getQueryData<ApiResponseData<ProviderDetail>>(detailKey);
      if (detail?.data) {
        queryClient.setQueryData(detailKey, {
          ...detail,
          data: { ...detail.data, is_enabled },
        } as ApiResponseData<ProviderDetail>);
      }

      return { previousProviders, previousDetail: detail } as const;
    },
    onSuccess: (_res, { provider }) => {
      toast.success(t('toggleSuccess'));
      // Invalidate to re-sync
      queryClient.invalidateQueries({ queryKey: PROVIDER_KEYS.all });
      queryClient.invalidateQueries({ queryKey: PROVIDER_KEYS.detail(provider) });
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
    onError: (error, { provider }, context) => {
      // Rollback
      if (context) {
        const { previousProviders, previousDetail } = context as unknown as {
          previousProviders: Array<[unknown, ApiResponseData<ProviderList> | undefined]>;
          previousDetail?: ApiResponseData<ProviderDetail>;
        };
        previousProviders.forEach(([qKey, qData]) => {
          queryClient.setQueryData(qKey as unknown[], qData);
        });
        if (previousDetail) {
          queryClient.setQueryData(PROVIDER_KEYS.detail(provider), previousDetail);
        }
      }
      showErrorToast(t('errors.toggleProvider'), error);
    },
  });

  const toggleProvider = useCallback(
    async (provider: string, is_enabled: boolean) => {
      await mutateAsync({ provider, is_enabled });
    },
    [mutateAsync]
  );

  return { toggleProvider, isToggling: isPending };
}

export function useCreateCustomProvider(): {
  createCustomProvider: (data: CreateCustomProviderRequest) => Promise<void>;
  isCreating: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('aiProviders');
  const { mutateAsync, isPending } = useMutation({
    mutationFn: (data: CreateCustomProviderRequest) => providerService.createCustomProvider(data),
    onSuccess: () => {
      toast.success(t('custom.messages.createSuccess'));
      queryClient.invalidateQueries({ queryKey: PROVIDER_KEYS.all });
      queryClient.invalidateQueries({ queryKey: ['CUSTOM_PROVIDER'] });
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
    onError: (error: unknown) => {
      showErrorToast(t('custom.messages.createFailed'), error);
    },
  });

  return {
    createCustomProvider: async (data: CreateCustomProviderRequest) => {
      await mutateAsync(data);
    },
    isCreating: isPending,
  };
}

export function useUpdateCustomProvider(): {
  updateCustomProvider: (id: string, data: UpdateCustomProviderRequest) => Promise<void>;
  isUpdating: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('aiProviders');
  const { mutateAsync, isPending } = useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateCustomProviderRequest }) =>
      providerService.updateCustomProvider(id, data),
    onSuccess: (_, { id }) => {
      toast.success(t('custom.messages.updateSuccess'));
      queryClient.invalidateQueries({ queryKey: PROVIDER_KEYS.all });
      queryClient.invalidateQueries({ queryKey: ['CUSTOM_PROVIDER'] });
      queryClient.invalidateQueries({ queryKey: PROVIDER_KEYS.detail(id) });
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
    onError: (error: unknown) => {
      showErrorToast(t('custom.messages.updateFailed'), error);
    },
  });

  return {
    updateCustomProvider: useCallback(
      async (id: string, data: UpdateCustomProviderRequest) => {
        await mutateAsync({ id, data });
      },
      [mutateAsync]
    ),
    isUpdating: isPending,
  };
}

export function useDeleteCustomProvider(): {
  deleteCustomProvider: (id: string) => Promise<void>;
  isDeleting: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('aiProviders');
  const { mutateAsync, isPending } = useMutation({
    mutationFn: (id: string) => providerService.deleteCustomProvider(id),
    onSuccess: () => {
      toast.success(t('custom.messages.deleteSuccess'));
      queryClient.invalidateQueries({ queryKey: PROVIDER_KEYS.all });
      queryClient.invalidateQueries({ queryKey: ['CUSTOM_PROVIDER'] });
      queryClient.invalidateQueries({ queryKey: MODEL_KEYS.allRoot });
    },
    onError: (error: unknown) => {
      showErrorToast(t('custom.messages.deleteFailed'), error);
    },
  });

  return {
    deleteCustomProvider: async (id: string) => {
      await mutateAsync(id);
    },
    isDeleting: isPending,
  };
}
