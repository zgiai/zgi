'use client';

import { useMemo, useState } from 'react';
import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { datasetService } from '@/services';
import type {
  DatasetFileCandidate,
  DatasetFileCandidateFilter,
  DatasetFileRef,
} from '@/services/types/dataset';
import type { ApiResponseData } from '@/services/types/common';
import type { FileProcessingRequestView } from '@/services/types/file';
import { DATASET_KEYS } from '@/hooks/query-keys';
import { useDebouncedValue } from '@/hooks/use-debounced-value';

type GenerateDatasetFileCandidateEmbeddingVariables =
  | string
  | {
      assetId: string;
      silent?: boolean;
    };

interface DatasetFileCandidateEmbeddingTaskRef {
  assetId: string;
  requestId: string;
}

const TERMINAL_PROCESSING_REQUEST_STATUSES = new Set(['completed', 'failed', 'cancelled']);

function getEmbeddingAssetId(variables: GenerateDatasetFileCandidateEmbeddingVariables) {
  return typeof variables === 'string' ? variables : variables.assetId;
}

function isEmbeddingMutationSilent(variables: GenerateDatasetFileCandidateEmbeddingVariables) {
  return typeof variables === 'string' ? false : variables.silent === true;
}

export function useDatasetFileCandidates(
  datasetId: string | undefined,
  params: {
    filter?: DatasetFileCandidateFilter;
    keyword?: string;
    page?: number;
    limit?: number;
  } = {},
  options: { enabled?: boolean; debounceDelay?: number; refetchInterval?: number | false } = {}
) {
  const [keyword, setKeyword] = useState(params.keyword ?? '');
  const debouncedKeyword = useDebouncedValue(keyword, options.debounceDelay ?? 500);

  const normalizedParams = useMemo(
    () => ({
      filter: params.filter ?? 'addable',
      keyword: debouncedKeyword,
      page: params.page ?? 1,
      limit: params.limit ?? 20,
    }),
    [params.filter, params.page, params.limit, debouncedKeyword]
  );

  const query = useQuery({
    queryKey: datasetId
      ? DATASET_KEYS.fileCandidates(datasetId, normalizedParams)
      : DATASET_KEYS.fileCandidates('undefined', normalizedParams),
    queryFn: () => {
      if (!datasetId) {
        throw new Error('datasetId is required');
      }
      return datasetService.getDatasetFileCandidates(datasetId, normalizedParams);
    },
    enabled: Boolean(datasetId) && (options.enabled ?? true),
    staleTime: 30 * 1000,
    refetchInterval: options.refetchInterval ?? false,
    retry: false,
  });

  const candidates = useMemo(
    () => (query.data?.data?.items ?? []) as DatasetFileCandidate[],
    [query.data?.data?.items]
  );

  return {
    ...query,
    candidates,
    total: query.data?.data?.total ?? 0,
    keyword,
    setKeyword,
  };
}

export function useDatasetFileRefs(
  datasetId: string | undefined,
  params: {
    sync_status?: string;
    page?: number;
    limit?: number;
  } = {},
  options: { enabled?: boolean; refetchInterval?: number | false } = {}
) {
  const normalizedParams = useMemo(
    () => ({
      sync_status: params.sync_status,
      page: params.page ?? 1,
      limit: params.limit ?? 100,
    }),
    [params.sync_status, params.page, params.limit]
  );

  const query = useQuery({
    queryKey: datasetId
      ? DATASET_KEYS.fileRefs(datasetId, normalizedParams)
      : DATASET_KEYS.fileRefs('undefined', normalizedParams),
    queryFn: () => {
      if (!datasetId) {
        throw new Error('datasetId is required');
      }
      return datasetService.getDatasetFileRefs(datasetId, normalizedParams);
    },
    enabled: Boolean(datasetId) && (options.enabled ?? true),
    staleTime: 30 * 1000,
    refetchInterval: options.refetchInterval ?? false,
    retry: false,
  });

  const refs = useMemo(
    () => (query.data?.data?.items ?? []) as DatasetFileRef[],
    [query.data?.data?.items]
  );

  return {
    ...query,
    refs,
    total: query.data?.data?.total ?? 0,
  };
}

export function useDatasetFileCandidateEmbeddingTasks(
  datasetId: string | undefined,
  tasks: DatasetFileCandidateEmbeddingTaskRef[],
  options: { enabled?: boolean; refetchInterval?: number } = {}
) {
  const queries = useQueries({
    queries: tasks.map(task => ({
      queryKey:
        datasetId && task.assetId && task.requestId
          ? DATASET_KEYS.fileCandidateEmbeddingTask(datasetId, task.assetId, task.requestId)
          : DATASET_KEYS.fileCandidateEmbeddingTask('undefined', task.assetId, task.requestId),
      queryFn: () => {
        if (!datasetId) {
          throw new Error('datasetId is required');
        }
        return datasetService.getDatasetFileCandidateEmbeddingTask(
          datasetId,
          task.assetId,
          task.requestId
        );
      },
      enabled: Boolean(datasetId) && Boolean(task.assetId) && Boolean(task.requestId) && (options.enabled ?? true),
      refetchInterval: (query: { state: { data?: unknown } }) => {
        const request = (query.state.data as ApiResponseData<FileProcessingRequestView> | undefined)
          ?.data;
        if (request && TERMINAL_PROCESSING_REQUEST_STATUSES.has(String(request.status))) {
          return false;
        }
        return options.refetchInterval ?? 2000;
      },
      retry: false,
    })),
  });

  return useMemo(() => {
    const byAssetId = new Map<string, FileProcessingRequestView>();
    queries.forEach((query, index) => {
      const request = query.data?.data;
      if (request) {
        byAssetId.set(tasks[index].assetId, request);
      }
    });
    return byAssetId;
  }, [queries, tasks]);
}

function invalidateDatasetFileRefQueries(queryClient: QueryClient, datasetId: string) {
  queryClient.invalidateQueries({ queryKey: DATASET_KEYS.fileRefs(datasetId) });
  queryClient.invalidateQueries({
    queryKey: [...DATASET_KEYS.all, 'file-candidates', datasetId],
  });
  queryClient.invalidateQueries({ queryKey: DATASET_KEYS.documents(datasetId) });
  queryClient.invalidateQueries({ queryKey: DATASET_KEYS.detail(datasetId) });
}

export function useCreateDatasetFileRefs(datasetId: string) {
  const t = useT('datasets');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (assetIds: string[]) => datasetService.createDatasetFileRefs(datasetId, assetIds),
    onSuccess: response => {
      const items = response.data?.items ?? [];
      const successCount = items.filter(item => item.success).length;
      if (successCount > 0) {
        toast.success(t('messages.fileRefsCreateSuccess', { count: successCount }));
      }
      const failedCount = items.length - successCount;
      if (failedCount > 0) {
        toast.error(t('messages.fileRefsCreatePartialFailed', { count: failedCount }));
      }
      invalidateDatasetFileRefQueries(queryClient, datasetId);
    },
    onError: (error: unknown) => {
      const message = (error as { message?: string }).message ?? t('messages.actionFailed');
      toast.error(message);
    },
  });
}

export function useGenerateDatasetFileCandidateEmbeddings(datasetId: string) {
  const t = useT('datasets');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (variables: GenerateDatasetFileCandidateEmbeddingVariables) =>
      datasetService.generateDatasetFileCandidateEmbeddings(datasetId, getEmbeddingAssetId(variables)),
    onSuccess: (response, variables) => {
      if (!isEmbeddingMutationSilent(variables)) {
        if (!response.data?.accepted && !response.data?.addable) {
          toast.error(response.data?.reason || t('messages.actionFailed'));
        }
      }
      invalidateDatasetFileRefQueries(queryClient, datasetId);
    },
    onError: (error: unknown, variables) => {
      if (!isEmbeddingMutationSilent(variables)) {
        const message = (error as { message?: string }).message ?? t('messages.actionFailed');
        toast.error(message);
      }
    },
  });
}

export function useRetryDatasetFileRefSync(datasetId: string) {
  const t = useT('datasets');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (refId: string) => datasetService.retryDatasetFileRefSync(datasetId, refId),
    onSuccess: (response: ApiResponseData<{ success?: boolean; reason?: string }>) => {
      if (response.data?.success === false) {
        toast.error(response.data.reason || t('messages.actionFailed'));
      } else {
        toast.success(t('messages.fileRefRetrySuccess'));
      }
      invalidateDatasetFileRefQueries(queryClient, datasetId);
    },
    onError: (error: unknown) => {
      const message = (error as { message?: string }).message ?? t('messages.actionFailed');
      toast.error(message);
    },
  });
}

export function useDeleteDatasetFileRef(datasetId: string) {
  const t = useT('datasets');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (refId: string) => datasetService.deleteDatasetFileRef(datasetId, refId),
    onSuccess: () => {
      toast.success(t('messages.deleteSuccess'));
      invalidateDatasetFileRefQueries(queryClient, datasetId);
    },
    onError: (error: unknown) => {
      const message = (error as { message?: string }).message ?? t('messages.actionFailed');
      toast.error(message);
    },
  });
}

export function useBulkDeleteDatasetFileRefs(datasetId: string) {
  const t = useT('datasets');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (refIds: string[]) => {
      const results = await Promise.allSettled(
        refIds.map(refId => datasetService.deleteDatasetFileRef(datasetId, refId))
      );
      const failedCount = results.filter(result => result.status === 'rejected').length;

      if (failedCount > 0) {
        throw new Error(t('documents.fileRefs.batchRemoveFailed', { count: failedCount }));
      }

      return { count: refIds.length };
    },
    onSuccess: result => {
      toast.success(t('documents.fileRefs.batchRemoveSuccess', { count: result.count }));
    },
    onError: (error: unknown) => {
      const message =
        (error as { message?: string }).message ?? t('documents.fileRefs.batchRemoveFailed');
      toast.error(message);
    },
    onSettled: () => {
      invalidateDatasetFileRefQueries(queryClient, datasetId);
    },
  });
}
