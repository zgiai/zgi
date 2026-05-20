'use client';

// Document management hooks powered by React Query
// All comments are in English for clarity and maintainability

import { useCallback, useMemo, useState } from 'react';
import {
  useMutation,
  useQueryClient,
  useInfiniteQuery,
  useQuery,
  type QueryKey,
} from '@tanstack/react-query';
import { useT } from '@/i18n';
import { datasetService } from '@/services';
import type {
  Document,
  DocumentList,
  DocumentListParams,
  DocumentIndexingStatus,
  DocumentDisplayStatus,
  DocumentExtractionStrategy,
  DocumentExtractionStrategiesResponse,
} from '@/services/types/dataset';
import type { ApiResponseData } from '@/services/types/common';
import { toast } from 'sonner';
import { useDebouncedValue } from '@/hooks/use-debounced-value';

import { DATASET_KEYS } from '@/hooks/query-keys';
import { reloadInfiniteQuery } from '@/hooks/query-utils';

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

// Strongly typed cache shape for infinite document lists
interface DocumentListInfiniteCache {
  pages: Array<ApiResponseData<DocumentList>>;
  pageParams: unknown[];
}

export interface UseDocumentsParams {
  /** Index status filter */
  indexing_status?: DocumentIndexingStatus[keyof DocumentIndexingStatus];
  /** Search keyword */
  keyword?: string;
  /** Items per page (max: 100) */
  limit?: number;
  /** Page number */
  page?: number;
  /** Sort field, prefix "-" means descending order, e.g. -created_at */
  sort?: string;
}

export interface UseDocumentsOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
  /** Enable search debouncing (default: true) */
  enableDebounce?: boolean;
  /** Debounce delay in milliseconds (default: 1000) */
  debounceDelay?: number;
}

export interface UseDocumentsReturn {
  /** Flattened documents across pages */
  documents: Document[];
  /** Paged documents (for advanced consumers) */
  pages: Document[][];
  /** Total number of documents on server */
  total: number;
  /** Loading state for initial request */
  isLoading: boolean;
  /** Fetching state for subsequent requests */
  isFetching: boolean;
  /** Error state */
  error: unknown;
  /** Refetch function */
  refetch: () => Promise<unknown>;
  /** Fetch next page for infinite scrolling */
  fetchNextPage: () => Promise<unknown>;
  /** Whether there is a next page available */
  hasNextPage: boolean;
  /** Whether next page is being fetched */
  isFetchingNextPage: boolean;
  /** Current search keyword */
  searchKeyword: string;
  /** Set search keyword function */
  setSearchKeyword: (keyword: string) => void;
  /** Reset to first page and reload */
  reload: () => Promise<void>;
}

export function useDocumentExtractionStrategies(options: { enabled?: boolean } = {}) {
  return useQuery({
    queryKey: DATASET_KEYS.extractionStrategies(),
    queryFn: () => datasetService.getDocumentExtractionStrategies(),
    enabled: options.enabled ?? true,
    staleTime: 5 * 60 * 1000,
    select: response => response.data,
  });
}

export function getPreferredDocumentExtractionStrategy(
  strategies: DocumentExtractionStrategy[] | undefined,
  recommended?: DocumentExtractionStrategiesResponse['recommended_strategy']
): DocumentExtractionStrategy | undefined {
  if (!strategies || strategies.length === 0) return undefined;
  if (recommended && strategies.includes(recommended)) return recommended;
  return strategies[0];
}

/* -------------------------------------------------------------------------- */
/* Hook: useDocuments – document list with pagination and search              */
/* -------------------------------------------------------------------------- */

export function useDocuments(
  datasetId: string | undefined,
  params: UseDocumentsParams = {},
  options: UseDocumentsOptions = {}
): UseDocumentsReturn {
  const { enableDebounce = true, debounceDelay = 1000, ...queryOptions } = options;
  const queryClient = useQueryClient();

  // Internal state for search (page handled by useInfiniteQuery)
  const [searchKeyword, setSearchKeyword] = useState(params.keyword || '');

  // Debounced search keyword
  const debouncedKeyword = useDebouncedValue(
    enableDebounce ? searchKeyword : params.keyword || '',
    debounceDelay
  );

  // Params without page, memoized to keep queryKey stable
  const normalizedParams = useMemo(() => {
    return {
      indexing_status: params.indexing_status,
      sort: params.sort,
      // keep limit stable and as string to match API type
      limit: (params.limit ?? 20).toString(),
    } as Pick<DocumentListParams, 'indexing_status' | 'sort' | 'limit'>;
  }, [params.indexing_status, params.sort, params.limit]);

  const {
    data,
    isLoading,
    isFetching,
    error,
    refetch,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useInfiniteQuery<ApiResponseData<DocumentList>, unknown>({
    queryKey: datasetId
      ? DATASET_KEYS.documentList(datasetId, { ...normalizedParams, keyword: debouncedKeyword })
      : DATASET_KEYS.documentList('undefined', { ...normalizedParams, keyword: debouncedKeyword }),
    initialPageParam: 1,
    queryFn: async ({ pageParam }) => {
      if (!datasetId) {
        throw new Error('datasetId is required');
      }
      const page = (pageParam as number) ?? 1;
      return datasetService.getDocuments(datasetId, {
        ...normalizedParams,
        keyword: debouncedKeyword,
        page: page.toString(),
      });
    },
    getNextPageParam: lastPage => {
      const meta = lastPage?.data as DocumentList | undefined;
      if (!meta) return undefined;
      // Compute if there are more pages based on total
      const { page = 1, limit = 20, total = 0 } = meta;
      const next = page * Number(limit) < Number(total) ? page + 1 : undefined;
      return next;
    },
    select: response => response, // keep ApiResponseData structure per page
    enabled: Boolean(datasetId) && (queryOptions.enabled ?? true),
    staleTime: queryOptions.staleTime ?? 2 * 60 * 1000,
    gcTime: queryOptions.gcTime ?? 15 * 60 * 1000,
    refetchOnWindowFocus: queryOptions.refetchOnWindowFocus ?? false,
    refetchInterval: queryOptions.refetchInterval ?? false,
    retry: false,
  });
  // Derived data: pages and flattened documents
  const pages = useMemo(() => {
    if (!data?.pages) return [] as Document[][];
    return data.pages.map(p => (p.data?.data ?? []) as Document[]);
  }, [data]);

  const documents = useMemo(() => pages.flat(), [pages]);

  const total = useMemo(() => {
    const last = data?.pages?.[0]?.data as DocumentList | undefined;
    // Prefer total from the first page meta; fallback to 0
    return last?.total ?? 0;
  }, [data]);

  const handleSetSearchKeyword = useCallback((keyword: string) => {
    setSearchKeyword(keyword);
    // No need to manually reset pages; queryKey changes and React Query resets
  }, []);

  const reload = useCallback(async () => {
    if (!datasetId) return;
    await reloadInfiniteQuery(
      queryClient,
      DATASET_KEYS.documentList(datasetId, { ...normalizedParams, keyword: debouncedKeyword })
    );
  }, [datasetId, normalizedParams, debouncedKeyword, queryClient]);

  return {
    documents,
    pages,
    total,
    isLoading,
    isFetching,
    error,
    refetch,
    fetchNextPage,
    hasNextPage: Boolean(hasNextPage),
    isFetchingNextPage,
    searchKeyword,
    setSearchKeyword: handleSetSearchKeyword,
    reload,
  };
}

/* -------------------------------------------------------------------------- */
/* Hook: useDownloadDocument                                                  */
/* -------------------------------------------------------------------------- */

export function useDownloadDocument() {
  const t = useT('datasets');
  return useMutation<Blob, unknown, { fileId: string; filename?: string }>({
    mutationFn: async ({ fileId }) => {
      // Use fileService to fetch blob
      const { fileService } = await import('@/services/file.service');
      return fileService.downloadFile(fileId);
    },
    onSuccess: (blob, { filename }) => {
      try {
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename || 'download';
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(url);
        toast.success(t('messages.downloadSuccess', { filename: filename ?? 'unknown' }));
      } catch (_e) {
        toast.error(t('messages.downloadFailed', { filename: filename ?? 'unknown' }));
      }
    },
    onError: (_, { filename }) => {
      toast.error(t('messages.downloadFailed', { filename: filename ?? 'unknown' }));
    },
  });
}

/* -------------------------------------------------------------------------- */
/* Hook: useCreateDocumentsInDataset – batch upload documents                  */
/* -------------------------------------------------------------------------- */

export function useCreateDocumentsInDataset(datasetId: string) {
  const queryClient = useQueryClient();
  const t = useT('datasets');
  type CreateDocumentsParams = Parameters<typeof datasetService.createDocumentsInDataset>[1];
  return useMutation({
    mutationFn: (data: CreateDocumentsParams) =>
      datasetService.createDocumentsInDataset(datasetId, data),
    onSuccess: () => {
      toast.success(t('messages.uploadSuccess'));
      queryClient.invalidateQueries({ queryKey: DATASET_KEYS.documents(datasetId) });
      // Ensure dataset meta reflects latest document_count, etc.
      queryClient.invalidateQueries({ queryKey: DATASET_KEYS.detail(datasetId) });
    },
    onError: (error: unknown) => {
      const msg = (error as { message?: string }).message ?? t('messages.uploadFailed');
      toast.error(msg);
    },
  });
}

/* -------------------------------------------------------------------------- */
/* Hook: useDeleteDocument – delete a document                                 */
/* -------------------------------------------------------------------------- */

export function useDeleteDocument() {
  const queryClient = useQueryClient();
  const t = useT('datasets');

  return useMutation<
    ApiResponseData<Record<string, unknown>>,
    unknown,
    { datasetId: string; documentId: string }
  >({
    mutationFn: ({ datasetId, documentId }) =>
      datasetService.deleteDocuments(datasetId, documentId),
    onSuccess: (_result, { datasetId }) => {
      toast.success(t('messages.deleteSuccess'));
      // Invalidate document list for this dataset
      queryClient.invalidateQueries({ queryKey: DATASET_KEYS.documents(datasetId) });
      // Also invalidate dataset detail so counts and stats refresh
      queryClient.invalidateQueries({ queryKey: DATASET_KEYS.detail(datasetId) });
    },
    onError: (error: unknown) => {
      const message = (error as { message?: string }).message ?? t('deleteFailed');
      toast.error(message);
    },
  });
}

/* -------------------------------------------------------------------------- */
/* Bulk document operations                                                    */
/* -------------------------------------------------------------------------- */

export function useBulkDeleteDocuments(datasetId: string) {
  const queryClient = useQueryClient();
  const t = useT('datasets');

  return useMutation<
    ApiResponseData<Record<string, unknown>>,
    unknown,
    { documentIds: string[] },
    { previousLists: Array<[QueryKey, DocumentListInfiniteCache | undefined]> }
  >({
    mutationFn: ({ documentIds }) => datasetService.batchDeleteDocuments(datasetId, documentIds),
    onSuccess: () => {
      toast.success(t('messages.deleteSuccess'));
      queryClient.invalidateQueries({ queryKey: DATASET_KEYS.documents(datasetId) });
      queryClient.invalidateQueries({ queryKey: DATASET_KEYS.detail(datasetId) });
    },
    onError: (error: unknown) => {
      const message = (error as { message?: string }).message ?? t('deleteFailed');
      toast.error(message);
    },
  });
}

export function useBulkEnableDocuments(datasetId: string) {
  const queryClient = useQueryClient();
  const t = useT('datasets');

  return useMutation<
    ApiResponseData<Record<string, unknown>>,
    unknown,
    { documentIds: string[] },
    { previousLists: Array<[QueryKey, DocumentListInfiniteCache | undefined]> }
  >({
    mutationFn: ({ documentIds }) => datasetService.batchEnableDocuments(datasetId, documentIds),
    onMutate: async ({ documentIds }) => {
      await queryClient.cancelQueries({ queryKey: DATASET_KEYS.documents(datasetId) });

      const previousLists = queryClient.getQueriesData<DocumentListInfiniteCache>({
        queryKey: DATASET_KEYS.documents(datasetId),
      });

      (previousLists as Array<[QueryKey, DocumentListInfiniteCache | undefined]>).forEach(
        ([key]) => {
          queryClient.setQueryData(key, (oldData: DocumentListInfiniteCache | undefined) => {
            if (!oldData) return oldData;
            const nextPages = oldData.pages.map(page => {
              const list = page?.data;
              if (!list) return page;
              const updated = {
                ...list,
                data: list.data.map(doc =>
                  documentIds.includes(doc.id)
                    ? { ...doc, enabled: true, display_status: 'enabled' }
                    : doc
                ),
              };
              return { ...page, data: updated } as ApiResponseData<DocumentList>;
            });
            return { ...oldData, pages: nextPages };
          });
        }
      );

      documentIds.forEach(documentId => {
        queryClient.setQueriesData<ApiResponseData<Document>>(
          { queryKey: DATASET_KEYS.documentDetail(datasetId, documentId) },
          (old: ApiResponseData<Document> | undefined) =>
            old
              ? {
                  ...old,
                  data: {
                    ...old.data,
                    enabled: true,
                    display_status: 'enabled' as DocumentDisplayStatus,
                  },
                }
              : old
        );
      });

      return {
        previousLists: previousLists as Array<[QueryKey, DocumentListInfiniteCache | undefined]>,
      };
    },
    onSuccess: () => {
      toast.success(t('messages.enableSuccess'));
      queryClient.invalidateQueries({ queryKey: DATASET_KEYS.documents(datasetId) });
      queryClient.invalidateQueries({ queryKey: DATASET_KEYS.detail(datasetId) });
    },
    onError: (error: unknown, _variables, context) => {
      if (context && context.previousLists) {
        (context.previousLists as Array<[QueryKey, DocumentListInfiniteCache | undefined]>).forEach(
          ([key, data]) => {
            queryClient.setQueryData(key, data);
          }
        );
      }
      const message = (error as { message?: string }).message ?? t('messages.actionFailed');
      toast.error(message);
    },
  });
}

export function useBulkDisableDocuments(datasetId: string) {
  const queryClient = useQueryClient();
  const t = useT('datasets');

  return useMutation<
    ApiResponseData<Record<string, unknown>>,
    unknown,
    { documentIds: string[] },
    { previousLists: Array<[unknown, DocumentListInfiniteCache | undefined]> }
  >({
    mutationFn: ({ documentIds }) => datasetService.batchDisableDocuments(datasetId, documentIds),
    onMutate: async ({ documentIds }) => {
      await queryClient.cancelQueries({ queryKey: DATASET_KEYS.documents(datasetId) });

      const previousLists = queryClient.getQueriesData<DocumentListInfiniteCache>({
        queryKey: DATASET_KEYS.documents(datasetId),
      });

      (previousLists as Array<[QueryKey, DocumentListInfiniteCache | undefined]>).forEach(
        ([key]) => {
          queryClient.setQueryData(key, (oldData: DocumentListInfiniteCache | undefined) => {
            if (!oldData) return oldData;
            const nextPages = oldData.pages.map(page => {
              const list = page?.data;
              if (!list) return page;
              const updated = {
                ...list,
                data: list.data.map(doc =>
                  documentIds.includes(doc.id)
                    ? { ...doc, enabled: false, display_status: 'disabled' }
                    : doc
                ),
              };
              return { ...page, data: updated } as ApiResponseData<DocumentList>;
            });
            return { ...oldData, pages: nextPages };
          });
        }
      );

      documentIds.forEach(documentId => {
        queryClient.setQueriesData<ApiResponseData<Document>>(
          { queryKey: DATASET_KEYS.documentDetail(datasetId, documentId) },
          (old: ApiResponseData<Document> | undefined) =>
            old
              ? {
                  ...old,
                  data: {
                    ...old.data,
                    enabled: false,
                    display_status: 'disabled' as DocumentDisplayStatus,
                  },
                }
              : old
        );
      });

      return {
        previousLists: previousLists as Array<[QueryKey, DocumentListInfiniteCache | undefined]>,
      };
    },
    onSuccess: () => {
      toast.success(t('messages.disableSuccess'));
      queryClient.invalidateQueries({ queryKey: DATASET_KEYS.documents(datasetId) });
      queryClient.invalidateQueries({ queryKey: DATASET_KEYS.detail(datasetId) });
    },
    onError: (error: unknown, _variables, context) => {
      if (context && context.previousLists) {
        (context.previousLists as Array<[QueryKey, DocumentListInfiniteCache | undefined]>).forEach(
          ([key, data]) => {
            queryClient.setQueryData(key, data);
          }
        );
      }
      const message = (error as { message?: string }).message ?? t('messages.actionFailed');
      toast.error(message);
    },
  });
}
