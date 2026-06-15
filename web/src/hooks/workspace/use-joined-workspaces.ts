'use client';

import { useEffect, useRef } from 'react';
import { useQuery } from '@tanstack/react-query';
import { workspaceService } from '@/services/workspace.service';
import { accountService } from '@/services/account.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useWorkspaceStore } from '@/store/workspace-store';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import type { WorkspaceManagementList } from '@/services/types/workspace';
import { sessionManager } from '@/lib/auth/session-manager';
import { clearProfileClientCache } from '@/utils/client-cache';

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
  const isSwitchingOrganization =
    useOrganizationStore.use.isSwitchingOrganization();
  const autoPersistedWorkspaceIdRef = useRef<string | null>(null);

  const organizationId = currentOrganization?.id ?? null;

  const fetchJoinedWorkspacePages = async (): Promise<JoinedWorkspacePagesResult> => {
    if (!organizationId) {
      throw new Error('No organization selected');
    }
    const requestOrganizationId = organizationId;

    const firstPage = await workspaceService.getWorkspaces(requestOrganizationId, { page, limit });
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
      latestPage = await workspaceService.getWorkspaces(requestOrganizationId, {
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
    queryKey: WORKSPACE_KEYS.forSwitcher(organizationId, { page, limit }),
    queryFn: fetchJoinedWorkspacePages,
    enabled: !!organizationId && !isSwitchingOrganization,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  useEffect(() => {
    autoPersistedWorkspaceIdRef.current = null;
  }, [organizationId]);

  // 1. Sync workspaces to store and handle fallback logic
  useEffect(() => {
    if (isSwitchingOrganization) return;
    if (!syncToStore || !responseData?.data) return;
    if (responseData.organizationId !== organizationId) return;

    const transformedWorkspaces = responseData.data.map(w => ({
      id: w.id,
      name: w.name,
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
        selectWorkspace(transformedWorkspaces[0]);
      } else if (contextStatus !== 'ready') {
        selectWorkspace(stillInWorkspace);
      }
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

  // 2. Synchronize from user profile ONLY when the profile's workspace ID changes
  // and it differs from our current store value.
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
      }));

      if (profileWorkspaceId && profileWorkspaceId !== '') {
        const profileWorkspace = workspaces.find(w => w.id === profileWorkspaceId);
        if (profileWorkspace) {
          selectWorkspace(profileWorkspace);
          autoPersistedWorkspaceIdRef.current = null;
        } else {
          const fallbackWorkspace = workspaces[0];
          if (fallbackWorkspace) {
            selectWorkspace(fallbackWorkspace);
            if (autoPersistedWorkspaceIdRef.current !== fallbackWorkspace.id) {
              autoPersistedWorkspaceIdRef.current = fallbackWorkspace.id;
              void accountService
                .updateContext({ current_workspace_id: fallbackWorkspace.id })
                .then(async () => {
                  clearProfileClientCache();
                  await useAuthStore.getState().refreshProfile();
                  sessionManager.broadcastContextChanged({
                    currentWorkspaceId: fallbackWorkspace.id,
                  });
                })
                .catch(error => {
                  autoPersistedWorkspaceIdRef.current = null;
                  console.error('Failed to persist fallback workspace:', error);
                });
            }
          } else {
            markWorkspaceRequired();
          }
        }
      } else {
        const fallbackWorkspace = workspaces[0];
        if (fallbackWorkspace) {
          selectWorkspace(fallbackWorkspace);
          if (autoPersistedWorkspaceIdRef.current !== fallbackWorkspace.id) {
            autoPersistedWorkspaceIdRef.current = fallbackWorkspace.id;
            void accountService
              .updateContext({ current_workspace_id: fallbackWorkspace.id })
              .then(async () => {
                clearProfileClientCache();
                await useAuthStore.getState().refreshProfile();
                sessionManager.broadcastContextChanged({
                  currentWorkspaceId: fallbackWorkspace.id,
                });
              })
              .catch(error => {
                autoPersistedWorkspaceIdRef.current = null;
                console.error('Failed to persist fallback workspace:', error);
              });
          }
        } else {
          markWorkspaceRequired();
        }
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
