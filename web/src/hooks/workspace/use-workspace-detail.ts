'use client';

import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { workspaceService } from '@/services/workspace.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import type { WorkspaceManagement } from '@/services/types/workspace';

import { WORKSPACE_KEYS } from '@/hooks/query-keys';

/**
 * Hook for fetching workspace detail by ID
 */
export function useWorkspaceDetail(workspaceId: string) {
  const t = useT();
  const { currentOrganization } = useOrganizations();

  const {
    data: workspaceDetail,
    isLoading,
    isFetching,
    error,
    refetch,
  } = useQuery<WorkspaceManagement>({
    queryKey: WORKSPACE_KEYS.detail(currentOrganization?.id ?? null, workspaceId),
    queryFn: async () => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      if (!workspaceId) {
        throw new Error('Workspace ID is required');
      }
      return await workspaceService.getWorkspaceDetail(currentOrganization.id, workspaceId);
    },
    enabled: !!currentOrganization?.id && !!workspaceId,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  // Show error toast if query fails
  useEffect(() => {
    if (!error) return;
    toast.error(
      getErrorMessage(error) || t('dashboard.organization.workspaceManagement.loadError')
    );
  }, [error, toast, t]);

  return {
    workspaceDetail,
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    refetch,
  };
}
