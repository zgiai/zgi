'use client';

// Dataset folder hooks powered by React Query
// All comments are in English for clarity and maintainability

import { useMemo } from 'react';
import { useQuery, useMutation, useQueryClient, useInfiniteQuery } from '@tanstack/react-query';
import { datasetFolderService } from '@/services';
import { toast } from 'sonner';
import type {
  DatasetFolder,
  DatasetFolderList,
  CreateDatasetFolderRequest,
  UpdateDatasetFolderRequest,
  FolderDatasetsResponse,
  MoveDatasetRequest,
} from '@/services/types/dataset-folder';
import type { ApiResponseData } from '@/services/types/common';
import { useT } from '@/i18n';
import type { Dataset } from '@/services/types/dataset';
import type { DatasetList } from '@/services/types/dataset';

/* -------------------------------------------------------------------------- */
/* Query-key helpers                                                          */
/* -------------------------------------------------------------------------- */

export const DATASET_FOLDERS_QUERY_KEY = 'dataset-folders';

export const getDatasetFoldersListKey = (
  params: { keyword?: string; workspace_id?: string } = {}
) => [DATASET_FOLDERS_QUERY_KEY, 'list', params];
export const getDatasetFolderDetailKey = (id: string) => [DATASET_FOLDERS_QUERY_KEY, 'detail', id];
export const getFolderDatasetsInfiniteKey = (
  folderId?: string,
  limit: number = 20,
  keyword?: string,
  workspace_id?: string
) => [
  DATASET_FOLDERS_QUERY_KEY,
  'folder-datasets-infinite',
  folderId || 'root',
  { limit, keyword, workspace_id },
];

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface UseDatasetFoldersOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
  // Optional filters
  keyword?: string;
  workspace_id?: string;
}

export interface UseDatasetFoldersReturn {
  data: DatasetFolder[] | undefined;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => void;
}

// Options for useFolderDatasetsInfinite
interface UseFolderDatasetsOptions extends UseDatasetFoldersOptions {
  keyword?: string;
}

/* -------------------------------------------------------------------------- */
/* Hook: useDatasetFolders – folder list                                     */
/* -------------------------------------------------------------------------- */

export function useDatasetFolders(options: UseDatasetFoldersOptions = {}): UseDatasetFoldersReturn {
  const {
    enabled = true,
    staleTime = 5 * 60 * 1000,
    gcTime = 30 * 60 * 1000,
    refetchOnWindowFocus = false,
    refetchInterval = false,
    keyword,
    workspace_id,
  } = options;
  const t = useT('datasets');

  const { data, isLoading, isFetching, error, refetch } = useQuery<
    ApiResponseData<DatasetFolderList>,
    unknown
  >({
    queryKey: getDatasetFoldersListKey({
      keyword: (keyword || '').trim(),
      workspace_id,
    }),
    queryFn: () => datasetFolderService.getDatasetFolders({ keyword, workspace_id }),
    enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    refetchInterval,
    retry: false,
  });

  if (error) {
    const message = (error as { message?: string }).message ?? t('folders.errors.loadListFailed');
    toast.error(message);
  }

  const folders = useMemo(() => {
    return data?.data?.data;
  }, [data]);

  return {
    data: folders,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch,
  };
}

/* -------------------------------------------------------------------------- */
/* Hook: useDatasetFolder – folder detail                                    */
/* -------------------------------------------------------------------------- */

export function useDatasetFolder(
  folderId: string | undefined,
  options: UseDatasetFoldersOptions = {}
) {
  const t = useT('datasets');
  const {
    enabled = true,
    staleTime = 5 * 60 * 1000,
    gcTime = 30 * 60 * 1000,
    refetchOnWindowFocus = false,
    refetchInterval = false,
  } = options;

  const { data, isLoading, isFetching, error, refetch } = useQuery<
    ApiResponseData<DatasetFolder>,
    unknown
  >({
    queryKey: folderId ? getDatasetFolderDetailKey(folderId) : [],
    queryFn: () =>
      folderId
        ? datasetFolderService.getDatasetFolder(folderId)
        : Promise.reject(new Error('Folder ID is required')),
    enabled: enabled && !!folderId,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    refetchInterval,
    retry: false,
  });

  // Show toast on error
  if (error) {
    const message = (error as { message?: string }).message ?? t('folders.errors.loadDetailFailed');
    toast.error(message);
  }

  const folder = useMemo(() => {
    return data?.data;
  }, [data]);

  return {
    data: folder,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch,
  };
}

/* -------------------------------------------------------------------------- */
/* Hook: useCreateDatasetFolder – create folder                              */
/* -------------------------------------------------------------------------- */

export function useCreateDatasetFolder() {
  const queryClient = useQueryClient();
  const t = useT('datasets');

  return useMutation({
    mutationFn: (data: CreateDatasetFolderRequest) =>
      datasetFolderService.createDatasetFolder(data),
    onMutate: async data => {
      // Optimistically insert into folders list
      await queryClient.cancelQueries({ queryKey: getDatasetFoldersListKey() });
      const previousFolders = queryClient.getQueryData<ApiResponseData<DatasetFolderList>>(
        getDatasetFoldersListKey({ workspace_id: data.workspace_id })
      );
      const now = new Date().toISOString();
      const optimisticFolder: DatasetFolder = {
        id: `optimistic-${Math.random().toString(36).slice(2)}`,
        workspace_id: data.workspace_id,
        name: data.name,
        description: data.description ?? '',
        parent_id: data.parent_id ?? null,
        created_by: 'me',
        created_at: now,
        updated_by: null,
        updated_at: now,
        position: 0,
        can_edit: true,
      };
      if (previousFolders?.data) {
        queryClient.setQueryData<ApiResponseData<DatasetFolderList>>(
          getDatasetFoldersListKey({ workspace_id: data.workspace_id }),
          {
            ...previousFolders,
            data: {
              ...previousFolders.data,
              data: [optimisticFolder, ...(previousFolders.data.data ?? [])],
              total: (previousFolders.data.total ?? 0) + 1,
            },
          }
        );
      }
      return { previousFolders } as {
        previousFolders?: ApiResponseData<DatasetFolderList>;
      };
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: getDatasetFoldersListKey() });
      toast.success(t('folders.toast.createSuccess'));
    },
    onError: (error: unknown, _variables, context) => {
      if (context?.previousFolders) {
        queryClient.setQueryData(
          getDatasetFoldersListKey({ workspace_id: _variables.workspace_id }),
          context.previousFolders
        );
      }
      const message = (error as { message?: string }).message ?? t('folders.errors.createFailed');
      toast.error(message);
    },
  });
}

/* -------------------------------------------------------------------------- */
/* Hook: useUpdateDatasetFolder – update folder                              */
/* -------------------------------------------------------------------------- */

export function useUpdateDatasetFolder() {
  const queryClient = useQueryClient();
  const t = useT('datasets');

  return useMutation({
    mutationFn: ({ folderId, data }: { folderId: string; data: UpdateDatasetFolderRequest }) =>
      datasetFolderService.updateDatasetFolder(folderId, data),
    onSuccess: (_, { folderId }) => {
      queryClient.invalidateQueries({ queryKey: getDatasetFolderDetailKey(folderId) });
      queryClient.invalidateQueries({ queryKey: getDatasetFoldersListKey() });
      toast.success(t('folders.toast.updateSuccess'));
    },
    onError: (error: unknown) => {
      const message = (error as { message?: string }).message ?? t('folders.errors.updateFailed');
      toast.error(message);
    },
  });
}

/* -------------------------------------------------------------------------- */
/* Hook: useDeleteDatasetFolder – delete folder                              */
/* -------------------------------------------------------------------------- */

export function useDeleteDatasetFolder() {
  const queryClient = useQueryClient();
  const t = useT('datasets');

  return useMutation({
    mutationFn: (folderId: string) => datasetFolderService.deleteDatasetFolder(folderId),
    onSuccess: (_, folderId) => {
      queryClient.removeQueries({ queryKey: getDatasetFolderDetailKey(folderId) });
      queryClient.invalidateQueries({ queryKey: getDatasetFoldersListKey() });
      toast.success(t('folders.toast.deleteSuccess'));
    },
    onError: (error: unknown) => {
      const message = (error as { message?: string }).message ?? t('folders.errors.deleteFailed');
      toast.error(message);
    },
  });
}

/* -------------------------------------------------------------------------- */
/* Hook: useMoveDatasetToFolder – move dataset to folder                     */
/* -------------------------------------------------------------------------- */

export function useMoveDatasetToFolder() {
  const queryClient = useQueryClient();
  const t = useT('datasets');

  return useMutation({
    mutationFn: (data: MoveDatasetRequest & { source_folder_id?: string }) =>
      datasetFolderService.moveDatasetToFolder({
        dataset_id: data.dataset_id,
        folder_id: data.folder_id,
      }),
    onMutate: async (data: MoveDatasetRequest & { source_folder_id?: string }) => {
      // Cancel any outgoing refetches for folder contents and datasets list
      await queryClient.cancelQueries({
        queryKey: [DATASET_FOLDERS_QUERY_KEY, 'folder-datasets-infinite'],
      });
      await queryClient.cancelQueries({ queryKey: ['datasets', 'list'] });
      // Snapshot previous queries to support rollback on error
      const previousInfiniteFolderQueries = queryClient.getQueriesData({
        queryKey: [DATASET_FOLDERS_QUERY_KEY, 'folder-datasets-infinite'],
      });
      const previousDatasetListQueries = queryClient.getQueriesData({
        queryKey: ['datasets', 'list'],
      });

      // Try to locate the dataset to move from cached infinite folder contents first
      let movedDataset: Dataset | undefined;
      let sourceFolderIdFound: string | undefined;
      previousInfiniteFolderQueries.forEach(([queryKey, queryData]) => {
        const oldInfinite = queryData as
          | { pages?: Array<ApiResponseData<FolderDatasetsResponse>>; pageParams?: unknown[] }
          | undefined;
        const pages = oldInfinite?.pages ?? [];
        let foundPageIndex = -1;
        let foundIdx = -1;
        for (let pi = 0; pi < pages.length && foundIdx < 0; pi++) {
          const list = pages[pi]?.data?.data ?? [];
          const idx = list.findIndex(d => d.id === data.dataset_id);
          if (idx >= 0) {
            foundPageIndex = pi;
            foundIdx = idx;
            movedDataset = list[idx];
          }
        }
        if (foundIdx >= 0) {
          // Derive source folder id from query key
          const keyParts = queryKey as readonly unknown[];
          const keyFolder = (keyParts[2] as string) || 'root';
          sourceFolderIdFound = keyFolder === 'root' ? undefined : keyFolder;
          // Optimistically remove the dataset from its source folder cache (from found page onwards)
          queryClient.setQueryData(
            queryKey,
            (
              oldVal:
                | { pages: Array<ApiResponseData<FolderDatasetsResponse>>; pageParams: unknown[] }
                | undefined
            ) => {
              if (!oldVal?.pages) return oldVal;
              const updatedPages = oldVal.pages.map((page, idx) => {
                if (idx < foundPageIndex) return page;
                const list = page?.data?.data ?? [];
                const filtered = list.filter(d => d.id !== data.dataset_id);
                return {
                  ...page,
                  data: page.data
                    ? {
                        ...page.data,
                        data: filtered,
                        total: Math.max(
                          0,
                          (page.data.total ?? list.length) - (list.length - filtered.length)
                        ),
                      }
                    : page.data,
                } as ApiResponseData<FolderDatasetsResponse>;
              });
              return { ...oldVal, pages: updatedPages };
            }
          );
        }
      });

      // If not found in folder infinite caches, search dataset list caches as fallback
      if (!movedDataset) {
        previousDatasetListQueries.some(([_qk, qd]) => {
          const pages = (qd as { pages?: Array<ApiResponseData<DatasetList>> } | undefined)?.pages;
          if (!pages) return false;
          for (const page of pages) {
            const list = page?.data?.data ?? [];
            const found = list.find(d => d.id === data.dataset_id);
            if (found) {
              movedDataset = found;
              return true;
            }
          }
          return false;
        });
      }

      // Optimistically add the dataset to the target folder's infinite cache (if cache exists)
      const targetFolderId =
        data.folder_id && data.folder_id.length > 0 ? data.folder_id : undefined;
      if (movedDataset) {
        const targetInfiniteQueries = queryClient.getQueriesData({
          queryKey: [
            DATASET_FOLDERS_QUERY_KEY,
            'folder-datasets-infinite',
            targetFolderId || 'root',
          ],
        });
        targetInfiniteQueries.forEach(([qk, qd]) => {
          const hasPages = (
            qd as { pages?: Array<ApiResponseData<FolderDatasetsResponse>> } | undefined
          )?.pages;
          if (!hasPages) return;
          queryClient.setQueryData(
            qk,
            (
              oldVal:
                | { pages: Array<ApiResponseData<FolderDatasetsResponse>>; pageParams: unknown[] }
                | undefined
            ) => {
              if (!oldVal?.pages || oldVal.pages.length === 0) return oldVal;
              const first = oldVal.pages[0];
              const list = first.data?.data ?? [];
              const nextFirst: ApiResponseData<FolderDatasetsResponse> = {
                ...first,
                data: first.data
                  ? {
                      ...first.data,
                      data: [movedDataset as Dataset, ...list],
                      total: (first.data.total ?? list.length) + 1,
                    }
                  : first.data,
              } as ApiResponseData<FolderDatasetsResponse>;
              const updatedPages = [nextFirst, ...oldVal.pages.slice(1)];
              return { ...oldVal, pages: updatedPages };
            }
          );
        });
      }

      const sourceFolderId =
        typeof data.source_folder_id === 'string'
          ? data.source_folder_id.length > 0
            ? data.source_folder_id
            : undefined
          : sourceFolderIdFound;

      // Return snapshot for rollback + ids for precise invalidation
      return {
        previousInfiniteFolderQueries,
        previousDatasetListQueries,
        sourceFolderId,
        targetFolderId,
      } as {
        previousInfiniteFolderQueries: Array<
          [
            readonly unknown[],
            { pages?: Array<ApiResponseData<FolderDatasetsResponse>> } | undefined,
          ]
        >;
        previousDatasetListQueries: Array<
          [readonly unknown[], { pages?: Array<ApiResponseData<DatasetList>> } | undefined]
        >;
        sourceFolderId?: string;
        targetFolderId?: string;
      };
    },
    onSuccess: (_resp, _variables, context) => {
      // Invalidate broad caches to sync with server state
      queryClient.invalidateQueries({ queryKey: ['datasets'] });

      // Precisely invalidate and refetch source and destination folder caches (infinite only)
      const invalidateAndRefetch = (folderId: string | undefined) => {
        // Infinite pagination cache (prefix match, regardless of limit)
        const infinitePrefix = [
          DATASET_FOLDERS_QUERY_KEY,
          'folder-datasets-infinite',
          folderId || 'root',
        ] as const;
        queryClient.invalidateQueries({ queryKey: infinitePrefix });
        queryClient.refetchQueries({ queryKey: infinitePrefix });
      };

      // Use variables when provided to decide source/destination precisely ('' means root)
      const vars = _variables as MoveDatasetRequest & { source_folder_id?: string };
      const hasSourceInVars = typeof vars?.source_folder_id === 'string';
      const sourceIdFromVars = hasSourceInVars
        ? vars.source_folder_id && vars.source_folder_id.length > 0
          ? vars.source_folder_id
          : undefined
        : undefined;

      // Source folder: if component passed source_folder_id (including empty string for root), refresh it;
      // otherwise fall back to context-derived source id when available
      if (hasSourceInVars) {
        invalidateAndRefetch(sourceIdFromVars); // undefined will map to 'root'
      } else if (context?.sourceFolderId !== undefined) {
        invalidateAndRefetch(context.sourceFolderId);
      }

      // Destination folder always needs refresh (prefer context, fallback to variables)
      const destId =
        context?.targetFolderId ??
        (vars.folder_id && vars.folder_id.length > 0 ? vars.folder_id : undefined);
      invalidateAndRefetch(destId);

      toast.success(t('folders.toast.moveSuccess'));
    },
    onError: (error: unknown, _variables, context) => {
      // Rollback all affected queries
      if (context?.previousInfiniteFolderQueries) {
        context.previousInfiniteFolderQueries.forEach(([queryKey, queryData]) => {
          queryClient.setQueryData(queryKey, queryData);
        });
      }
      if (context?.previousDatasetListQueries) {
        context.previousDatasetListQueries.forEach(([queryKey, queryData]) => {
          queryClient.setQueryData(queryKey, queryData);
        });
      }
      const message = (error as { message?: string }).message ?? t('folders.errors.moveFailed');
      toast.error(message);
    },
  });
}

/* -------------------------------------------------------------------------- */
/* Hook: useFolderAncestors – compute breadcrumb chain                        */
/* -------------------------------------------------------------------------- */

export function useFolderAncestors(folderId?: string) {
  const queryClient = useQueryClient();

  return useQuery<DatasetFolder[], unknown>({
    queryKey: [DATASET_FOLDERS_QUERY_KEY, 'ancestors', folderId || 'root'],
    enabled: !!folderId,
    // Build ancestors chain by walking parent_id upwards
    queryFn: async () => {
      if (!folderId) return [];

      const chain: DatasetFolder[] = [];
      let currentId: string | null = folderId;

      // Safety cap to avoid infinite loops
      let guard = 0;
      while (currentId && guard < 20) {
        guard += 1;
        // Try cache first
        const cached: ApiResponseData<DatasetFolder> | undefined = queryClient.getQueryData<
          ApiResponseData<DatasetFolder>
        >(getDatasetFolderDetailKey(currentId));
        let current: DatasetFolder | undefined = cached?.data;
        if (!current) {
          const resp = await datasetFolderService.getDatasetFolder(currentId);
          current = resp.data;
          // Prime cache
          queryClient.setQueryData(getDatasetFolderDetailKey(currentId), resp);
        }
        if (!current) break;
        chain.unshift(current);
        currentId = current.parent_id;
      }
      return chain;
    },
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    retry: false,
  });
}

/**
 * Infinite pagination for datasets inside a folder (or root when folderId is undefined)
 * Uses /console/api/dataset-folders/datasets endpoint without folder_id for homepage list
 */
export function useFolderDatasetsInfinite(
  folderId: string | undefined,
  limit: number = 20,
  options: UseFolderDatasetsOptions = {}
): {
  pages: Dataset[][];
  fetchNextPage: () => Promise<unknown>;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetchFromPage: (startIndex: number) => Promise<unknown>;
  refetchFromPageAndAfter: (startIndex: number) => Promise<unknown>;
} {
  const t = useT('datasets');
  const queryClient = useQueryClient();
  const {
    enabled = true,
    staleTime = 5 * 60 * 1000,
    gcTime = 30 * 60 * 1000,
    refetchOnWindowFocus = false,
    refetchInterval = false,
    keyword,
    workspace_id,
  } = options;

  const {
    data,
    isLoading,
    isFetching,
    error,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    refetch,
  } = useInfiniteQuery<ApiResponseData<FolderDatasetsResponse>, unknown>({
    queryKey: getFolderDatasetsInfiniteKey(folderId, limit, keyword, workspace_id),
    initialPageParam: 1,
    queryFn: async ({ pageParam }) => {
      const page = (pageParam as number) ?? 1;
      return datasetFolderService.getFolderDatasets({
        folder_id: folderId,
        page,
        limit,
        keyword,
        workspace_id,
      });
    },
    getNextPageParam: lastPage => {
      const meta = lastPage?.data as FolderDatasetsResponse | undefined;
      if (!meta) return undefined;
      return meta.has_more ? (meta.page ?? 1) + 1 : undefined;
    },
    select: response => response,
    enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    refetchInterval,
    retry: false,
  });

  // Show toast on error
  if (error) {
    const message =
      (error as { message?: string }).message ?? t('folders.errors.loadContentsFailed');
    toast.error(message);
  }

  // Refetch from a specific cached page index to sync metadata after mutations
  const refetchFromPage = async (startIndex: number) => {
    const key = getFolderDatasetsInfiniteKey(folderId, limit, keyword, workspace_id);
    const cached = queryClient.getQueryData<{
      pages: Array<ApiResponseData<FolderDatasetsResponse>>;
      pageParams: unknown[];
    }>(key);
    if (cached && Array.isArray(cached.pages)) {
      const keepCount = Math.min(cached.pages.length, Math.max(0, startIndex) + 1);
      const nextPages = cached.pages.slice(0, keepCount);
      const nextParams = cached.pageParams.slice(0, keepCount);
      queryClient.setQueryData(key, { pages: nextPages, pageParams: nextParams });

      const targetPageNumber = Math.max(1, startIndex + 1);
      try {
        const fresh = await datasetFolderService.getFolderDatasets({
          folder_id: folderId,
          page: targetPageNumber,
          limit,
          keyword,
          workspace_id,
        });
        queryClient.setQueryData(
          key,
          (
            oldData:
              | { pages: Array<ApiResponseData<FolderDatasetsResponse>>; pageParams: unknown[] }
              | undefined
          ) => {
            if (!oldData) return oldData;
            const updatedPages = [...oldData.pages];
            if (startIndex < updatedPages.length) {
              updatedPages[startIndex] = fresh;
            }
            return { ...oldData, pages: updatedPages };
          }
        );
      } catch {
        // Ignore single-page refresh errors
      }
    } else {
      await refetch();
    }
  };

  // Refetch from startIndex to the end of currently cached pages without touching earlier pages
  const refetchFromPageAndAfter = async (startIndex: number) => {
    const key = getFolderDatasetsInfiniteKey(folderId, limit, keyword, workspace_id);
    const cached = queryClient.getQueryData<{
      pages: Array<ApiResponseData<FolderDatasetsResponse>>;
      pageParams: unknown[];
    }>(key);
    if (cached && Array.isArray(cached.pages)) {
      const start = Math.max(0, startIndex);
      for (let i = start; i < cached.pages.length; i++) {
        try {
          const fresh = await datasetFolderService.getFolderDatasets({
            folder_id: folderId,
            page: i + 1,
            limit,
            keyword,
            workspace_id,
          });
          queryClient.setQueryData(
            key,
            (
              oldData:
                | { pages: Array<ApiResponseData<FolderDatasetsResponse>>; pageParams: unknown[] }
                | undefined
            ) => {
              if (!oldData) return oldData;
              const updatedPages = [...oldData.pages];
              if (i < updatedPages.length) {
                updatedPages[i] = fresh;
              }
              return { ...oldData, pages: updatedPages };
            }
          );
        } catch {
          // Ignore per-page refresh errors
        }
      }
    } else {
      await refetch();
    }
  };

  // Map pages to arrays of Dataset for rendering
  const pagesMapped: Dataset[][] = (data?.pages || []).map(page => page?.data?.data ?? []);

  return {
    pages: pagesMapped,
    fetchNextPage,
    hasNextPage: !!hasNextPage,
    isFetchingNextPage,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetchFromPage,
    refetchFromPageAndAfter,
  };
}
