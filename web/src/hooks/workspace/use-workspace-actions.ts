'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { workspaceService } from '@/services/workspace.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from '@/hooks/organization/use-organizations';

import { WORKSPACE_KEYS } from '@/hooks/query-keys';
import { invalidateOrganizationMemberGraph } from '@/hooks/organization/invalidate-organization-member-graph';
import type { CreateWorkspaceRequest, UpdateWorkspaceRequest } from '@/services/types/workspace';

/**
 * Hook for workspace actions
 */
export function useWorkspaceActions() {
  const t = useT();
  const { currentOrganization } = useOrganizations();
  const queryClient = useQueryClient();

  const organizationId = currentOrganization?.id || '';

  // Create workspace
  const createWorkspaceMutation = useMutation({
    mutationFn: async (data: CreateWorkspaceRequest) => {
      if (!organizationId) throw new Error('No organization selected');
      return await workspaceService.createWorkspace(organizationId, data);
    },
    onSuccess: () => {
      invalidateOrganizationMemberGraph(queryClient, organizationId);
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.managed(organizationId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.all,
      });
      toast.success(t('workspace.messages.workspaceCreatedSuccess'));
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('workspace.messages.workspaceCreateError'));
    },
  });

  // Update workspace
  const updateWorkspaceMutation = useMutation({
    mutationFn: async ({
      workspaceId,
      data,
    }: {
      workspaceId: string;
      data: UpdateWorkspaceRequest;
    }) => {
      if (!organizationId) throw new Error('No organization selected');
      return await workspaceService.updateWorkspace(organizationId, workspaceId, data);
    },
    onSuccess: (_, { workspaceId, data }) => {
      invalidateOrganizationMemberGraph(queryClient, organizationId);
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.stats(workspaceId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.detail(organizationId, workspaceId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.managed(organizationId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.all,
      });
      if (data.name) {
        toast.success(t('workspace.messages.workspaceNameUpdatedSuccess'));
      }
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('workspace.messages.workspaceNameUpdateError'));
    },
  });

  // Delete workspace
  const deleteWorkspaceMutation = useMutation({
    mutationFn: async (workspaceId: string) => {
      if (!organizationId) throw new Error('No organization selected');
      return await workspaceService.deleteWorkspace(organizationId, workspaceId);
    },
    onSuccess: (_, workspaceId) => {
      invalidateOrganizationMemberGraph(queryClient, organizationId);
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.managed(organizationId),
      });
      queryClient.removeQueries({
        queryKey: WORKSPACE_KEYS.stats(workspaceId),
      });
      queryClient.removeQueries({
        queryKey: WORKSPACE_KEYS.members(organizationId, workspaceId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.all,
      });
      toast.success(t('dashboard.organization.workspaceManagement.deleteSuccess'));
    },
    onError: error => {
      toast.error(
        getErrorMessage(error) || t('dashboard.organization.workspaceManagement.deleteError')
      );
    },
  });

  // Transfer ownership
  const transferOwnershipMutation = useMutation({
    mutationFn: async ({
      workspaceId,
      data,
    }: {
      workspaceId: string;
      data: { new_owner_id: string };
    }) => {
      if (!organizationId) throw new Error('No organization selected');
      return await workspaceService.transferOwnership(organizationId, workspaceId, data);
    },
    onSuccess: (_, { workspaceId }) => {
      invalidateOrganizationMemberGraph(queryClient, organizationId);
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.stats(workspaceId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.managed(organizationId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.members(organizationId, workspaceId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.permissions(organizationId, workspaceId, 'current'),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.all,
      });
      toast.success(t('workspace.messages.ownershipTransferredSuccess'));
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('workspace.messages.ownershipTransferError'));
    },
  });

  // Leave workspace
  const leaveWorkspaceMutation = useMutation({
    mutationFn: async (workspaceId: string) => {
      if (!organizationId) throw new Error('No organization selected');
      return await workspaceService.leaveWorkspace(organizationId, workspaceId);
    },
    onSuccess: (_, workspaceId) => {
      invalidateOrganizationMemberGraph(queryClient, organizationId);
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.all,
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.permissions(organizationId, workspaceId, 'current'),
      });
      toast.success(t('workspace.messages.leaveWorkspaceSuccess'));
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('workspace.messages.leaveWorkspaceError'));
    },
  });

  return {
    createWorkspace: createWorkspaceMutation.mutateAsync,
    isCreating: createWorkspaceMutation.isPending,
    updateWorkspace: updateWorkspaceMutation.mutateAsync,
    isUpdating: updateWorkspaceMutation.isPending,
    deleteWorkspace: deleteWorkspaceMutation.mutateAsync,
    isDeleting: deleteWorkspaceMutation.isPending,
    transferOwnership: transferOwnershipMutation.mutateAsync,
    isTransferring: transferOwnershipMutation.isPending,
    leaveWorkspace: leaveWorkspaceMutation.mutateAsync,
    isLeaving: leaveWorkspaceMutation.isPending,
  };
}

/**
 * Backward compatibility alias
 */
export const useDeleteWorkspace = () => {
  const { deleteWorkspace, isDeleting } = useWorkspaceActions();
  return { deleteWorkspace, isDeleting };
};
