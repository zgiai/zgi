'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { organizationService } from '@/services/organization.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { invalidateOrganizationMemberGraph } from '@/hooks/organization/invalidate-organization-member-graph';
import type {
  CreateDepartmentRequest,
  UpdateDepartmentRequest,
} from '@/services/types/organization';

/**
 * Hook for department actions (create, update, delete)
 */
export function useCreateDepartment() {
  const t = useT('dashboard');
  const { currentOrganization } = useOrganizations();
  const queryClient = useQueryClient();

  // Create department mutation
  const createDepartmentMutation = useMutation({
    mutationFn: async (data: CreateDepartmentRequest) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.createDepartment(currentOrganization.id, data);
    },
    onSuccess: () => {
      toast.success(t('organization.contacts.createDepartment.createSuccess'));
      invalidateOrganizationMemberGraph(queryClient, currentOrganization?.id);
    },
    onError: error => {
      toast.error(
        getErrorMessage(error) || t('organization.contacts.createDepartment.createError')
      );
    },
  });

  return {
    createDepartment: createDepartmentMutation.mutateAsync,
    isCreating: createDepartmentMutation.isPending,
  };
}

/**
 * Hook for updating department
 */
export function useUpdateDepartment() {
  const t = useT('dashboard');
  const { currentOrganization } = useOrganizations();
  const queryClient = useQueryClient();

  // Update department mutation
  const updateDepartmentMutation = useMutation({
    mutationFn: async ({ deptId, data }: { deptId: string; data: UpdateDepartmentRequest }) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.updateDepartment(currentOrganization.id, deptId, data);
    },
    onSuccess: () => {
      toast.success(t('organization.contacts.editDepartment.updateSuccess'));
      invalidateOrganizationMemberGraph(queryClient, currentOrganization?.id);
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.contacts.editDepartment.updateError'));
    },
  });

  return {
    updateDepartment: updateDepartmentMutation.mutateAsync,
    isUpdating: updateDepartmentMutation.isPending,
  };
}

/**
 * Hook for deleting department
 */
export function useDeleteDepartment() {
  const t = useT('dashboard');
  const { currentOrganization } = useOrganizations();
  const queryClient = useQueryClient();

  // Delete department mutation
  const deleteDepartmentMutation = useMutation({
    mutationFn: async (deptId: string) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.deleteDepartment(currentOrganization.id, deptId);
    },
    onSuccess: () => {
      toast.success(t('organization.contacts.deleteDepartment.deleteSuccess'));
      invalidateOrganizationMemberGraph(queryClient, currentOrganization?.id);
    },
    onError: error => {
      toast.error(
        getErrorMessage(error) || t('organization.contacts.deleteDepartment.deleteError')
      );
    },
  });

  return {
    deleteDepartment: deleteDepartmentMutation.mutateAsync,
    isDeleting: deleteDepartmentMutation.isPending,
  };
}
