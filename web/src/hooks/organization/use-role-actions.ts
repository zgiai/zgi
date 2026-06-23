'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';
import { organizationService } from '@/services/organization.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { ORGANIZATION_KEYS } from '@/hooks/query-keys';
import { invalidateOrganizationMemberGraph } from '@/hooks/organization/invalidate-organization-member-graph';
import type {
  CreateRoleRequest,
  UpdateRolePermissionsRequest,
  UpdateRoleInfoRequest,
} from '@/services/types/organization';

/**
 * Hook for role creation and update actions
 */
export function useRoleActions() {
  const router = useRouter();
  const t = useT('dashboard');
  const { currentOrganization } = useOrganizations();
  const queryClient = useQueryClient();

  // Create role mutation
  const createRoleMutation = useMutation({
    mutationFn: async (data: CreateRoleRequest) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.createRole(currentOrganization.id, data);
    },
    onSuccess: response => {
      toast.success(t('organization.permissions.config.createSuccess'));
      invalidateOrganizationMemberGraph(queryClient, currentOrganization?.id);
      // Navigate to the newly created role detail page
      if (response?.id) {
        router.push(`/dashboard/organization/permissions/${response.id}`);
      } else {
        router.push('/dashboard/organization/permissions');
      }
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.permissions.config.saveError'));
    },
  });

  // Update role permissions mutation
  const updateRolePermissionsMutation = useMutation({
    mutationFn: async ({
      roleId,
      data,
    }: {
      roleId: string;
      data: UpdateRolePermissionsRequest;
    }) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.updateRolePermissions(currentOrganization.id, roleId, data);
    },
    onSuccess: (_, variables) => {
      toast.success(t('organization.permissions.config.updateSuccess'));
      invalidateOrganizationMemberGraph(queryClient, currentOrganization?.id);
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.roleDetail(currentOrganization?.id || '', variables.roleId),
      });
      router.push('/dashboard/organization/permissions');
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.permissions.config.saveError'));
    },
  });

  // Update role info mutation
  const updateRoleInfoMutation = useMutation({
    mutationFn: async ({ roleId, data }: { roleId: string; data: UpdateRoleInfoRequest }) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.updateRoleInfo(currentOrganization.id, roleId, data);
    },
    onSuccess: (_, variables) => {
      toast.success(t('organization.permissions.config.updateSuccess'));
      // Invalidate role detail to trigger refetch
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.roleDetail(currentOrganization?.id || '', variables.roleId),
      });
      invalidateOrganizationMemberGraph(queryClient, currentOrganization?.id);
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.permissions.config.saveError'));
    },
  });

  // Delete role mutation
  const deleteRoleMutation = useMutation({
    mutationFn: async (roleId: string) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.deleteRole(currentOrganization.id, roleId);
    },
    onSuccess: (_, roleId) => {
      toast.success(t('organization.permissions.config.deleteSuccess'));
      invalidateOrganizationMemberGraph(queryClient, currentOrganization?.id);
      queryClient.removeQueries({
        queryKey: ORGANIZATION_KEYS.roleDetail(currentOrganization?.id || '', roleId),
      });
      queryClient.removeQueries({
        queryKey: ORGANIZATION_KEYS.roleMembers(currentOrganization?.id || '', roleId),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.permissions.config.deleteError'));
    },
  });

  return {
    createRole: createRoleMutation.mutateAsync,
    updateRolePermissions: updateRolePermissionsMutation.mutateAsync,
    updateRoleInfo: updateRoleInfoMutation.mutateAsync,
    deleteRole: deleteRoleMutation.mutateAsync,
    isCreating: createRoleMutation.isPending,
    isUpdating: updateRolePermissionsMutation.isPending,
    isUpdatingInfo: updateRoleInfoMutation.isPending,
    isDeleting: deleteRoleMutation.isPending,
    isSaving: createRoleMutation.isPending || updateRolePermissionsMutation.isPending,
  };
}
