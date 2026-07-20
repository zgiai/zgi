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
  expected_result?: string;
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

export interface WorkflowTestAnalysisSummary {
  status: 'passed' | 'failed' | 'review' | string;
  main_issue?: string;
  failed_stage?: string;
  reference_score?: number;
  total: number;
  passed: number;
  failed: number;
  review: number;
  critical_failed: number;
}

export interface WorkflowTestTraceNode {
  node_id: string;
  node_name: string;
  node_type: string;
  status: string;
  duration_ms: number;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
  error?: string;
}

export interface WorkflowTestCheckResult {
  id: string;
  type: string;
  label: string;
  status: 'passed' | 'failed' | 'review' | string;
  severity: 'critical' | 'normal' | 'hint' | string;
  evidence: string;
  suggestion?: string;
}

export interface WorkflowTestTurnAnalysis {
  turn_index: number;
  user_input: string;
  expected?: string;
  actual: string;
  status: 'passed' | 'failed' | 'review' | string;
  evidence: string;
  suggestion?: string;
}

export interface WorkflowTestAnalysis {
  mode: 'task' | 'conversation' | string;
  evaluation_schema?: WorkflowTestEvaluationSchema;
  trace: {
    nodes: WorkflowTestTraceNode[];
  };
  comparisons: {
    overall?: {
      status: 'passed' | 'failed' | 'review' | string;
      expected: string;
      actual: string;
      evidence: string;
      suggestion?: string;
    };
    checks: WorkflowTestCheckResult[];
    turns?: WorkflowTestTurnAnalysis[];
  };
  summary: WorkflowTestAnalysisSummary;
  suggestions: Array<{
    target: string;
    type: string;
    content: string;
  }>;
}

export interface WorkflowTestEvaluationAssertion {
  id?: string;
  type?: string;
  description?: string;
  values?: string[];
  operator?: string;
  severity?: string;
  match_mode?: string;
  source?: string;
}

export interface WorkflowTestEvaluationSchema {
  goal_type?: string;
  primary_objective?: string;
  assertions?: WorkflowTestEvaluationAssertion[];
  missing_policy?: {
    mode?: string;
    accept_markers?: string[];
    forbid_claims?: string[];
    clarify_allowed?: boolean;
  };
  allowed_extra_types?: string[];
  format?: {
    type?: string;
    fields?: string[];
    strict?: boolean;
    markdown?: boolean;
  };
  source_grounding?: string;
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

export interface DeleteWorkflowTestCasesRequest {
  case_ids: string[];
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
  case_mode?: 'task' | 'conversation';
  file_generation?: {
    enabled: boolean;
    formats?: string[];
    files_per_case?: number;
    complexities?: string[];
    content_types?: string[];
  };
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
    turns?: WorkflowTestTurn[];
    file_fixtures?: Array<Record<string, unknown>>;
  }>;
  items: WorkflowTestCase[];
}

export type WorkflowTestGenerationTaskStatus =
  | 'queued'
  | 'running'
  | 'canceling'
  | 'canceled'
  | 'completed'
  | 'failed';

export interface WorkflowTestGenerationTask {
  id: string;
  agent_id: string;
  workspace_id: string;
  account_id: string;
  status: WorkflowTestGenerationTaskStatus;
  requested_count: number;
  created_count: number;
  scenario_ids: string[];
  question_types: string[];
  turn_strategy: 'single' | 'multi' | 'mixed' | string;
  prompt: string;
  context: string;
  model_provider: string;
  model_name: string;
  error: string;
  started_at?: string;
  cancel_requested_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface WorkflowTestGenerationTaskResponse {
  task: WorkflowTestGenerationTask | null;
}

export type CreateWorkflowTestGenerationTaskRequest = GenerateWorkflowTestCasesRequest;

export interface RecognizeWorkflowTestScenariosRequest {
  context?: string;
  prompt?: string;
  case_mode?: 'task' | 'conversation';
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

export interface WorkflowTestScenarioRecognitionTask {
  id: string;
  agent_id: string;
  workspace_id: string;
  account_id: string;
  status: WorkflowTestGenerationTaskStatus;
  prompt: string;
  context: string;
  workflow_context_snapshot: string;
  model_provider: string;
  model_name: string;
  recognized_count: number;
  assigned_case_count: number;
  error: string;
  started_at?: string;
  cancel_requested_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface WorkflowTestScenarioRecognitionTaskResponse {
  task: WorkflowTestScenarioRecognitionTask | null;
}

export type CreateWorkflowTestScenarioRecognitionTaskRequest =
  RecognizeWorkflowTestScenariosRequest;
