'use client';

import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { organizationService } from '@/services/organization.service';
import type { Department } from '@/services/types/organization';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { ORGANIZATION_KEYS } from '@/hooks/query-keys';

interface UseDepartmentsOptions {
  enabled?: boolean;
}

/**
 * Hook for fetching department tree
 */
export function useDepartments(options: UseDepartmentsOptions = {}) {
  const t = useT('dashboard');
  const { currentOrganization } = useOrganizations();
  const enabled = options.enabled ?? true;

  const {
    data: responseData,
    isLoading,
    isFetching,
    error,
    refetch,
  } = useQuery<{ departments: Department[] }>({
    queryKey: ORGANIZATION_KEYS.departments(currentOrganization?.id || ''),
    queryFn: async () => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.getDepartments(currentOrganization.id);
    },
    enabled: enabled && !!currentOrganization?.id,
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  // Show error toast if query fails
  useEffect(() => {
    if (!error) return;
    toast.error(getErrorMessage(error) || t('organization.contacts.loadDepartmentsError'));
  }, [error, t]);

  return {
    departments: responseData?.departments ?? [],
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    refetch,
  };
}
