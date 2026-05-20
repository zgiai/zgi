// Statistics types for LLM usage analytics.

export type ModelUsageAppType = 'workflow' | 'dataset' | 'agent' | 'aichat' | 'unknown';

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
  completion_tokens: number;
  total_tokens: number;
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
