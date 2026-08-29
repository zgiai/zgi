import {
  type InfiniteData,
  type QueryKey,
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
  VideoRuntimeTasksResponse,
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

function removeVideoTaskFromListCache(
  data: InfiniteData<VideoRuntimeTasksResponse> | undefined,
  taskId: string
): InfiniteData<VideoRuntimeTasksResponse> | undefined {
  if (!data) return data;

  let didRemove = false;
  const pages = data.pages.map(page => {
    const items = page.data?.data;
    if (!Array.isArray(items)) return page;

    const nextItems = items.filter(task => task.task_id !== taskId);
    if (nextItems.length === items.length) return page;

    didRemove = true;
    return {
      ...page,
      data: {
        ...page.data,
        data: nextItems,
      },
    };
  });

  if (!didRemove) return data;

  return {
    ...data,
    pages: pages.map(page => ({
      ...page,
      data: {
        ...page.data,
        total: Math.max(0, (page.data?.total ?? 0) - 1),
      },
    })),
  };
}

function upsertVideoTaskInListCache(
  data: InfiniteData<VideoRuntimeTasksResponse> | undefined,
  task: VideoRuntimeTask,
  insertIfMissing: boolean
): InfiniteData<VideoRuntimeTasksResponse> | undefined {
  if (!data?.pages.length) return data;

  let didUpdate = false;
  let didInsert = false;
  const pages = data.pages.map((page, pageIndex) => {
    const items = page.data?.data;
    if (!Array.isArray(items)) return page;

    const existingIndex = items.findIndex(item => item.task_id === task.task_id);
    if (existingIndex >= 0) {
      didUpdate = true;
      const nextItems = [...items];
      nextItems[existingIndex] = task;
      return {
        ...page,
        data: {
          ...page.data,
          data: nextItems,
        },
      };
    }

    if (pageIndex === 0 && insertIfMissing) {
      didInsert = true;
      return {
        ...page,
        data: {
          ...page.data,
          data: [task, ...items],
          total: (page.data?.total ?? items.length) + 1,
        },
      };
    }

    return page;
  });

  if (!didUpdate && !didInsert) return data;
  return { ...data, pages };
}

function getTaskListSearchFromQueryKey(queryKey: QueryKey) {
  const queryPart = Array.isArray(queryKey) ? queryKey[3] : undefined;
  if (!queryPart || typeof queryPart !== 'object' || !('search' in queryPart)) return '';
  const search = (queryPart as { search?: unknown }).search;
  return typeof search === 'string' ? search.trim().toLowerCase() : '';
}

function videoTaskMatchesSearch(task: VideoRuntimeTask, search: string) {
  if (!search) return true;
  return [task.task_id, task.model, task.model_label, task.prompt, task.status]
    .filter((value): value is string => typeof value === 'string')
    .some(value => value.toLowerCase().includes(search));
}

function upsertVideoTaskIntoListCaches(queryClient: QueryClient, task: VideoRuntimeTask) {
  if (!task.task_id) return;
  queryClient.setQueryData<VideoRuntimeTaskResponse>(
    VIDEO_RUNTIME_KEYS.task(task.task_id),
    createCachedVideoTaskResponse(task)
  );

  queryClient
    .getQueriesData<InfiniteData<VideoRuntimeTasksResponse>>({ queryKey: VIDEO_RUNTIME_KEYS.taskLists })
    .forEach(([queryKey, data]) => {
      const search = getTaskListSearchFromQueryKey(queryKey);
      const hasExistingTask = Boolean(
        data?.pages.some(page => page.data?.data?.some(item => item.task_id === task.task_id))
      );
      const insertIfMissing = hasExistingTask || videoTaskMatchesSearch(task, search);
      const nextData = upsertVideoTaskInListCache(data, task, insertIfMissing);
      if (nextData !== data) {
        queryClient.setQueryData(queryKey, nextData);
      }
    });
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
  const queryKey = useMemo(() => VIDEO_RUNTIME_KEYS.taskList(normalizedSearch), [normalizedSearch]);
  const query = useInfiniteQuery({
    queryKey,
    queryFn: ({ pageParam }) =>
      VideoRuntimeService.listTasks({
        limit: 20,
        search: normalizedSearch || undefined,
        cursor: typeof pageParam === 'string' && pageParam ? pageParam : undefined,
      }),
    initialPageParam: '',
    getNextPageParam: lastPage => (lastPage.data?.has_more ? lastPage.data.next_cursor : undefined),
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
    await queryClient.invalidateQueries({ queryKey, exact: true, refetchType: 'active' });
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
    if (!taskIdValue || taskActive || !task) return;
    upsertVideoTaskIntoListCaches(queryClient, task);
    void queryClient.invalidateQueries({
      queryKey: VIDEO_RUNTIME_KEYS.taskLists,
      refetchType: 'none',
    });
  }, [queryClient, task, taskActive, taskIdValue, taskStatus]);

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
    onSuccess: response => {
      const task = response.data?.task;
      if (task) upsertVideoTaskIntoListCaches(queryClient, task);
      void queryClient.invalidateQueries({
        queryKey: VIDEO_RUNTIME_KEYS.taskLists,
        refetchType: 'none',
      });
    },
  });
}

export function useDeleteVideoTask() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (taskId: string) => VideoRuntimeService.deleteTask(taskId),
    onMutate: async taskId => {
      await queryClient.cancelQueries({ queryKey: VIDEO_RUNTIME_KEYS.taskLists });

      const snapshots = queryClient.getQueriesData<InfiniteData<VideoRuntimeTasksResponse>>({
        queryKey: VIDEO_RUNTIME_KEYS.taskLists,
      });

      queryClient.setQueriesData<InfiniteData<VideoRuntimeTasksResponse>>(
        { queryKey: VIDEO_RUNTIME_KEYS.taskLists },
        current => removeVideoTaskFromListCache(current, taskId)
      );

      return { snapshots };
    },
    onError: (_error, _taskId, context) => {
      context?.snapshots.forEach(
        ([queryKey, data]: [QueryKey, InfiniteData<VideoRuntimeTasksResponse> | undefined]) => {
          queryClient.setQueryData(queryKey, data);
        }
      );
    },
    onSuccess: (_data, taskId) => {
      queryClient.removeQueries({ queryKey: VIDEO_RUNTIME_KEYS.task(taskId) });
    },
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: VIDEO_RUNTIME_KEYS.taskLists,
        refetchType: 'active',
      });
    },
  });
}

function isVideoRuntimeTaskActive(task: Pick<VideoRuntimeTask, 'status'>) {
  const status = task.status?.toLowerCase?.() ?? '';
  return (
    status === 'pending' ||
    status === 'running' ||
    status === 'processing' ||
    status === 'in_progress'
  );
}
