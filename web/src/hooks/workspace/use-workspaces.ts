'use client';

import { useEffect } from 'react';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { workspaceService } from '@/services/workspace.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from '@/hooks/organization/use-organizations';

import { WORKSPACE_KEYS } from '@/hooks/query-keys';

/**
 * Hook for fetching workspaces list with search and pagination
 */
export function useWorkspaces(
  keyword: string = '',
  page: number = 1,
  limit: number = 20,
  options: { keepPreviousData?: boolean } = {}
) {
  const t = useT();
  const { currentOrganization } = useOrganizations();
  const { keepPreviousData: keepData = false } = options;

  const {
    data: responseData,
    isLoading,
    isFetching,
    isPlaceholderData,
    error,
    refetch,
  } = useQuery({
    queryKey: WORKSPACE_KEYS.list(currentOrganization?.id ?? null, keyword, page, limit),
    queryFn: async () => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await workspaceService.getWorkspaces(currentOrganization.id, {
        keyword: keyword || undefined,
        page,
        limit,
      });
    },
    enabled: !!currentOrganization?.id,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
    placeholderData: keepData ? keepPreviousData : undefined,
  });

  // Show error toast if query fails
  useEffect(() => {
    if (!error) return;
    toast.error(
      getErrorMessage(error) || t('dashboard.organization.workspaceManagement.loadError')
    );
  }, [error, t]);

  return {
    workspaces: responseData?.data ?? [],
    total: responseData?.total ?? 0,
    hasMore: responseData?.has_more ?? false,
    page: responseData?.page ?? page,
    limit: responseData?.limit ?? limit,
    isLoading,
    isFetching,
    isPlaceholderData,
    error: error ? getErrorMessage(error) : null,
    refetch,
  };
}
