export interface WorkflowTestSettings {
  id: string;
  agent_id: string;
  judge_prompt_template: string;
  judge_model_provider: string;
  judge_model_name: string;
  created_at: string;
  updated_at: string;
}

export interface WorkflowTestScenario {
  id: string;
  agent_id: string;
  name: string;
  description: string;
  source: string;
  case_count: number;
  created_at: string;
  updated_at: string;
}

export interface WorkflowTestAttachment {
  type: string;
  transfer_method: string;
  url?: string;
  upload_file_id?: string;
  name?: string;
}

export interface WorkflowTestTurn {
  role: string;
  content: string;
  attachments?: WorkflowTestAttachment[];
  inputs?: Record<string, unknown>;
}

export interface WorkflowTestCase {
  id: string;
  agent_id: string;
  scenario_id?: string;
  content: string;
  expected_result: string;
  question_type: string;
  status: string;
  turns: WorkflowTestTurn[];
  created_at: string;
  updated_at: string;
}

export interface WorkflowTestBatch {
  id: string;
  agent_id: string;
  name: string;
  status: string;
  case_count: number;
  passed_count: number;
  failed_count: number;
  review_count: number;
  judge_prompt_snapshot: string;
  judge_model_provider_snapshot: string;
  judge_model_name_snapshot: string;
  workflow_version_mode: string;
  workflow_version_uuid?: string;
  workflow_version_label: string;
  summary: string;
  created_at: string;
  updated_at: string;
}

export interface WorkflowTestCaseSnapshot {
  id: string;
  scenario_id?: string;
  content: string;
  expected_result: string;
  question_type: string;
  turns: WorkflowTestTurn[];
}

export interface WorkflowTestBatchItem {
  id: string;
  agent_id: string;
  batch_id: string;
  case_id: string;
  case_snapshot: WorkflowTestCaseSnapshot;
  status: string;
  workflow_run_id: string;
  outputs: Record<string, unknown>;
  error: string;
  judge_reason: string;
  judge_suggestion: string;
  judge_confidence: number;
  created_at: string;
  updated_at: string;
}

export interface WorkflowTestListResponse<T> {
  items: T[];
}

export interface UpdateWorkflowTestSettingsRequest {
  judge_prompt_template: string;
  judge_model_provider?: string;
  judge_model_name?: string;
}

export interface CreateWorkflowTestScenarioRequest {
  name: string;
  description?: string;
}

export interface SaveWorkflowTestScenarioItem {
  id?: string;
  name: string;
  description?: string;
}

export interface SaveWorkflowTestScenariosRequest {
  scenarios: SaveWorkflowTestScenarioItem[];
}

export interface CreateWorkflowTestCaseRequest {
  content: string;
  expected_result?: string;
  scenario_id?: string;
  question_type?: string;
  status?: string;
  turns?: WorkflowTestTurn[];
}

export interface UpdateWorkflowTestCaseRequest {
  content: string;
  expected_result?: string;
  scenario_id?: string;
  question_type?: string;
  status?: string;
  turns?: WorkflowTestTurn[];
}

export interface CreateWorkflowTestBatchRequest {
  name: string;
  case_ids?: string[];
  workflow_version_mode?: string;
  workflow_version_uuid?: string;
}

export interface RetestWorkflowTestBatchRequest {
  name?: string;
}

export interface GenerateWorkflowTestCasesRequest {
  count: number;
  scenario_ids?: string[];
  scenario_id?: string;
  context?: string;
  question_types?: string[];
  turn_strategy?: 'single' | 'multi' | 'mixed';
  prompt?: string;
  model?: {
    provider: string;
    name: string;
  };
}

export interface GenerateWorkflowTestCasesResponse {
  cases: Array<{
    content: string;
    expected_result: string;
    question_type: string;
  }>;
  items: WorkflowTestCase[];
}

export interface RecognizeWorkflowTestScenariosRequest {
  context?: string;
  prompt?: string;
  model?: {
    provider: string;
    name: string;
  };
}

export interface RecognizeWorkflowTestScenariosResponse {
  scenarios: Array<{
    name: string;
    description: string;
  }>;
  assignments: Array<{
    case_id: string;
    scenario_name: string;
  }>;
  cases: WorkflowTestCase[];
}
