import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  CreateWorkflowTestBatchRequest,
  CreateWorkflowTestCaseRequest,
  CreateWorkflowTestScenarioRequest,
  DeleteWorkflowTestCasesRequest,
  GenerateWorkflowTestCasesRequest,
  GenerateWorkflowTestCasesResponse,
  RecognizeWorkflowTestScenariosRequest,
  RecognizeWorkflowTestScenariosResponse,
  RetestWorkflowTestBatchRequest,
  SaveWorkflowTestScenariosRequest,
  UpdateWorkflowTestCaseRequest,
  UpdateWorkflowTestSettingsRequest,
  WorkflowTestBatch,
  WorkflowTestBatchItem,
  WorkflowTestCase,
  WorkflowTestListResponse,
  WorkflowTestScenario,
  WorkflowTestSettings,
} from './types/workflow-test';

const WORKFLOW_TEST_LLM_TIMEOUT_MS = 120000;

class WorkflowTestService extends BaseService {
  constructor() {
    super({
      basePath: '/console/api',
      endpoint: 'main',
    });
  }

  getSettings(agentId: string): Promise<ApiResponseData<WorkflowTestSettings>> {
    return this.request('get', `/agents/${agentId}/workflow-tests/settings`);
  }

  updateSettings(
    agentId: string,
    data: UpdateWorkflowTestSettingsRequest
  ): Promise<ApiResponseData<WorkflowTestSettings>> {
    return this.request('put', `/agents/${agentId}/workflow-tests/settings`, data);
  }

  resetJudgePrompt(agentId: string): Promise<ApiResponseData<WorkflowTestSettings>> {
    return this.request('post', `/agents/${agentId}/workflow-tests/settings/reset-judge-prompt`);
  }

  listScenarios(
    agentId: string
  ): Promise<ApiResponseData<WorkflowTestListResponse<WorkflowTestScenario>>> {
    return this.request('get', `/agents/${agentId}/workflow-tests/scenarios`);
  }

  createScenario(
    agentId: string,
    data: CreateWorkflowTestScenarioRequest
  ): Promise<ApiResponseData<WorkflowTestScenario>> {
    return this.request('post', `/agents/${agentId}/workflow-tests/scenarios`, data);
  }

  saveScenarios(
    agentId: string,
    data: SaveWorkflowTestScenariosRequest
  ): Promise<ApiResponseData<WorkflowTestListResponse<WorkflowTestScenario>>> {
    return this.request('put', `/agents/${agentId}/workflow-tests/scenarios`, data);
  }

  recognizeScenarios(
    agentId: string,
    data: RecognizeWorkflowTestScenariosRequest
  ): Promise<ApiResponseData<RecognizeWorkflowTestScenariosResponse>> {
    return this.request('post', `/agents/${agentId}/workflow-tests/scenarios/recognize`, data, {
      timeout: WORKFLOW_TEST_LLM_TIMEOUT_MS,
    });
  }

  listCases(
    agentId: string,
    params?: { status?: string }
  ): Promise<ApiResponseData<WorkflowTestListResponse<WorkflowTestCase>>> {
    return this.request('get', `/agents/${agentId}/workflow-tests/cases`, undefined, { params });
  }

  createCase(
    agentId: string,
    data: CreateWorkflowTestCaseRequest
  ): Promise<ApiResponseData<WorkflowTestCase>> {
    return this.request('post', `/agents/${agentId}/workflow-tests/cases`, data);
  }

  updateCase(
    agentId: string,
    caseId: string,
    data: UpdateWorkflowTestCaseRequest
  ): Promise<ApiResponseData<WorkflowTestCase>> {
    return this.request('put', `/agents/${agentId}/workflow-tests/cases/${caseId}`, data);
  }

  deleteCase(agentId: string, caseId: string): Promise<ApiResponseData<{ deleted: number }>> {
    return this.request('delete', `/agents/${agentId}/workflow-tests/cases/${caseId}`);
  }

  deleteCases(
    agentId: string,
    data: DeleteWorkflowTestCasesRequest
  ): Promise<ApiResponseData<{ deleted: number }>> {
    return this.request('delete', `/agents/${agentId}/workflow-tests/cases`, undefined, { data });
  }

  generateCases(
    agentId: string,
    data: GenerateWorkflowTestCasesRequest
  ): Promise<ApiResponseData<GenerateWorkflowTestCasesResponse>> {
    return this.request('post', `/agents/${agentId}/workflow-tests/cases/generate`, data, {
      timeout: WORKFLOW_TEST_LLM_TIMEOUT_MS,
    });
  }

  listBatches(
    agentId: string
  ): Promise<ApiResponseData<WorkflowTestListResponse<WorkflowTestBatch>>> {
    return this.request('get', `/agents/${agentId}/workflow-tests/batches`);
  }

  createBatch(
    agentId: string,
    data: CreateWorkflowTestBatchRequest
  ): Promise<ApiResponseData<WorkflowTestBatch>> {
    return this.request('post', `/agents/${agentId}/workflow-tests/batches`, data);
  }

  listBatchItems(
    agentId: string,
    batchId: string
  ): Promise<ApiResponseData<WorkflowTestListResponse<WorkflowTestBatchItem>>> {
    return this.request('get', `/agents/${agentId}/workflow-tests/batches/${batchId}/items`);
  }

  startBatch(agentId: string, batchId: string): Promise<ApiResponseData<WorkflowTestBatch>> {
    return this.request('post', `/agents/${agentId}/workflow-tests/batches/${batchId}/start`);
  }

  executeBatch(agentId: string, batchId: string): Promise<ApiResponseData<WorkflowTestBatch>> {
    return this.request('post', `/agents/${agentId}/workflow-tests/batches/${batchId}/execute`);
  }

  cancelBatch(agentId: string, batchId: string): Promise<ApiResponseData<WorkflowTestBatch>> {
    return this.request('post', `/agents/${agentId}/workflow-tests/batches/${batchId}/cancel`);
  }

  retestBatch(
    agentId: string,
    batchId: string,
    data?: RetestWorkflowTestBatchRequest
  ): Promise<ApiResponseData<WorkflowTestBatch>> {
    return this.request(
      'post',
      `/agents/${agentId}/workflow-tests/batches/${batchId}/retest`,
      data
    );
  }
}

export const workflowTestService = new WorkflowTestService();
export default workflowTestService;
