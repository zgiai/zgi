import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { workspaceQuotaService } from '@/services/workspace-quota.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  WorkspaceQuota,
  WorkspaceQuotaList,
  GetWorkspaceQuotasParams,
  UpdateWorkspaceQuotaRequest,
} from '@/services/types/workspace-quota';
import { WORKSPACE_QUOTA_KEYS } from '@/hooks/query-keys';
import {
  denormalizeAiCreditValue,
  normalizeWorkspaceQuota,
  normalizeWorkspaceQuotaList,
} from '@/utils/ai-credits';

/**
 * Hook for fetching workspace quotas list with pagination
 */
export function useWorkspaceQuotas(params?: GetWorkspaceQuotasParams): {
  items: WorkspaceQuota[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const t = useT('workspace');

  const { data, isLoading, isFetching, error, refetch } = useQuery<
    ApiResponseData<WorkspaceQuotaList>
  >({
    queryKey: WORKSPACE_QUOTA_KEYS.list(params),
    queryFn: async () => {
      const response = await workspaceQuotaService.getWorkspaceQuotas(params);
      return {
        ...response,
        data: normalizeWorkspaceQuotaList(response.data),
      };
    },
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  if (error) {
    const message = (error as { message?: string }).message ?? t('quota.loadError');
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
 * Hook for fetching single workspace quota detail
 */
export function useWorkspaceQuota(workspaceId?: string): {
  quota: WorkspaceQuota | undefined;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const t = useT('workspace');

  const { data, isLoading, isFetching, error, refetch } = useQuery<ApiResponseData<WorkspaceQuota>>(
    {
      queryKey: WORKSPACE_QUOTA_KEYS.detail(workspaceId ?? ''),
      queryFn: async () => {
        const response = await workspaceQuotaService.getWorkspaceQuota(workspaceId ?? '');
        return {
          ...response,
          data: normalizeWorkspaceQuota(response.data),
        };
      },
      enabled: Boolean(workspaceId),
      staleTime: 5 * 60 * 1000,
      gcTime: 30 * 60 * 1000,
      refetchOnWindowFocus: false,
      retry: false,
    }
  );

  if (error) {
    const message = (error as { message?: string }).message ?? t('quota.loadError');
    toast.error(message);
  }

  return {
    quota: data?.data,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch: async () => {
      await refetch();
    },
  };
}

/**
 * Hook for updating workspace quota
 */
export function useUpdateWorkspaceQuota(): {
  updateQuota: (
    workspaceId: string,
    data: UpdateWorkspaceQuotaRequest
  ) => Promise<WorkspaceQuota | undefined>;
  isUpdating: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('workspace');

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<WorkspaceQuota>,
    Error,
    { workspaceId: string; data: UpdateWorkspaceQuotaRequest }
  >({
    mutationFn: async ({ workspaceId, data }) => {
      const payload: UpdateWorkspaceQuotaRequest = {
        ...data,
        quota_amount: denormalizeAiCreditValue(data.quota_amount) ?? undefined,
        remain_quota: denormalizeAiCreditValue(data.remain_quota) ?? undefined,
      };
      const response = await workspaceQuotaService.updateWorkspaceQuota(workspaceId, payload);
      return {
        ...response,
        data: normalizeWorkspaceQuota(response.data),
      };
    },
    onSuccess: (_result, variables) => {
      toast.success(t('quota.updateSuccess'));
      // Invalidate list queries to refetch
      queryClient.invalidateQueries({ queryKey: WORKSPACE_QUOTA_KEYS.lists() });
      // Invalidate specific detail query
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_QUOTA_KEYS.detail(variables.workspaceId),
      });
    },
    onError: error => {
      const message = error.message ?? t('quota.updateError');
      toast.error(message);
    },
  });

  return {
    updateQuota: async (workspaceId: string, data: UpdateWorkspaceQuotaRequest) => {
      const res = await mutateAsync({ workspaceId, data });
      return res?.data;
    },
    isUpdating: isPending,
  };
}
