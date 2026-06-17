'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { organizationService } from '@/services/organization.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { ORGANIZATION_KEYS, WORKSPACE_KEYS } from '@/hooks/query-keys';
import type {
  AdminRegisterMemberRequest,
  DirectAddMemberRequest,
  ResetCurrentOrgMemberPasswordRequest,
} from '@/services/types/organization';

/**
 * Hook for member actions (status update, etc.)
 */
export function useMemberActions() {
  const t = useT('dashboard');
  const tCommon = useT('common');
  const { currentOrganization } = useOrganizations();
  const queryClient = useQueryClient();

  // Update member status mutation
  const updateMemberStatusMutation = useMutation({
    mutationFn: async ({
      memberId,
      status,
    }: {
      memberId: string;
      status: 'active' | 'inactive';
    }) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.updateMemberStatus(currentOrganization.id, memberId, status);
    },
    onSuccess: (_, variables) => {
      toast.success(
        variables.status === 'active'
          ? t('organization.contacts.enableSuccess')
          : t('organization.contacts.disableSuccess')
      );
      // Invalidate department members list to trigger refetch
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departmentMembers(currentOrganization?.id || ''),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.contacts.updateStatusError'));
    },
  });

  // Remove member from department mutation
  const removeMemberMutation = useMutation({
    mutationFn: async ({ deptId, accountId }: { deptId: string; accountId: string }) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.removeDepartmentMember(
        currentOrganization.id,
        deptId,
        accountId
      );
    },
    onSuccess: () => {
      toast.success(t('organization.contacts.removeSuccess'));
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departmentMembers(currentOrganization?.id || ''),
      });
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departments(currentOrganization?.id || ''),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.contacts.removeError'));
    },
  });

  // Remove member from organization mutation
  const removeMemberFromOrganizationMutation = useMutation({
    mutationFn: async ({ accountId }: { accountId: string }) => {
      return await organizationService.removeMemberFromOrganization(accountId);
    },
    onSuccess: () => {
      toast.success(t('organization.contacts.removeSuccess'));
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departmentMembers(''),
      });
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departments(currentOrganization?.id || ''),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.contacts.removeError'));
    },
  });

  // Update member department mutation
  const updateMemberDepartmentMutation = useMutation({
    mutationFn: async ({
      deptId,
      accountId,
      newDeptId,
    }: {
      deptId: string;
      accountId: string;
      newDeptId: string;
    }) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.updateMemberDepartment(
        currentOrganization.id,
        deptId,
        accountId,
        newDeptId
      );
    },
    onSuccess: () => {
      toast.success(t('organization.contacts.updateDepartmentSuccess'));
      // Invalidate department members list to trigger refetch
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departmentMembers(currentOrganization?.id || ''),
      });
      // Invalidate departments list to update member_count
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departments(currentOrganization?.id || ''),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.contacts.updateDepartmentError'));
    },
  });

  // Direct add member mutation
  const directAddMemberMutation = useMutation({
    mutationFn: async (data: DirectAddMemberRequest) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.directAddMember(currentOrganization.id, data);
    },
    onSuccess: () => {
      toast.success(t('organization.contacts.addMember.addSuccess'));
      // Invalidate department members list to trigger refetch
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departmentMembers(currentOrganization?.id || ''),
      });
      // Invalidate departments list to update member_count
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departments(currentOrganization?.id || ''),
      });
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.all,
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.contacts.addMember.addError'));
    },
  });

  // Admin register member mutation for non-cloud deployments
  const adminRegisterMemberMutation = useMutation({
    mutationFn: async (data: AdminRegisterMemberRequest) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.adminRegisterCurrentOrganizationMember(data);
    },
    onSuccess: data => {
      if (data.already_member) {
        toast.success(t('organization.contacts.addMember.alreadyMemberSuccess'));
      } else if (data.created_account) {
        toast.success(t('organization.contacts.addMember.createdAccountSuccess'));
      } else {
        toast.success(t('organization.contacts.addMember.existingAccountSuccess'));
      }
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departmentMembers(currentOrganization?.id || ''),
      });
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departments(currentOrganization?.id || ''),
      });
      if (data.workspace?.id) {
        queryClient.invalidateQueries({
          queryKey: WORKSPACE_KEYS.members(currentOrganization?.id || null, data.workspace.id),
        });
        queryClient.invalidateQueries({
          queryKey: WORKSPACE_KEYS.availableMembers(currentOrganization?.id || null, data.workspace.id),
        });
      }
      queryClient.invalidateQueries({
        queryKey: WORKSPACE_KEYS.forSwitcher(currentOrganization?.id || null),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.contacts.addMember.addError'));
    },
  });

  // Reset current organization member password mutation
  const resetMemberPasswordMutation = useMutation({
    mutationFn: async (data: ResetCurrentOrgMemberPasswordRequest) => {
      return await organizationService.resetCurrentOrganizationMemberPassword(data);
    },
    onSuccess: () => {
      toast.success(t('organization.contacts.resetPassword.success'));
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.contacts.resetPassword.error'));
    },
  });

  // Approve join request mutation
  const approveJoinRequestMutation = useMutation({
    mutationFn: async (requestId: string) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.approveJoinRequest(currentOrganization.id, requestId);
    },
    onSuccess: () => {
      toast.success(t('organization.contacts.addMember.approveSuccess'));
      // Invalidate join requests list to trigger refetch
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.joinRequests(currentOrganization?.id || ''),
      });
      // Also invalidate department members list
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departmentMembers(currentOrganization?.id || ''),
      });
      // Invalidate departments list to update member_count
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departments(currentOrganization?.id || ''),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.contacts.addMember.approveError'));
    },
  });

  // Reject join request mutation
  const rejectJoinRequestMutation = useMutation({
    mutationFn: async (requestId: string) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.rejectJoinRequest(currentOrganization.id, requestId);
    },
    onSuccess: () => {
      toast.success(t('organization.contacts.addMember.rejectSuccess'));
      // Invalidate join requests list to trigger refetch
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.joinRequests(currentOrganization?.id || ''),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.contacts.addMember.rejectError'));
    },
  });

  // Update member nickname mutation
  const updateMemberMutation = useMutation({
    mutationFn: async ({ memberId, member_name }: { memberId: string; member_name: string }) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.updateMember(currentOrganization.id, memberId, {
        name: member_name,
      });
    },
    onSuccess: () => {
      toast.success(tCommon('success'));
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departmentMembers(currentOrganization?.id || ''),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || tCommon('error'));
    },
  });

  // Check if member name exists
  const checkMemberName = async (name: string) => {
    if (!currentOrganization?.id) return { is_exist: false };
    return await organizationService.checkMemberName(currentOrganization.id, name);
  };

  return {
    updateMemberStatus: updateMemberStatusMutation.mutateAsync,
    isUpdatingStatus: updateMemberStatusMutation.isPending,
    removeMember: removeMemberMutation.mutateAsync,
    isRemoving: removeMemberMutation.isPending,
    removeMemberFromOrganization: removeMemberFromOrganizationMutation.mutateAsync,
    isRemovingFromOrg: removeMemberFromOrganizationMutation.isPending,
    updateMemberDepartment: updateMemberDepartmentMutation.mutateAsync,
    isUpdatingDepartment: updateMemberDepartmentMutation.isPending,
    directAddMember: directAddMemberMutation.mutateAsync,
    isAddingMember: directAddMemberMutation.isPending,
    adminRegisterMember: adminRegisterMemberMutation.mutateAsync,
    isAdminRegisteringMember: adminRegisterMemberMutation.isPending,
    resetMemberPassword: resetMemberPasswordMutation.mutateAsync,
    isResettingPassword: resetMemberPasswordMutation.isPending,
    approveJoinRequest: approveJoinRequestMutation.mutateAsync,
    isApprovingRequest: approveJoinRequestMutation.isPending,
    rejectJoinRequest: rejectJoinRequestMutation.mutateAsync,
    isRejectingRequest: rejectJoinRequestMutation.isPending,
    updateMemberNickname: updateMemberMutation.mutateAsync,
    isUpdatingNickname: updateMemberMutation.isPending,
    checkMemberName,
  };
}
