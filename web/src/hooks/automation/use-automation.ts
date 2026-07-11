'use client';

import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { AUTOMATION_KEYS } from '@/hooks/query-keys';
import { automationService } from '@/services/automation.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  AutomationManualRunData,
  AutomationOperationResult,
  AutomationTaskActionParams,
  AutomationTaskCounts,
  AutomationTaskDetailData,
  AutomationTaskList,
  AutomationTaskListItem,
  AutomationTaskRunItem,
  AutomationTaskRunsList,
  CreateAutomationTaskRequest,
  GenerateAutomationTaskDraftRequest,
  GenerateAutomationTaskDraftResponse,
  GetAutomationTaskParams,
  GetAutomationTaskRunsParams,
  GetAutomationTasksParams,
  UpdateAutomationTaskRequest,
} from '@/services/types/automation';
import { getErrorMessage } from '@/utils/error-notifications';

function hasActiveAutomationRun(list?: AutomationTaskRunsList): boolean {
  return Boolean(
    list?.runs.some(item => item.run.status === 'queued' || item.run.status === 'running')
  );
}

/**
 * Hook for fetching status counts without loading full task pages.
 */
export function useAutomationTaskCounts(workspaceId?: string): {
  counts: Partial<AutomationTaskCounts>;
  isFetching: boolean;
} {
  const params = { workspace_id: workspaceId };
  const { data, isFetching } = useQuery<ApiResponseData<AutomationTaskCounts>, unknown>({
    queryKey: AUTOMATION_KEYS.count(params),
    queryFn: () => automationService.getTaskCounts(params),
    enabled: Boolean(workspaceId),
    staleTime: 60 * 1000,
    gcTime: 10 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  return {
    counts: data?.data ?? {},
    isFetching,
  };
}

/**
 * Hook for generating an editable automation task draft from natural language.
 */
export function useGenerateAutomationTaskDraft(): {
  generateAutomationTaskDraft: (
    data: GenerateAutomationTaskDraftRequest
  ) => Promise<GenerateAutomationTaskDraftResponse | undefined>;
  isGenerating: boolean;
} {
  const t = useT('common');

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<GenerateAutomationTaskDraftResponse>,
    unknown,
    GenerateAutomationTaskDraftRequest
  >({
    mutationFn: data => automationService.generateTaskDraft(data),
    onSuccess: () => {
      toast.success(t('toasts.operationSuccess'));
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('toasts.operationFailed'));
    },
  });

  return {
    generateAutomationTaskDraft: async (data: GenerateAutomationTaskDraftRequest) => {
      const res = await mutateAsync(data);
      return res?.data;
    },
    isGenerating: isPending,
  };
}

/**
 * Hook for fetching automation task list with pagination.
 */
export function useAutomationTasks(params?: GetAutomationTasksParams): {
  items: AutomationTaskListItem[];
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const t = useT('common');
  const { data, isLoading, isFetching, error, refetch } = useQuery<
    ApiResponseData<AutomationTaskList>,
    unknown
  >({
    queryKey: AUTOMATION_KEYS.list(params),
    queryFn: () => automationService.getTasks(params),
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  useEffect(() => {
    if (!error) return;
    toast.error(getErrorMessage(error) || t('toasts.operationFailed'));
  }, [error, t]);

  const list = data?.data;

  return {
    items: list?.items ?? [],
    total: list?.total ?? 0,
    page: list?.page ?? params?.page ?? 1,
    limit: list?.limit ?? params?.limit ?? 20,
    has_more: list?.has_more ?? false,
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    refetch: async () => {
      await refetch();
    },
  };
}

/**
 * Hook for fetching single automation task detail.
 */
export function useAutomationTask(
  id?: string,
  params?: GetAutomationTaskParams,
  enabled: boolean = true
): {
  taskDetail: AutomationTaskDetailData | undefined;
  task: AutomationTaskDetailData['task'] | undefined;
  actions: AutomationTaskDetailData['actions'];
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const t = useT('common');
  const { data, isLoading, isFetching, error, refetch } = useQuery<
    ApiResponseData<AutomationTaskDetailData>,
    unknown
  >({
    queryKey: AUTOMATION_KEYS.detail(id ?? '', params),
    queryFn: () => automationService.getTask(id ?? '', params),
    enabled: Boolean(id) && enabled,
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  useEffect(() => {
    if (!error) return;
    toast.error(getErrorMessage(error) || t('toasts.operationFailed'));
  }, [error, t]);

  return {
    taskDetail: data?.data,
    task: data?.data?.task,
    actions: data?.data?.actions ?? [],
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    refetch: async () => {
      await refetch();
    },
  };
}

/**
 * Hook for fetching automation task run history with pagination.
 */
export function useAutomationTaskRuns(
  id?: string,
  params?: GetAutomationTaskRunsParams,
  enabled: boolean = true
): {
  taskId: string | undefined;
  runs: AutomationTaskRunItem[];
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<void>;
} {
  const t = useT('common');
  const { data, isLoading, isFetching, error, refetch } = useQuery<
    ApiResponseData<AutomationTaskRunsList>,
    unknown
  >({
    queryKey: AUTOMATION_KEYS.runList(id ?? '', params),
    queryFn: () => automationService.getTaskRuns(id ?? '', params),
    enabled: Boolean(id) && enabled,
    staleTime: 60 * 1000,
    gcTime: 10 * 60 * 1000,
    refetchOnWindowFocus: false,
    refetchInterval: query => (hasActiveAutomationRun(query.state.data?.data) ? 2000 : false),
    retry: false,
  });

  useEffect(() => {
    if (!error) return;
    toast.error(getErrorMessage(error) || t('toasts.operationFailed'));
  }, [error, t]);

  const runList = data?.data;

  return {
    taskId: runList?.task_id,
    runs: runList?.runs ?? [],
    total: runList?.total ?? 0,
    page: runList?.page ?? params?.page ?? 1,
    limit: runList?.limit ?? params?.limit ?? 20,
    has_more: runList?.has_more ?? false,
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    refetch: async () => {
      await refetch();
    },
  };
}

/**
 * Hook for creating automation tasks.
 */
export function useCreateAutomationTask(): {
  createAutomationTask: (
    data: CreateAutomationTaskRequest
  ) => Promise<AutomationTaskDetailData | undefined>;
  isCreating: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('common');

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<AutomationTaskDetailData>,
    unknown,
    CreateAutomationTaskRequest
  >({
    mutationFn: data => automationService.createTask(data),
    onSuccess: () => {
      toast.success(t('toasts.createSuccess'));
      queryClient.invalidateQueries({ queryKey: AUTOMATION_KEYS.lists() });
      queryClient.invalidateQueries({ queryKey: AUTOMATION_KEYS.counts() });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('toasts.createFailed'));
    },
  });

  return {
    createAutomationTask: async (data: CreateAutomationTaskRequest) => {
      const res = await mutateAsync(data);
      return res?.data;
    },
    isCreating: isPending,
  };
}

/**
 * Hook for updating automation tasks.
 */
export function useUpdateAutomationTask(): {
  updateAutomationTask: (
    id: string,
    data: UpdateAutomationTaskRequest
  ) => Promise<AutomationTaskDetailData | undefined>;
  isUpdating: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('common');

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<AutomationTaskDetailData>,
    unknown,
    { id: string; data: UpdateAutomationTaskRequest }
  >({
    mutationFn: ({ id, data }) => automationService.updateTask(id, data),
    onSuccess: (_result, variables) => {
      toast.success(t('toasts.updateSuccess'));
      queryClient.invalidateQueries({ queryKey: AUTOMATION_KEYS.lists() });
      queryClient.invalidateQueries({ queryKey: AUTOMATION_KEYS.counts() });
      queryClient.invalidateQueries({ queryKey: AUTOMATION_KEYS.detailPrefix(variables.id) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('toasts.updateFailed'));
    },
  });

  return {
    updateAutomationTask: async (id: string, data: UpdateAutomationTaskRequest) => {
      const res = await mutateAsync({ id, data });
      return res?.data;
    },
    isUpdating: isPending,
  };
}

/**
 * Hook for manually running automation tasks once.
 */
export function useRunAutomationTask(): {
  runAutomationTask: (
    id: string,
    params?: AutomationTaskActionParams
  ) => Promise<AutomationManualRunData | undefined>;
  isRunning: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('common');

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<AutomationManualRunData>,
    unknown,
    { id: string; params?: AutomationTaskActionParams }
  >({
    mutationFn: ({ id, params }) => automationService.runTask(id, params),
    onSuccess: (_result, variables) => {
      toast.success(t('toasts.operationSuccess'));
      queryClient.invalidateQueries({ queryKey: AUTOMATION_KEYS.lists() });
      queryClient.invalidateQueries({ queryKey: AUTOMATION_KEYS.counts() });
      queryClient.invalidateQueries({ queryKey: AUTOMATION_KEYS.detailPrefix(variables.id) });
      queryClient.invalidateQueries({ queryKey: AUTOMATION_KEYS.runs(variables.id) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('toasts.operationFailed'));
    },
  });

  return {
    runAutomationTask: async (id: string, params?: AutomationTaskActionParams) => {
      const res = await mutateAsync({ id, params });
      return res?.data;
    },
    isRunning: isPending,
  };
}

function useAutomationOperationMutation(operation: 'pause' | 'resume' | 'archive'): {
  mutateAsync: (
    id: string,
    params?: AutomationTaskActionParams
  ) => Promise<AutomationOperationResult | undefined>;
  isPending: boolean;
} {
  const queryClient = useQueryClient();
  const t = useT('common');

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<AutomationOperationResult>,
    unknown,
    { id: string; params?: AutomationTaskActionParams }
  >({
    mutationFn: ({ id, params }) => {
      switch (operation) {
        case 'pause':
          return automationService.pauseTask(id, params);
        case 'resume':
          return automationService.resumeTask(id, params);
        case 'archive':
          return automationService.archiveTask(id, params);
        default:
          throw new Error(`Unsupported automation operation: ${operation}`);
      }
    },
    onSuccess: (_result, variables) => {
      toast.success(t('toasts.operationSuccess'));
      queryClient.invalidateQueries({ queryKey: AUTOMATION_KEYS.lists() });
      queryClient.invalidateQueries({ queryKey: AUTOMATION_KEYS.counts() });
      queryClient.invalidateQueries({ queryKey: AUTOMATION_KEYS.detailPrefix(variables.id) });
      queryClient.invalidateQueries({ queryKey: AUTOMATION_KEYS.runs(variables.id) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('toasts.operationFailed'));
    },
  });

  return {
    mutateAsync: async (id: string, params?: AutomationTaskActionParams) => {
      const res = await mutateAsync({ id, params });
      return res?.data;
    },
    isPending,
  };
}

/**
 * Hook for pausing automation tasks.
 */
export function usePauseAutomationTask(): {
  pauseAutomationTask: (
    id: string,
    params?: AutomationTaskActionParams
  ) => Promise<AutomationOperationResult | undefined>;
  isPausing: boolean;
} {
  const mutation = useAutomationOperationMutation('pause');

  return {
    pauseAutomationTask: mutation.mutateAsync,
    isPausing: mutation.isPending,
  };
}

/**
 * Hook for resuming automation tasks.
 */
export function useResumeAutomationTask(): {
  resumeAutomationTask: (
    id: string,
    params?: AutomationTaskActionParams
  ) => Promise<AutomationOperationResult | undefined>;
  isResuming: boolean;
} {
  const mutation = useAutomationOperationMutation('resume');

  return {
    resumeAutomationTask: mutation.mutateAsync,
    isResuming: mutation.isPending,
  };
}

/**
 * Hook for archiving automation tasks.
 */
export function useArchiveAutomationTask(): {
  archiveAutomationTask: (
    id: string,
    params?: AutomationTaskActionParams
  ) => Promise<AutomationOperationResult | undefined>;
  isArchiving: boolean;
} {
  const mutation = useAutomationOperationMutation('archive');

  return {
    archiveAutomationTask: mutation.mutateAsync,
    isArchiving: mutation.isPending,
  };
}
