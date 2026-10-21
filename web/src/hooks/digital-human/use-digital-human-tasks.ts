'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import { digitalHumanService } from '@/services/digital-human.service';
import type {
  CreateDigitalHumanTaskRequest,
  DigitalHumanTaskReportParams,
  ReviewDigitalHumanTaskReportRequest,
  UpdateDigitalHumanTaskRequest,
} from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';

export interface DigitalHumanTaskFilters {
  page?: number;
  query?: string;
  status?: string[];
  priority?: string[];
  schedule_type?: string;
  run_finished_after?: string;
  run_finished_before?: string;
  // Also list tasks outside `status` whose final report waits for review.
  include_unreviewed?: boolean;
}

export function useDigitalHumanTasks(id: string, filters: DigitalHumanTaskFilters = {}) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.tasks(id, filters),
    queryFn: () => digitalHumanService.listTasks(id, { page: filters.page ?? 1, page_size: 100, ...filters }),
    enabled: Boolean(id),
    refetchInterval: current => current.state.data?.data.items.some(item => item.work_status === 'in_progress') ? 3_000 : 15_000,
  });
}

export function useDigitalHumanMyTasks(id: string, filters: DigitalHumanTaskFilters = {}, enabled = true) {
  return useQuery({
    queryKey: [...DIGITAL_HUMAN_KEYS.tasks(id), 'mine', filters],
    queryFn: () => digitalHumanService.listMyTasks(id, { page: filters.page ?? 1, page_size: 100, ...filters }),
    enabled: Boolean(id) && enabled,
    refetchInterval: current => current.state.data?.data.items.some(item => item.work_status === 'in_progress') ? 3_000 : 15_000,
  });
}

export function useDigitalHumanTaskDetail(id: string, taskId: string, runPage = 1, activityPage = 1) {
  const enabled = Boolean(id && taskId);
  const task = useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.task(id, taskId),
    queryFn: () => digitalHumanService.getTask(id, taskId),
    enabled,
    refetchInterval: query => query.state.data?.data.work_status === 'in_progress' ? 3_000 : 15_000,
  });
  const interval = task.data?.data.work_status === 'in_progress' ? 3_000 : 15_000;
  const runs = useQuery({
    queryKey: [...DIGITAL_HUMAN_KEYS.taskRuns(id, taskId), runPage],
    queryFn: () => digitalHumanService.listTaskRuns(id, taskId, runPage),
    enabled,
    refetchInterval: interval,
  });
  const activities = useQuery({
    queryKey: [...DIGITAL_HUMAN_KEYS.taskActivities(id, taskId), activityPage],
    queryFn: () => digitalHumanService.listTaskActivities(id, taskId, activityPage),
    enabled,
    refetchInterval: interval,
  });
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

export function useDigitalHumanMyTaskMutations(id: string) {
  const client = useQueryClient();
  const invalidate = () => client.invalidateQueries({ queryKey: [...DIGITAL_HUMAN_KEYS.tasks(id), 'mine'] });
  const handleError = (error: unknown) => toast.error(getErrorMessage(error) || 'Request failed');
  const create = useMutation({ mutationFn: (request: CreateDigitalHumanTaskRequest) => digitalHumanService.createMyTask(id, request), onSuccess: invalidate, onError: handleError });
  const update = useMutation({ mutationFn: ({ taskId, request }: { taskId: string; request: UpdateDigitalHumanTaskRequest }) => digitalHumanService.updateMyTask(id, taskId, request), onSuccess: invalidate, onError: handleError });
  const action = useMutation({ mutationFn: ({ taskId, action, revision }: { taskId: string; action: string; revision: number }) => digitalHumanService.applyMyTaskAction(id, taskId, action, revision), onSuccess: invalidate, onError: handleError });
  return { create, update, action };
}

// Task reports of the task center, or of the reader's own tasks when `mine`.
export function useDigitalHumanTaskReports(
  id: string,
  params: DigitalHumanTaskReportParams = {},
  mine = false,
  enabled = true
) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.taskReports(id, { ...params, mine }),
    queryFn: () => digitalHumanService.listTaskReports(id, params, mine),
    enabled: Boolean(id) && enabled,
  });
}

export function useDigitalHumanTaskReport(id: string, reportId: string | null, mine = false, options: { retry?: boolean } = {}) {
  return useQuery({
    queryKey: [...DIGITAL_HUMAN_KEYS.taskReports(id), 'report', reportId, mine],
    queryFn: () => digitalHumanService.getTaskReport(id, reportId ?? '', mine),
    enabled: Boolean(id && reportId),
    ...options,
  });
}

// Accepting a report or asking for rework changes the report, its task, and
// the reader's inbox.
export function useDigitalHumanTaskReportReview(id: string, mine = false) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ reportId, request }: { reportId: string; request: ReviewDigitalHumanTaskReportRequest }) =>
      digitalHumanService.reviewTaskReport(id, reportId, request, mine),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.detail(id) });
    },
    onError: (error: unknown) => toast.error(getErrorMessage(error)),
  });
}
