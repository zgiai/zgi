'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { workflowTestService } from '@/services/workflow-test.service';
import { WORKFLOW_TEST_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import type {
  CreateWorkflowTestBatchRequest,
  CreateWorkflowTestCaseRequest,
  CreateWorkflowTestGenerationTaskRequest,
  CreateWorkflowTestScenarioRecognitionTaskRequest,
  DeleteWorkflowTestCasesRequest,
  CreateWorkflowTestScenarioRequest,
  GenerateWorkflowTestCasesRequest,
  RecognizeWorkflowTestScenariosRequest,
  RetestWorkflowTestBatchRequest,
  SaveWorkflowTestScenariosRequest,
  UpdateWorkflowTestCaseRequest,
  UpdateWorkflowTestSettingsRequest,
  WorkflowTestCase,
  WorkflowTestListResponse,
} from '@/services/types/workflow-test';
import type { ApiResponseData } from '@/services/types/common';

export function useWorkflowTestSettings(agentId: string) {
  return useQuery({
    queryKey: WORKFLOW_TEST_KEYS.settings(agentId),
    queryFn: () => workflowTestService.getSettings(agentId),
    enabled: !!agentId,
    refetchOnWindowFocus: false,
  });
}

export function useUpdateWorkflowTestSettings(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: UpdateWorkflowTestSettingsRequest) =>
      workflowTestService.updateSettings(agentId, data),
    onSuccess: () => {
      toast.success(t('settingsSaved'));
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.settings(agentId) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('settingsSaveFailed'));
    },
  });
}

export function useResetWorkflowTestJudgePrompt(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => workflowTestService.resetJudgePrompt(agentId),
    onSuccess: () => {
      toast.success(t('settingsReset'));
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.settings(agentId) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('settingsResetFailed'));
    },
  });
}

export function useWorkflowTestScenarios(agentId: string) {
  return useQuery({
    queryKey: WORKFLOW_TEST_KEYS.scenarios(agentId),
    queryFn: () => workflowTestService.listScenarios(agentId),
    enabled: !!agentId,
    refetchOnWindowFocus: false,
  });
}

export function useCreateWorkflowTestScenario(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateWorkflowTestScenarioRequest) =>
      workflowTestService.createScenario(agentId, data),
    onSuccess: () => {
      toast.success(t('scenarioCreated'));
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.scenarios(agentId) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('scenarioCreateFailed'));
    },
  });
}

export function useSaveWorkflowTestScenarios(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: SaveWorkflowTestScenariosRequest) =>
      workflowTestService.saveScenarios(agentId, data),
    onSuccess: () => {
      toast.success(t('scenariosSaved'));
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.all });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('scenariosSaveFailed'));
    },
  });
}

export function useRecognizeWorkflowTestScenarios(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: RecognizeWorkflowTestScenariosRequest) =>
      workflowTestService.recognizeScenarios(agentId, data),
    onSuccess: response => {
      toast.success(t('scenariosRecognized', { count: response.data.scenarios.length }));
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.all });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('scenariosRecognizeFailed'));
    },
  });
}

const ACTIVE_SCENARIO_RECOGNITION_STATUSES = new Set(['queued', 'running', 'canceling']);

export function useActiveWorkflowTestScenarioRecognitionTask(agentId: string) {
  return useQuery({
    queryKey: WORKFLOW_TEST_KEYS.scenarioRecognitionTaskActive(agentId),
    queryFn: () => workflowTestService.getActiveScenarioRecognitionTask(agentId),
    enabled: !!agentId,
    refetchOnWindowFocus: true,
    refetchInterval: query => {
      const task = query.state.data?.data?.task;
      return task && ACTIVE_SCENARIO_RECOGNITION_STATUSES.has(task.status) ? 3000 : false;
    },
  });
}

export function useLatestWorkflowTestScenarioRecognitionTask(agentId: string) {
  return useQuery({
    queryKey: WORKFLOW_TEST_KEYS.scenarioRecognitionTaskLatest(agentId),
    queryFn: () => workflowTestService.getLatestScenarioRecognitionTask(agentId),
    enabled: !!agentId,
    refetchOnWindowFocus: true,
    refetchInterval: query => {
      const task = query.state.data?.data?.task;
      return task && ACTIVE_SCENARIO_RECOGNITION_STATUSES.has(task.status) ? 3000 : false;
    },
  });
}

export function useCreateWorkflowTestScenarioRecognitionTask(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateWorkflowTestScenarioRecognitionTaskRequest) =>
      workflowTestService.createScenarioRecognitionTask(agentId, data),
    onSuccess: response => {
      toast.info(t('scenariosRecognitionStarted'));
      queryClient.setQueryData(WORKFLOW_TEST_KEYS.scenarioRecognitionTaskLatest(agentId), response);
      queryClient.setQueryData(WORKFLOW_TEST_KEYS.scenarioRecognitionTaskActive(agentId), response);
      queryClient.invalidateQueries({
        queryKey: WORKFLOW_TEST_KEYS.scenarioRecognitionTaskActive(agentId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKFLOW_TEST_KEYS.scenarioRecognitionTaskLatest(agentId),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('scenariosRecognizeFailed'));
    },
  });
}

export function useCancelWorkflowTestScenarioRecognitionTask(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (taskId: string) => workflowTestService.cancelScenarioRecognitionTask(agentId, taskId),
    onSuccess: response => {
      toast.info(t('scenariosRecognitionCancelRequested'));
      queryClient.setQueryData(WORKFLOW_TEST_KEYS.scenarioRecognitionTaskLatest(agentId), response);
      queryClient.setQueryData(WORKFLOW_TEST_KEYS.scenarioRecognitionTaskActive(agentId), response);
      queryClient.invalidateQueries({
        queryKey: WORKFLOW_TEST_KEYS.scenarioRecognitionTaskActive(agentId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKFLOW_TEST_KEYS.scenarioRecognitionTaskLatest(agentId),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('scenariosRecognitionCancelFailed'));
    },
  });
}

export function useWorkflowTestCases(agentId: string, params?: { status?: string }) {
  return useQuery({
    queryKey: WORKFLOW_TEST_KEYS.cases(agentId, params),
    queryFn: () => workflowTestService.listCases(agentId, params),
    enabled: !!agentId,
    refetchOnWindowFocus: false,
  });
}

export function useCreateWorkflowTestCase(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateWorkflowTestCaseRequest) =>
      workflowTestService.createCase(agentId, data),
    onSuccess: () => {
      toast.success(t('caseCreated'));
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.all });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('caseCreateFailed'));
    },
  });
}

export function useUpdateWorkflowTestCase(agentId: string, options?: { silent?: boolean }) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ caseId, data }: { caseId: string; data: UpdateWorkflowTestCaseRequest }) =>
      workflowTestService.updateCase(agentId, caseId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.all });
      if (!options?.silent) {
        toast.success(t('caseUpdated'));
      }
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('caseUpdateFailed'));
    },
  });
}

export function useDeleteWorkflowTestCases(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: DeleteWorkflowTestCasesRequest) =>
      workflowTestService.deleteCases(agentId, data),
    onMutate: async data => {
      const ids = new Set(data.case_ids);
      const casesQueryFilter = {
        queryKey: [...WORKFLOW_TEST_KEYS.all, agentId, 'cases'],
      };
      await queryClient.cancelQueries(casesQueryFilter);
      const previousCases =
        queryClient.getQueriesData<ApiResponseData<WorkflowTestListResponse<WorkflowTestCase>>>(
          casesQueryFilter
        );

      queryClient.setQueriesData<ApiResponseData<WorkflowTestListResponse<WorkflowTestCase>>>(
        casesQueryFilter,
        previous => {
          if (!previous?.data?.items) return previous;
          return {
            ...previous,
            data: {
              ...previous.data,
              items: previous.data.items.filter(item => !ids.has(item.id)),
            },
          };
        }
      );

      return { previousCases };
    },
    onSuccess: (_response, data) => {
      toast.success(
        data.case_ids.length > 1
          ? t('casesDeleted', { count: data.case_ids.length })
          : t('caseDeleted')
      );
    },
    onError: (error, _data, context) => {
      context?.previousCases.forEach(([queryKey, previous]) => {
        queryClient.setQueryData(queryKey, previous);
      });
      toast.error(getErrorMessage(error) || t('caseDeleteFailed'));
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.all });
    },
  });
}

export function useGenerateWorkflowTestCases(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: GenerateWorkflowTestCasesRequest) =>
      workflowTestService.generateCases(agentId, data),
    onSuccess: response => {
      toast.success(t('casesGenerated', { count: response.data.items.length }));
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.all });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('casesGenerateFailed'));
    },
  });
}

const ACTIVE_GENERATION_STATUSES = new Set(['queued', 'running', 'canceling']);

export function useActiveWorkflowTestGenerationTask(agentId: string) {
  return useQuery({
    queryKey: WORKFLOW_TEST_KEYS.generationTaskActive(agentId),
    queryFn: () => workflowTestService.getActiveGenerationTask(agentId),
    enabled: !!agentId,
    refetchOnWindowFocus: true,
    refetchInterval: query => {
      const task = query.state.data?.data?.task;
      return task && ACTIVE_GENERATION_STATUSES.has(task.status) ? 3000 : false;
    },
  });
}

export function useLatestWorkflowTestGenerationTask(agentId: string) {
  return useQuery({
    queryKey: WORKFLOW_TEST_KEYS.generationTaskLatest(agentId),
    queryFn: () => workflowTestService.getLatestGenerationTask(agentId),
    enabled: !!agentId,
    refetchOnWindowFocus: true,
    refetchInterval: query => {
      const task = query.state.data?.data?.task;
      return task && ACTIVE_GENERATION_STATUSES.has(task.status) ? 3000 : false;
    },
  });
}

export function useCreateWorkflowTestGenerationTask(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateWorkflowTestGenerationTaskRequest) =>
      workflowTestService.createGenerationTask(agentId, data),
    onSuccess: response => {
      const count = response.data.task?.requested_count ?? 0;
      toast.info(t('casesGenerationStarted', { count }));
      queryClient.setQueryData(WORKFLOW_TEST_KEYS.generationTaskLatest(agentId), response);
      queryClient.setQueryData(WORKFLOW_TEST_KEYS.generationTaskActive(agentId), response);
      queryClient.invalidateQueries({
        queryKey: WORKFLOW_TEST_KEYS.generationTaskActive(agentId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKFLOW_TEST_KEYS.generationTaskLatest(agentId),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('casesGenerateFailed'));
    },
  });
}

export function useCancelWorkflowTestGenerationTask(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (taskId: string) => workflowTestService.cancelGenerationTask(agentId, taskId),
    onSuccess: response => {
      toast.info(t('casesGenerationCancelRequested'));
      queryClient.setQueryData(WORKFLOW_TEST_KEYS.generationTaskLatest(agentId), response);
      queryClient.setQueryData(WORKFLOW_TEST_KEYS.generationTaskActive(agentId), response);
      queryClient.invalidateQueries({
        queryKey: WORKFLOW_TEST_KEYS.generationTaskActive(agentId),
      });
      queryClient.invalidateQueries({
        queryKey: WORKFLOW_TEST_KEYS.generationTaskLatest(agentId),
      });
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.cases(agentId) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('casesGenerationCancelFailed'));
    },
  });
}

export function useWorkflowTestBatches(agentId: string) {
  return useQuery({
    queryKey: WORKFLOW_TEST_KEYS.batches(agentId),
    queryFn: () => workflowTestService.listBatches(agentId),
    enabled: !!agentId,
    refetchOnWindowFocus: false,
    refetchInterval: query => {
      const batches = query.state.data?.data?.items ?? [];
      return batches.some(batch => batch.status === 'queued' || batch.status === 'running')
        ? 3000
        : false;
    },
  });
}

export function useWorkflowTestBatchItems(agentId: string, batchId: string) {
  return useQuery({
    queryKey: WORKFLOW_TEST_KEYS.batchItems(agentId, batchId),
    queryFn: () => workflowTestService.listBatchItems(agentId, batchId),
    enabled: !!agentId && !!batchId,
    refetchOnWindowFocus: false,
    refetchInterval: query => {
      const items = query.state.data?.data?.items ?? [];
      return items.some(item => item.status === 'pending' || item.status === 'running')
        ? 3000
        : false;
    },
  });
}

export function useCreateWorkflowTestBatch(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateWorkflowTestBatchRequest) =>
      workflowTestService.createBatch(agentId, data),
    onSuccess: () => {
      toast.success(t('batchCreated'));
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.batches(agentId) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('batchCreateFailed'));
    },
  });
}

export function useStartWorkflowTestBatch(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (batchId: string) => workflowTestService.startBatch(agentId, batchId),
    onSuccess: () => {
      toast.success(t('batchStarted'));
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.batches(agentId) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('batchStartFailed'));
    },
  });
}

export function useExecuteWorkflowTestBatch(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (batchId: string) => workflowTestService.executeBatch(agentId, batchId),
    onSuccess: (_data, batchId) => {
      toast.success(t('batchStarted'));
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.batches(agentId) });
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.batchItems(agentId, batchId) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('batchStartFailed'));
    },
  });
}

export function useCancelWorkflowTestBatch(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (batchId: string) => workflowTestService.cancelBatch(agentId, batchId),
    onSuccess: (_data, batchId) => {
      toast.success(t('batchCanceled'));
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.batches(agentId) });
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.batchItems(agentId, batchId) });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('batchCancelFailed'));
    },
  });
}

export function useRetestWorkflowTestBatch(agentId: string) {
  const t = useT('agents.workflowTest.toasts');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      batchId,
      data,
    }: {
      batchId: string;
      data?: RetestWorkflowTestBatchRequest;
    }) => {
      const response = await workflowTestService.retestBatch(agentId, batchId, data);
      await workflowTestService.executeBatch(agentId, response.data.id);
      return response;
    },
    onSuccess: response => {
      toast.success(t('batchStarted'));
      queryClient.invalidateQueries({ queryKey: WORKFLOW_TEST_KEYS.batches(agentId) });
      queryClient.invalidateQueries({
        queryKey: WORKFLOW_TEST_KEYS.batchItems(agentId, response.data.id),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('batchRetestFailed'));
    },
  });
}
