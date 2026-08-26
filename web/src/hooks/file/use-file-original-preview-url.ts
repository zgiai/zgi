'use client';

import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { FILE_KEYS } from '@/hooks/query-keys';
import { fileManageService } from '@/services/file-manage.service';
import type { FileOriginalPreviewUrlResponse } from '@/services/types/file';

interface UseFileOriginalPreviewUrlOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
}

interface FilePreviewRequestError {
  message?: string;
  response?: {
    status?: number;
    data?: { code?: string | number; message?: string; errorMessage?: string };
  };
  businessError?: { code?: string | number; message?: string };
}

function isMissingFilePreviewError(error: unknown): boolean {
  const candidate = error as FilePreviewRequestError | null | undefined;
  if (!candidate) return false;
  if (candidate.response?.status === 404) return true;

  const code = String(candidate.businessError?.code ?? candidate.response?.data?.code ?? '').trim();
  if (code === '210001') return true;

  const message = (
    candidate.businessError?.message ||
    candidate.response?.data?.message ||
    candidate.response?.data?.errorMessage ||
    candidate.message ||
    ''
  )
    .trim()
    .toLowerCase();
  return (
    message.includes('file not found') ||
    message.includes('文件不存在') ||
    message.includes('文件已删除')
  );
}

export const getFileOriginalPreviewUrlKey = (fileId?: string) =>
  FILE_KEYS.originalPreviewUrl(fileId);

/**
 * @hook useFileOriginalPreviewUrl
 * @description Fetches a signed original file preview URL for browser-renderable files.
 */
export function useFileOriginalPreviewUrl(
  fileId?: string,
  options: UseFileOriginalPreviewUrlOptions = {}
): {
  preview: FileOriginalPreviewUrlResponse | null;
  previewUrl: string;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  isMissing: boolean;
  refetch: () => void;
} {
  const t = useT('files');
  const queryClient = useQueryClient();
  const {
    enabled = !!fileId,
    staleTime = 60 * 1000,
    gcTime = 5 * 60 * 1000,
    refetchOnWindowFocus = false,
  } = options;
  const queryKey = getFileOriginalPreviewUrlKey(fileId);
  const cachedError = queryClient.getQueryState(queryKey)?.error;

  const { data, isLoading, isFetching, error, refetch } = useQuery<FileOriginalPreviewUrlResponse>({
    queryKey,
    enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    retry: (failureCount, requestError) =>
      !isMissingFilePreviewError(requestError) && failureCount < 2,
    retryOnMount: !isMissingFilePreviewError(cachedError),
    refetchOnReconnect: query => !isMissingFilePreviewError(query.state.error),
    queryFn: async () => {
      if (!fileId) {
        throw new Error(t('preview.noFileSelected'));
      }

      const response = await fileManageService.getOriginalPreviewUrl(fileId);
      if (!response.data?.url) {
        throw new Error(response.message || t('preview.loadError'));
      }

      return response.data;
    },
  });

  const isMissing = isMissingFilePreviewError(error);
  const errorMessage = error
    ? isMissing
      ? t('preview.fileMissing')
      : ((error as { message?: string }).message ?? t('preview.loadError'))
    : null;

  return {
    preview: data ?? null,
    previewUrl: data?.url ?? '',
    isLoading,
    isFetching,
    error: errorMessage,
    isMissing,
    refetch: () => {
      void refetch();
    },
  };
}
