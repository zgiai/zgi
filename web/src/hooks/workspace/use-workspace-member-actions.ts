'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { workspaceService } from '@/services/workspace.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { WORKSPACE_KEYS } from '@/hooks/query-keys';
import { invalidateOrganizationMemberGraph } from '@/hooks/organization/invalidate-organization-member-graph';
import type { BatchAddMemberRequest } from '@/services/types/workspace';

/**
 * Hook for workspace member actions (add, remove, update role)
 */
export function useWorkspaceMemberActions() {
  const t = useT();
  const { currentOrganization } = useOrganizations();
  const queryClient = useQueryClient();

  const organizationId = currentOrganization?.id || '';

  // Remove workspace member
  const removeWorkspaceMemberMutation = useMutation({
    mutationFn: async ({ workspaceId, memberId }: { workspaceId: string; memberId: string }) => {
      if (!organizationId) throw new Error('No organization selected');
      return await workspaceService.removeWorkspaceMember(organizationId, workspaceId, memberId);
    },
    onSuccess: (_, { workspaceId }) => {
      invalidateOrganizationMemberGraph(queryClient, organizationId);
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.members(organizationId, workspaceId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.stats(workspaceId),
      });
      // Invalidate workspace detail to refresh member count
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.detail(organizationId, workspaceId),
      });
      // Invalidate workspace list to refresh member counts
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.all,
      });
      toast.success(t('workspace.messages.membersRemovedSuccess'));
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('workspace.messages.membersRemoveError'));
    },
  });

  // Update workspace member role
  const updateWorkspaceMemberRoleMutation = useMutation({
    mutationFn: async ({
      workspaceId,
      memberId,
      role_id,
    }: {
      workspaceId: string;
      memberId: string;
      role_id: string;
    }) => {
      if (!organizationId) throw new Error('No organization selected');
      return await workspaceService.updateWorkspaceMemberRole(
        organizationId,
        workspaceId,
        memberId,
        role_id
      );
    },
    onSuccess: (_, { workspaceId }) => {
      toast.success(t('workspace.messages.memberUpdatedSuccess'));
      invalidateOrganizationMemberGraph(queryClient, organizationId);
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.members(organizationId, workspaceId),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('workspace.messages.memberUpdateError'));
    },
  });

  const updateWorkspaceMemberPermissionsMutation = useMutation({
    mutationFn: async ({
      workspaceId,
      memberId,
      permissions,
    }: {
      workspaceId: string;
      memberId: string;
      permissions: string[];
    }) => {
      if (!organizationId) throw new Error('No organization selected');
      return await workspaceService.updateWorkspaceMemberPermissions(
        organizationId,
        workspaceId,
        memberId,
        permissions
      );
    },
    onSuccess: (_, { workspaceId, memberId }) => {
      toast.success(t('workspace.messages.memberUpdatedSuccess'));
      invalidateOrganizationMemberGraph(queryClient, organizationId);
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.members(organizationId, workspaceId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.memberDetail(organizationId, workspaceId, memberId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.permissions(organizationId, workspaceId, memberId),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('workspace.messages.memberUpdateError'));
    },
  });

  // Batch add workspace members
  const batchAddWorkspaceMembersMutation = useMutation({
    mutationFn: async ({
      workspaceId,
      data,
    }: {
      workspaceId: string;
      data: BatchAddMemberRequest;
    }) => {
      if (!organizationId) throw new Error('No organization selected');
      return await workspaceService.batchAddWorkspaceMembers(organizationId, workspaceId, data);
    },
    onSuccess: (_, { workspaceId }) => {
      invalidateOrganizationMemberGraph(queryClient, organizationId);
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.members(organizationId, workspaceId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.availableMembers(organizationId, workspaceId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.detail(organizationId, workspaceId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.stats(workspaceId),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('workspace.messages.membersAddError'));
    },
  });

  return {
    removeWorkspaceMember: removeWorkspaceMemberMutation.mutateAsync,
    isRemovingMember: removeWorkspaceMemberMutation.isPending,
    updateWorkspaceMemberRole: updateWorkspaceMemberRoleMutation.mutateAsync,
    isUpdatingRole: updateWorkspaceMemberRoleMutation.isPending,
    updateWorkspaceMemberPermissions: updateWorkspaceMemberPermissionsMutation.mutateAsync,
    isUpdatingPermissions: updateWorkspaceMemberPermissionsMutation.isPending,
    batchAddWorkspaceMembers: batchAddWorkspaceMembersMutation.mutateAsync,
    isBatchAddingWorkspaceMembers: batchAddWorkspaceMembersMutation.isPending,
  };
}
