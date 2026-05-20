'use client';

import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { toast } from 'sonner';
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
  error: string | null;
  refetch: () => void;
} {
  const t = useT('files');
  const {
    enabled = !!fileId,
    staleTime = 60 * 1000,
    gcTime = 5 * 60 * 1000,
    refetchOnWindowFocus = false,
  } = options;

  const { data, isLoading, error, refetch } = useQuery<FileOriginalPreviewUrlResponse>({
    queryKey: getFileOriginalPreviewUrlKey(fileId),
    enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    retry: false,
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

  useEffect(() => {
    if (!error) return;

    toast.error((error as { message?: string }).message ?? t('preview.loadError'));
  }, [error, t]);

  return {
    preview: data ?? null,
    previewUrl: data?.url ?? '',
    isLoading,
    error: error ? ((error as { message?: string }).message ?? t('preview.loadError')) : null,
    refetch: () => {
      void refetch();
    },
  };
}
