'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import { digitalHumanService } from '@/services/digital-human.service';
import type { CreateDigitalHumanTaskRequest, UpdateDigitalHumanTaskRequest } from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';

export interface DigitalHumanTaskFilters {
  query?: string;
  status?: string[];
  priority?: string[];
  schedule_type?: string;
}

export function useDigitalHumanTasks(id: string, filters: DigitalHumanTaskFilters = {}) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.tasks(id, filters),
    queryFn: () => digitalHumanService.listTasks(id, { page: 1, page_size: 100, ...filters }),
    enabled: Boolean(id),
    refetchInterval: current => current.state.data?.data.items.some(item => item.work_status === 'in_progress') ? 3_000 : 15_000,
  });
}

export function useDigitalHumanTaskDetail(id: string, taskId: string) {
  const task = useQuery({ queryKey: DIGITAL_HUMAN_KEYS.task(id, taskId), queryFn: () => digitalHumanService.getTask(id, taskId), enabled: Boolean(id && taskId) });
  const runs = useQuery({ queryKey: DIGITAL_HUMAN_KEYS.taskRuns(id, taskId), queryFn: () => digitalHumanService.listTaskRuns(id, taskId), enabled: Boolean(id && taskId) });
  const activities = useQuery({ queryKey: DIGITAL_HUMAN_KEYS.taskActivities(id, taskId), queryFn: () => digitalHumanService.listTaskActivities(id, taskId), enabled: Boolean(id && taskId) });
  return { task, runs, activities };
}

export function useDigitalHumanTaskMutations(id: string) {
  const client = useQueryClient();
  const invalidate = () => client.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.detail(id) });
  const handleError = (error: unknown) => toast.error(getErrorMessage(error) || 'Request failed');
  const create = useMutation({ mutationFn: (request: CreateDigitalHumanTaskRequest) => digitalHumanService.createTask(id, request), onSuccess: invalidate, onError: handleError });
  const update = useMutation({ mutationFn: ({ taskId, request }: { taskId: string; request: UpdateDigitalHumanTaskRequest }) => digitalHumanService.updateTask(id, taskId, request), onSuccess: invalidate, onError: handleError });
  const action = useMutation({ mutationFn: ({ taskId, action, revision }: { taskId: string; action: string; revision: number }) => digitalHumanService.applyTaskAction(id, taskId, action, revision), onSuccess: invalidate, onError: handleError });
  return { create, update, action };
}

export function useChannelTaskPolicy(id: string, installationId: string) {
  const client = useQueryClient();
  const query = useQuery({ queryKey: DIGITAL_HUMAN_KEYS.channelTaskPolicy(id, installationId), queryFn: () => digitalHumanService.getChannelTaskPolicy(id, installationId), enabled: Boolean(id && installationId) });
  const update = useMutation({
    mutationFn: (enabled: boolean) => digitalHumanService.updateChannelTaskPolicy(id, installationId, enabled, query.data?.data.revision ?? 0),
    onSuccess: () => client.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.channelTaskPolicy(id, installationId) }),
    onError: error => toast.error(getErrorMessage(error) || 'Request failed'),
  });
  return { query, update };
}
