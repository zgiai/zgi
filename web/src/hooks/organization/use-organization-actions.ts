'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { ORGANIZATION_KEYS } from '@/hooks/query-keys';
import { organizationService } from '@/services/organization.service';
import type {
  OrganizationMemberRole,
  OrganizationUpdateRequest,
} from '@/services/types/organization';
import { useOrganizationStore } from '@/store/organization-store';
import { getErrorMessage } from '@/utils/error-notifications';

export function useOrganizationActions() {
  const t = useT('dashboard');
  const queryClient = useQueryClient();
  const organizations = useOrganizationStore.use.organizations();
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const setOrganizations = useOrganizationStore.use.setOrganizations();
  const setCurrentOrganization = useOrganizationStore.use.setCurrentOrganization();

  const updateOrganizationMutation = useMutation({
    mutationFn: async (data: OrganizationUpdateRequest) => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.updateOrganization(currentOrganization.id, data);
    },
    onSuccess: updatedOrganization => {
      setOrganizations(
        organizations.map(organization =>
          organization.id === updatedOrganization.id
            ? { ...organization, ...updatedOrganization }
            : organization
        )
      );
      if (currentOrganization?.id === updatedOrganization.id) {
        setCurrentOrganization({ ...currentOrganization, ...updatedOrganization });
      }
      queryClient.invalidateQueries({ queryKey: ORGANIZATION_KEYS.lists() });
      queryClient.invalidateQueries({ queryKey: ORGANIZATION_KEYS.current() });
      toast.success(t('organization.settings.updateSuccess'));
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('organization.settings.updateError'));
    },
  });

  const updateCurrentOrganizationMemberRoleMutation = useMutation({
    mutationFn: async ({
      memberId,
      role,
    }: {
      memberId: string;
      role: Exclude<OrganizationMemberRole, 'owner'>;
    }) => {
      return await organizationService.updateCurrentOrganizationMemberRole(memberId, { role });
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: ORGANIZATION_KEYS.currentMembers() });
      queryClient.invalidateQueries({
        queryKey: ORGANIZATION_KEYS.departmentMembers(currentOrganization?.id || ''),
      });
      queryClient.invalidateQueries({ queryKey: ORGANIZATION_KEYS.current() });
      toast.success(
        variables.role === 'admin'
          ? t('organization.settings.adminManagement.promoteSuccess')
          : t('organization.settings.adminManagement.demoteSuccess')
      );
    },
    onError: error => {
      toast.error(
        getErrorMessage(error) || t('organization.settings.adminManagement.updateRoleError')
      );
    },
  });

  return {
    updateOrganization: updateOrganizationMutation.mutateAsync,
    isUpdatingOrganization: updateOrganizationMutation.isPending,
    updateCurrentOrganizationMemberRole: updateCurrentOrganizationMemberRoleMutation.mutateAsync,
    isUpdatingCurrentOrganizationMemberRole: updateCurrentOrganizationMemberRoleMutation.isPending,
  };
}
