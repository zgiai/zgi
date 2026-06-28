'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { fileManageService } from '@/services/file-manage.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  CreateFileProcessingRequest,
  CreateFileProcessingResponse,
} from '@/services/types/file';
import { FILES_QUERY_KEY } from '@/hooks/use-files';
import { getFileDetailKey } from '@/hooks/file/use-file-detail';
import { FILE_PARSE_PREVIEW_QUERY_KEY } from '@/hooks/file/use-file-parse-preview';
import { FILE_CHUNKS_QUERY_KEY } from '@/hooks/file/use-file-chunks';

export function useCreateFileProcessingRequest(fileId: string) {
  const t = useT('files');
  const queryClient = useQueryClient();

  return useMutation<
    ApiResponseData<CreateFileProcessingResponse>,
    unknown,
    CreateFileProcessingRequest
  >({
    mutationFn: payload => fileManageService.createProcessingRequest(fileId, payload),
    onSuccess: async () => {
      toast.success(t('detail.reparse.toasts.started'));
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: getFileDetailKey(fileId) }),
        queryClient.invalidateQueries({ queryKey: [FILES_QUERY_KEY] }),
        queryClient.invalidateQueries({ queryKey: [FILE_PARSE_PREVIEW_QUERY_KEY, fileId] }),
        queryClient.invalidateQueries({ queryKey: [FILE_CHUNKS_QUERY_KEY, fileId] }),
      ]);
    },
    onError: error => {
      toast.error((error as { message?: string }).message || t('detail.reparse.toasts.failed'));
    },
  });
}
