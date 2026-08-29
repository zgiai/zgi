import { useMemo } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { ImageRuntimeService } from '@/services/image-runtime.service';

export const IMAGE_RUNTIME_KEYS = {
  models: ['image-runtime', 'models'] as const,
  conversations: ['image-runtime', 'conversations'] as const,
  search: ['image-runtime', 'search'] as const,
  taskLists: ['image-runtime', 'tasks', 'list'] as const,
  taskList: (search: string) => ['image-runtime', 'tasks', 'list', { search }] as const,
  task: (taskId: string) => ['image-runtime', 'tasks', taskId] as const,
};

export function useImageRuntimeModels() {
  const query = useQuery({
    queryKey: IMAGE_RUNTIME_KEYS.models,
    queryFn: () => ImageRuntimeService.listModels(),
    staleTime: 60 * 1000,
    retry: false,
  });

  return {
    ...query,
    models: query.data?.data ?? [],
  };
}

export function useImageRuntimeTasks(search = '') {
  const normalizedSearch = search.trim();
  const queryKey = useMemo(
    () => IMAGE_RUNTIME_KEYS.taskList(normalizedSearch),
    [normalizedSearch]
  );
  const query = useInfiniteQuery({
    queryKey,
    queryFn: ({ pageParam }) =>
      ImageRuntimeService.listTasks({
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

  return {
    ...query,
    tasks,
    total: query.data?.pages[0]?.data?.total ?? 0,
  };
}
