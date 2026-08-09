'use client';

import { useEffect, useState, useMemo, useCallback } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { organizationService } from '@/services/organization.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { getWorkspaceRoleErrorTranslationKey } from '@/utils/workspace-role-errors';
import { useOrganizations } from './use-organizations';
import type { RoleMemberList } from '@/services/types/organization';

import { ORGANIZATION_KEYS } from '@/hooks/query-keys';

/**
 * Hook for fetching role members with infinite scroll
 */
export function useRoleMembers(roleId: string | null, enabled: boolean = true) {
  const t = useT('dashboard');
  const { currentOrganization } = useOrganizations();
  const [searchKeyword, setSearchKeyword] = useState('');

  const {
    data,
    isLoading,
    isFetching,
    error,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    refetch,
  } = useInfiniteQuery<RoleMemberList, unknown>({
    queryKey: [
      ...ORGANIZATION_KEYS.roleMembers(currentOrganization?.id || '', roleId || ''),
      { keyword: searchKeyword.trim() },
    ],
    initialPageParam: 1,
    queryFn: async ({ pageParam }) => {
      if (!currentOrganization?.id || !roleId) {
        throw new Error(t('organization.permissions.errors.noOrganization'));
      }
      const page = (pageParam as number) ?? 1;
      const response = await organizationService.getRoleMembers(currentOrganization.id, roleId, {
        page,
        limit: 20,
        keyword: searchKeyword.trim() || undefined,
      });
      return response;
    },
    getNextPageParam: lastPage => {
      if (!lastPage) return undefined;
      return lastPage.has_more ? lastPage.page + 1 : undefined;
    },
    enabled: enabled && !!currentOrganization?.id && !!roleId,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  const errorMessage = error
    ? (() => {
        const translationKey = getWorkspaceRoleErrorTranslationKey(error);
        return translationKey ? t(translationKey) : getErrorMessage(error);
      })()
    : null;

  // Show error toast if query fails
  useEffect(() => {
    if (!errorMessage) return;
    toast.error(errorMessage || t('organization.permissions.loadError'));
  }, [errorMessage, t]);

  // Flatten all pages into single array
  const allMembers = useMemo(() => {
    const pages = data?.pages ?? [];
    return pages.flatMap(page => page?.items ?? []);
  }, [data]);

  // Get total count from first page
  const total = useMemo(() => {
    return data?.pages?.[0]?.total ?? 0;
  }, [data]);

  const stableFetchNextPage = useCallback(async () => {
    await fetchNextPage();
  }, [fetchNextPage]);

  return {
    members: allMembers,
    total,
    isLoading,
    isFetching,
    hasNextPage: !!hasNextPage,
    fetchNextPage: stableFetchNextPage,
    isFetchingNextPage,
    error: errorMessage,
    refetch,
    searchKeyword,
    setSearchKeyword,
  };
}
