'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { fileManageService } from '@/services/file-manage.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  ListFileChunksRequest,
  ListFileChunksResponse,
  UpdateFileChunkRequest,
  UpdateFileChunkResponse,
} from '@/services/types/file';
import { FILES_QUERY_KEY } from '@/hooks/use-files';
import { getFileDetailKey } from '@/hooks/file/use-file-detail';

export const FILE_CHUNKS_QUERY_KEY = 'file-chunks';

export const getFileChunksKey = (fileId: string, params?: ListFileChunksRequest) => [
  FILE_CHUNKS_QUERY_KEY,
  fileId,
  params ?? {},
];

export function useFileChunks(
  fileId: string,
  params: ListFileChunksRequest = {},
  options: { enabled?: boolean } = {}
) {
  const { enabled = true } = options;

  return useQuery<ApiResponseData<ListFileChunksResponse>>({
    queryKey: getFileChunksKey(fileId, params),
    queryFn: () => fileManageService.getFileChunks(fileId, params),
    enabled: enabled && Boolean(fileId),
    retry: false,
  });
}

export function useUpdateFileChunk(fileId: string) {
  const t = useT('files');
  const queryClient = useQueryClient();

  return useMutation<
    ApiResponseData<UpdateFileChunkResponse>,
    unknown,
    { chunkId: string; data: UpdateFileChunkRequest }
  >({
    mutationFn: ({ chunkId, data }) => fileManageService.updateFileChunk(fileId, chunkId, data),
    onSuccess: async () => {
      toast.success(t('detail.chunks.toasts.updated'));
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: [FILE_CHUNKS_QUERY_KEY, fileId] }),
        queryClient.invalidateQueries({ queryKey: getFileDetailKey(fileId) }),
        queryClient.invalidateQueries({ queryKey: [FILES_QUERY_KEY] }),
      ]);
    },
    onError: error => {
      toast.error((error as { message?: string }).message || t('detail.chunks.toasts.updateFailed'));
    },
  });
}
