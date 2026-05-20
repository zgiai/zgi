'use client';

import { useQuery } from '@tanstack/react-query';
import { workspaceService } from '@/services/workspace.service';
import { WORKSPACE_KEYS } from '@/hooks/query-keys';
import type { WorkspaceList } from '@/services/types/workspace';

const MAX_MANAGED_WORKSPACE_PAGES = 100;

/**
 * Hook for fetching managed workspaces list for an organization.
 * Managed workspaces are those where the user has management permissions.
 */
export function useManagedWorkspaces(organizationId: string, enabled = true) {
  return useQuery({
    queryKey: WORKSPACE_KEYS.managed(organizationId),
    queryFn: async () => {
      if (!organizationId) {
        return [];
      }

      const firstPage = await workspaceService.getManagedWorkspaces(organizationId, {
        limit: 100,
        page: 1,
      });
      const seenWorkspaceIds = new Set<string>();
      const mergedWorkspaces = firstPage.data.filter(workspace => {
        if (seenWorkspaceIds.has(workspace.id)) return false;
        seenWorkspaceIds.add(workspace.id);
        return true;
      });

      let latestPage: WorkspaceList = firstPage;
      let pagesFetched = 1;
      let nextPage = (firstPage.page || 1) + 1;

      while (latestPage.has_more && pagesFetched < MAX_MANAGED_WORKSPACE_PAGES) {
        latestPage = await workspaceService.getManagedWorkspaces(organizationId, {
          limit: 100,
          page: nextPage,
        });

        latestPage.data.forEach(workspace => {
          if (seenWorkspaceIds.has(workspace.id)) return;
          seenWorkspaceIds.add(workspace.id);
          mergedWorkspaces.push(workspace);
        });

        pagesFetched += 1;
        nextPage = (latestPage.page || nextPage) + 1;
      }

      return mergedWorkspaces;
    },
    enabled: enabled && !!organizationId,
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}
