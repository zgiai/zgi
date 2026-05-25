// Webapp-specific types kept minimal to reduce coupling to main project
// Copy the necessary shapes to be easily portable to a separate project.

import type { WorkflowPrecheckResult } from './workflow';
import type { QuestionAnswerRequestedSseData, QuestionAnswerSubmittedSseData } from './workflow';

// Strict unions mirrored from workflow store for compatibility
export type WebAppFileUploadType = 'image' | 'audio' | 'video' | 'document' | 'custom';
export type WebAppFileUploadMethod = 'local_file' | 'remote_url';

export interface WebAppWorkflowConfigFeature {
  allow_view_run_detail: boolean;
  auto_expand_run_detail: boolean;
}

export interface WebAppFeatures {
  opening_statement_type?: 'slogan' | 'message';
  opening_slogan?: string;
  opening_statement: string;
  opening_statement_enabled?: boolean;
  suggested_questions: string[];
  suggested_questions_after_answer: {
    enabled: boolean;
  };
  text_to_speech: {
    enabled: boolean;
    language: string;
    voice: string;
  };
  speech_to_text: {
    enabled: boolean;
  };
  retriever_resource: {
    enabled: boolean;
  };
  sensitive_word_avoidance: {
    enabled: boolean;
  };
  conversation_history?: {
    enabled: boolean;
    history_window_size: number;
  };
  file_upload: {
    enabled: boolean;
    allowed_file_types: WebAppFileUploadType[];
    allowed_file_extensions: string[];
    allowed_file_upload_methods: WebAppFileUploadMethod[];
    number_limits: number;
  };
  webapp_workflow_config: WebAppWorkflowConfigFeature;
}

// Minimal variable definition aligned with Start node input vars
export type WebAppInputVarType =
  | 'text-input'
  | 'paragraph'
  | 'select'
  | 'number'
  | 'checkbox'
  | 'file'
  | 'file-list';

export interface WebAppVariable {
  type: WebAppInputVarType;
  variable: string;
  label: string;
  description?: string;
  required: boolean;
  max_length?: number;
  default?: string | boolean;
  options?: string[];
  // For file related vars
  allowed_file_upload_methods?: string[];
  allowed_file_types?: string[];
  allowed_file_extensions?: string[];
}

// Webapp config meta alongside features
export interface WebAppWorkflowMeta {
  icon?: string;
  icon_type?: 'image' | 'text' | string;
  icon_url?: string;
  type?: 'WORKFLOW' | 'CONVERSATIONAL_WORKFLOW' | 'AGENT' | string;
  title: string;
  /** Agent ID for stop functionality */
  agent_id?: string;
  web_app_id?: string;
}

export interface WebAppWorkflowConfig {
  variables: WebAppVariable[];
  features: WebAppFeatures;
  config: WebAppWorkflowMeta;
  agent_config?: {
    system_prompt?: string;
    model_provider?: string;
    model?: string;
    model_parameters?: Record<string, unknown>;
    enabled_skill_ids?: string[];
    use_memory?: boolean;
  };
}

export interface WebAppRunRequest {
  query: string;
  conversation_id?: string;
  history_window_size?: number;
  files?: unknown[];
  inputs?: Record<string, unknown>;
}

export type WebAppPrecheckResult = WorkflowPrecheckResult;

// Unified SSE callbacks for webapp workflow run
export interface WebAppRunSseCallbacks {
  onWorkflowStarted?: (payload: unknown) => void;
  onWorkflowPaused?: (payload: unknown) => void;
  onApprovalRequested?: (payload: unknown) => void;
  onApprovalResultFilled?: (payload: unknown) => void;
  onApprovalExpired?: (payload: unknown) => void;
  onQuestionAnswerRequested?: (payload: QuestionAnswerRequestedSseData | unknown) => void;
  onQuestionAnswerSubmitted?: (payload: QuestionAnswerSubmittedSseData | unknown) => void;
  onWorkflowFinished?: (payload: unknown) => void;
  onError?: (payload: unknown) => void;
  onNodeStarted?: (payload: unknown) => void;
  onNodeFinished?: (payload: unknown) => void;
  onNodeRetry?: (payload: unknown) => void;
  onAgentLog?: (payload: unknown) => void;
  onTextChunk?: (payload: unknown) => void;
  onTextReplace?: (payload: unknown) => void;
  onMessage?: (payload: unknown) => void;
  onMessageEnd?: (payload: unknown) => void;
  onIterationStarted?: (payload: unknown) => void;
  onIterationNext?: (payload: unknown) => void;
  onIterationCompleted?: (payload: unknown) => void;
  onLoopStarted?: (payload: unknown) => void;
  onLoopNext?: (payload: unknown) => void;
  onLoopCompleted?: (payload: unknown) => void;
}

// Lightweight API response wrapper for webapp endpoints
export interface WebAppApiResponseData<T> {
  code: string;
  message: string;
  data: T;
}

// Conversation history for current webapp token
export interface WebAppConversation {
  id: string;
  name: string;
  status: string;
  invoke_from: string;
  dialogue_count: number;
  workflow_version_uuid: string;
  created_at: number;
  updated_at: number;
}

export interface WebAppConversationList {
  data: WebAppConversation[];
  has_more: boolean;
  limit: number;
  page: number;
  total: number;
}

// Conversation detail types
export interface WebAppConversationHistoryItem {
  content: string;
  message_id: string;
  role: 'user' | 'assistant';
}

export interface WebAppConversationParamsMeta {
  from_source: string;
  invoke_from: string;
}

export type WebAppConversationInputValue =
  | string
  | number
  | boolean
  | WebAppConversationParamsMeta
  | WebAppConversationHistoryItem[];

export interface WebAppConversationInputs {
  conversation_id: string;
  conversation_params: WebAppConversationParamsMeta;
  query: string;
  'sys.conversation_id'?: string;
  'sys.dialogue_count'?: number;
  'sys.parent_message_id'?: string;
  'sys.query'?: string;
  'sys.version_uuid'?: string;
  'sys.workflow_type'?: string;
  'sys.conversation_history'?: WebAppConversationHistoryItem[];
  [key: string]: WebAppConversationInputValue | undefined;
}

export interface WebAppConversationMessageItem {
  answer: string;
  created_at: number;
  id: string;
  inputs: WebAppConversationInputs;
  invoke_from: string;
  message_metadata?: Record<string, unknown>;
  query: string;
  status: string;
  workflow_run_id: string;
}

export interface WebAppConversationDetail {
  agent_id: string;
  created_at: number;
  dialogue_count: number;
  id: string;
  inputs: WebAppConversationInputs;
  invoke_from: string;
  messages: WebAppConversationMessageItem[];
  mode: string;
  name: string;
  status: string;
  updated_at: number;
  workflow_version_uuid: string;
}
