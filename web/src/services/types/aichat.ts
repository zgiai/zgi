import type { ApiResponseData } from './common';

export type AIChatConversationStatus = 'normal' | 'archived';
export type AIChatConversationSource = 'console' | 'webapp' | 'migration';
export type AIChatConversationRuntimeStatus = 'idle' | 'streaming';
export type AIChatMessageStatus = 'pending' | 'streaming' | 'completed' | 'error' | 'stopped';

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
  | 'success'
  | 'blocked'
  | 'error';
export type AIChatSkillInvocationKind =
  | 'metadata_exposed'
  | 'skill_load'
  | 'reference_read'
  | 'tool_call'
  | 'intermediate_answer';

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
}

export type AIChatSkillListResponse = ApiResponseData<AIChatSkillMetadata[]>;
export type AIChatSkillDetailResponse = ApiResponseData<AIChatSkillMetadata>;

export interface AIChatSkillOrganizationConfig {
  enabled_skill_ids: string[];
}

export type AIChatSkillConfigResponse = ApiResponseData<AIChatSkillOrganizationConfig>;

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
  file_count?: number;
  files?: AIChatMessageFile[];
  generated_file_count?: number;
  generated_files?: AIChatGeneratedFile[];
  context_control?: Record<string, unknown>;
  [key: string]: unknown;
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
  billing_reason_source?: 'aichat';
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
  model: string;
  provider?: string;
  file_ids?: string[];
  response_mode: 'streaming';
  parameters?: AIChatModelParameters;
  use_memory?: boolean;
}

export interface AIChatRegenerateMessageRequest {
  query?: string;
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
  skill_id: string;
  tool_name: string;
  arguments?: Record<string, unknown>;
  arguments_summary?: Record<string, unknown>;
  created_at?: number;
}

export interface AIChatSkillCallEndEventData {
  conversation_id: string;
  message_id: string;
  skill_id: string;
  tool_name: string;
  duration_ms?: number;
  status: 'success';
  message?: string;
  result?: Record<string, unknown> | null;
  created_at?: number;
}

export interface AIChatSkillCallErrorEventData {
  conversation_id: string;
  message_id: string;
  skill_id: string;
  tool_name?: string;
  duration_ms?: number;
  status: 'error';
  message?: string;
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

export interface AIChatAgentProgressEventData {
  conversation_id: string;
  message_id: string;
  content?: string;
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
  | 'skill_load_start'
  | 'skill_load_end'
  | 'skill_reference_read'
  | 'skill_call_start'
  | 'skill_call_end'
  | 'skill_call_error'
  | 'skill_artifact_created'
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
