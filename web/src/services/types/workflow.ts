// Workflow run related type definitions

import type { WorkflowEdge, WorkflowNode } from '@/components/workflow/store';
import type { Viewport } from '@xyflow/react';

export type WorkflowRunStatus =
  | 'queued'
  | 'running'
  | 'in_progress'
  | 'completed'
  | 'succeeded'
  | 'success'
  | 'failed'
  | 'error'
  | 'stopped'
  | 'paused'
  | 'pending_question'
  | 'partial-succeeded';

export interface WorkflowRunsQuery {
  page?: number;
  limit?: number;
  status?: 'succeeded' | 'failed' | 'stopped' | 'running';
  keyword?: string;
  triggered_from?: string;
}

// Minimal item for workflow run list
export interface WorkflowRunItem {
  id: string;
  sequence_number?: number;
  version?: string;
  status: WorkflowRunStatus | string; // allow unknown backend statuses gracefully
  created_at?: number; // unix seconds or milliseconds
  finished_at?: number | null; // unix seconds or milliseconds
  elapsed_time?: number; // milliseconds
  total_steps?: number;
  total_tokens?: number;
  conversation_id?: string;
  message_id?: string;
  created_by_account?: {
    id: string;
    name?: string;
    email?: string;
  } | null;
  exceptions_count?: number;
  retry_index?: number;
}

// Detailed step information for a run (best-effort, backend may omit fields)
export interface WorkflowRunStep {
  node_id: string;
  node_type?: string;
  title?: string;
  status: WorkflowRunStatus | 'completed' | 'failed' | 'running' | string;
  started_at?: number;
  finished_at?: number;
  elapsed_time?: number;
  error?: string | Record<string, unknown> | null;
  outputs?: unknown;
}

// Graph snapshot returned by backend for a run
export interface WorkflowRunGraph {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  viewport: Viewport;
}

export type WorkflowPrecheckStatus = 'ok' | 'warning' | 'unknown' | string;

export type WorkflowPrecheckWarningCode = '207008' | '207009' | '207010' | 207008 | 207009 | 207010;

export type WorkflowRunBillingErrorCode = '207011' | '207012' | '207013' | 207011 | 207012 | 207013;

export interface WorkflowRunBillingError {
  code?: string | number;
  message?: string;
  params?: Record<string, unknown>;
}

export interface WorkflowPrecheckWarning extends WorkflowRunBillingError {
  current_value?: number;
  threshold?: number;
}

export interface WorkflowPrecheckResult {
  status: WorkflowPrecheckStatus;
  warning?: WorkflowPrecheckWarning | null;
  warnings?: WorkflowPrecheckWarning[] | null;
  error?: WorkflowRunBillingError | null;
}

export interface WorkflowNodeRunRequest {
  inputs: Record<string, unknown>;
}

export interface WorkflowNodeRunResponse {
  task_id?: string;
  workflow_run_id?: string;
  status?: string;
  outputs?: unknown;
  error?: string;
}

// Full run detail response shape
export interface WorkflowRunDetail {
  id: string;
  sequence_number?: number;
  version?: string;
  status: WorkflowRunStatus | string;
  created_at?: number;
  finished_at?: number | null;
  elapsed_time?: number;
  total_steps?: number;
  total_tokens?: number;
  workflow_id?: string;
  features?: Record<string, unknown>;
  inputs?: Record<string, unknown>;
  outputs?: unknown;
  error?: string | Record<string, unknown> | null;
  steps?: WorkflowRunStep[];
  graph?: WorkflowRunGraph;
  conversation_id?: string;
  message_id?: string;
  created_by_role?: string;
  created_by_account?: {
    id: string;
    name?: string;
    email?: string;
  } | null;
  created_by_end_user?: {
    id?: string;
    name?: string;
    email?: string;
    [key: string]: unknown;
  } | null;
  exceptions_count?: number;
}

// Paginated list response for workflow runs
export interface WorkflowRunList {
  page?: number;
  limit: number;
  total?: number;
  has_more: boolean;
  data: WorkflowRunItem[];
}

// Detailed per-node execution record for a workflow run
export interface WorkflowNodeExecution {
  id: string;
  index: number;
  predecessor_node_id?: string | null;
  node_id: string;
  node_type: string;
  title: string;
  inputs: Record<string, unknown> | null;
  process_data: Record<string, unknown> | null;
  outputs: unknown | null;
  output_handle?: string;
  graph?: Record<string, unknown> | null;
  features?: Record<string, unknown> | null;
  status: WorkflowRunStatus | string;
  error: string | Record<string, unknown> | null;
  elapsed_time: number; // milliseconds
  execution_metadata: Record<string, unknown> | null;
  extras: Record<string, unknown>;
  created_at: number | string; // unix seconds or RFC3339 string
  created_by_role?: string;
  created_by_account?: {
    id: string;
    name?: string;
    email?: string;
    avatar?: string;
  } | null;
  created_by_end_user?: {
    id?: string;
    type?: string;
    [key: string]: unknown;
  } | null;
  finished_at: number | string | null; // unix seconds or RFC3339 string
  inputs_truncated?: boolean;
  outputs_truncated?: boolean;
  process_data_truncated?: boolean;
}

// Shared workflow SSE envelope shapes. Some events are nested in `data`,
// while message-style events are delivered as flat payloads.
export interface WorkflowSseEnvelopeBase<TEvent extends string> {
  event: TEvent;
  task_id?: string;
  workflow_run_id?: string;
}

export type WorkflowSseNestedEnvelope<
  TEvent extends string,
  TData,
> = WorkflowSseEnvelopeBase<TEvent> & {
  data: TData;
};

export type WorkflowSseFlatEnvelope<
  TEvent extends string,
  TPayload extends Record<string, unknown>,
> = WorkflowSseEnvelopeBase<TEvent> & TPayload;

export interface WorkflowStreamTokenUsage {
  prompt_tokens?: number;
  prompt_unit_price?: string | number;
  prompt_price_unit?: string | number;
  prompt_price?: string | number;
  completion_tokens?: number;
  completion_unit_price?: string | number;
  completion_price_unit?: string | number;
  completion_price?: string | number;
  total_tokens?: number;
  total_price?: string | number;
  currency?: string;
  latency?: number;
  PromptTokens?: number;
  PromptUnitPrice?: string | number;
  PromptPriceUnit?: string | number;
  PromptPrice?: string | number;
  CompletionTokens?: number;
  CompletionUnitPrice?: string | number;
  CompletionPriceUnit?: string | number;
  CompletionPrice?: string | number;
  TotalTokens?: number;
  TotalPrice?: string | number;
  Currency?: string;
  Latency?: number;
  [key: string]: unknown;
}

export interface WorkflowLlmGatewayMessage {
  role?: string;
  content?: unknown;
  [key: string]: unknown;
}

export interface WorkflowLlmGatewayRequest {
  messages?: WorkflowLlmGatewayMessage[];
  model?: string;
  params?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface WorkflowNodeProcessData {
  auto_injected_user_prompt?: boolean;
  final_prompt_contains_inline_data?: boolean;
  final_prompt_content_types?: string[];
  final_prompt_roles?: string[];
  finish_reason?: string;
  llm_gateway_request?: WorkflowLlmGatewayRequest;
  model_mode?: string;
  model_name?: string;
  model_provider?: string;
  resolved_file_count?: number;
  resolved_file_types?: string[];
  resolved_file_urls_present?: boolean[];
  resolved_model_name?: string;
  resolved_model_provider?: string;
  resolved_model_source?: string;
  selected_file_transport?: string;
  selected_file_url_host?: string;
  selected_file_url_is_public?: boolean;
  selected_file_url_scheme?: string;
  usage?: WorkflowStreamTokenUsage;
  vision_enabled?: boolean;
  vision_selector?: string[];
  [key: string]: unknown;
}

export interface WorkflowNodeExecutionMetadata {
  total_tokens?: number;
  total_price?: string | number;
  currency?: string;
  resolved_model_name?: string;
  resolved_model_provider?: string;
  resolved_model_source?: string;
  iteration_id?: string;
  iteration_index?: number;
  [key: string]: unknown;
}

export interface WorkflowStartedSseData {
  id: string; // workflow_run_id
  workflow_id?: string;
  created_at: number;
  inputs?: Record<string, unknown>;
  sequence_number?: number;
  [key: string]: unknown;
}

export interface WorkflowNodeStartedSseData {
  id: string; // node execution id
  index: number;
  predecessor_node_id?: string | null;
  node_id: string;
  node_type: string;
  title: string;
  inputs?: Record<string, unknown>;
  inputs_truncated?: boolean;
  created_at: number;
  iteration_index?: number | null;
  iteration_id?: string | null;
  loop_id?: string | null;
  loop_index?: number | null;
  parallel_id?: string | null;
  parent_parallel_id?: string | null;
  [key: string]: unknown;
}

export interface WorkflowNodeFinishedSseData {
  id: string; // node execution id
  index: number;
  predecessor_node_id?: string | null;
  node_id: string;
  node_type: string;
  title: string;
  status: WorkflowRunStatus | string;
  error?: string | Record<string, unknown> | null;
  elapsed_time?: number; // backend duration value; current node SSE payloads use milliseconds
  inputs?: Record<string, unknown>;
  inputs_truncated?: boolean;
  process_data?: WorkflowNodeProcessData | Record<string, unknown> | null;
  process_data_truncated?: boolean;
  outputs?: unknown;
  outputs_truncated?: boolean;
  output_handle?: string;
  execution_metadata?: WorkflowNodeExecutionMetadata;
  files?: unknown[];
  created_at: number;
  finished_at?: number;
  iteration_index?: number | null;
  iteration_id?: string | null;
  loop_id?: string | null;
  loop_index?: number | null;
  parallel_id?: string | null;
  parallel_start_node_id?: string | null;
  parent_parallel_id?: string | null;
  parent_parallel_start_node_id?: string | null;
  [key: string]: unknown;
}

export interface WorkflowMessageSseData extends Record<string, unknown> {
  event?: 'message';
  answer?: string;
  text?: string;
  delta?: string;
  content?: string;
  created_at: number;
  id?: string;
  task_id?: string;
  workflow_run_id?: string;
  conversation_id?: string;
  message_id?: string;
  from_variable_selector?: string[];
}

export interface WorkflowFinishedSseData {
  id: string; // workflow_run_id
  workflow_id?: string;
  status: WorkflowRunStatus | string;
  created_at: number;
  finished_at: number;
  elapsed_time: number; // backend duration value; current workflow SSE payloads use milliseconds
  total_steps?: number | string;
  total_tokens?: number;
  token_usage?: WorkflowStreamTokenUsage;
  exceptions_count?: number;
  files?: unknown[];
  outputs?: unknown;
  error?: string | Record<string, unknown> | null;
  [key: string]: unknown;
}

export interface WorkflowPausedSseData {
  status: 'paused' | string;
  node_id: string;
  node_type: string;
  title?: string;
  outputs?: Record<string, unknown>;
  approval_form?: {
    id: string;
    token: string;
    [key: string]: unknown;
  };
  [key: string]: unknown;
}

export interface ApprovalRequestedSseData {
  form_id: string;
  workflow_run_id: string;
  node_id: string;
  node_title: string;
  content: string;
  fields: unknown[];
  actions: unknown[];
  submit_methods?: Record<string, unknown>;
  token: string;
  expires_at?: number;
  [key: string]: unknown;
}

export interface ApprovalResultFilledSseData {
  form_id: string;
  workflow_run_id: string;
  node_id: string;
  node_title: string;
  action_id: string;
  action_label?: string;
  inputs?: Record<string, unknown>;
  rendered_content?: string;
  [key: string]: unknown;
}

export interface ApprovalExpiredSseData {
  form_id: string;
  workflow_run_id: string;
  node_id: string;
  node_title?: string;
  expires_at?: number;
  [key: string]: unknown;
}

export interface QuestionAnswerChoice {
  id: string;
  label?: string;
  value?: string;
  [key: string]: unknown;
}

export interface QuestionAnswerRequestedSseData {
  workflow_run_id: string;
  node_id: string;
  node_title?: string;
  question: string;
  choices?: QuestionAnswerChoice[];
  round?: number;
  created_at?: number;
  [key: string]: unknown;
}

export interface QuestionAnswerSubmittedSseData {
  workflow_run_id: string;
  node_id?: string;
  node_title?: string;
  answer: string;
  created_at?: number;
  [key: string]: unknown;
}

export interface WorkflowMessageEndSseData extends Record<string, unknown> {
  event?: 'message_end';
  conversation_id?: string;
  message_id?: string;
  created_at: number;
  id?: string;
  task_id?: string;
  workflow_run_id?: string;
  metadata?: Record<string, unknown>;
  files?: unknown[];
}

export type WorkflowStartedSseEnvelope = WorkflowSseNestedEnvelope<
  'workflow_started',
  WorkflowStartedSseData
>;

export type WorkflowNodeStartedSseEnvelope = WorkflowSseNestedEnvelope<
  'node_started',
  WorkflowNodeStartedSseData
>;

export type WorkflowNodeFinishedSseEnvelope = WorkflowSseNestedEnvelope<
  'node_finished',
  WorkflowNodeFinishedSseData
>;

export type WorkflowFinishedSseEnvelope = WorkflowSseNestedEnvelope<
  'workflow_finished',
  WorkflowFinishedSseData
>;

export type WorkflowPausedSseEnvelope = WorkflowSseNestedEnvelope<
  'workflow_paused',
  WorkflowPausedSseData
>;

export type ApprovalRequestedSseEnvelope = WorkflowSseNestedEnvelope<
  'approval_requested',
  ApprovalRequestedSseData
>;

export type ApprovalResultFilledSseEnvelope = WorkflowSseNestedEnvelope<
  'approval_result_filled',
  ApprovalResultFilledSseData
>;

export type ApprovalExpiredSseEnvelope = WorkflowSseNestedEnvelope<
  'approval_expired',
  ApprovalExpiredSseData
>;

export type QuestionAnswerRequestedSseEnvelope = WorkflowSseNestedEnvelope<
  'question_answer_requested',
  QuestionAnswerRequestedSseData
>;

export type QuestionAnswerSubmittedSseEnvelope = WorkflowSseNestedEnvelope<
  'question_answer_submitted',
  QuestionAnswerSubmittedSseData
>;

export type WorkflowMessageSseEnvelope = WorkflowSseFlatEnvelope<'message', WorkflowMessageSseData>;

export type WorkflowMessageEndSseEnvelope = WorkflowSseFlatEnvelope<
  'message_end',
  WorkflowMessageEndSseData
>;
export interface WorkflowChatMessagesQuery {
  conversation_id: string;
  page?: number;
  limit?: number;
}

export interface WorkflowChatMessageItem {
  id: string;
  conversation_id: string;
  query: string;
  answer: string;
  created_at: number;
  workflow_run_id?: string;
  parent_message_id?: string;
  invoke_from?: string;
  inputs?: Record<string, unknown>;
}

export interface WorkflowChatMessagesList {
  data: WorkflowChatMessageItem[];
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
}
// Iteration SSE event payloads
export interface IterationStartedData {
  created_at: number;
  extras: Record<string, unknown>;
  id: string;
  inputs: { [key: string]: unknown };
  inputs_truncated?: boolean;
  metadata: { iteration_length: number };
  node_id: string;
  node_type: 'iteration' | string;
  title: string;
}

export interface IterationNextData {
  created_at: number;
  extras: Record<string, unknown>;
  id: string;
  index: number;
  node_id: string;
  node_type: 'iteration' | string;
  title: string;
}

export interface IterationCompletedData {
  created_at: number;
  finished_at: number;
  elapsed_time: number; // backend duration value; current iteration payloads use milliseconds
  error: string | null;
  execution_metadata: {
    iteration_duration_map: Record<string, number>;
    total_tokens?: number;
    [key: string]: unknown;
  };
  extras: Record<string, unknown>;
  id: string;
  inputs: { [key: string]: unknown };
  inputs_truncated?: boolean;
  node_id: string;
  node_type: 'iteration' | string;
  outputs: { output: unknown[] };
  outputs_truncated?: boolean;
  status: WorkflowRunStatus | 'succeeded' | 'failed' | string;
  steps: number;
  title: string;
  total_tokens?: number;
}

export interface LoopStartedData {
  created_at: number;
  created_at_ms?: number;
  extras: Record<string, unknown>;
  id: string;
  inputs: {
    loop_count?: number;
    loop_variables?: Record<string, unknown>;
    [key: string]: unknown;
  };
  inputs_truncated?: boolean;
  metadata: { loop_length?: number; [key: string]: unknown };
  node_id: string;
  node_type: 'loop' | string;
  title: string;
}

export interface LoopNextData {
  created_at: number;
  created_at_ms?: number;
  extras: Record<string, unknown>;
  id: string;
  index: number;
  node_id: string;
  node_type: 'loop' | string;
  title: string;
}

export interface LoopCompletedData {
  created_at: number;
  created_at_ms?: number;
  finished_at: number;
  finished_at_ms?: number;
  elapsed_time: number; // backend duration value; current loop payloads use milliseconds
  error: string | null;
  execution_metadata: {
    loop_duration_map?: Record<string, number>;
    loop_variable_map?: Record<string, Record<string, unknown>>;
    total_tokens?: number;
    [key: string]: unknown;
  };
  extras: Record<string, unknown>;
  id: string;
  inputs: { [key: string]: unknown };
  inputs_truncated?: boolean;
  node_id: string;
  node_type: 'loop' | string;
  outputs: {
    loop_round?: number;
    [key: string]: unknown;
  };
  outputs_truncated?: boolean;
  status: WorkflowRunStatus | 'succeeded' | 'failed' | string;
  steps?: number;
  title: string;
  total_tokens?: number;
}

export type WorkflowExportVersion = 'draft' | 'published';

export type WorkflowImportWarningType =
  | 'unsupported_node'
  | 'datasource_requires_config'
  | 'knowledge_base_requires_config'
  | string;

export interface WorkflowImportWarning {
  type: WorkflowImportWarningType;
  node_id?: string;
  name?: string;
  message: string;
}

export interface WorkflowImportStats {
  node_count: number;
  edge_count: number;
  node_types: string[];
  variable_count: number;
}

export interface WorkflowImportResult {
  success: boolean;
  agent_id: string;
  workflow_id: string;
  stats: WorkflowImportStats;
  warnings: WorkflowImportWarning[] | null;
}

// Latest workflow version info returned by
// GET /console/api/agents/{agentId}/workflows/latest-version
// Note: workflow_id is only present for published agents
export interface WorkflowLatestVersion {
  web_app_id: string;
  workflow_id?: string;
  version_uuid?: string;
}

export interface WorkflowPublishedVersion {
  workflow_id: string;
  version_uuid: string;
  version: string;
  created_at?: string;
}

export interface WorkflowPublishedVersionsResponse {
  data: WorkflowPublishedVersion[];
  total: number;
}

// Result returned by POST /console/api/agents/{agentId}/workflows/publish
export interface PublishWorkflowResult {
  created_at: string;
  version_uuid?: string;
  version: string;
  workflow_id: string;
}

// Built-in workflow scenario types
export type BuiltInWorkflowScenario = 'bi_chat' | 'global_chat' | string;

// Built-in workflow agent types
export type BuiltInWorkflowAgentType = 'CONVERSATIONAL_WORKFLOW' | 'WORKFLOW' | string;

// Built-in workflow icon types
export type BuiltInWorkflowIconType = 'text' | 'image' | string;

// Built-in workflow item returned by GET /console/api/built-in-workflows
export interface BuiltInWorkflow {
  scenario: BuiltInWorkflowScenario;
  agent_id: string;
  agent_name: string;
  workflow_id: string;
  web_app_id: string;
  description: string;
  agent_type: BuiltInWorkflowAgentType;
  icon: string;
  icon_type: BuiltInWorkflowIconType;
}

// Built-in workflows list
export type BuiltInWorkflowList = BuiltInWorkflow[];

export type PublishedRuntimeSurface =
  | 'webapp'
  | 'api'
  | 'app_center'
  | 'builtin_app'
  | 'internal'
  | string;

export type PublishedRuntimeGrantSubject =
  | 'public'
  | 'organization'
  | 'department'
  | 'workspace'
  | 'account'
  | 'internal'
  | string;

export interface PublishedRuntimeSurfaceGrant {
  subject_type: PublishedRuntimeGrantSubject;
  subject_id: string | null;
  enabled: boolean;
}

export interface PublishedRuntimeSurfaceAuthorization {
  surface: PublishedRuntimeSurface;
  enabled: boolean;
  compatibility_source: string;
  grants: PublishedRuntimeSurfaceGrant[];
}

export interface UpdatePublishedRuntimeSurfaceGrant {
  subject_type: PublishedRuntimeGrantSubject;
  subject_id?: string | null;
  enabled?: boolean;
}

export interface UpdatePublishedRuntimeSurfaceAuthorization {
  surface: PublishedRuntimeSurface;
  enabled: boolean;
  grants?: UpdatePublishedRuntimeSurfaceGrant[];
}

export interface UpdatePublishedRuntimeSurfacesRequest {
  surfaces: UpdatePublishedRuntimeSurfaceAuthorization[];
}

export interface BuiltInWorkflowRuntimeSurfaceAuthorizationResponse {
  scenario: BuiltInWorkflowScenario;
  agent_id: string;
  organization_id: string;
  surfaces: PublishedRuntimeSurfaceAuthorization[];
}

// Stop workflow task response
// POST /console/api/agents/{agent_id}/workflow-runs/tasks/{workflow_run_id}/stop
export interface StopWorkflowTaskResponse {
  result: string;
}
