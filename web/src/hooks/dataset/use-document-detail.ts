'use client';

import { useRef, useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { datasetService } from '@/services';
import type {
  DocumentIndexingStatusResponse,
  RandomQuestionsResponse,
} from '@/services/types/dataset';
import type { ApiResponseData } from '@/services/types/common';
import { DATASET_KEYS } from '@/hooks/query-keys';

interface UseDocumentDetailProps {
  datasetId: string;
  documentId: string;
  /** Whether to enable queries - set to false to skip all API calls */
  enabled?: boolean;
}

/**
 * Hook for document detail and metadata queries only.
 * For segments, use useDocumentSegments directly.
 * For questions, use useSegmentQuestions directly.
 * For document actions, use useDocumentActions directly.
 */
export function useDocumentDetail({
  datasetId,
  documentId,
  enabled = true,
}: UseDocumentDetailProps) {
  // Base enabled condition - requires valid IDs and enabled flag
  const queryEnabled = enabled && !!datasetId && !!documentId;

  // Document detail query
  const {
    data: documentResponse,
    isLoading: isDocumentLoading,
    error: documentError,
    refetch: refetchDocument,
  } = useQuery({
    queryKey: DATASET_KEYS.documentDetail(datasetId, documentId),
    queryFn: () => datasetService.getDocumentDetail(datasetId, documentId, 'without'),
    staleTime: 30000, // 30 seconds
    enabled: queryEnabled,
  });

  // Document metadata query
  const { data: metadataResponse, isLoading: isMetadataLoading } = useQuery({
    queryKey: [...DATASET_KEYS.documentDetail(datasetId, documentId), 'metadata'],
    queryFn: () => datasetService.getDocumentMetadata(datasetId, documentId),
    staleTime: 300000, // 5 minutes - metadata rarely changes
    enabled: queryEnabled,
  });

  const document = documentResponse?.data;
  const metadata = metadataResponse?.data;

  // Loading states
  const isLoading = isDocumentLoading || isMetadataLoading;

  return {
    // Document data
    document,
    metadata,
    isLoading,
    documentError,
    refetchDocument,
  };
}

export function useDocumentIndexingInfo(datasetId: string, documentId: string) {
  const queryClient = useQueryClient();

  const statusQuery = useQuery<DocumentIndexingStatusResponse, unknown>({
    queryKey: DATASET_KEYS.indexingStatus(datasetId, documentId),
    queryFn: () =>
      datasetService.getDocumentIndexingStatus(datasetId, documentId).then(res => res.data),
    enabled: !!datasetId && !!documentId,
  });

  // Track last seen indexing status to detect transitions
  const lastStatusRef = useRef<string | undefined>(undefined);

  // When indexing completes, refresh document list and current document detail
  useEffect(() => {
    const currentStatus = statusQuery.data?.indexing_status;
    if (!currentStatus || !datasetId || !documentId) return;

    if (lastStatusRef.current !== currentStatus) {
      lastStatusRef.current = currentStatus;
      if (currentStatus === 'completed') {
        // Invalidate dataset documents list caches
        queryClient.invalidateQueries({ queryKey: DATASET_KEYS.documents(datasetId) });
        // Invalidate the single document cache to sync status immediately
        queryClient.invalidateQueries({
          queryKey: DATASET_KEYS.documentDetail(datasetId, documentId),
        });
        queryClient.invalidateQueries({
          queryKey: [...DATASET_KEYS.documentDetail(datasetId, documentId), 'metadata'],
        });
      }
    }
  }, [statusQuery.data?.indexing_status, queryClient, datasetId, documentId]);

  return {
    progressData: statusQuery.data,
    isLoading: statusQuery.isLoading,
    isError: statusQuery.isError,
    refetchStatus: statusQuery.refetch,
  };
}

// Random questions for dataset
export function useRandomQuestions(
  datasetId: string | undefined,
  limit: number = 10,
  enabled: boolean = true
) {
  return useQuery<ApiResponseData<RandomQuestionsResponse>, unknown>({
    queryKey: DATASET_KEYS.randomQuestions(datasetId ?? 'undefined', { limit }),
    queryFn: () => {
      if (!datasetId) throw new Error('datasetId is required');
      return datasetService.getRandomQuestions(datasetId, limit);
    },
    enabled: Boolean(datasetId) && enabled,
    staleTime: 30 * 1000,
  });
}
