import { useCallback, useMemo } from 'react';
import type { QueryClient, InfiniteData } from '@tanstack/react-query';
import { getFolderDatasetsInfiniteKey } from '@/hooks/dataset/use-dataset-folders';
import type { ApiResponseData } from '@/services/types/common';
import type { FolderDatasetsResponse } from '@/services/types/dataset-folder';

// Strictly typed options for optimistic deletion behavior
export interface UseDatasetDeletionOptimisticOptions {
  queryClient: QueryClient;
  pageSize: number;
  keyword?: string;
  refetchFromPageAndAfter: (pageIndex: number) => Promise<unknown>;
  activeFolderId?: string;
}

// Returns a stable callback that performs optimistic removal from infinite cache
// and then refetches from the affected page to keep metadata (has_more/total) correct.
export function useDatasetDeletionOptimistic({
  queryClient,
  pageSize,
  keyword,
  refetchFromPageAndAfter,
  activeFolderId,
}: UseDatasetDeletionOptimisticOptions) {
  const queryKey = useMemo(
    () => getFolderDatasetsInfiniteKey(activeFolderId, pageSize, keyword),
    [activeFolderId, pageSize, keyword]
  );

  return useCallback(
    (deletedId: string, pageIndex: number) => {
      // Optimistically update cached pages for root or specific folder
      queryClient.setQueriesData<InfiniteData<ApiResponseData<FolderDatasetsResponse>>>(
        { queryKey },
        old => {
          if (!old) return old;
          const nextPages = old.pages.map((page, idx) => {
            if (idx < pageIndex) return page;
            const listData = page.data?.data ?? [];
            const filtered = listData.filter(item => item.id !== deletedId);
            return {
              ...page,
              data: page.data
                ? {
                    ...page.data,
                    data: filtered,
                    total: Math.max(
                      0,
                      (page.data.total ?? page.data.data?.length ?? 0) -
                        ((page.data.data?.length ?? 0) - filtered.length)
                    ),
                  }
                : page.data,
            } as ApiResponseData<FolderDatasetsResponse>;
          });
          return { ...old, pages: nextPages };
        }
      );

      // Ensure pagination metadata stays consistent
      void refetchFromPageAndAfter(pageIndex);
    },
    [queryClient, queryKey, refetchFromPageAndAfter]
  );
}
