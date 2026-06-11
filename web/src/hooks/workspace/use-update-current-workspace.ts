'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { accountService } from '@/services/account.service';
import { useWorkspaceStore, type Workspace } from '@/store/workspace-store';
import { useAuthStore } from '@/store/auth-store';
import { WORKSPACE_KEYS } from '@/hooks/query-keys';
import { sessionManager } from '@/lib/auth/session-manager';
import { clearProfileClientCache } from '@/utils/client-cache';

/**
 * Hook to update the current workspace selection in the user profile
 * Persists the selection to the backend and refreshes the local profile
 */
export function useUpdateCurrentWorkspace() {
  const queryClient = useQueryClient();
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const contextStatus = useWorkspaceStore.use.contextStatus();
  const selectWorkspace = useWorkspaceStore.use.selectWorkspace();
  const markWorkspaceRequired = useWorkspaceStore.use.markWorkspaceRequired();

  return useMutation({
    mutationFn: async (workspace: Workspace) => {
      return accountService.updateContext({
        current_workspace_id: workspace.id,
      });
    },
    onMutate: async (workspace: Workspace) => {
      // Optimistically update local store
      const previousWorkspace = currentWorkspace;
      const previousContextStatus = contextStatus;

      selectWorkspace(workspace);

      return { previousWorkspace, previousContextStatus };
    },
    onSuccess: async (_data, workspace) => {
      // Ensure next profile fetch hits network instead of stale client cache
      clearProfileClientCache();
      // Refresh store profile to keep everything in sync (Synchronize backend ID)
      await useAuthStore.getState().refreshProfile();
      // Targeted invalidation for all workspace-related data using the unified prefix
      // With the refactored query keys, all workspace data starts with ['workspace']
      await queryClient.invalidateQueries({ queryKey: WORKSPACE_KEYS.all });
      sessionManager.broadcastContextChanged({
        currentWorkspaceId: workspace.id,
      });
    },
    onError: (error, _workspace, context) => {
      console.error('Failed to update current workspace in profile:', error);
      // Rollback on error
      if (context) {
        if (context.previousContextStatus === 'workspace_required') {
          markWorkspaceRequired();
        } else if (context.previousWorkspace) {
          selectWorkspace(context.previousWorkspace);
        }
      }
    },
  });
}
