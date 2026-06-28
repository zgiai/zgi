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
import { useOrganizationStore } from '@/store/organization-store';
import type { WorkspaceManagementList } from '@/services/types/workspace';

import { WORKSPACE_KEYS } from '@/hooks/query-keys';

const MAX_JOINED_WORKSPACE_PAGES = 100;

type JoinedWorkspacePagesResult = WorkspaceManagementList & {
  organizationId: string;
};

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
  const contextStatus = useWorkspaceStore.use.contextStatus();
  const markWorkspaceRequired = useWorkspaceStore.use.markWorkspaceRequired();
  const selectWorkspace = useWorkspaceStore.use.selectWorkspace();
  const user = useAuthStore.use.user();
  const isSwitchingOrganization = useOrganizationStore.use.isSwitchingOrganization();

  const organizationId = currentOrganization?.id ?? null;
  const accountId = user?.id ?? null;

  const fetchJoinedWorkspacePages = async (): Promise<JoinedWorkspacePagesResult> => {
    if (!organizationId || !accountId) {
      throw new Error('No organization or account selected');
    }
    const requestOrganizationId = organizationId;
    const requestAccountId = accountId;

    const firstPage = await workspaceService.getJoinedWorkspaces(
      requestOrganizationId,
      requestAccountId,
      { page, limit }
    );
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
      latestPage = await workspaceService.getJoinedWorkspaces(
        requestOrganizationId,
        requestAccountId,
        {
          page: nextPage,
          limit,
        }
      );

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
      organizationId: requestOrganizationId,
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
    queryKey: WORKSPACE_KEYS.forSwitcher(organizationId, { page, limit, accountId }),
    queryFn: fetchJoinedWorkspacePages,
    enabled: !!organizationId && !!accountId && !isSwitchingOrganization,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  // 1. Sync workspaces to store and handle fallback logic
  useEffect(() => {
    if (isSwitchingOrganization) return;
    if (!syncToStore || !responseData?.data) return;
    if (responseData.organizationId !== organizationId) return;

    const transformedWorkspaces = responseData.data.map(w => ({
      id: w.id,
      name: w.name,
      leader_id: w.leader_id,
      leader_name: w.leader_name,
    }));

    setWorkspaces(transformedWorkspaces);

    if (transformedWorkspaces.length === 0) {
      if (contextStatus !== 'workspace_required' || currentWorkspace) {
        markWorkspaceRequired();
      }
      return;
    }

    if (currentWorkspace) {
      const stillInWorkspace = transformedWorkspaces.find(w => w.id === currentWorkspace.id);
      if (!stillInWorkspace) {
        markWorkspaceRequired();
      } else if (contextStatus !== 'ready') {
        selectWorkspace(stillInWorkspace);
      }
      return;
    }

    if (contextStatus !== 'workspace_required') {
      markWorkspaceRequired();
    }
  }, [
    responseData,
    responseData?.organizationId,
    syncToStore,
    setWorkspaces,
    currentWorkspace,
    contextStatus,
    markWorkspaceRequired,
    selectWorkspace,
    organizationId,
    isSwitchingOrganization,
  ]);

  // 2. Synchronize from user profile ONLY when the profile's workspace ID changes.
  // A missing profile workspace is a valid organization-only state; do not
  // auto-persist the first workspace as a fallback.
  useEffect(() => {
    if (isSwitchingOrganization) return;
    if (!user || !responseData?.data || !syncToStore) return;
    if (responseData.organizationId !== organizationId) return;

    const profileWorkspaceId = user.current_workspace_id || null;
    const storeWorkspaceId = currentWorkspace?.id ?? null;

    if (profileWorkspaceId !== storeWorkspaceId) {
      // Transform again to ensure we have the list
      const workspaces = responseData.data.map(w => ({
        id: w.id,
        name: w.name,
        leader_id: w.leader_id,
        leader_name: w.leader_name,
      }));

      if (profileWorkspaceId && profileWorkspaceId !== '') {
        const profileWorkspace = workspaces.find(w => w.id === profileWorkspaceId);
        if (profileWorkspace) {
          selectWorkspace(profileWorkspace);
        } else {
          markWorkspaceRequired();
        }
      } else {
        markWorkspaceRequired();
      }
    }
  }, [
    user,
    user?.current_workspace_id,
    responseData?.data,
    responseData?.organizationId,
    organizationId,
    syncToStore,
    currentWorkspace?.id,
    selectWorkspace,
    markWorkspaceRequired,
    isSwitchingOrganization,
  ]);

  // Show error toast if query fails
  useEffect(() => {
    if (!error) return;
    toast.error(getErrorMessage(error) || t('switchWorkspace'));
  }, [error, t]);

  return {
    workspaces:
      !isSwitchingOrganization && responseData?.organizationId === organizationId
        ? responseData.data
        : [],
    total:
      !isSwitchingOrganization && responseData?.organizationId === organizationId
        ? responseData.total
        : 0,
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    refetch,
  };
}
