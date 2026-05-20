'use client';

import { useCallback } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { datasetService } from '@/services';
import type { DocumentDetail } from '@/services/types/dataset';
import type { ApiResponseData } from '@/services/types/common';
import { DATASET_KEYS } from '@/hooks/query-keys';

interface UseDocumentActionsProps {
  datasetId: string;
  documentId: string;
  /** Current document data for checking state before actions */
  document?: DocumentDetail | null;
  /** Callback to refetch document after changes */
  onDocumentChange?: () => void;
}

/**
 * Hook for document-level operations
 * Handles enable/disable, archive/unarchive, delete
 */
export function useDocumentActions({
  datasetId,
  documentId,
  document,
  onDocumentChange,
}: UseDocumentActionsProps) {
  const t = useT('datasets');
  const queryClient = useQueryClient();

  // Enable document mutation with optimistic update
  const enableDocumentMutation = useMutation({
    mutationFn: () => datasetService.batchEnableDocuments(datasetId, [documentId]),
    onMutate: async () => {
      await queryClient.cancelQueries({
        queryKey: DATASET_KEYS.documentDetail(datasetId, documentId),
      });

      const prevDetail = queryClient.getQueryData<ApiResponseData<DocumentDetail>>(
        DATASET_KEYS.documentDetail(datasetId, documentId)
      );

      queryClient.setQueryData(
        DATASET_KEYS.documentDetail(datasetId, documentId),
        (old: ApiResponseData<DocumentDetail> | undefined) =>
          old ? { ...old, data: { ...old.data, enabled: true, display_status: 'enabled' } } : old
      );

      return { prevDetail };
    },
    onSuccess: () => {
      toast.success(t('messages.enableSuccess'));
      onDocumentChange?.();
    },
    onError: (_error, _vars, ctx) => {
      if (ctx?.prevDetail) {
        queryClient.setQueryData(
          DATASET_KEYS.documentDetail(datasetId, documentId),
          ctx.prevDetail
        );
      }
      toast.error(t('messages.enableFailed'));
    },
  });

  // Disable document mutation with optimistic update
  const disableDocumentMutation = useMutation({
    mutationFn: () => datasetService.batchDisableDocuments(datasetId, [documentId]),
    onMutate: async () => {
      await queryClient.cancelQueries({
        queryKey: DATASET_KEYS.documentDetail(datasetId, documentId),
      });

      const prevDetail = queryClient.getQueryData<ApiResponseData<DocumentDetail>>(
        DATASET_KEYS.documentDetail(datasetId, documentId)
      );

      queryClient.setQueryData(
        DATASET_KEYS.documentDetail(datasetId, documentId),
        (old: ApiResponseData<DocumentDetail> | undefined) =>
          old ? { ...old, data: { ...old.data, enabled: false, display_status: 'disabled' } } : old
      );

      return { prevDetail };
    },
    onSuccess: () => {
      toast.success(t('messages.disableSuccess'));
      onDocumentChange?.();
    },
    onError: (_error, _vars, ctx) => {
      if (ctx?.prevDetail) {
        queryClient.setQueryData(
          DATASET_KEYS.documentDetail(datasetId, documentId),
          ctx.prevDetail
        );
      }
      toast.error(t('messages.disableFailed'));
    },
  });

  // Archive document mutation
  const archiveDocumentMutation = useMutation({
    mutationFn: () => datasetService.archiveDocuments(datasetId, [documentId]),
    onSuccess: () => {
      toast.success(t('messages.archiveSuccess'));
      onDocumentChange?.();
    },
    onError: () => {
      toast.error(t('messages.archiveFailed'));
    },
  });

  // Unarchive document mutation
  const unarchiveDocumentMutation = useMutation({
    mutationFn: () => datasetService.unarchiveDocuments(datasetId, [documentId]),
    onSuccess: () => {
      toast.success(t('messages.unarchiveSuccess'));
      onDocumentChange?.();
    },
    onError: () => {
      toast.error(t('messages.unarchiveFailed'));
    },
  });

  // Delete document mutation
  const deleteDocumentMutation = useMutation({
    mutationFn: () => datasetService.deleteDocuments(datasetId, documentId),
    onSuccess: () => {
      toast.success(t('messages.deleteSuccess'));
      // Don't call onDocumentChange as the document is deleted
    },
    onError: () => {
      toast.error(t('messages.deleteFailed'));
    },
  });

  // Actions
  const enableDocument = useCallback(() => {
    if (document?.enabled) return;
    enableDocumentMutation.mutate();
  }, [document?.enabled, enableDocumentMutation]);

  const disableDocument = useCallback(() => {
    if (!document?.enabled) return;
    disableDocumentMutation.mutate();
  }, [document?.enabled, disableDocumentMutation]);

  const archiveDocument = useCallback(() => {
    if (document?.archived) return;
    archiveDocumentMutation.mutate();
  }, [document?.archived, archiveDocumentMutation]);

  const unarchiveDocument = useCallback(() => {
    if (!document?.archived) return;
    unarchiveDocumentMutation.mutate();
  }, [document?.archived, unarchiveDocumentMutation]);

  const deleteDocument = useCallback(() => {
    deleteDocumentMutation.mutate();
  }, [deleteDocumentMutation]);

  // Loading state
  const isDocumentMutating =
    enableDocumentMutation.isPending ||
    disableDocumentMutation.isPending ||
    archiveDocumentMutation.isPending ||
    unarchiveDocumentMutation.isPending ||
    deleteDocumentMutation.isPending;

  return {
    // Actions
    enableDocument,
    disableDocument,
    archiveDocument,
    unarchiveDocument,
    deleteDocument,

    // Loading state
    isDocumentMutating,
  };
}
