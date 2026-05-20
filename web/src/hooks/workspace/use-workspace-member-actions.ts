'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { workspaceService } from '@/services/workspace.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { WORKSPACE_KEYS } from '@/hooks/query-keys';
import type { BatchAddMemberRequest, WorkspaceMemberAccount } from '@/services/types/workspace';

interface WorkspaceMembersCache {
  data: WorkspaceMemberAccount[];
  total: number;
  has_more: boolean;
  page: number;
  limit: number;
}

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

  // Update workspace member role (with optimistic update)
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
    onMutate: async ({ workspaceId, memberId, role_id }) => {
      // Cancel any outgoing refetches
      await queryClient.cancelQueries({
        queryKey: WORKSPACE_KEYS.members(organizationId, workspaceId),
      });

      // Snapshot the previous value
      const previousMembersRes = queryClient.getQueryData<WorkspaceMembersCache>(
        WORKSPACE_KEYS.members(organizationId, workspaceId)
      );

      // Optimistically update the member's role_id
      if (previousMembersRes?.data) {
        queryClient.setQueryData(WORKSPACE_KEYS.members(organizationId, workspaceId), {
          ...previousMembersRes,
          data: previousMembersRes.data.map(member =>
            member.id === memberId ? { ...member, role_id: role_id } : member
          ),
        });
      }

      return { previousMembersRes };
    },
    onSuccess: (_, { workspaceId }) => {
      toast.success(t('workspace.messages.memberUpdatedSuccess'));
      // Invalidate workspace members list to trigger refetch and get latest data
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.members(organizationId, workspaceId),
      });
    },
    onError: (error, { workspaceId }, context) => {
      // Rollback to previous value on error
      if (context?.previousMembersRes) {
        queryClient.setQueryData(
          WORKSPACE_KEYS.members(organizationId, workspaceId),
          context.previousMembersRes
        );
      }
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
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.members(organizationId, workspaceId),
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
    batchAddWorkspaceMembers: batchAddWorkspaceMembersMutation.mutateAsync,
    isBatchAddingWorkspaceMembers: batchAddWorkspaceMembersMutation.isPending,
  };
}
