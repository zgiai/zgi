'use client';

import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { toast } from 'sonner';
import { runtimeWorkspaceService } from '@/services/runtime-workspace.service';
import type {
  RuntimeAppListParams,
  RuntimeCandidateParams,
} from '@/services/types/runtime-workspace';
import { RUNTIME_WORKSPACE_KEYS } from '@/hooks/query-keys';
import { getErrorMessage } from '@/utils/error-notifications';

/** A page of a Runtime Workspace's bound apps, and adding or removing apps (ADR 0012). */
export function useRuntimeManagedApps(
  runtimeWorkspaceId: string | null,
  params: RuntimeAppListParams
) {
  const queryClient = useQueryClient();
  const page = useQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.managedApps(runtimeWorkspaceId, params),
    queryFn: () => runtimeWorkspaceService.listManagedApps(runtimeWorkspaceId as string, params),
    enabled: Boolean(runtimeWorkspaceId),
    placeholderData: keepPreviousData,
    retry: false,
  });
  const change = useMutation({
    mutationFn: (
      input:
        | { expected_revision: number; apps: Array<{ type: string; id: string }> }
        | { expected_revision: number; app_ids: string[] }
    ) => runtimeWorkspaceService.changeApps(runtimeWorkspaceId as string, input),
    onSuccess: () => {
      // The overview's counts and the candidates change with the selection.
      void queryClient.invalidateQueries({ queryKey: RUNTIME_WORKSPACE_KEYS.all });
    },
    onError: error => {
      toast.error(getErrorMessage(error));
    },
  });
  return { page, change };
}

/** Apps that may be added, searched and paged on the server. */
export function useRuntimeAppCandidates(
  runtimeWorkspaceId: string | null,
  params: RuntimeCandidateParams,
  enabled: boolean
) {
  const pages = useInfiniteQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.appCandidatesPage(runtimeWorkspaceId, params),
    queryFn: ({ pageParam }) =>
      runtimeWorkspaceService.listAppCandidatesPage(
        runtimeWorkspaceId as string,
        params,
        pageParam
      ),
    initialPageParam: 1,
    getNextPageParam: (last, all) => (last.has_more ? all.length + 1 : undefined),
    enabled: enabled && Boolean(runtimeWorkspaceId),
    retry: false,
  });
  return {
    items: pages.data?.pages.flatMap(page => page.items) ?? [],
    meta: pages.data?.pages[0],
    isLoading: pages.isLoading,
    hasMore: Boolean(pages.hasNextPage),
    loadingMore: pages.isFetchingNextPage,
    loadMore: () => void pages.fetchNextPage(),
  };
}
