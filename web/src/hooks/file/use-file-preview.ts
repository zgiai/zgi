'use client';

import { useQuery } from '@tanstack/react-query';
import { fileService } from '@/services/file.service';
import { toast } from 'sonner';
import { FILE_KEYS } from '@/hooks/query-keys';

interface UseFilePreviewOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
}

export const getFilePreviewKey = (fileId?: string) => FILE_KEYS.preview(fileId);

export function useFilePreview(
  fileId?: string,
  options: UseFilePreviewOptions = {}
): {
  content: string;
  isLoading: boolean;
  error: string | null;
  refetch: () => void;
} {
  const {
    enabled = !!fileId,
    staleTime = 5 * 60 * 1000,
    gcTime = 5 * 60 * 1000,
    refetchOnWindowFocus = false,
  } = options;

  const { data, isLoading, error, refetch } = useQuery<string>({
    queryKey: getFilePreviewKey(fileId),
    enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    retry: false,
    queryFn: async () => {
      if (!fileId) return '';
      const content = await fileService.getFilePreview(fileId);
      return typeof content === 'string' ? content : '';
    },
  });

  if (error) {
    const message = (error as { message?: string }).message ?? 'Failed to load file preview';
    toast.error(message);
  }

  return {
    content: typeof data === 'string' ? data : '',
    isLoading,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch,
  };
}
