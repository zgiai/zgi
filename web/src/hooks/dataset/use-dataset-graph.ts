'use client';

import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from '@tanstack/react-query';
import { datasetService } from '@/services';
import { DATASET_KEYS } from '@/hooks/query-keys';
import type {
  DatasetGraph,
  GraphDatasetStatus,
  GraphRuntimeCapability,
  GraphQueryParams,
} from '@/services/types/dataset';
import type { ApiResponseData } from '@/services/types/common';

/**
 * Hook to fetch dataset graph data
 * @param datasetId The ID of the dataset
 * @returns Query result for dataset graph
 */
export function useDatasetGraph(
  datasetId: string,
  params: GraphQueryParams = {},
  options: Omit<UseQueryOptions<ApiResponseData<DatasetGraph>, Error>, 'queryKey' | 'queryFn'> = {}
) {
  return useQuery<ApiResponseData<DatasetGraph>, Error>({
    queryKey: [...DATASET_KEYS.graph(datasetId), params],
    queryFn: () => datasetService.getDatasetGraph(datasetId, params),
    ...options,
    enabled: !!datasetId && (options.enabled ?? true),
  });
}

export function useGraphRuntimeCapability() {
  return useQuery<ApiResponseData<GraphRuntimeCapability>, Error>({
    queryKey: DATASET_KEYS.graphCapability(),
    queryFn: () => datasetService.getGraphRuntimeCapability(),
    staleTime: 30_000,
    retry: false,
  });
}

export function useDatasetGraphStatus(datasetId: string, enabled = true) {
  return useQuery<ApiResponseData<GraphDatasetStatus>, Error>({
    queryKey: DATASET_KEYS.graphStatus(datasetId),
    queryFn: () => datasetService.getDatasetGraphStatus(datasetId),
    enabled: Boolean(datasetId) && enabled,
    refetchInterval: query => {
      const graphStatus = query.state.data?.data;
      if (!graphStatus) return 2_000;
      if (graphStatus.can_search) return false;
      return ['empty', 'failed', 'disabled', 'unavailable'].includes(graphStatus.status)
        ? false
        : 2_000;
    },
    retry: false,
  });
}

export function useRebuildDatasetGraph(datasetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => datasetService.rebuildDatasetGraph(datasetId),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: DATASET_KEYS.graphStatus(datasetId) }),
  });
}

export function useRetryDocumentGraph(datasetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (documentId: string) => datasetService.retryDocumentGraph(datasetId, documentId),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: DATASET_KEYS.graphStatus(datasetId) }),
  });
}
