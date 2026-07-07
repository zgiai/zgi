'use client';

// Dataset hooks powered by React Query
// All comments are in English for clarity and maintainability

import { useCallback, useMemo } from 'react';
import { useRouter } from 'next/navigation';
import {
  useMutation,
  useQuery,
  useQueryClient,
  useInfiniteQuery,
  type UseQueryOptions,
} from '@tanstack/react-query';
import { useT } from '@/i18n';
import { datasetService } from '@/services';
import type { Dataset, DatasetList, UpdateDatasetRequest } from '@/services/types/dataset';
import type { ApiResponseData } from '@/services/types/common';
import { toast } from 'sonner';
import { DATASET_KEYS } from '@/hooks/query-keys';
import { useCurrentWorkspace } from '@/store/workspace-store';
import {
  workspaceInvalidatePredicate,
  reloadInfiniteQuery,
  infiniteQueryUtils,
} from '@/hooks/query-utils';

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface UseDatasetsParams {
  // page is intentionally managed by the hook via useInfiniteQuery
  limit?: number;
  keyword?: string;
  ids?: string[];
  tag_ids?: string[];
  include_all?: boolean;
  workspace_id?: string;
}

export interface UseDatasetsOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
}

export interface UseDatasetsReturn {
  pages: Dataset[][]; // paged data structure [[...], [...]]
  fetchNextPage: () => Promise<unknown>;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  reload: () => Promise<void>;
  refetchFromPage: (startIndex: number) => Promise<unknown>;
}

/* -------------------------------------------------------------------------- */
/* Hook: useDatasets – list                                                    */
/* -------------------------------------------------------------------------- */

export function useDatasets(
  params: UseDatasetsParams = {},
  {
    enabled = true,
    staleTime = 5 * 60 * 1000,
    gcTime = 30 * 60 * 1000,
    refetchOnWindowFocus = false,
    refetchInterval = false,
  }: UseDatasetsOptions = {}
): UseDatasetsReturn {
  const queryClient = useQueryClient();

  // Normalize params so that queryKey is stable and does not include page
  const normalizedParams = useMemo(
    () => ({
      limit: params.limit,
      keyword: params.keyword,
      ids: params.ids,
      tag_ids: params.tag_ids,
      include_all: params.include_all,
      workspace_id: params.workspace_id,
    }),
    [
      params.limit,
      params.keyword,
      params.ids,
      params.tag_ids,
      params.include_all,
      params.workspace_id,
    ]
  );

  const { data, isLoading, isFetching, error, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useInfiniteQuery<ApiResponseData<DatasetList>, unknown>({
      queryKey: DATASET_KEYS.list(normalizedParams),
      initialPageParam: 1,
      queryFn: async ({ pageParam }) => {
        const page = (pageParam as number) ?? 1;
        return datasetService.getDatasets({
          ...normalizedParams,
          page,
        });
      },
      getNextPageParam: lastPage => {
        const meta = lastPage?.data as DatasetList | undefined;
        if (!meta) return undefined;
        return meta.has_more ? (meta.page ?? 1) + 1 : undefined;
      },
      select: response => response, // keep ApiResponseData structure per page
      enabled,
      staleTime,
      gcTime,
      refetchOnWindowFocus,
      refetchInterval,
      retry: false,
    });

  // Show toast on error - using inline t() would cause hook rules violation,
  // so we keep fallback message in code (rarely seen by users)

  const reload = useCallback(async () => {
    await reloadInfiniteQuery(queryClient, DATASET_KEYS.list(normalizedParams));
  }, [queryClient, normalizedParams]);

  const refetchFromPage = useCallback(
    async (startIndex: number) => {
      await infiniteQueryUtils.refetchFromPage(
        queryClient,
        DATASET_KEYS.list(normalizedParams),
        startIndex,
        fetchNextPage
      );
    },
    [fetchNextPage, normalizedParams, queryClient]
  );

  const pages: Dataset[][] = useMemo(() => {
    if (!data?.pages) return [];
    return data.pages.map(p => p.data?.data ?? []);
  }, [data]);

  return {
    pages,
    fetchNextPage,
    hasNextPage: Boolean(hasNextPage),
    isFetchingNextPage,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    reload,
    refetchFromPage,
  };
}

/* -------------------------------------------------------------------------- */
/* Hook: useDataset – detail                                                   */
/* -------------------------------------------------------------------------- */

export function useDataset(
  datasetId: string | undefined,
  options: Omit<UseQueryOptions<ApiResponseData<Dataset>, unknown>, 'queryKey' | 'queryFn'> = {}
) {
  return useQuery<ApiResponseData<Dataset>, unknown>({
    queryKey: datasetId ? DATASET_KEYS.detail(datasetId) : DATASET_KEYS.detail('undefined'),
    queryFn: () => {
      if (!datasetId) {
        throw new Error('datasetId is required');
      }
      return datasetService.getDataset(datasetId);
    },
    enabled: Boolean(datasetId) && (options.enabled ?? true),
    ...options,
  });
}

/* -------------------------------------------------------------------------- */
/* Mutations: create/update dataset                                            */
/* -------------------------------------------------------------------------- */

export function useCreateDataset(targetFolderId?: string) {
  const queryClient = useQueryClient();
  const t = useT('datasets');
  const router = useRouter();
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  // Import lazily to avoid hardcoding query key strings here
  const DATASET_FOLDERS_QUERY_KEY = 'dataset-folders';

  type CreateDatasetParams = Parameters<typeof datasetService.createDataset>[0];

  return useMutation({
    mutationFn: (payload: CreateDatasetParams) => datasetService.createDataset(payload),
    onSuccess: response => {
      toast.success(t('datasetCreated'));
      // Invalidate both paginated and basic lists that are either unfiltered or belong to current workspace
      queryClient.invalidateQueries({
        queryKey: DATASET_KEYS.all,
        predicate: workspaceInvalidatePredicate(DATASET_KEYS.all[0], currentWorkspaceId),
      });
      // Invalidate the folder datasets infinite list for the corresponding view (root or specific folder)
      queryClient.invalidateQueries({
        queryKey: [DATASET_FOLDERS_QUERY_KEY, 'folder-datasets-infinite', targetFolderId || 'root'],
      });

      // Let the detail root choose the first child page this account can open.
      if (response?.data?.id) {
        router.push(`/console/dataset/${response.data.id}`);
      }
    },
    onError: (error: unknown) => {
      const msg = (error as { message?: string }).message ?? t('messages.createFailed');
      toast.error(msg);
    },
  });
}

/**
 * Hook to update dataset
 */
export function useUpdateDataset(datasetId: string | undefined) {
  const queryClient = useQueryClient();
  const t = useT('datasets');
  const currentWorkspaceId = useCurrentWorkspace()?.id;
  // Lazily define folders query key to avoid cross-file import coupling
  const DATASET_FOLDERS_QUERY_KEY = 'dataset-folders';

  return useMutation({
    mutationFn: (data: UpdateDatasetRequest) => {
      if (!datasetId) {
        return Promise.reject(new Error('datasetId is required'));
      }
      return datasetService.updateDataset(datasetId, data);
    },
    onSuccess: () => {
      toast.success(t('settings.saveSuccess'));
      if (datasetId) {
        // Refresh dataset detail consumers
        queryClient.invalidateQueries({ queryKey: DATASET_KEYS.detail(datasetId) });
        // Invalidate both paginated and basic lists that are either unfiltered or belong to current workspace
        queryClient.invalidateQueries({
          queryKey: DATASET_KEYS.all,
          predicate: workspaceInvalidatePredicate(DATASET_KEYS.all[0], currentWorkspaceId),
        });
        // Crucially, invalidate folder infinite pagination caches so paginated views refetch
        queryClient.invalidateQueries({
          queryKey: [DATASET_FOLDERS_QUERY_KEY, 'folder-datasets-infinite'],
        });
        // Actively refetch active folder infinite queries to ensure immediate UI sync
        queryClient.refetchQueries({
          queryKey: [DATASET_FOLDERS_QUERY_KEY, 'folder-datasets-infinite'],
        });
      }
    },
    onError: (error: unknown) => {
      const msg = (error as { message?: string }).message ?? t('settings.saveError');
      toast.error(msg);
    },
  });
}

/* -------------------------------------------------------------------------- */
/* Mutations: delete dataset                                                   */
/* -------------------------------------------------------------------------- */
/**
 * Hook: useDeleteDataset – delete a dataset
 */
export function useDeleteDataset() {
  const queryClient = useQueryClient();
  const t = useT('datasets');
  const currentWorkspaceId = useCurrentWorkspace()?.id;
  const DATASET_FOLDERS_QUERY_KEY = 'dataset-folders';

  return useMutation<ApiResponseData<Record<string, unknown>>, unknown, string>({
    mutationFn: (datasetId: string) => datasetService.deleteDataset(datasetId),
    onSuccess: (_result, datasetId) => {
      toast.success(t('deleteSuccess'));
      // Remove the deleted dataset's detail cache
      if (datasetId) {
        queryClient.removeQueries({ queryKey: DATASET_KEYS.detail(datasetId) });
      }
      // Invalidate both paginated and basic lists that are either unfiltered or belong to current workspace
      queryClient.invalidateQueries({
        queryKey: DATASET_KEYS.all,
        predicate: workspaceInvalidatePredicate(DATASET_KEYS.all[0], currentWorkspaceId),
      });
      // Also invalidate folder infinite list
      queryClient.invalidateQueries({
        queryKey: [DATASET_FOLDERS_QUERY_KEY, 'folder-datasets-infinite'],
      });
    },
    onError: (error: unknown) => {
      const message = (error as { message?: string }).message ?? t('deleteFailed');
      toast.error(message);
    },
  });
}
