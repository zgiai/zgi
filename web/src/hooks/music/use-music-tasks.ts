'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { shouldPollMusicTask } from '@/components/music/music-task-state';
import { MUSIC_KEYS } from '@/hooks/query-keys';
import { musicService, type CreateMusicTasksResult } from '@/services/music.service';
import type { CreateMusicTasksRequest, ListMusicTasksParams } from '@/services/types/music';
import { useCurrentWorkspace } from '@/store/workspace-store';

export function useMusicTasks(params: ListMusicTasksParams) {
  const workspaceId = useCurrentWorkspace()?.id ?? '';
  return useQuery({
    queryKey: MUSIC_KEYS.list(workspaceId, params),
    queryFn: () => musicService.listTasks(params),
    enabled: Boolean(workspaceId),
    refetchOnWindowFocus: true,
    refetchInterval: query =>
      query.state.data?.data.items.some(task => shouldPollMusicTask(task.status)) ? 2000 : false,
  });
}

export function useMusicTask(id: string | null) {
  const workspaceId = useCurrentWorkspace()?.id ?? '';
  return useQuery({
    queryKey: MUSIC_KEYS.detail(workspaceId, id ?? ''),
    queryFn: () => musicService.getTask(id ?? ''),
    enabled: Boolean(workspaceId && id),
    refetchOnWindowFocus: true,
    refetchInterval: query => (shouldPollMusicTask(query.state.data?.data.status) ? 2000 : false),
  });
}

export function useCreateMusicTasks() {
  const queryClient = useQueryClient();
  const workspaceId = useCurrentWorkspace()?.id ?? '';

  return useMutation<CreateMusicTasksResult, unknown, CreateMusicTasksRequest>({
    mutationFn: data => {
      if (!workspaceId) throw new Error('Workspace context is required');
      return musicService.createTasks(data);
    },
    onSuccess: result => {
      for (const response of result.responses) {
        queryClient.setQueryData(MUSIC_KEYS.detail(workspaceId, response.data.id), response);
      }
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: MUSIC_KEYS.lists(workspaceId) });
    },
  });
}

export function useDeleteMusicTask() {
  const queryClient = useQueryClient();
  const workspaceId = useCurrentWorkspace()?.id ?? '';

  return useMutation({
    mutationFn: (id: string) => {
      if (!workspaceId) throw new Error('Workspace context is required');
      return musicService.deleteTask(id);
    },
    onSuccess: async (_response, id) => {
      queryClient.removeQueries({ queryKey: MUSIC_KEYS.detail(workspaceId, id) });
      await queryClient.invalidateQueries({ queryKey: MUSIC_KEYS.lists(workspaceId) });
    },
  });
}
