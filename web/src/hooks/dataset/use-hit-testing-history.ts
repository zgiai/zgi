import { useQuery } from '@tanstack/react-query';
import { useState, useCallback } from 'react';
import { datasetService } from '@/services';
import type { HitTestingHistoryRecord } from '@/services/types/dataset';
import { DATASET_KEYS } from '@/hooks/query-keys';

interface HistoryPage {
  data: HitTestingHistoryRecord[];
  has_more: boolean;
  page: number;
  total: number;
}

export function useHitTestingHistory(datasetId: string, limit = 20) {
  const [currentPage, setCurrentPage] = useState(1);
  const params = { page: currentPage, limit };

  const query = useQuery<HistoryPage>({
    queryKey: DATASET_KEYS.hitTestingHistory(datasetId, params),
    queryFn: async () => {
      const res = await datasetService.getHitTestingRecords(datasetId, params);
      return res.data as HistoryPage;
    },
    enabled: !!datasetId,
    staleTime: 60_000,
    retry: false,
  });

  const records: HitTestingHistoryRecord[] = query.data?.data ?? [];
  const total = query.data?.total ?? 0;
  const hasMore = query.data?.has_more ?? false;
  const hasPreviousPage = currentPage > 1;
  const totalPages = Math.ceil(total / limit);

  // Navigation functions
  const goToNextPage = useCallback(() => {
    if (hasMore) {
      setCurrentPage(prev => prev + 1);
    }
  }, [hasMore]);

  const goToPreviousPage = useCallback(() => {
    if (hasPreviousPage) {
      setCurrentPage(prev => prev - 1);
    }
  }, [hasPreviousPage]);

  const goToPage = useCallback(
    (page: number) => {
      if (page >= 1 && page <= totalPages) {
        setCurrentPage(page);
      }
    },
    [totalPages]
  );

  return {
    records,
    total,
    isLoading: query.isLoading,
    isFetchingNextPage: query.isFetching,
    fetchNextPage: goToNextPage,
    fetchPreviousPage: goToPreviousPage,
    hasMore,
    hasPreviousPage,
    currentPage,
    totalPages,
    goToPage,
  };
}
