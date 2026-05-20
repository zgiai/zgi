'use client';

import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { workspaceService } from '@/services/workspace.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useWorkspaceStore } from '@/store/workspace-store';
import { useAuthStore } from '@/store/auth-store';
import type { WorkspaceManagementList } from '@/services/types/workspace';

import { WORKSPACE_KEYS } from '@/hooks/query-keys';

const MAX_JOINED_WORKSPACE_PAGES = 100;

interface UseJoinedWorkspacesOptions {
  page?: number;
  limit?: number;
  /** If true, automatically sync to workspace store */
  syncToStore?: boolean;
}

/**
 * Hook for fetching workspaces list for workspace switcher
 * Uses /console/api/organizations/{organization_id}/workspaces endpoint
 */
export function useJoinedWorkspaces(options: UseJoinedWorkspacesOptions = {}) {
  const { page = 1, limit = 100, syncToStore = true } = options;

  const t = useT('navigation');
  const { currentOrganization } = useOrganizations();
  const setWorkspaces = useWorkspaceStore.use.setWorkspaces();
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const isOrganizationMode = useWorkspaceStore.use.isOrganizationMode();
  const enterOrganizationMode = useWorkspaceStore.use.enterOrganizationMode();
  const selectWorkspace = useWorkspaceStore.use.selectWorkspace();
  const user = useAuthStore.use.user();

  const organizationId = currentOrganization?.id ?? null;

  const fetchJoinedWorkspacePages = async (): Promise<WorkspaceManagementList> => {
    if (!organizationId) {
      throw new Error('No organization selected');
    }

    const firstPage = await workspaceService.getWorkspaces(organizationId, { page, limit });
    const seenWorkspaceIds = new Set<string>();
    const mergedWorkspaces = firstPage.data.filter(workspace => {
      if (seenWorkspaceIds.has(workspace.id)) return false;
      seenWorkspaceIds.add(workspace.id);
      return true;
    });

    let latestPage = firstPage;
    let pagesFetched = 1;
    let nextPage = (firstPage.page || page) + 1;

    while (latestPage.has_more && pagesFetched < MAX_JOINED_WORKSPACE_PAGES) {
      latestPage = await workspaceService.getWorkspaces(organizationId, {
        page: nextPage,
        limit,
      });

      latestPage.data.forEach(workspace => {
        if (seenWorkspaceIds.has(workspace.id)) return;
        seenWorkspaceIds.add(workspace.id);
        mergedWorkspaces.push(workspace);
      });

      pagesFetched += 1;
      nextPage = (latestPage.page || nextPage) + 1;
    }

    return {
      ...firstPage,
      data: mergedWorkspaces,
      total: Math.max(firstPage.total, mergedWorkspaces.length),
      has_more: latestPage.has_more,
      page: latestPage.page,
      limit: latestPage.limit,
    };
  };

  const {
    data: responseData,
    isLoading,
    isFetching,
    error,
    refetch,
  } = useQuery({
    queryKey: WORKSPACE_KEYS.forSwitcher(organizationId, { page, limit }),
    queryFn: fetchJoinedWorkspacePages,
    enabled: !!organizationId,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  // 1. Sync workspaces to store and handle fallback logic
  useEffect(() => {
    if (!syncToStore || !responseData?.data) return;

    const transformedWorkspaces = responseData.data.map(w => ({
      id: w.id,
      name: w.name,
    }));

    setWorkspaces(transformedWorkspaces);

    // If we are not in organization mode, ensure the selected workspace is still valid.
    // If it's missing or no longer in the list, fallback to Organization View.
    if (!isOrganizationMode) {
      if (!currentWorkspace) {
        enterOrganizationMode();
      } else {
        const stillInWorkspace = transformedWorkspaces.find(w => w.id === currentWorkspace.id);
        if (!stillInWorkspace) {
          enterOrganizationMode();
        }
      }
    }
  }, [
    responseData,
    syncToStore,
    setWorkspaces,
    currentWorkspace,
    isOrganizationMode,
    enterOrganizationMode,
  ]);

  // 2. Synchronize from user profile ONLY when the profile's workspace ID changes
  // and it differs from our current store value.
  useEffect(() => {
    if (!user || !responseData?.data || !syncToStore) return;

    const profileWorkspaceId = user.current_workspace_id || null;
    const storeWorkspaceId = currentWorkspace?.id ?? null;

    if (profileWorkspaceId !== storeWorkspaceId) {
      // Transform again to ensure we have the list
      const workspaces = responseData.data.map(w => ({
        id: w.id,
        name: w.name,
      }));

      // if profileWorkspaceId is not null or empty string
      if (profileWorkspaceId && profileWorkspaceId !== '') {
        const profileWorkspace = workspaces.find(w => w.id === profileWorkspaceId);
        if (profileWorkspace) {
          selectWorkspace(profileWorkspace);
        } else {
          enterOrganizationMode();
        }
      } else {
        enterOrganizationMode();
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [user?.current_workspace_id, responseData?.data, syncToStore]);

  // Show error toast if query fails
  useEffect(() => {
    if (!error) return;
    toast.error(getErrorMessage(error) || t('switchWorkspace'));
  }, [error, t]);

  return {
    workspaces: responseData?.data ?? [],
    total: responseData?.total ?? 0,
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    refetch,
  };
}
