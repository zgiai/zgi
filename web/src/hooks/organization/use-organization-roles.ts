'use client';

import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { organizationService } from '@/services/organization.service';
import type { Role } from '@/services/types/organization';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from './use-organizations';

import { ORGANIZATION_KEYS } from '@/hooks/query-keys';

/**
 * Hook for fetching Organization roles
 */
export function useOrganizationRoles() {
  const t = useT('dashboard');
  const { currentOrganization } = useOrganizations();

  const {
    data: responseData,
    isLoading,
    isFetching,
    error,
    refetch,
  } = useQuery<{ roles: Role[] }>({
    queryKey: ORGANIZATION_KEYS.roles(currentOrganization?.id || ''),
    queryFn: async () => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.getRoles(currentOrganization.id);
    },
    enabled: !!currentOrganization?.id,
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
    refetchOnWindowFocus: false,
  });

  // Show error toast if query fails
  useEffect(() => {
    if (!error) return;
    toast.error(getErrorMessage(error) || t('organization.permissions.loadError'));
  }, [error, toast, t]);

  return {
    roles: responseData?.roles ?? [],
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    refetch,
  };
}
