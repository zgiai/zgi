'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { organizationService } from '@/services/organization.service';
import type {
  WorkspaceAssetMoveItem,
  WorkspaceAssetMoveRequest,
} from '@/services/types/organization';
import { AGENT_KEYS, DATASET_KEYS, DB_KEYS, WORKSPACE_KEYS } from '@/hooks/query-keys';
import { DATASET_FOLDERS_QUERY_KEY } from '@/hooks/dataset/use-dataset-folders';
import { FILES_QUERY_KEY } from '@/hooks/use-files';
import { getErrorMessage } from '@/utils/error-notifications';
import { getAgentResourceBoundImpact } from '@/utils/agent-resource-bound';
import { useOrganizations } from '@/hooks/organization/use-organizations';

export function useWorkspaceAssetMove() {
  const t = useT('common');
  const queryClient = useQueryClient();

  const previewMutation = useMutation({
    mutationFn: async (request: WorkspaceAssetMoveRequest) =>
      organizationService.previewWorkspaceAssetMove(request),
  });

  const dependencyMutation = useMutation({
    mutationFn: async (request: { items: WorkspaceAssetMoveItem[] }) =>
      organizationService.previewWorkspaceAssetMoveDependencies(request),
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
      if (getAgentResourceBoundImpact(error)) return;
      toast.error(getErrorMessage(error) || t('assetMove.moveFailed'));
    },
  });

  return {
    dependencyMutation,
    previewMutation,
    moveMutation,
  };
}

export function useWorkspaceAssetMoveEligibleTargets(
  items: WorkspaceAssetMoveItem[],
  enabled: boolean
) {
  const { currentOrganization } = useOrganizations();
  const itemKey = items
    .map(item => `${item.type}:${item.id}`)
    .sort()
    .join('|');

  return useQuery({
    queryKey: [
      'workspace-asset-move',
      'eligible-targets',
      currentOrganization?.id ?? null,
      itemKey,
    ],
    enabled:
      enabled &&
      Boolean(currentOrganization?.id) &&
      items.length > 0 &&
      items.every(item => Boolean(item.type && item.id)),
    staleTime: 0,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
    queryFn: async () => {
      const limit = 100;
      const firstPage = await organizationService.getWorkspaceAssetMoveEligibleTargets({
        items,
        page: 1,
        limit,
      });
      const targets = [...firstPage.data];
      const totalPages = Math.ceil(firstPage.total / limit);
      for (let page = 2; page <= totalPages; page += 1) {
        const response = await organizationService.getWorkspaceAssetMoveEligibleTargets({
          items,
          page,
          limit,
        });
        targets.push(...response.data);
      }
      return targets;
    },
  });
}
