'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { organizationService } from '@/services/organization.service';
import type { WorkspaceAssetMoveRequest } from '@/services/types/organization';
import {
  AGENT_KEYS,
  DATASET_KEYS,
  DB_KEYS,
  WORKSPACE_KEYS,
} from '@/hooks/query-keys';
import { DATASET_FOLDERS_QUERY_KEY } from '@/hooks/dataset/use-dataset-folders';
import { FILES_QUERY_KEY } from '@/hooks/use-files';
import { getErrorMessage } from '@/utils/error-notifications';

export function useWorkspaceAssetMove() {
  const t = useT('common');
  const queryClient = useQueryClient();

  const previewMutation = useMutation({
    mutationFn: async (request: WorkspaceAssetMoveRequest) =>
      organizationService.previewWorkspaceAssetMove(request),
    onError: error => {
      toast.error(getErrorMessage(error) || t('assetMove.previewFailed'));
    },
  });

  const moveMutation = useMutation({
    mutationFn: async (request: WorkspaceAssetMoveRequest) =>
      organizationService.moveWorkspaceAssets(request),
    onSuccess: () => {
      toast.success(t('assetMove.moveSuccess'));
      void queryClient.invalidateQueries({ queryKey: AGENT_KEYS.all });
      void queryClient.invalidateQueries({ queryKey: DATASET_KEYS.all });
      void queryClient.invalidateQueries({ queryKey: [DATASET_FOLDERS_QUERY_KEY] });
      void queryClient.invalidateQueries({ queryKey: [FILES_QUERY_KEY] });
      void queryClient.invalidateQueries({ queryKey: DB_KEYS.all });
      void queryClient.invalidateQueries({ queryKey: WORKSPACE_KEYS.all });
      void queryClient.refetchQueries({ queryKey: [DATASET_FOLDERS_QUERY_KEY], type: 'active' });
      void queryClient.refetchQueries({ queryKey: [FILES_QUERY_KEY], type: 'active' });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('assetMove.moveFailed'));
    },
  });

  return {
    previewMutation,
    moveMutation,
  };
}
