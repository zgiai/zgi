import { useMutation } from '@tanstack/react-query';
import { datasetService } from '@/services';
import type {
  HitTestingRequest,
  HitTestingResponse,
  ExternalHitTestingRequest,
  ExternalDatasetHitTestingResponse,
} from '@/services/types/dataset';

/**
 * @hook useVectorRetrieval
 * @description Hook for vector retrieval in dataset hit testing
 * @usage Use for internal datasets to perform vector-based retrieval
 */
export function useVectorRetrieval(datasetId: string) {
  return useMutation({
    mutationFn: (data: HitTestingRequest) => datasetService.vectorRetrieval(datasetId, data),
  });
}

/**
 * @hook useGraphRetrieval
 * @description Hook for graph retrieval in dataset hit testing
 * @usage Use for graph-enabled datasets to perform graph-based retrieval
 */
export function useGraphRetrieval(datasetId: string) {
  return useMutation({
    mutationFn: (data: HitTestingRequest) => datasetService.graphRetrieval(datasetId, data),
  });
}

/**
 * @hook useExternalHitTesting
 * @description Hook for external dataset hit testing
 * @usage Use for external knowledge base retrieval
 */
export function useExternalHitTesting(datasetId: string) {
  return useMutation({
    mutationFn: (data: ExternalHitTestingRequest) =>
      datasetService.externalHitTesting(datasetId, data),
  });
}

/**
 * @hook useDualRetrieval
 * @description Hook for parallel vector and graph retrieval
 * @usage Use for graph-enabled datasets to perform both retrievals in parallel
 * @returns Object with vectorMutate and graphMutate functions, plus loading states
 */
export function useDualRetrieval(datasetId: string) {
  const vectorMutation = useVectorRetrieval(datasetId);
  const graphMutation = useGraphRetrieval(datasetId);

  return {
    vectorMutate: vectorMutation.mutate,
    vectorMutateAsync: vectorMutation.mutateAsync,
    graphMutate: graphMutation.mutate,
    graphMutateAsync: graphMutation.mutateAsync,
    isVectorLoading: vectorMutation.isPending,
    isGraphLoading: graphMutation.isPending,
    vectorData: vectorMutation.data,
    graphData: graphMutation.data,
    vectorError: vectorMutation.error,
    graphError: graphMutation.error,
  };
}
