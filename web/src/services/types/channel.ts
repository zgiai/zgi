export interface GetChannelsParams {
  type?: 'system' | 'organization';
  protocol?: string;
  is_active?: boolean;
  search?: string;
  page?: number;
  page_size?: number;
}

// User channel item from channel list response
export interface ChannelItem {
  id: string;
  name: string;
  models: string[];
  priority: number;
  weight: number;
  is_enabled: boolean;
  is_official: boolean;
  created_at: number;
  updated_at: number;
  supported_protocols?: string[];
  api_base_url?: string;
  api_key_masked?: string;
  auto_ban?: boolean;
  description?: string;
  tags?: string[];
  validation_report?: Record<string, { success: boolean; message: string }>;
  provider?: string;
  channel_provider?: string;
  remaining_funds?: number;
}

// Extended channel detail (for detail API if needed)
export interface ChannelDetail {
  id: string;
  name: string;
  models: string[];
  priority: number;
  weight: number;
  is_enabled: boolean;
  is_official: boolean;
  tenant_id?: string;
  provider?: string;
  channel_provider?: string;
  supported_protocols?: string[];
  api_key?: string;
  api_key_masked?: string;
  api_base_url?: string;
  model_maps?: Record<string, unknown>;
  param_override?: Record<string, unknown>;
  header_override?: Record<string, unknown>;
  status_code_maps?: Record<string, unknown>;
  tags?: string[];
  auto_ban?: boolean;
  remaining_funds?: number;
  balance?: string;
  currency?: string;
  created_at?: number;
  updated_at?: number;
}

export interface ChannelsResponse {
  data: ChannelItem[];
  page: number;
  page_size: number;
  total: number;
}

// Platform/Official channel item from /platform response (Aggregated)
export interface PlatformChannelItem {
  id?: string; // Optional, might use a constant in frontend
  name: string;
  provider?: string;
  model_count: number;
  priority: number;
  weight: number;
  is_enabled: boolean;
  is_official?: boolean;
  created_at?: number;
  updated_at?: number;
}

export interface PlatformChannelsResponse {
  name: string;
  model_count: number;
  priority: number;
  weight: number;
  is_enabled: boolean;
}

export interface PlatformChannelModelsResponse {
  models: string[];
}

export interface UpdateChannelRequest {
  name?: string;
  channel_provider?: string;
  provider?: string;
  models?: string[];
  priority?: number;
  weight?: number;
  is_enabled?: boolean;
  api_base_url?: string;
  api_key?: string;
  description?: string;
  tags?: string[];
}

export interface UpdateOfficialGroupSettingsRequest {
  priority?: number;
  weight?: number;
  is_enabled?: boolean;
}

export interface CreateChannelRequest {
  name: string;
  channel_provider: string;
  provider?: string;
  api_key: string;
  models?: string[];
  api_base_url?: string;
  initial_funds?: number;
  priority?: number;
  weight?: number;
  model_maps?: Record<string, unknown>;
  param_override?: Record<string, unknown>;
  header_override?: Record<string, unknown>;
  tags?: string[];
  description?: string;
}

// Test method types for channel model testing
export type ChannelTestMethod = 'chat' | 'embedding' | 'image-gen' | 'rerank';

// Request to test a model before creating the channel
export interface DraftTestChannelModelRequest {
  channel_provider: string;
  api_key: string;
  api_base_url?: string;
  model: string;
  test_method?: ChannelTestMethod;
}

export interface ChannelModelTestResult {
  success: boolean;
  message: string;
  model: string;
  use_case?: string;
  test_method?: string;
  response_time_ms: number;
}

export interface DiscoverDraftChannelModelsRequest {
  channel_provider: string;
  api_key: string;
  api_base_url?: string;
}

export interface DiscoveredChannelModel {
  id: string;
  name: string;
  display_name: string;
  provider?: string;
  owned_by?: string;
  context_length?: number;
  capabilities?: string[];
  created?: number;
}

export interface DiscoverDraftChannelModelsResponse {
  models: DiscoveredChannelModel[];
  total: number;
}

// Request to batch test multiple models in a channel (SSE)
export interface BatchTestChannelModelsRequest {
  models: string[];
  test_message?: string;
  test_method?: ChannelTestMethod;
}

// SSE event for individual model test result (Aligned with documentation 3.6)
export interface BatchTestModelResult {
  model: string;
  success: boolean;
  message: string;
  response_time_ms: number;
  completed: false;
  // Metadata for UI
  index?: number;
  test_method?: string;
}

// SSE event for batch test completion (Aligned with documentation 3.6)
export interface BatchTestCompletedResult {
  model: '';
  success: false;
  message: string;
  response_time_ms: 0;
  completed: true;
  // Optional stats for UI
  total_tests?: number;
  success_count?: number;
  failure_count?: number;
}

// Union type for batch test SSE events
export type BatchTestChannelModelsEvent = BatchTestModelResult | BatchTestCompletedResult;

// Request to adjust private channel wallet balance
export interface AdjustChannelWalletRequest {
  amount: number; // != 0, positive = add, negative = subtract
  note?: string;
}

// Response from channel wallet adjustment
export interface AdjustChannelWalletResponse {
  channel_id: string;
  organization_id: string;
  amount: number;
  balance_before: number;
  balance_after: number;
  status: 'ACTIVE' | 'DEBT' | string;
  transaction_id: string;
  updated_at: string;
}
