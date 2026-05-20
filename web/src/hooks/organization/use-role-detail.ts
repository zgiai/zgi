'use client';

import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { organizationService } from '@/services/organization.service';
import type { Role } from '@/services/types/organization';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from '@/hooks/organization/use-organizations';

import { ORGANIZATION_KEYS } from '@/hooks/query-keys';

/**
 * Hook for fetching role detail
 */
export function useRoleDetail(roleId: string | null, enabled: boolean = true) {
  const t = useT('dashboard');
  const { currentOrganization } = useOrganizations();

  // Extract stable organizationId to avoid unnecessary re-renders
  const organizationId = currentOrganization?.id ?? null;

  const { data, isLoading, isFetching, error, refetch } = useQuery<Role>({
    queryKey: ORGANIZATION_KEYS.roleDetail(organizationId || '', roleId || ''),
    queryFn: async () => {
      if (!organizationId || !roleId) {
        throw new Error('No organization or role selected');
      }
      return await organizationService.getRoleDetail(organizationId, roleId);
    },
    enabled: enabled && !!organizationId && !!roleId,
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  // Show error toast if query fails
  useEffect(() => {
    if (!error) return;
    toast.error(getErrorMessage(error) || t('organization.permissions.loadError'));
  }, [error, toast, t]);

  return {
    role: data,
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    refetch,
  };
}
