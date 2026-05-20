import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { apiKeyService } from '@/services/apikey.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  ApiKeyList,
  ApiKeyDetail,
  GetApiKeysParams,
  CreateApiKeyRequest,
  CreateApiKeyResponse,
  UpdateApiKeyRequest,
  ApiKeyItem,
} from '@/services/types/apikey';
import { APIKEY_KEYS } from '@/hooks/query-keys';
import {
  denormalizeAiCreditValue,
  normalizeApiKeyItem,
  normalizeCreateApiKeyResponse,
} from '@/utils/ai-credits';

/**
 * Hook for fetching API keys list with pagination
 */
export function useApiKeys(params?: GetApiKeysParams): {
  items: ApiKeyItem[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const { data, isLoading, isFetching, error, refetch } = useQuery<ApiResponseData<ApiKeyList>>({
    queryKey: APIKEY_KEYS.list(params),
    queryFn: async () => {
      const response = await apiKeyService.getApiKeys(params);
      return {
        ...response,
        data: {
          ...response.data,
          items: response.data.items.map(normalizeApiKeyItem),
        },
      };
    },
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  if (error) {
    const message = (error as { message?: string }).message ?? 'Failed to load API keys';
    toast.error(message);
  }

  const list = data?.data;

  return {
    items: list?.items ?? [],
    total: list?.total ?? 0,
    page: list?.page ?? params?.page ?? 1,
    limit: list?.limit ?? params?.limit ?? 20,
    total_pages: list?.total_pages ?? 1,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch: async () => {
      await refetch();
    },
  };
}

/**
 * Hook for fetching single API key detail
 */
export function useApiKey(id?: string): {
  apiKey: ApiKeyDetail | undefined;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const { data, isLoading, isFetching, error, refetch } = useQuery<ApiResponseData<ApiKeyDetail>>({
    queryKey: APIKEY_KEYS.detail(id ?? ''),
    queryFn: async () => {
      const response = await apiKeyService.getApiKey(id ?? '');
      return {
        ...response,
        data: normalizeApiKeyItem(response.data),
      };
    },
    enabled: Boolean(id),
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  if (error) {
    const message = (error as { message?: string }).message ?? 'Failed to load API key';
    toast.error(message);
  }

  return {
    apiKey: data?.data,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch: async () => {
      await refetch();
    },
  };
}

/**
 * Hook for creating API keys
 */
export function useCreateApiKey(): {
  createApiKey: (data: CreateApiKeyRequest) => Promise<CreateApiKeyResponse | undefined>;
  isCreating: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('apikeys');

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<CreateApiKeyResponse>,
    unknown,
    CreateApiKeyRequest
  >({
    mutationFn: async data => {
      const payload: CreateApiKeyRequest = {
        ...data,
        quota_amount: denormalizeAiCreditValue(data.quota_amount) ?? undefined,
      };
      const response = await apiKeyService.createApiKey(payload);
      return {
        ...response,
        data: normalizeCreateApiKeyResponse(response.data),
      };
    },
    onSuccess: () => {
      toast.success(t('createSuccess'));
      // Invalidate list queries to refetch
      queryClient.invalidateQueries({ queryKey: APIKEY_KEYS.lists() });
    },
    onError: error => {
      const message = (error as { message?: string }).message ?? 'Failed to create API key';
      toast.error(message);
    },
  });

  return {
    createApiKey: async (data: CreateApiKeyRequest) => {
      const res = await mutateAsync(data);
      return res?.data;
    },
    isCreating: isPending,
  };
}

/**
 * Hook for updating API key
 */
export function useUpdateApiKey(): {
  updateApiKey: (id: string, data: UpdateApiKeyRequest) => Promise<ApiKeyDetail | undefined>;
  isUpdating: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('apikeys');

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<ApiKeyDetail>,
    unknown,
    { id: string; data: UpdateApiKeyRequest }
  >({
    mutationFn: async ({ id, data }) => {
      const payload: UpdateApiKeyRequest = {
        ...data,
        quota_limit: denormalizeAiCreditValue(data.quota_limit) ?? undefined,
        remain_quota: denormalizeAiCreditValue(data.remain_quota) ?? undefined,
      };
      const response = await apiKeyService.updateApiKey(id, payload);
      return {
        ...response,
        data: normalizeApiKeyItem(response.data),
      };
    },
    onSuccess: () => {
      toast.success(t('updateSuccess'));
      queryClient.invalidateQueries({ queryKey: APIKEY_KEYS.lists() });
    },
    onError: error => {
      const message = (error as { message?: string }).message ?? 'Failed to update API key';
      toast.error(message);
    },
  });

  return {
    updateApiKey: async (id: string, data: UpdateApiKeyRequest) => {
      const res = await mutateAsync({ id, data });
      return res?.data;
    },
    isUpdating: isPending,
  };
}

/**
 * Hook for deleting API key
 */
export function useDeleteApiKey(): {
  deleteApiKey: (id: string) => Promise<void>;
  isDeleting: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('apikeys');

  const { mutateAsync, isPending } = useMutation<ApiResponseData<null>, unknown, string>({
    mutationFn: id => apiKeyService.deleteApiKey(id),
    onSuccess: () => {
      toast.success(t('deleteSuccess'));
      queryClient.invalidateQueries({ queryKey: APIKEY_KEYS.lists() });
    },
    onError: error => {
      const message = (error as { message?: string }).message ?? 'Failed to delete API key';
      toast.error(message);
    },
  });

  return {
    deleteApiKey: async (id: string) => {
      await mutateAsync(id);
    },
    isDeleting: isPending,
  };
}
