import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';

import { VideoRuntimeService } from '@/services/video-runtime.service';
import type { VideoRuntimeGenerateRequest, VideoRuntimeTask } from '@/services/types/video-runtime';

export const VIDEO_RUNTIME_KEYS = {
  models: ['video-runtime', 'models'] as const,
  tasks: ['video-runtime', 'tasks'] as const,
  task: (taskId: string) => ['video-runtime', 'tasks', taskId] as const,
};

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

export function useVideoRuntimeTasks() {
  const query = useQuery({
    queryKey: VIDEO_RUNTIME_KEYS.tasks,
    queryFn: () => VideoRuntimeService.listTasks(),
    refetchInterval: query => {
      const tasks = query.state.data?.data ?? [];
      return tasks.some(isVideoRuntimeTaskActive) ? 8000 : false;
    },
    retry: false,
  });

  return {
    ...query,
    tasks: query.data?.data ?? [],
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
    retry: false,
  });

  const task = query.data?.data ?? null;
  const taskIdValue = task?.task_id;
  const taskStatus = task?.status;
  const taskActive = task ? isVideoRuntimeTaskActive(task) : false;

  useEffect(() => {
    if (!taskIdValue || taskActive) return;
    void queryClient.invalidateQueries({ queryKey: VIDEO_RUNTIME_KEYS.tasks });
  }, [queryClient, taskActive, taskIdValue, taskStatus]);

  return {
    ...query,
    task,
  };
}

export function useGenerateVideoTask() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: VideoRuntimeGenerateRequest) => VideoRuntimeService.generate(payload),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: VIDEO_RUNTIME_KEYS.tasks });
    },
  });
}

export function useDeleteVideoTask() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (taskId: string) => VideoRuntimeService.deleteTask(taskId),
    onSuccess: (_data, taskId) => {
      void queryClient.invalidateQueries({ queryKey: VIDEO_RUNTIME_KEYS.tasks });
      queryClient.removeQueries({ queryKey: VIDEO_RUNTIME_KEYS.task(taskId) });
    },
  });
}

function isVideoRuntimeTaskActive(task: Pick<VideoRuntimeTask, 'status'>) {
  const status = task.status?.toLowerCase?.() ?? '';
  return status === 'pending' || status === 'running' || status === 'processing' || status === 'in_progress';
}
