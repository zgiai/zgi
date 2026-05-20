'use client';

import { useQuery } from '@tanstack/react-query';
import { datasetService } from '@/services';
import { DATASET_KEYS } from '@/hooks/query-keys';
import type { DatasetGraph } from '@/services/types/dataset';
import type { ApiResponseData } from '@/services/types/common';

/**
 * Hook to fetch dataset graph data
 * @param datasetId The ID of the dataset
 * @returns Query result for dataset graph
 */
export function useDatasetGraph(datasetId: string) {
  return useQuery<ApiResponseData<DatasetGraph>, Error>({
    queryKey: DATASET_KEYS.graph(datasetId),
    queryFn: () => datasetService.getDatasetGraph(datasetId),
    enabled: !!datasetId,
  });
}
