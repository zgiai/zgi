'use client';

import { useQuery } from '@tanstack/react-query';
import { workspaceService } from '@/services/workspace.service';
import { WORKSPACE_KEYS } from '@/hooks/query-keys';

/**
 * Hook to fetch workspace statistics (credits, usage, member count)
 */
export function useWorkspaceStatistics(workspaceId: string, enabled = true) {
  return useQuery({
    queryKey: WORKSPACE_KEYS.stats(workspaceId),
    queryFn: async () => {
      if (!workspaceId) {
        throw new Error('Workspace ID is required');
      }
      return await workspaceService.getWorkspaceStatistics(workspaceId);
    },
    enabled: enabled && !!workspaceId,
    staleTime: 2 * 60 * 1000, // 2 minutes
  });
}
