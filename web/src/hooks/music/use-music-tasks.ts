'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { shouldPollMusicTask } from '@/components/music/music-task-state';
import { MUSIC_KEYS } from '@/hooks/query-keys';
import { musicService, type CreateMusicTasksResult } from '@/services/music.service';
import type { CreateMusicTasksRequest, ListMusicTasksParams } from '@/services/types/music';
import { useOrganizationStore } from '@/store/organization-store';
import { useCurrentWorkspace } from '@/store/workspace-store';

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
        queryClient.setQueryData(
          MUSIC_KEYS.detail(organizationId, workspaceId, response.data.id),
          response
        );
      }
    },
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: MUSIC_KEYS.lists(organizationId, workspaceId),
      });
    },
  });
}
