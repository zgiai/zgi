// Statistics types for LLM usage analytics.

import type { ModelUsageAppType } from '@/utils/model-usage-app-type';

export type { ModelUsageAppType } from '@/utils/model-usage-app-type';

export interface ModelUsagePeriod {
  start_time: number;
  end_time: number;
}

export interface ModelUsageSummary {
  attempt_count: number;
  success_count: number;
  failed_count: number;
  partial_count: number;
  prompt_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  official_points: number;
  private_points: number;
  total_points: number;
}

export interface ModelUsageByModelItem {
  model_id: string;
  model_name: string;
  provider_id: string;
  provider_name: string;
  attempt_count: number;
  success_count: number;
  failed_count: number;
  partial_count: number;
  prompt_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  official_points: number;
  private_points: number;
  total_points: number;
  points_share: number;
}

export interface ModelUsageByAppTypeItem {
  app_type: ModelUsageAppType;
  attempt_count: number;
  success_count: number;
  failed_count: number;
  partial_count: number;
  prompt_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  official_points: number;
  private_points: number;
  total_points: number;
  points_share: number;
}

export interface ModelUsageDailyItem {
  date: string;
  attempt_count: number;
  success_count: number;
  failed_count: number;
  partial_count: number;
  prompt_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  official_tokens: number;
  private_tokens: number;
  official_points: number;
  private_points: number;
  total_points: number;
}

export interface ModelUsageData {
  period: ModelUsagePeriod;
  summary: ModelUsageSummary;
  by_model: ModelUsageByModelItem[];
  by_app_type: ModelUsageByAppTypeItem[];
  daily_trend: ModelUsageDailyItem[];
}

export interface WorkspaceQuotaSummary {
  total_workspaces: number;
  unlimited_count: number;
  total_used_quota: number;
  total_remain_quota: number;
  total_quota_limit: number;
}

export interface WorkspaceQuotaItem {
  workspace_id: string;
  workspace_name: string;
  used_quota: number;
  remain_quota: number;
  quota_limit: number | null;
  is_unlimited: boolean;
}

export interface WorkspaceQuotaData {
  summary: WorkspaceQuotaSummary;
  items: WorkspaceQuotaItem[];
}

// Request Parameters

export interface GetModelUsageParams {
  start_time: number;
  end_time: number;
  app_type?: ModelUsageAppType;
  app_id?: string;
  model_name?: string;
  use_system_provider?: boolean;
}

export interface GetWorkspaceQuotaParams {
  workspace_id?: string;
}

export type InvocationSource = 'api' | 'product' | 'unknown';
export type InvocationStatus = 'success' | 'failed' | 'partial';

export interface InvocationLogSummary {
  invocation_count: number;
  api_count: number;
  product_count: number;
  unknown_count: number;
  total_tokens: number;
  total_points: number;
}

export interface InvocationLogItem {
  invocation_id: string;
  invocation_source: InvocationSource;
  app_id?: string;
  app_type: string;
  model_name: string;
  provider_name: string;
  status: InvocationStatus;
  attempt_count: number;
  prompt_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  total_points: number;
  total_cost_usd?: string;
  duration_ms: number;
  started_at: number;
  settled_at: number;
  error_code?: string;
  content_available: boolean;
  content_expires_at?: number;
  input?: unknown;
  output?: unknown;
}

export interface InvocationLogCursor {
  time: string;
  id: string;
}

export interface InvocationLogData {
  summary: InvocationLogSummary;
  items: InvocationLogItem[];
  next_cursor?: InvocationLogCursor;
}

export interface GetInvocationLogParams {
  start_time: number;
  end_time: number;
  invocation_source?: InvocationSource;
  app_type?: string;
  model_name?: string;
  cursor_time?: string;
  cursor_id?: string;
  limit?: number;
  include_summary?: boolean;
}

export interface InvocationContentSettings {
  /** @deprecated Always true; retained for rolling-deploy compatibility. */
  available: boolean;
  enabled: boolean;
  max_bytes: number;
  retention_days: number;
  stored_count: number;
  stored_count_capped: boolean;
}

export interface UpdateInvocationContentSettingsInput {
  enabled: boolean;
  retention_days: number;
}

export interface InvocationContentPurgeResult {
  deleted_count: number;
  has_more: boolean;
}

export interface InvocationContentDetail {
  invocation_id: string;
  input_text: string;
  output_text: string;
  input_json: string;
  output_json: string;
  content_status: 'available' | string;
  input_truncated: boolean;
  output_truncated: boolean;
  redaction_version: string;
  expires_at: number;
}
