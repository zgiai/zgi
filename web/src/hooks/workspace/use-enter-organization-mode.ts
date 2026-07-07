'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { accountService } from '@/services/account.service';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { useWorkspaceStore, type Workspace } from '@/store/workspace-store';
import {
  AGENT_KEYS,
  DATASET_KEYS,
  DB_KEYS,
  FILE_KEYS,
  PROFILE_KEYS,
  PROMPT_KEYS,
  WORKFLOW_KEYS,
  WORKFLOW_TEST_KEYS,
  WORKSPACE_KEYS,
} from '@/hooks/query-keys';
import { sessionManager } from '@/lib/auth/session-manager';
import { clearProfileClientCache } from '@/utils/client-cache';

function normalizeContextID(value: string | null | undefined): string | null {
  if (!value) return null;
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : null;
}

export function useEnterOrganizationMode() {
  const queryClient = useQueryClient();
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const user = useAuthStore.use.user();
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const contextStatus = useWorkspaceStore.use.contextStatus();
  const markWorkspaceRequired = useWorkspaceStore.use.markWorkspaceRequired();
  const selectWorkspace = useWorkspaceStore.use.selectWorkspace();
  const resetForOrganizationSwitch = useWorkspaceStore.use.resetForOrganizationSwitch();

  return useMutation({
    mutationFn: async () => {
      const organizationID = normalizeContextID(
        currentOrganization?.id ?? user?.current_organization_id
      );
      if (!organizationID) {
        throw new Error('No organization selected');
      }

      return accountService.updateContext({
        mode: 'organization',
        current_organization_id: organizationID,
      });
    },
    onMutate: async () => {
      const previousWorkspace: Workspace | null = currentWorkspace;
      const previousContextStatus = contextStatus;

      markWorkspaceRequired();

      return { previousWorkspace, previousContextStatus };
    },
    onSuccess: async data => {
      const organizationID = normalizeContextID(data.data?.current_organization_id);

      clearProfileClientCache();
      queryClient.removeQueries({ queryKey: AGENT_KEYS.details() });
      queryClient.removeQueries({ queryKey: DATASET_KEYS.details() });
      queryClient.removeQueries({ queryKey: DB_KEYS.details() });
      queryClient.removeQueries({ queryKey: PROMPT_KEYS.details() });
      queryClient.removeQueries({ queryKey: WORKFLOW_KEYS.runDetails() });

      await useAuthStore.getState().refreshProfile({ refresh: true });

      await Promise.all([
        queryClient.invalidateQueries({ queryKey: PROFILE_KEYS.capabilities() }),
        queryClient.invalidateQueries({ queryKey: WORKSPACE_KEYS.all }),
        queryClient.invalidateQueries({ queryKey: AGENT_KEYS.all }),
        queryClient.invalidateQueries({ queryKey: DATASET_KEYS.all }),
        queryClient.invalidateQueries({ queryKey: DB_KEYS.all }),
        queryClient.invalidateQueries({ queryKey: PROMPT_KEYS.all }),
        queryClient.invalidateQueries({ queryKey: FILE_KEYS.all }),
        queryClient.invalidateQueries({ queryKey: WORKFLOW_KEYS.all }),
        queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.all }),
      ]);

      sessionManager.broadcastContextChanged({
        currentOrganizationId: organizationID,
        currentWorkspaceId: null,
        mode: 'organization',
      });
    },
    onError: (error, _variables, context) => {
      console.error('Failed to enter organization mode:', error);
      if (!context) {
        return;
      }
      if (context.previousContextStatus === 'workspace_required') {
        markWorkspaceRequired();
        return;
      }
      if (context.previousContextStatus === 'ready' && context.previousWorkspace) {
        selectWorkspace(context.previousWorkspace);
        return;
      }
      resetForOrganizationSwitch();
    },
  });
}
