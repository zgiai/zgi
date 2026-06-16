import type { ApiResponseData } from './common';

export type AIChatConversationStatus = 'normal' | 'archived';
export type AIChatConversationSource = 'console' | 'webapp' | 'migration';
export type AIChatConversationRuntimeStatus = 'idle' | 'streaming';
export type AIChatMessageStatus =
  | 'pending'
  | 'streaming'
  | 'waiting_approval'
  | 'waiting_question'
  | 'completed'
  | 'error'
  | 'stopped';

export interface AIChatUsage {
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
}

export type AIChatSkillMode = 'disabled' | 'auto' | 'required';
export type AIChatSkillSource = 'system' | 'custom';
export type AIChatSkillRuntimeType = 'tool' | 'prompt' | 'hybrid';
export type AIChatSkillStatus = 'active' | 'invalid';
export type AIChatSkillActivityStatus =
  | 'loading'
  | 'loaded'
  | 'running'
  | 'allowed'
  | 'needs_approval'
  | 'needs_resolution'
  | 'denied'
  | 'success'
  | 'blocked'
  | 'error';
export type AIChatSkillInvocationKind =
  | 'metadata_exposed'
  | 'skill_load'
  | 'reference_read'
  | 'tool_call'
  | 'tool_governance'
  | 'intermediate_answer'
  | 'user_input_request'
  | 'memory_planner';

export type AIChatMemoryMutationAction = 'create' | 'update' | 'delete' | 'clear';

export interface AIChatConversationMetadata {
  [key: string]: unknown;
}

export interface AIChatSkillDisplayMetadata {
  icon?: string;
  category?: string;
  label?: Record<string, string>;
  description?: Record<string, string>;
  when_to_use?: Record<string, string>;
  tags?: Record<string, string[]>;
}

export interface AIChatSkillMetadata {
  skill_id: string;
  source?: AIChatSkillSource;
  name: string;
  description: string;
  when_to_use: string;
  runtime_type: AIChatSkillRuntimeType;
  enabled: boolean;
  display?: AIChatSkillDisplayMetadata;
  has_tools: boolean;
  has_references: boolean;
  has_scripts: boolean;
  scripts_supported: boolean;
  max_calls_per_turn: number;
  timeout_seconds: number;
  status?: AIChatSkillStatus;
  validation_error?: string;
  supported_callers?: Array<'aichat' | 'agent'>;
  required_config?: string[];
}

export type AIChatSkillListResponse = ApiResponseData<AIChatSkillMetadata[]>;
export type AIChatSkillDetailResponse = ApiResponseData<AIChatSkillMetadata>;

export interface AIChatSkillOrganizationConfig {
  enabled_skill_ids: string[];
}

export type AIChatSkillConfigResponse = ApiResponseData<AIChatSkillOrganizationConfig>;

export interface AIChatSkillPreference {
  enabled_skill_ids: string[];
  defaulted?: boolean;
}

export type AIChatSkillPreferenceResponse = ApiResponseData<AIChatSkillPreference>;

export interface AIChatDeleteSkillResponseData {
  deleted: boolean;
}

export interface AIChatCancelImportSkillPreviewResponseData {
  canceled: boolean;
}

export type AIChatDeleteSkillResponse = ApiResponseData<AIChatDeleteSkillResponseData>;
export type AIChatCancelImportSkillPreviewResponse =
  ApiResponseData<AIChatCancelImportSkillPreviewResponseData>;

export interface AIChatImportSkillPreviewFile {
  path: string;
  size: number;
}

export interface AIChatImportSkillPreview {
  import_id?: string;
  expires_at?: number;
  skill?: AIChatSkillMetadata;
  will_overwrite: boolean;
  existing_skill?: AIChatExistingSkill;
  file_count: number;
  total_size: number;
  files: AIChatImportSkillPreviewFile[];
  references: string[];
  has_scripts: boolean;
  scripts_supported: boolean;
  warnings: string[];
  validation_errors: string[];
  can_import: boolean;
}

export interface AIChatConfirmImportSkillRequest {
  import_id: string;
  overwrite_confirmed?: boolean;
}

export type AIChatImportSkillPreviewResponse = ApiResponseData<AIChatImportSkillPreview>;

export interface AIChatExistingSkill {
  skill_id: string;
  name: string;
  updated_at?: number;
}

export interface AIChatSkillInvocation {
  kind?: AIChatSkillInvocationKind;
  runtime_id?: string;
  answer_id?: string;
  skill_id: string;
  tool_name?: string;
  title?: string;
  status: AIChatSkillActivityStatus;
  duration_ms?: number;
  arguments?: Record<string, unknown> | null;
  result?: Record<string, unknown> | null;
  path?: string;
  message?: string;
  error?: string;
  governance?: AIChatToolGovernanceDecision | null;
  asset_operation_audit?: AIChatAssetOperationAudit;
  created_at?: number;
}

export interface AIChatWorkflowRunNodeMetadata {
  node_id?: string;
  execution_id?: string;
  id?: string;
  node_type?: string;
  type?: string;
  title?: string;
  node_title?: string;
  name?: string;
  label?: string;
  status?: string;
  inputs?: unknown;
  outputs?: unknown;
  elapsed_time?: number;
  error?: string;
  created_at?: number;
  iteration_inputs?: unknown;
  iteration_outputs?: unknown;
  iteration_rounds?: Array<{
    index?: number;
    nodes?: AIChatWorkflowRunNodeMetadata[];
    elapsed_time?: number;
  }>;
  loop_inputs?: unknown;
  loop_outputs?: unknown;
  loop_rounds?: Array<{
    index?: number;
    nodes?: AIChatWorkflowRunNodeMetadata[];
    elapsed_time?: number;
    variables?: unknown;
  }>;
  steps?: number;
}

export interface AIChatWorkflowRunApprovalMetadata extends Record<string, unknown> {
  approval_form_id?: string;
  approval_token?: string;
  approval_url?: string;
  approval_form?: unknown;
  status?: string;
}

export interface AIChatWorkflowRunMetadata {
  workflow_run_id?: string;
  task_id?: string;
  id?: string;
  workflow_id?: string;
  agent_id?: string;
  version?: string;
  status?: string;
  inputs?: unknown;
  outputs?: unknown;
  elapsed_time?: number;
  error?: string;
  approval?: AIChatWorkflowRunApprovalMetadata;
  approval_result?: Record<string, unknown>;
  approval_expired?: Record<string, unknown>;
  nodes?: AIChatWorkflowRunNodeMetadata[];
  created_at?: number;
}

export type AIChatFileContentStatus =
  | 'pending'
  | 'extracted'
  | 'empty'
  | 'vision_ready'
  | 'filtered';
export type AIChatFileParseStatus = 'pending' | 'parsing' | 'completed' | 'error';
export type AIChatMessageFileKind = 'document' | 'image';
export type AIChatVisionDetail = 'high';
export type AIChatFilteredReason = 'model_without_vision';

export interface AIChatMessageFile {
  id: string;
  name: string;
  size: number;
  extension: string;
  mime_type: string;
  workspace_id?: string | null;
  is_temporary: boolean;
  content_status: AIChatFileContentStatus;
  content_chars: number;
  content_preview: string;
  from_cache: boolean;
  kind?: AIChatMessageFileKind;
  vision_detail?: AIChatVisionDetail | null;
  filtered_reason?: AIChatFilteredReason | null;
  parse_status?: AIChatFileParseStatus;
  error?: string;
}

export interface AIChatGeneratedFile {
  artifact_type: 'file';
  skill_id: string;
  tool_name: string;
  file_id: string;
  filename: string;
  extension: string;
  mime_type: string;
  size: number;
  url: string;
  download_url?: string;
  transfer_method: string;
  file_type?: string;
  created_at: number;
}

export interface AIChatMessageMetadata {
  usage?: AIChatUsage;
  system_prompt_version?: string;
  trace_id?: string;
  has_trace?: boolean;
  skill_mode?: AIChatSkillMode;
  configured_skill_ids?: string[];
  selected_skill_ids?: string[];
  loaded_skill_ids?: string[];
  skill_step_count?: number;
  skill_call_count?: number;
  skill_names?: string[];
  tool_call_count?: number;
  tool_names?: string[];
  skill_invocations?: AIChatSkillInvocation[];
  workflow_run_count?: number;
  workflow_runs?: AIChatWorkflowRunMetadata[];
  file_count?: number;
  files?: AIChatMessageFile[];
  generated_file_count?: number;
  generated_files?: AIChatGeneratedFile[];
  user_input_request?: AIChatUserInputRequest;
  context_control?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface AIChatUserInputOption {
  label: string;
  value?: string;
  option_id?: string;
  description?: string;
}

export interface AIChatUserInputQuestion {
  id?: string;
  question: string;
  options?: AIChatUserInputOption[];
}

export interface AIChatUserInputRequest {
  request_id?: string;
  source?: string;
  workflow_run_id?: string;
  node_id?: string;
  conversation_id?: string;
  message_id?: string;
  round?: string | number;
  questions: AIChatUserInputQuestion[];
  created_at?: number;
}

export interface AIChatConversation {
  id: string;
  organization_id: string;
  workspace_id?: string;
  account_id: string;
  title: string;
  status: AIChatConversationStatus;
  runtime_status: AIChatConversationRuntimeStatus;
  active_message_id?: string;
  current_leaf_message_id?: string;
  dialogue_count: number;
  source: AIChatConversationSource;
  source_conversation_id?: string;
  source_web_app_id?: string;
  metadata?: AIChatConversationMetadata;
  created_at: number;
  updated_at: number;
}

export interface AIChatMessage {
  id: string;
  conversation_id: string;
  parent_id?: string;
  query: string;
  answer: string;
  status: AIChatMessageStatus;
  error?: string;
  model_provider?: string;
  model_name: string;
  billing_reason_source?: 'aichat' | 'agent';
  model_parameters?: Record<string, unknown>;
  metadata?: AIChatMessageMetadata;
  source_message_id?: string;
  created_at: number;
  updated_at: number;
}

export interface AIChatPageData<T> {
  data: T[];
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
}

export type AIChatConversationListResponse = ApiResponseData<AIChatPageData<AIChatConversation>>;
export type AIChatMessageListResponse = ApiResponseData<AIChatPageData<AIChatMessage>>;
export type AIChatAssetOperationAuditListResponse = ApiResponseData<
  AIChatPageData<AIChatAssetOperationAuditRecord>
>;

export interface AIChatCreateConversationRequest {
  title: string;
}

export interface AIChatUpdateConversationRequest {
  title?: string;
  status?: AIChatConversationStatus;
  current_leaf_message_id?: string;
}

export interface AIChatModelParameters {
  temperature?: number;
  top_p?: number;
  max_tokens?: number;
  presence_penalty?: number;
  frequency_penalty?: number;
  stop?: string[];
  seed?: number;
  [key: string]: number | string | boolean | string[] | undefined;
}

export interface AIChatChatRequest {
  conversation_id?: string;
  parent_id?: string;
  query: string;
  runtime_context?: string;
  operation_context?: unknown;
  model: string;
  provider?: string;
  file_ids?: string[];
  response_mode: 'streaming';
  parameters?: AIChatModelParameters;
  use_memory?: boolean;
}

export interface AIChatRegenerateMessageRequest {
  query?: string;
  runtime_context?: string;
  operation_context?: unknown;
  model?: string;
  provider?: string;
  parameters?: AIChatModelParameters;
}

export interface AIChatMessageStartEventData {
  conversation_id: string;
  message_id: string;
  parent_id?: string | null;
  title?: string;
  model?: string;
  replace?: boolean;
  created_at?: number;
}

export interface AIChatMessageChunkEventData {
  conversation_id: string;
  message_id: string;
  answer?: string;
  __sensitiveOutputBlocked?: boolean;
}

export interface AIChatMessageRetractEventData {
  conversation_id: string;
  message_id: string;
  content?: string;
  length?: number;
  created_at?: number;
}

export interface AIChatMessageEndEventData {
  conversation_id: string;
  message_id: string;
  status: AIChatMessageStatus;
  metadata?: AIChatMessageMetadata;
}

export interface AIChatErrorEventData {
  conversation_id?: string;
  message_id?: string;
  message?: string;
  code?: string | number;
  params?: Record<string, unknown>;
}

export interface AIChatSkillLoadStartEventData {
  conversation_id: string;
  message_id: string;
  skill_id: string;
  created_at?: number;
}

export interface AIChatSkillLoadEndEventData {
  conversation_id: string;
  message_id: string;
  skill_id: string;
  duration_ms?: number;
  status: 'success';
  created_at?: number;
}

export interface AIChatSkillReferenceReadEventData {
  conversation_id: string;
  message_id: string;
  skill_id: string;
  path: string;
  duration_ms?: number;
  status: 'success';
  created_at?: number;
}

export interface AIChatSkillCallStartEventData {
  conversation_id: string;
  message_id: string;
  kind?: AIChatSkillInvocationKind;
  runtime_id?: string;
  skill_id: string;
  tool_name: string;
  arguments?: Record<string, unknown>;
  arguments_summary?: Record<string, unknown>;
  created_at?: number;
}

export interface AIChatSkillCallEndEventData {
  conversation_id: string;
  message_id: string;
  kind?: AIChatSkillInvocationKind;
  runtime_id?: string;
  skill_id: string;
  tool_name: string;
  duration_ms?: number;
  status?: AIChatSkillActivityStatus;
  message?: string;
  result?: Record<string, unknown> | null;
  governance?: AIChatToolGovernanceDecision | null;
  asset_operation_audit?: AIChatAssetOperationAudit;
  created_at?: number;
}

export interface AIChatSkillCallErrorEventData {
  conversation_id: string;
  message_id: string;
  kind?: AIChatSkillInvocationKind;
  runtime_id?: string;
  skill_id: string;
  tool_name?: string;
  duration_ms?: number;
  status: 'error';
  message?: string;
  governance?: AIChatToolGovernanceDecision | null;
  asset_operation_audit?: AIChatAssetOperationAudit;
  created_at?: number;
}

export interface AIChatSkillArtifactFile {
  artifact_type?: 'file';
  file_id: string;
  filename: string;
  extension: string;
  mime_type: string;
  size: number;
  url: string;
  download_url?: string;
  transfer_method?: string;
  file_type?: string;
  created_at?: number;
}

export interface AIChatSkillArtifactCreatedEventData extends Partial<AIChatGeneratedFile> {
  conversation_id: string;
  message_id: string;
  skill_id: string;
  tool_name: string;
  file?: AIChatSkillArtifactFile;
}

export type AIChatToolGovernanceDecisionStatus =
  | 'allowed'
  | 'needs_approval'
  | 'denied'
  | 'needs_resolution'
  | 'blocked'
  | (string & {});

export type AIChatToolGovernanceRiskLevel = 'low' | 'medium' | 'high' | 'critical' | (string & {});

export type AIChatToolGovernanceEffect =
  | 'none'
  | 'read'
  | 'create'
  | 'update'
  | 'delete'
  | 'publish'
  | 'invoke'
  | 'schedule'
  | 'external_send'
  | (string & {});

export interface AIChatToolGovernanceAssetRef extends Record<string, unknown> {
  id?: string;
  type?: string;
  name?: string;
  title?: string;
  label?: string;
  filename?: string;
  file_name?: string;
  file_type?: string;
  extension?: string;
  mime_type?: string;
  size?: number;
  workspace_id?: string;
  source?: string;
  metadata?: Record<string, unknown>;
}

export interface AIChatToolGovernanceManifest extends Record<string, unknown> {
  tool_id?: string;
  skill_id?: string;
  domain?: string;
  effect?: AIChatToolGovernanceEffect;
  asset_type?: string;
  risk_level?: AIChatToolGovernanceRiskLevel;
  requires_asset_resolution?: boolean;
  reversible?: boolean;
  bulk_sensitive?: boolean;
  external_side_effect?: boolean;
  permission_scopes?: string[];
  default_approval_policy?: string;
  allowed_permission_tiers?: string[];
  audit_required?: boolean;
  idempotency_required?: boolean;
}

export interface AIChatToolGovernanceApprovalEvent extends Record<string, unknown> {
  type?: string;
  correlation_id?: string;
  tool_id?: string;
  skill_id?: string;
  domain?: string;
  effect?: AIChatToolGovernanceEffect;
  asset_type?: string;
  risk_level?: AIChatToolGovernanceRiskLevel;
  assets?: AIChatToolGovernanceAssetRef[];
  reversible?: boolean;
  bulk_sensitive?: boolean;
  external_side_effect?: boolean;
  permission_tier?: string;
  grant?: Record<string, unknown>;
}

export type AIChatAssetOperationAuditSource =
  | 'tool_governance_decision'
  | 'skill_invocation'
  | (string & {});

export interface AIChatAssetOperationAudit extends Record<string, unknown> {
  schema_version?: string;
  event_type?: string;
  correlation_id?: string;
  conversation_id?: string;
  governance_status?: AIChatToolGovernanceDecisionStatus | (string & {});
  approval_status?: 'pending' | 'approved' | 'rejected' | (string & {});
  requires_approval?: boolean;
  decision_reason?: string;
  tool_id?: string;
  skill_id?: string;
  domain?: string;
  effect?: AIChatToolGovernanceEffect;
  asset_type?: string;
  asset_count?: number;
  risk_level?: AIChatToolGovernanceRiskLevel;
  permission_tier?: string;
  reversible?: boolean;
  bulk_sensitive?: boolean;
  external_side_effect?: boolean;
  audit_required?: boolean;
  idempotency_required?: boolean;
  permission_scopes?: string[];
  assets?: AIChatToolGovernanceAssetRef[];
  approved_by_correlation_id?: string;
  matched_grant?: Record<string, unknown>;
  approved_grant?: Record<string, unknown>;
  session_grant?: Record<string, unknown>;
}

export interface AIChatAssetOperationAuditRecord extends Record<string, unknown> {
  id: string;
  source: AIChatAssetOperationAuditSource;
  source_id?: string;
  conversation_id: string;
  message_id: string;
  runtime_id?: string;
  correlation_id: string;
  schema_version?: string;
  status?: AIChatSkillActivityStatus | (string & {});
  skill_id?: string;
  tool_name?: string;
  tool_id?: string;
  effect?: AIChatToolGovernanceEffect;
  asset_type?: string;
  risk_level?: AIChatToolGovernanceRiskLevel;
  approval_status?: 'pending' | 'approved' | 'rejected' | (string & {});
  governance_status?: AIChatToolGovernanceDecisionStatus | (string & {});
  action?: string;
  reason?: string;
  resolved_at?: number;
  resolved_by?: string;
  requires_approval?: boolean;
  remember_for_session?: boolean;
  asset_count?: number;
  workspace_id?: string;
  assets?: AIChatToolGovernanceAssetRef[];
  created_at?: number;
  message_created_at?: number;
}

export interface AIChatToolGovernanceDecision extends Record<string, unknown> {
  status?: AIChatToolGovernanceDecisionStatus;
  requires_approval?: boolean;
  reason?: string;
  correlation_id?: string;
  approval_status?: 'approved' | 'rejected' | (string & {});
  manifest?: AIChatToolGovernanceManifest;
  assets?: AIChatToolGovernanceAssetRef[];
  approval_event?: AIChatToolGovernanceApprovalEvent;
  asset_operation_audit?: AIChatAssetOperationAudit;
  matched_grant?: Record<string, unknown>;
  approval_result?: Record<string, unknown>;
  model_feedback?: Record<string, unknown>;
}

export interface AIChatToolGovernanceDecisionEventData extends Record<string, unknown> {
  conversation_id: string;
  message_id: string;
  skill_id?: string;
  tool_name?: string;
  status?: AIChatToolGovernanceDecisionStatus;
  duration_ms?: number;
  created_at?: number;
  governance?: AIChatToolGovernanceDecision;
  correlation_id?: string;
  decision?: AIChatToolGovernanceDecisionStatus;
  requires_approval?: boolean;
  reason?: string;
  risk_level?: AIChatToolGovernanceRiskLevel;
  effect?: AIChatToolGovernanceEffect;
  asset_type?: string;
  asset_operation_audit?: AIChatAssetOperationAudit;
  approval_status?: 'approved' | 'rejected' | (string & {});
  approval_event?: AIChatToolGovernanceApprovalEvent;
  matched_grant?: Record<string, unknown>;
  approval_result?: Record<string, unknown>;
  model_feedback?: Record<string, unknown>;
  session_grant?: Record<string, unknown>;
}

export interface AIChatToolGovernanceDecisionRequest {
  action: 'approve' | 'reject';
  reason?: string;
  remember_for_session?: boolean;
}

export interface AIChatToolGovernanceDecisionResponse {
  conversation_id: string;
  message_id: string;
  correlation_id: string;
  action: 'approve' | 'reject' | (string & {});
  approval_status: 'approved' | 'rejected' | (string & {});
  remember_for_session?: boolean;
  session_grant?: Record<string, unknown>;
  event: AIChatToolGovernanceDecisionEventData;
}

export interface AIChatAgentProgressEventData {
	conversation_id: string;
	message_id: string;
  content?: string;
  phase?: 'planning' | 'tool_planning';
  meta_tool_name?: string;
  skill_id?: string;
  tool_name?: string;
  arguments_chars?: number;
  created_at?: number;
}

export interface AIChatIntermediateAnswerEventData {
  conversation_id: string;
  message_id: string;
  answer_id?: string;
  title?: string;
  content?: string;
  delta?: boolean;
  index?: number;
  done?: boolean;
  status?: 'streaming' | 'success';
  created_at?: number;
}

export interface AIChatUserInputRequestedEventData extends AIChatUserInputRequest {
  conversation_id: string;
  message_id: string;
}

export interface AIChatMemoryMutationEventData {
  conversation_id: string;
  message_id: string;
  memory_scope?: 'account' | 'agent';
  action: AIChatMemoryMutationAction;
  entry_id?: string;
  key?: string;
  category?: string;
  memory_type?: string;
  status?: AIChatSkillActivityStatus;
  content?: string;
  content_preview?: string;
  created_at?: number;
}

export interface AIChatWorkflowEventData extends Record<string, unknown> {
  conversation_id: string;
  message_id: string;
  workflow_run_id?: string;
  task_id?: string;
  workflow_id?: string;
  agent_id?: string;
  status?: string;
  elapsed_time?: number;
  error?: string;
  created_at?: number;
}

export interface AIChatWorkflowNodeEventData extends AIChatWorkflowEventData {
  node_id?: string;
  id?: string;
  execution_id?: string;
  node_type?: string;
  type?: string;
  title?: string;
  node_title?: string;
  name?: string;
  label?: string;
  inputs?: unknown;
  outputs?: unknown;
}

export interface AIChatWorkflowPausedEventData extends AIChatWorkflowEventData {
  node_id?: string;
  node_ids?: string[];
  node_type?: string;
  title?: string;
  approval_form_id?: string;
  approval_token?: string;
  approval_url?: string;
  approval_form?: unknown;
}

export interface AIChatFileParseStartEventData {
  conversation_id: string;
  message_id: string;
  file_id: string;
  name: string;
  kind: AIChatMessageFileKind;
  index: number;
  total: number;
  status: 'parsing';
}

export interface AIChatFileParseEndEventData {
  conversation_id: string;
  message_id: string;
  file_id: string;
  name: string;
  kind: AIChatMessageFileKind;
  index: number;
  total: number;
  status: 'completed';
  content_status: AIChatFileContentStatus;
  content_chars: number;
  from_cache: boolean;
  vision_detail?: AIChatVisionDetail | null;
  filtered_reason?: AIChatFilteredReason | null;
}

export interface AIChatFileParseErrorEventData {
  conversation_id: string;
  message_id: string;
  file_id: string;
  name: string;
  kind: AIChatMessageFileKind;
  index: number;
  total: number;
  status: 'error';
  message: string;
}

export interface AIChatStopConversationResponseData {
  conversation_id: string;
  message_id?: string;
  runtime_status: AIChatConversationRuntimeStatus;
  status: AIChatMessageStatus | 'idle';
}

export type AIChatSseEventName =
  | 'message_start'
  | 'agent_progress'
  | 'agent_intermediate_answer'
  | 'user_input_requested'
  | 'skill_load_start'
  | 'skill_load_end'
  | 'skill_reference_read'
  | 'skill_call_start'
  | 'skill_call_end'
  | 'skill_call_error'
  | 'skill_artifact_created'
  | 'tool_governance_decision'
  | 'memory_create'
  | 'memory_update'
  | 'memory_delete'
  | 'memory_clear'
  | 'workflow_started'
  | 'node_started'
  | 'node_finished'
  | 'workflow_paused'
  | 'approval_requested'
  | 'approval_result_filled'
  | 'approval_expired'
  | 'question_answer_requested'
  | 'question_answer_submitted'
  | 'workflow_finished'
  | 'workflow_failed'
  | 'workflow_stopped'
  | 'file_parse_start'
  | 'file_parse_end'
  | 'file_parse_error'
  | 'message'
  | 'message_retract'
  | 'message_end'
  | 'error';

export interface AIChatSseEnvelope<TData = unknown> {
  event?: string;
  data?: TData;
}
