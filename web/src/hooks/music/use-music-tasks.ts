'use client';

import { type QueryKey, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { shouldPollMusicTask } from '@/components/music/music-task-state';
import { MUSIC_KEYS } from '@/hooks/query-keys';
import { musicService, type CreateMusicTasksResult } from '@/services/music.service';
import type { ApiResponseData } from '@/services/types/common';
import type { CreateMusicTasksRequest, ListMusicTasksParams, MusicTask, MusicTaskList } from '@/services/types/music';
import { useOrganizationStore } from '@/store/organization-store';
import { useCurrentWorkspace } from '@/store/workspace-store';

function createCachedMusicTaskResponse(task: MusicTask): ApiResponseData<MusicTask> {
  return {
    code: '0',
    message: 'success',
    data: task,
  };
}

function getMusicListParamsFromQueryKey(queryKey: QueryKey) {
  const params = Array.isArray(queryKey) ? queryKey[4] : undefined;
  return params && typeof params === 'object' ? (params as ListMusicTasksParams) : {};
}

function musicTaskMatchesSearch(task: MusicTask, search: string | undefined) {
  const normalizedSearch = search?.trim().toLowerCase();
  if (!normalizedSearch) return true;
  return [task.id, task.model, task.prompt, task.lyrics, task.title, task.status]
    .filter((value): value is string => typeof value === 'string')
    .some(value => value.toLowerCase().includes(normalizedSearch));
}

function upsertMusicTaskInList(
  list: MusicTaskList | undefined,
  task: MusicTask,
  insertIfMissing: boolean
): MusicTaskList | undefined {
  if (!list) return list;

  const existingIndex = list.items.findIndex(item => item.id === task.id);
  if (existingIndex >= 0) {
    const nextItems = [...list.items];
    nextItems[existingIndex] = task;
    return { ...list, items: nextItems };
  }

  if (!insertIfMissing || list.page !== 1) return list;
  return {
    ...list,
    items: [task, ...list.items].slice(0, list.page_size),
    total: list.total + 1,
  };
}

function upsertMusicTaskIntoListCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  organizationId: string,
  workspaceId: string | null,
  task: MusicTask
) {
  if (!organizationId || !task.id) return;
  queryClient.setQueryData(MUSIC_KEYS.detail(organizationId, workspaceId, task.id), createCachedMusicTaskResponse(task));

  queryClient
    .getQueriesData<ApiResponseData<MusicTaskList>>({
      queryKey: MUSIC_KEYS.lists(organizationId, workspaceId),
    })
    .forEach(([queryKey, data]) => {
      const params = getMusicListParamsFromQueryKey(queryKey);
      const hasExistingTask = Boolean(data?.data.items.some(item => item.id === task.id));
      const insertIfMissing = hasExistingTask || musicTaskMatchesSearch(task, params.search);
      const nextList = upsertMusicTaskInList(data?.data, task, insertIfMissing);
      if (!data || nextList === data.data) return;
      queryClient.setQueryData(queryKey, { ...data, data: nextList });
    });
}

export function useMusicTasks(params: ListMusicTasksParams) {
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const organizationId = currentOrganization?.id ?? '';
  const workspaceId = useCurrentWorkspace()?.id ?? null;
  return useQuery({
    queryKey: MUSIC_KEYS.list(organizationId, workspaceId, params),
    queryFn: () => musicService.listTasks(params),
    enabled: Boolean(organizationId),
    refetchOnWindowFocus: true,
    refetchInterval: query =>
      query.state.data?.data.items.some(task => shouldPollMusicTask(task.status)) ? 2000 : false,
  });
}

export function useMusicTask(id: string | null) {
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const organizationId = currentOrganization?.id ?? '';
  const workspaceId = useCurrentWorkspace()?.id ?? null;
  return useQuery({
    queryKey: MUSIC_KEYS.detail(organizationId, workspaceId, id ?? ''),
    queryFn: () => musicService.getTask(id ?? ''),
    enabled: Boolean(organizationId && id),
    refetchOnWindowFocus: true,
    refetchInterval: query => (shouldPollMusicTask(query.state.data?.data.status) ? 2000 : false),
  });
}

export function useCreateMusicTasks() {
  const queryClient = useQueryClient();
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const organizationId = currentOrganization?.id ?? '';
  const workspaceId = useCurrentWorkspace()?.id ?? null;

  return useMutation<CreateMusicTasksResult, unknown, CreateMusicTasksRequest>({
    mutationFn: data => {
      if (!organizationId) throw new Error('Organization context is required');
      return musicService.createTasks(data);
    },
    onSuccess: result => {
      for (const response of result.responses) {
        upsertMusicTaskIntoListCaches(queryClient, organizationId, workspaceId, response.data);
      }
      void queryClient.invalidateQueries({
        queryKey: MUSIC_KEYS.lists(organizationId, workspaceId),
        refetchType: 'none',
      });
    },
  });
}

export function useDeleteMusicTask() {
  const queryClient = useQueryClient();
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const organizationId = currentOrganization?.id ?? '';
  const workspaceId = useCurrentWorkspace()?.id ?? null;

  return useMutation({
    mutationFn: (id: string) => {
      if (!organizationId) throw new Error('Organization context is required');
      return musicService.deleteTask(id);
    },
    onSuccess: async (_response, id) => {
      queryClient.removeQueries({ queryKey: MUSIC_KEYS.detail(organizationId, workspaceId, id) });
      await queryClient.invalidateQueries({
        queryKey: MUSIC_KEYS.lists(organizationId, workspaceId),
      });
    },
  });
}
