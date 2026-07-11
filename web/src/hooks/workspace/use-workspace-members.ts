'use client';

import { keepPreviousData, useInfiniteQuery, useQueries, useQuery } from '@tanstack/react-query';
import { workspaceService } from '@/services/workspace.service';
import type { WorkspaceMemberAccount, WorkspaceMemberOption } from '@/services/types/workspace';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useCurrentWorkspace } from '@/store/workspace-store';

import { WORKSPACE_KEYS } from '@/hooks/query-keys';

/**
 * Get query key for workspace members
 */
export const getWorkspaceMembersQueryKey = (
  organizationId: string | null,
  workspaceId: string | null,
  params?: { keyword?: string; page?: number; limit?: number }
) => WORKSPACE_KEYS.members(organizationId, workspaceId, params);

interface UseWorkspaceMembersOptions {
  enabled?: boolean;
  page?: number;
  limit?: number;
  keepPreviousData?: boolean;
}

interface UseWorkspaceMemberDetailsOptions {
  enabled?: boolean;
}

/**
 * Hook for fetching workspace members
 */
export function useWorkspaceMembers(
  orgId?: string | null,
  wsId?: string | null,
  options: UseWorkspaceMembersOptions & { keyword?: string } = {}
) {
  const { enabled = true, keyword, page = 1, limit = 20, keepPreviousData: keepData } = options;

  const { currentOrganization } = useOrganizations();
  const currentWorkspace = useCurrentWorkspace();

  const organizationId = orgId ?? currentOrganization?.id ?? null;
  const workspaceId = wsId ?? currentWorkspace?.id ?? null;

  const {
    data: responseData,
    isLoading,
    isFetching,
    isPlaceholderData,
    error,
    refetch,
  } = useQuery({
    queryKey: WORKSPACE_KEYS.members(organizationId, workspaceId, { keyword, page, limit }),
    queryFn: async () => {
      if (!organizationId || !workspaceId) {
        return { data: [], total: 0, has_more: false, page, limit };
      }
      return await workspaceService.getWorkspaceMembers(organizationId, workspaceId, {
        keyword,
        page,
        limit,
      });
    },
    enabled: enabled && !!organizationId && !!workspaceId,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
    placeholderData: keepData ? keepPreviousData : undefined,
  });

  return {
    members: (responseData?.data ?? []) as WorkspaceMemberAccount[],
    total: responseData?.total ?? 0,
    page: responseData?.page ?? page,
    limit: responseData?.limit ?? limit,
    hasMore: responseData?.has_more ?? false,
    isLoading,
    isFetching,
    isPlaceholderData,
    error,
    refetch,
  };
}

/**
 * Hook for fetching workspace members with infinite pagination.
 */
export function useWorkspaceMembersInfinite(
  orgId?: string | null,
  wsId?: string | null,
  options: UseWorkspaceMembersOptions & { keyword?: string } = {}
) {
  const { enabled = true, keyword, limit = 20 } = options;

  const { currentOrganization } = useOrganizations();
  const currentWorkspace = useCurrentWorkspace();

  const organizationId = orgId ?? currentOrganization?.id ?? null;
  const workspaceId = wsId ?? currentWorkspace?.id ?? null;
  const normalizedKeyword = keyword?.trim() || undefined;

  const query = useInfiniteQuery({
    queryKey: WORKSPACE_KEYS.membersInfinite(organizationId, workspaceId, {
      keyword: normalizedKeyword,
      limit,
    }),
    queryFn: async ({ pageParam }) => {
      const page = typeof pageParam === 'number' ? pageParam : 1;
      if (!organizationId || !workspaceId) {
        return { data: [], total: 0, has_more: false, page, limit };
      }
      return await workspaceService.getWorkspaceMembers(organizationId, workspaceId, {
        keyword: normalizedKeyword,
        page,
        limit,
      });
    },
    initialPageParam: 1,
    getNextPageParam: lastPage => (lastPage.has_more ? lastPage.page + 1 : undefined),
    enabled: enabled && !!organizationId && !!workspaceId,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  const members = query.data?.pages.flatMap(page => page.data) ?? [];
  const lastPage = query.data?.pages.at(-1);

  return {
    members: members as WorkspaceMemberAccount[],
    total: lastPage?.total ?? 0,
    hasMore: Boolean(query.hasNextPage),
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    isFetchingNextPage: query.isFetchingNextPage,
    error: query.error,
    fetchNextPage: query.fetchNextPage,
    refetch: query.refetch,
  };
}

/**
 * Hook for fetching workspace member options with infinite pagination.
 * This intentionally uses the picker endpoint, not the management member list.
 */
export function useWorkspaceMemberOptionsInfinite(
  orgId?: string | null,
  wsId?: string | null,
  options: UseWorkspaceMembersOptions & { keyword?: string } = {}
) {
  const { enabled = true, keyword, limit = 20 } = options;

  const { currentOrganization } = useOrganizations();
  const currentWorkspace = useCurrentWorkspace();

  const organizationId = orgId ?? currentOrganization?.id ?? null;
  const workspaceId = wsId ?? currentWorkspace?.id ?? null;
  const normalizedKeyword = keyword?.trim() || undefined;

  const query = useInfiniteQuery({
    queryKey: WORKSPACE_KEYS.memberOptionsInfinite(organizationId, workspaceId, {
      keyword: normalizedKeyword,
      limit,
    }),
    queryFn: async ({ pageParam }) => {
      const page = typeof pageParam === 'number' ? pageParam : 1;
      if (!organizationId || !workspaceId) {
        return { data: [], total: 0, has_more: false, page, limit };
      }
      return await workspaceService.getWorkspaceMemberOptions(organizationId, workspaceId, {
        keyword: normalizedKeyword,
        page,
        limit,
      });
    },
    initialPageParam: 1,
    getNextPageParam: lastPage => (lastPage.has_more ? lastPage.page + 1 : undefined),
    enabled: enabled && !!organizationId && !!workspaceId,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  const members = query.data?.pages.flatMap(page => page.data) ?? [];
  const lastPage = query.data?.pages.at(-1);

  return {
    members: members as WorkspaceMemberOption[],
    total: lastPage?.total ?? 0,
    hasMore: Boolean(query.hasNextPage),
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    isFetchingNextPage: query.isFetchingNextPage,
    error: query.error,
    fetchNextPage: query.fetchNextPage,
    refetch: query.refetch,
  };
}

export function useWorkspaceMemberOptionDetails(
  orgId?: string | null,
  wsId?: string | null,
  memberIds: string[] = [],
  options: UseWorkspaceMemberDetailsOptions = {}
) {
  const { enabled = true } = options;

  const { currentOrganization } = useOrganizations();
  const currentWorkspace = useCurrentWorkspace();

  const organizationId = orgId ?? currentOrganization?.id ?? null;
  const workspaceId = wsId ?? currentWorkspace?.id ?? null;
  const normalizedMemberIds = Array.from(
    new Set(memberIds.map(memberId => memberId.trim()).filter(Boolean))
  );

  const queries = useQueries({
    queries: normalizedMemberIds.map(memberId => ({
      queryKey: WORKSPACE_KEYS.memberOptionDetail(organizationId, workspaceId, memberId),
      queryFn: async () => {
        if (!organizationId || !workspaceId) {
          throw new Error('workspace member option detail requires organization and workspace');
        }
        return await workspaceService.getWorkspaceMemberOption(organizationId, workspaceId, memberId);
      },
      enabled: enabled && !!organizationId && !!workspaceId,
      staleTime: 2 * 60 * 1000,
      gcTime: 5 * 60 * 1000,
      refetchOnWindowFocus: false,
      retry: false,
    })),
  });

  return queries.map((query, index) => ({
    memberId: normalizedMemberIds[index],
    ...query,
  }));
}

/**
 * Hook for fetching specific workspace members by ID.
 */
export function useWorkspaceMemberDetails(
  orgId?: string | null,
  wsId?: string | null,
  memberIds: string[] = [],
  options: UseWorkspaceMemberDetailsOptions = {}
) {
  const { enabled = true } = options;

  const { currentOrganization } = useOrganizations();
  const currentWorkspace = useCurrentWorkspace();

  const organizationId = orgId ?? currentOrganization?.id ?? null;
  const workspaceId = wsId ?? currentWorkspace?.id ?? null;
  const normalizedMemberIds = Array.from(
    new Set(memberIds.map(memberId => memberId.trim()).filter(Boolean))
  );

  const queries = useQueries({
    queries: normalizedMemberIds.map(memberId => ({
      queryKey: WORKSPACE_KEYS.memberDetail(organizationId, workspaceId, memberId),
      queryFn: async () => {
        if (!organizationId || !workspaceId) {
          throw new Error('workspace member detail requires organization and workspace');
        }
        return await workspaceService.getWorkspaceMember(organizationId, workspaceId, memberId);
      },
      enabled: enabled && !!organizationId && !!workspaceId,
      staleTime: 2 * 60 * 1000,
      gcTime: 5 * 60 * 1000,
      refetchOnWindowFocus: false,
      retry: false,
    })),
  });

  return normalizedMemberIds.map((memberId, index) => {
    const query = queries[index];
    return {
      memberId,
      data: query.data as WorkspaceMemberAccount | undefined,
      isLoading: query.isLoading,
      isFetching: query.isFetching,
      isError: query.isError,
      error: query.error,
      refetch: query.refetch,
    };
  });
}
