'use client';

import { useQuery } from '@tanstack/react-query';
import { organizationService } from '@/services/organization.service';
import { useOrganizations } from '@/hooks/organization/use-organizations';

import { ORGANIZATION_KEYS } from '@/hooks/query-keys';

/**
 * Hook for fetching invite link
 */
export function useInviteLink(departmentId?: string, enabled = true) {
  const { currentOrganization } = useOrganizations();

  const {
    data: responseData,
    isLoading,
    isFetching,
    error,
    refetch,
  } = useQuery<{ url: string }>({
    queryKey: ORGANIZATION_KEYS.inviteLink(currentOrganization?.id || '', departmentId),
    queryFn: async () => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.getInviteLink(currentOrganization.id, departmentId);
    },
    enabled: enabled && !!currentOrganization?.id,
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  return {
    inviteLink: responseData?.url ?? '',
    isLoading,
    isFetching,
    error,
    refetch,
  };
}
