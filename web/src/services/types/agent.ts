import type { IconType } from '@/utils/icon-helpers';

// Agent type enumeration
export enum AgentType {
  AGENT = 'AGENT',
  WORKFLOW = 'WORKFLOW',
  CONVERSATIONAL_AGENT = 'CONVERSATIONAL_WORKFLOW',
}

// Agent icon type (aligned with shared UI icon type)
export type AgentIconType = IconType | undefined;

// Agent model config interface
export interface AgentModelConfig {
  id: string;
  model_provider: string | null;
  model_version_id: string | null;
  prompt_type: string;
  created_at: number;
  updated_at: number;
}

// Owner account interface
export interface OwnerAccount {
  id: string;
  name: string;
}

export type WebAppStatus = 'active' | 'inactive';

// Basic agent interface (for list response)
export interface Agent {
  id: string;
  web_app_id?: string;
  name: string;
  description: string;
  agent_type: AgentType;
  icon_type: AgentIconType;
  icon: string;
  icon_url?: string;
  is_public: boolean;
  is_published: boolean;
  created_by: string;
  created_at: number;
  updated_at: number;
  can_edit: boolean;
  web_app_status?: WebAppStatus;
}

// Agent detail interface (for detail response)
export interface AgentDetail {
  id: string;
  web_app_id?: string;
  name: string;
  description: string;
  agent_type: AgentType;
  icon_type: AgentIconType;
  icon: string;
  icon_url?: string;
  enable_api: boolean;
  is_editor: boolean;
  agent_config: AgentModelConfig;
  owner_account: OwnerAccount;
  workspace: {
    id: string;
    name: string;
  };
  created_at: number;
  updated_at: number;
  created_by: string;
  can_edit: boolean;
  is_published?: boolean;
  web_app_status?: WebAppStatus;
}

// Agent create response interface
export interface AgentCreateResponse {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  agents_type: AgentType; // API returns 'agents_type' in create response
  icon_type: AgentIconType;
  icon: string;
  agents_model_config_id: string;
  workflow_id: string | null;
  enable_api: boolean;
  is_public: boolean;
  is_universal: boolean;
  created_by: string;
  created_at: string;
  updated_by: string | null;
  updated_at: string;
  deleted_by: string | null;
  deleted_at: string | null;
}

// Agent list response
export interface AgentList {
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
  data: Agent[];
}

// Create agent request
export interface CreateAgentRequest {
  name: string;
  icon_type: AgentIconType;
  icon: string;
  agent_type: AgentType;
  description: string;
  workspace_id?: string;
}

// Update agent request
export interface UpdateAgentRequest {
  name?: string;
  icon_type?: AgentIconType;
  icon?: string;
  description?: string;
  workspace_id?: string;
}

// Agent query parameters
export interface AgentListParams {
  page?: number;
  limit?: number;
  name?: string;
  keyword?: string;
  workspace_id?: string;
  agent_type?: AgentType;
  sort?: string;
  order?: 'asc' | 'desc';
}

export interface RunnableWebAppsParams {
  workspace_id?: string;
}

export interface RunnableWebAppItem {
  agent_id: string;
  workspace_id: string;
  web_app_id: string;
  web_app_status?: WebAppStatus;
  meta_data?: RunnableWebAppMetaData;
}

export interface RunnableWebAppMetaData {
  title: string;
  icon: string;
  icon_type?: string;
  icon_url?: string;
  desc: string | null;
  agent_type: AgentType | string;
}

export interface RunnableWebAppsData {
  items: RunnableWebAppItem[];
}

export interface UpdateWebAppStatusRequest {
  status: WebAppStatus;
  reason?: string;
}

export interface UpdateWebAppStatusResponse {
  agent_id: string;
  web_app_id: string;
  web_app_status: WebAppStatus;
  updated_at: number;
}

export interface AgentRuntimeConfig {
  agent_id: string;
  system_prompt: string;
  model_provider: string;
  model: string;
  model_parameters: Record<string, unknown>;
  enabled_skill_ids: string[];
  use_memory: boolean;
  agent_memory_enabled?: boolean;
  agent_memory_slots?: AgentMemorySlotConfig[];
  file_upload_enabled: boolean;
  home_title: string;
  input_placeholder: string;
  theme_color: string;
  suggested_questions: string[];
  knowledge_dataset_ids?: string[];
  knowledge_retrieval_config?: Record<string, unknown>;
  database_bindings?: AgentDatabaseBinding[];
  workflow_bindings?: AgentWorkflowBinding[];
  updated_at: number;
}

export interface AgentDatabaseBinding {
  data_source_id: string;
  table_ids: string[];
  writable_table_ids?: string[];
}

export type AgentWorkflowVersionStrategy = 'latest_published' | 'pinned';

export interface AgentWorkflowBinding {
  binding_id: string;
  label: string;
  description?: string;
  agent_id: string;
  workflow_id: string;
  version_strategy: AgentWorkflowVersionStrategy;
  version_uuid?: string;
  timeout_seconds?: number;
}

export interface AgentWorkflowBindingCandidate extends AgentWorkflowBinding {
  version?: string;
  icon?: string;
  icon_type?: AgentIconType | string;
  icon_url?: string;
  updated_at?: number;
}

export interface AgentWorkflowBindingCandidatesResponse {
  data: AgentWorkflowBindingCandidate[];
}

export interface AgentMemorySlotConfig {
  id?: string;
  key: string;
  description: string;
  max_chars: number;
  enabled: boolean;
  sort_order: number;
  created_at?: number;
  updated_at?: number;
  created_at_unix?: number;
  updated_at_unix?: number;
  created_at_iso?: string;
  updated_at_iso?: string;
  created_at_display?: string;
  updated_at_display?: string;
}

export interface AgentMemoryValue extends AgentMemorySlotConfig {
  content: string;
}

export interface AgentMemoryValuesResponse {
  user_scope: 'account' | 'end_user';
  user_id: string;
  values: AgentMemoryValue[];
}

export interface UpdateAgentMemoryValueRequest {
  key: string;
  content: string;
}

export interface UpdateAgentRuntimeConfigRequest {
  system_prompt: string;
  model_provider: string;
  model: string;
  model_parameters: Record<string, unknown>;
  enabled_skill_ids: string[];
  use_memory: boolean;
  agent_memory_enabled?: boolean;
  agent_memory_slots?: AgentMemorySlotConfig[];
  file_upload_enabled: boolean;
  home_title: string;
  input_placeholder: string;
  theme_color: string;
  suggested_questions: string[];
  knowledge_dataset_ids?: string[];
  knowledge_retrieval_config?: Record<string, unknown>;
  database_bindings?: AgentDatabaseBinding[];
  workflow_bindings?: AgentWorkflowBinding[];
}

export interface AgentSuggestedQuestionSkillContext {
  id?: string;
  name?: string;
  description?: string;
}

export interface GenerateAgentSuggestedQuestionsRequest {
  locale?: string;
  count?: number;
  provider?: string;
  model?: string;
  system_prompt?: string;
  home_title?: string;
  existing_questions?: string[];
  skills?: AgentSuggestedQuestionSkillContext[];
  knowledge_refs?: string[];
}

export interface AgentSuggestedQuestionCandidate {
  text: string;
  reason?: string;
}

export interface GenerateAgentSuggestedQuestionsResponse {
  questions: AgentSuggestedQuestionCandidate[];
  warnings?: string[];
  provider?: string;
  model?: string;
}

export interface AgentChatRequest {
  query: string;
  conversation_id?: string;
  parent_id?: string;
  files?: string[];
  response_mode?: 'streaming' | 'blocking';
}

export interface AgentChatSseData {
  conversation_id?: string;
  message_id?: string;
  parent_id?: string;
  title?: string;
  model?: string;
  answer?: string;
  message?: string;
  status?: string;
  metadata?: Record<string, unknown>;
}

export interface AgentChatSseEnvelope {
  event?: string;
  data?: AgentChatSseData;
}

export interface AgentChatStreamCallbacks {
  onMessageStart?: (data: AgentChatSseData) => void;
  onMessage?: (data: AgentChatSseData) => void;
  onMessageEnd?: (data: AgentChatSseData) => void;
  onError?: (error: Error | AgentChatSseData) => void;
  onClose?: () => void;
}

export interface PublishAgentResponse {
  agent_id: string;
  version_uuid: string;
  version: string;
  web_app_id: string;
  published_at: number;
}

export interface AgentPublishedVersion {
  id: string;
  agent_id: string;
  version_uuid: string;
  version: string;
  description: string;
  config_snapshot: AgentRuntimeConfig;
  is_current: boolean;
  created_at: number;
}

export interface AgentPublishedVersionsResponse {
  data: AgentPublishedVersion[];
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
}

export interface RollbackAgentPublishedVersionRequest {
  version_id: string;
}
export interface AgentApiKeyCreateResponse {
  id: string;
  agent_id: string;
  key_prefix: string;
  name: string;
  status: 'active' | 'revoked';
  expires_at: string | null;
  created_at: string;
  updated_at: string;
  api_key: string;
}

export interface AgentApiKey {
  id: string;
  agent_id: string;
  key_prefix: string;
  name: string;
  status: 'active' | 'revoked';
  expires_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface AgentApiKeyList {
  api_keys: AgentApiKey[];
  total: number;
}

export interface CreateAgentApiKeyRequest {
  name: string;
  expires_at?: string | null; // ISO timestamp
}

export interface UpdateAgentApiKeyRequest {
  name?: string;
  status?: 'active' | 'revoked';
  expires_at?: string | null; // ISO timestamp
}
