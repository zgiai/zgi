import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query';
import { useCallback, useEffect, useMemo } from 'react';

import { VideoRuntimeService } from '@/services/video-runtime.service';
import type {
  VideoRuntimeGenerateRequest,
  VideoRuntimeTask,
  VideoRuntimeTaskResponse,
} from '@/services/types/video-runtime';

export const VIDEO_RUNTIME_KEYS = {
  models: ['video-runtime', 'models'] as const,
  tasks: ['video-runtime', 'tasks'] as const,
  taskLists: ['video-runtime', 'tasks', 'list'] as const,
  taskList: (search: string) => ['video-runtime', 'tasks', 'list', { search }] as const,
  task: (taskId: string) => ['video-runtime', 'tasks', taskId] as const,
};

const VIDEO_TASK_DETAIL_STALE_TIME = 10 * 1000;

function createCachedVideoTaskResponse(task: VideoRuntimeTask): VideoRuntimeTaskResponse {
  return {
    code: '0',
    message: 'success',
    data: task,
  };
}

function seedVideoTaskDetailCache(queryClient: QueryClient, task: VideoRuntimeTask) {
  if (!task.task_id) return;
  const queryKey = VIDEO_RUNTIME_KEYS.task(task.task_id);
  if (queryClient.getQueryData(queryKey)) return;
  queryClient.setQueryData<VideoRuntimeTaskResponse>(
    queryKey,
    createCachedVideoTaskResponse(task),
    { updatedAt: 0 }
  );
}

export function useVideoRuntimeModels() {
  const query = useQuery({
    queryKey: VIDEO_RUNTIME_KEYS.models,
    queryFn: () => VideoRuntimeService.listModels(),
    staleTime: 60 * 1000,
    retry: false,
  });

  return {
    ...query,
    models: query.data?.data ?? [],
  };
}

export function useVideoRuntimeTasks(search = '') {
  const queryClient = useQueryClient();
  const normalizedSearch = search.trim();
  const queryKey = useMemo(
    () => VIDEO_RUNTIME_KEYS.taskList(normalizedSearch),
    [normalizedSearch]
  );
  const query = useInfiniteQuery({
    queryKey,
    queryFn: ({ pageParam }) =>
      VideoRuntimeService.listTasks({
        limit: 20,
        search: normalizedSearch || undefined,
        cursor: typeof pageParam === 'string' && pageParam ? pageParam : undefined,
      }),
    initialPageParam: '',
    getNextPageParam: lastPage =>
      lastPage.data?.has_more ? lastPage.data.next_cursor : undefined,
    retry: false,
  });

  const tasks = useMemo(() => {
    const seen = new Set<string>();
    return (query.data?.pages ?? [])
      .flatMap(page => page.data?.data ?? [])
      .filter(task => {
        if (seen.has(task.task_id)) return false;
        seen.add(task.task_id);
        return true;
      });
  }, [query.data?.pages]);

  useEffect(() => {
    tasks.forEach(task => {
      seedVideoTaskDetailCache(queryClient, task);
    });
  }, [queryClient, tasks]);

  const reload = useCallback(async () => {
    await queryClient.resetQueries({ queryKey, exact: true });
  }, [queryClient, queryKey]);

  return {
    ...query,
    tasks,
    total: query.data?.pages[0]?.data?.total ?? 0,
    hasNextPage: Boolean(query.hasNextPage),
    reload,
  };
}

export function useVideoRuntimeTask(taskId?: string | null) {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: VIDEO_RUNTIME_KEYS.task(taskId ?? ''),
    queryFn: () => VideoRuntimeService.getTask(taskId as string),
    enabled: Boolean(taskId),
    refetchInterval: query => {
      const task = query.state.data?.data;
      return task && isVideoRuntimeTaskActive(task) ? 5000 : false;
    },
    staleTime: VIDEO_TASK_DETAIL_STALE_TIME,
    retry: false,
  });

  const task = query.data?.data ?? null;
  const taskIdValue = task?.task_id;
  const taskStatus = task?.status;
  const taskActive = task ? isVideoRuntimeTaskActive(task) : false;

  useEffect(() => {
    if (!taskIdValue || taskActive) return;
    void queryClient.resetQueries({ queryKey: VIDEO_RUNTIME_KEYS.taskLists });
  }, [queryClient, taskActive, taskIdValue, taskStatus]);

  return {
    ...query,
    task,
  };
}

export function usePrefetchVideoRuntimeTask() {
  const queryClient = useQueryClient();

  return useCallback(
    (task: VideoRuntimeTask) => {
      if (!task.task_id) return;
      seedVideoTaskDetailCache(queryClient, task);
      void queryClient.prefetchQuery({
        queryKey: VIDEO_RUNTIME_KEYS.task(task.task_id),
        queryFn: () => VideoRuntimeService.getTask(task.task_id),
        staleTime: VIDEO_TASK_DETAIL_STALE_TIME,
      });
    },
    [queryClient]
  );
}

export function useGenerateVideoTask() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: VideoRuntimeGenerateRequest) => VideoRuntimeService.generate(payload),
    onSuccess: () => {
      void queryClient.resetQueries({ queryKey: VIDEO_RUNTIME_KEYS.taskLists });
    },
  });
}

export function useDeleteVideoTask() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (taskId: string) => VideoRuntimeService.deleteTask(taskId),
    onSuccess: (_data, taskId) => {
      void queryClient.resetQueries({ queryKey: VIDEO_RUNTIME_KEYS.taskLists });
      queryClient.removeQueries({ queryKey: VIDEO_RUNTIME_KEYS.task(taskId) });
    },
  });
}

function isVideoRuntimeTaskActive(task: Pick<VideoRuntimeTask, 'status'>) {
  const status = task.status?.toLowerCase?.() ?? '';
  return status === 'pending' || status === 'running' || status === 'processing' || status === 'in_progress';
}
