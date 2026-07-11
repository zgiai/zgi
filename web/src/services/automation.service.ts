import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  AutomationManualRunData,
  AutomationOperationResult,
  AutomationTaskDetailData,
  AutomationTaskCounts,
  AutomationTaskList,
  AutomationTaskActionParams,
  CreateAutomationTaskRequest,
  GenerateAutomationTaskDraftRequest,
  GenerateAutomationTaskDraftResponse,
  GetAutomationTaskParams,
  GetAutomationTaskCountsParams,
  GetAutomationTaskRunsParams,
  GetAutomationTasksParams,
  UpdateAutomationTaskRequest,
  AutomationTaskRunsList,
} from './types/automation';

/**
 * Automation service for scheduled task APIs.
 */
export class AutomationService extends BaseService {
  constructor() {
    super({
      endpoint: 'main',
      basePath: '/console/api',
    });
  }

  createTask(
    data: CreateAutomationTaskRequest
  ): Promise<ApiResponseData<AutomationTaskDetailData>> {
    return this.request('post', '/automations/tasks', data);
  }

  generateTaskDraft(
    data: GenerateAutomationTaskDraftRequest
  ): Promise<ApiResponseData<GenerateAutomationTaskDraftResponse>> {
    return this.request('post', '/automations/tasks/draft/generate', data);
  }

  getTasks(params?: GetAutomationTasksParams): Promise<ApiResponseData<AutomationTaskList>> {
    return this.request('get', '/automations/tasks', undefined, { params });
  }

  getTaskCounts(
    params?: GetAutomationTaskCountsParams
  ): Promise<ApiResponseData<AutomationTaskCounts>> {
    return this.request('get', '/automations/tasks/counts', undefined, { params });
  }

  getTask(
    id: string,
    params?: GetAutomationTaskParams
  ): Promise<ApiResponseData<AutomationTaskDetailData>> {
    const encoded = encodeURIComponent(id);
    return this.request('get', `/automations/tasks/${encoded}`, undefined, { params });
  }

  updateTask(
    id: string,
    data: UpdateAutomationTaskRequest
  ): Promise<ApiResponseData<AutomationTaskDetailData>> {
    const encoded = encodeURIComponent(id);
    return this.request('patch', `/automations/tasks/${encoded}`, data);
  }

  getTaskRuns(
    id: string,
    params?: GetAutomationTaskRunsParams
  ): Promise<ApiResponseData<AutomationTaskRunsList>> {
    const encoded = encodeURIComponent(id);
    return this.request('get', `/automations/tasks/${encoded}/runs`, undefined, { params });
  }

  runTask(
    id: string,
    params?: AutomationTaskActionParams
  ): Promise<ApiResponseData<AutomationManualRunData>> {
    const encoded = encodeURIComponent(id);
    return this.request('post', `/automations/tasks/${encoded}/run`, undefined, { params });
  }

  pauseTask(
    id: string,
    params?: AutomationTaskActionParams
  ): Promise<ApiResponseData<AutomationOperationResult>> {
    const encoded = encodeURIComponent(id);
    return this.request('post', `/automations/tasks/${encoded}/pause`, undefined, { params });
  }

  resumeTask(
    id: string,
    params?: AutomationTaskActionParams
  ): Promise<ApiResponseData<AutomationOperationResult>> {
    const encoded = encodeURIComponent(id);
    return this.request('post', `/automations/tasks/${encoded}/resume`, undefined, { params });
  }

  archiveTask(
    id: string,
    params?: AutomationTaskActionParams
  ): Promise<ApiResponseData<AutomationOperationResult>> {
    const encoded = encodeURIComponent(id);
    return this.request('post', `/automations/tasks/${encoded}/archive`, undefined, { params });
  }
}

export const automationService = new AutomationService();
export default automationService;
