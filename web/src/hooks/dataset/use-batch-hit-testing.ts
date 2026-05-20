import { useMutation } from '@tanstack/react-query';
import { datasetService } from '@/services';
import { useQuery } from '@tanstack/react-query';
import type {
  BatchHitTestingStatusResponse,
  BatchHitTestingReportResponse,
} from '@/components/datasets/batch-testing/type';

// Hook for saving batch hit testing record
export function useSaveBatchHitTestingRecord(datasetId: string, taskId: string) {
  return useMutation({
    mutationFn: async (data: { batch_name: string }) => {
      const response = await datasetService.saveBatchHitTestingRecord(datasetId, taskId, data);
      return response.data;
    },
    onSuccess: () => {
      // Could invalidate related queries if needed
      // queryClient.invalidateQueries({ queryKey: ['batch-hit-testing-records', datasetId] });
    },
  });
}

// Hook to fetch batch hit testing report
export function useBatchHitTestingReport(
  datasetId: string | undefined,
  taskId: string | undefined
) {
  return useQuery<BatchHitTestingReportResponse | undefined>({
    queryKey: ['batch-testing-report', datasetId, taskId],
    enabled: !!datasetId && !!taskId,
    queryFn: async () => {
      if (!datasetId || !taskId) return undefined;
      const resp = await datasetService.getBatchHitTestingReport(datasetId, taskId);
      return resp.data;
    },
  });
}

// Hook to fetch batch hit testing status (for live updates)
export function useBatchHitTestingStatus(
  datasetId: string | undefined,
  taskId: string | undefined
) {
  return useQuery<BatchHitTestingStatusResponse | undefined>({
    queryKey: ['batch-testing-status', datasetId, taskId],
    enabled: !!datasetId && !!taskId,
    // Poll while there are running items; stop when all reach a terminal state
    refetchInterval: query => {
      const data = query.state.data as BatchHitTestingStatusResponse | undefined;
      if (!data) return 2000;
      const results = data.results ?? [];
      const hasRunning = results.some(r => r.status === 'pending' || r.status === 'processing');
      return hasRunning ? 2000 : false;
    },
    refetchOnWindowFocus: false,
    queryFn: async () => {
      if (!datasetId || !taskId) return undefined;
      const resp = await datasetService.getBatchHitTestingStatus(datasetId, taskId);
      return resp.data;
    },
  });
}
