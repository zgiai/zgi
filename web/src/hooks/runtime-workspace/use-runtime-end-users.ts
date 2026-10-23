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
  RuntimeCreditGrant,
  RuntimeEndUserListParams,
  RuntimeEndUserPatch,
} from '@/services/types/runtime-workspace';
import { RUNTIME_WORKSPACE_KEYS } from '@/hooks/query-keys';
import { getErrorMessage } from '@/utils/error-notifications';
import { useT } from '@/i18n';

/** End users of a Runtime Workspace, with their usage and quotas. */
export function useRuntimeEndUsers(
  runtimeWorkspaceId: string | null,
  params: RuntimeEndUserListParams
) {
  const queryClient = useQueryClient();
  const t = useT('workspace');
  const list = useQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.endUserList(runtimeWorkspaceId, params),
    queryFn: () => runtimeWorkspaceService.listEndUsers(runtimeWorkspaceId as string, params),
    enabled: Boolean(runtimeWorkspaceId),
    placeholderData: keepPreviousData,
    retry: false,
  });
  const update = useMutation({
    mutationFn: ({ subjectId, patch }: { subjectId: string; patch: RuntimeEndUserPatch }) =>
      runtimeWorkspaceService.updateEndUser(runtimeWorkspaceId as string, subjectId, patch),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: RUNTIME_WORKSPACE_KEYS.endUsers(runtimeWorkspaceId),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error));
    },
  });
  // Exports run as jobs: queue one, wait for it, then download its JSON.
  const exportData = useMutation({
    mutationFn: async (subjectId: string) => {
      const workspaceId = runtimeWorkspaceId as string;
      let job = await runtimeWorkspaceService.requestEndUserExport(workspaceId, subjectId);
      const deadline = Date.now() + 10 * 60_000;
      while (job.status === 'pending' || job.status === 'running') {
        if (Date.now() > deadline) throw new Error(t('runtime.endUsers.exportTimeout'));
        await new Promise(resolve => setTimeout(resolve, 1500));
        job = await runtimeWorkspaceService.getEndUserExport(workspaceId, subjectId, job.id);
      }
      if (job.status === 'failed') {
        throw new Error(job.error || t('runtime.endUsers.exportFailed'));
      }
      return runtimeWorkspaceService.downloadEndUserExport(workspaceId, subjectId, job.id);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: RUNTIME_WORKSPACE_KEYS.endUsers(runtimeWorkspaceId),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error));
    },
  });
  const batchUpdate = useMutation({
    mutationFn: ({ subjectIds, patch }: { subjectIds: string[]; patch: RuntimeEndUserPatch }) =>
      runtimeWorkspaceService.batchUpdateEndUsers(runtimeWorkspaceId as string, subjectIds, patch),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: RUNTIME_WORKSPACE_KEYS.endUsers(runtimeWorkspaceId),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error));
    },
  });
  const erase = useMutation({
    mutationFn: (subjectId: string) =>
      runtimeWorkspaceService.eraseEndUser(runtimeWorkspaceId as string, subjectId),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: RUNTIME_WORKSPACE_KEYS.endUsers(runtimeWorkspaceId),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error));
    },
  });
  return { list, update, batchUpdate, exportData, erase };
}

/** One end user's credit ledger, and granting or taking back credits. */
export function useRuntimeEndUserCredits(
  runtimeWorkspaceId: string | null,
  subjectId: string | null
) {
  const queryClient = useQueryClient();
  const pages = useInfiniteQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.endUserCredits(runtimeWorkspaceId, subjectId),
    queryFn: ({ pageParam }) =>
      runtimeWorkspaceService.listEndUserCredits(
        runtimeWorkspaceId as string,
        subjectId as string,
        pageParam
      ),
    initialPageParam: 1,
    getNextPageParam: last => (last.has_more ? last.page + 1 : undefined),
    enabled: Boolean(runtimeWorkspaceId && subjectId),
    retry: false,
  });
  const ledger = {
    data: pages.data
      ? {
          credits: pages.data.pages[0]?.credits,
          items: pages.data.pages.flatMap(page => page.items),
        }
      : undefined,
    isLoading: pages.isLoading,
    hasMore: Boolean(pages.hasNextPage),
    loadingMore: pages.isFetchingNextPage,
    loadMore: () => void pages.fetchNextPage(),
  };
  const grant = useMutation({
    mutationFn: (data: RuntimeCreditGrant) =>
      runtimeWorkspaceService.grantEndUserCredits(
        runtimeWorkspaceId as string,
        subjectId as string,
        data
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: RUNTIME_WORKSPACE_KEYS.endUsers(runtimeWorkspaceId),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error));
    },
  });
  return { ledger, grant };
}

/** Totals, counts, and top consumers of a Runtime Workspace's end users. */
export function useRuntimeEndUserSummary(runtimeWorkspaceId: string | null) {
  return useQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.endUserSummary(runtimeWorkspaceId),
    queryFn: () => runtimeWorkspaceService.getEndUserSummary(runtimeWorkspaceId as string),
    enabled: Boolean(runtimeWorkspaceId),
    retry: false,
  });
}

/** Daily usage of a Runtime Workspace's end users, or of one of them. */
export function useRuntimeEndUserUsageSeries(
  runtimeWorkspaceId: string | null,
  subjectId: string | null,
  days: number,
  enabled = true
) {
  return useQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.endUserUsageSeries(runtimeWorkspaceId, subjectId, days),
    queryFn: () =>
      runtimeWorkspaceService.getEndUserUsageSeries(runtimeWorkspaceId as string, {
        subjectId: subjectId ?? undefined,
        days,
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      }),
    enabled: Boolean(runtimeWorkspaceId) && enabled,
    placeholderData: keepPreviousData,
    retry: false,
  });
}

/** The recorded changes to one end user. */
export function useRuntimeEndUserEvents(
  runtimeWorkspaceId: string | null,
  subjectId: string | null
) {
  const pages = useInfiniteQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.endUserEvents(runtimeWorkspaceId, subjectId),
    queryFn: ({ pageParam }) =>
      runtimeWorkspaceService.listEndUserEvents(
        runtimeWorkspaceId as string,
        subjectId as string,
        pageParam
      ),
    initialPageParam: 1,
    getNextPageParam: last => (last.has_more ? last.page + 1 : undefined),
    enabled: Boolean(runtimeWorkspaceId && subjectId),
    retry: false,
  });
  return {
    data: pages.data?.pages.flatMap(page => page.items),
    hasMore: Boolean(pages.hasNextPage),
    loadingMore: pages.isFetchingNextPage,
    loadMore: () => void pages.fetchNextPage(),
  };
}
