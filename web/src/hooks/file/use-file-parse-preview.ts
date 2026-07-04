'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { fileManageService } from '@/services/file-manage.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  BatchIgnoreFileParseConfirmationsRequest,
  BatchIgnoreFileParseConfirmationsResponse,
  FileParseConfirmationAction,
  FileParsePreviewResponse,
  ResolveFileParseConfirmationRequest,
  ResolveFileParseConfirmationResponse,
} from '@/services/types/file';
import { FILES_QUERY_KEY } from '@/hooks/use-files';
import { getFileDetailKey } from '@/hooks/file/use-file-detail';

export const FILE_PARSE_PREVIEW_QUERY_KEY = 'file-parse-preview';

export const getFileParsePreviewKey = (fileId: string) => [FILE_PARSE_PREVIEW_QUERY_KEY, fileId];

export function useFileParsePreview(fileId: string, options: { enabled?: boolean } = {}) {
  const { enabled = true } = options;

  return useQuery<ApiResponseData<FileParsePreviewResponse>>({
    queryKey: getFileParsePreviewKey(fileId),
    queryFn: () => fileManageService.getParsePreview(fileId),
    enabled: enabled && Boolean(fileId),
    retry: false,
  });
}

export function useFileParseConfirmationActions(fileId: string) {
  const t = useT('files');
  const queryClient = useQueryClient();

  const invalidateCurrentFile = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: getFileParsePreviewKey(fileId) }),
      queryClient.invalidateQueries({ queryKey: getFileDetailKey(fileId) }),
      queryClient.invalidateQueries({ queryKey: [FILES_QUERY_KEY] }),
    ]);
  };

  const resolveMutation = useMutation<
    ApiResponseData<ResolveFileParseConfirmationResponse>,
    unknown,
    {
      itemId: string;
      action: FileParseConfirmationAction;
      finalContent?: string;
    }
  >({
    mutationFn: ({ itemId, action, finalContent }) => {
      const payload: ResolveFileParseConfirmationRequest = { action };
      if (finalContent !== undefined) {
        payload.final_content = finalContent;
      }
      return fileManageService.resolveParseConfirmationItem(fileId, itemId, payload);
    },
    onSuccess: async response => {
      toast.success(
        response.data.should_generate
          ? t('detail.parseReview.toasts.generateQueued')
          : t('detail.parseReview.toasts.resolved')
      );
      await invalidateCurrentFile();
    },
    onError: error => {
      toast.error((error as { message?: string }).message || t('detail.parseReview.toasts.resolveFailed'));
    },
  });

  const batchIgnoreMutation = useMutation<
    ApiResponseData<BatchIgnoreFileParseConfirmationsResponse>,
    unknown,
    BatchIgnoreFileParseConfirmationsRequest
  >({
    mutationFn: payload => fileManageService.batchIgnoreParseConfirmationItems(fileId, payload),
    onSuccess: async response => {
      toast.success(
        response.data.should_generate
          ? t('detail.parseReview.toasts.generateQueued')
          : t('detail.parseReview.toasts.batchIgnored')
      );
      await invalidateCurrentFile();
    },
    onError: error => {
      toast.error(
        (error as { message?: string }).message || t('detail.parseReview.toasts.batchIgnoreFailed')
      );
    },
  });

  return {
    resolveConfirmation: resolveMutation.mutateAsync,
    batchIgnoreConfirmations: batchIgnoreMutation.mutateAsync,
    isResolving: resolveMutation.isPending,
    isBatchIgnoring: batchIgnoreMutation.isPending,
  };
}
