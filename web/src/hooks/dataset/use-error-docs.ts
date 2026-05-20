'use client';

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { datasetService } from '@/services';
import type { ApiResponseData } from '@/services/types/common';
import type { ErrorDocsResponse } from '@/services/types/dataset';

const DATASETS_QUERY_KEY = 'datasets';

export function useErrorDocs(
  datasetId: string | undefined,
  options: Omit<
    Parameters<typeof useQuery<ApiResponseData<ErrorDocsResponse>, unknown>>[0],
    'queryKey' | 'queryFn'
  > = {}
) {
  return useQuery<ApiResponseData<ErrorDocsResponse>, unknown>({
    queryKey: datasetId
      ? [DATASETS_QUERY_KEY, 'error-docs', datasetId]
      : [DATASETS_QUERY_KEY, 'error-docs', 'undefined'],
    queryFn: () => {
      if (!datasetId) throw new Error('datasetId is required');
      return datasetService.getErrorDocs(datasetId);
    },
    enabled: Boolean(datasetId) && (options.enabled ?? true),
    ...options,
  });
}

export function useRetryErrorDocs(datasetId: string | undefined) {
  const queryClient = useQueryClient();
  const t = useT('datasets');

  return useMutation({
    mutationFn: async ({ documentIds }: { documentIds: string[] }) => {
      if (!datasetId) throw new Error('datasetId is required');
      return datasetService.retryErrorDocs(datasetId, { document_ids: documentIds });
    },
    onSuccess: () => {
      toast.success(t('documents.errorBanner.retriedSuccess'));
      queryClient.invalidateQueries({ queryKey: [DATASETS_QUERY_KEY, 'error-docs', datasetId] });
      queryClient.invalidateQueries({ queryKey: [DATASETS_QUERY_KEY, 'documents', datasetId] });
    },
    onError: (error: unknown) => {
      const message =
        (error as { message?: string }).message || t('documents.errorBanner.retriedFail');
      toast.error(message);
    },
  });
}
