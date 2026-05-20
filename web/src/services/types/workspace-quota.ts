/**
 * Quota type for workspace LLM quota configuration
 */
export enum WorkspaceQuotaType {
  Unlimited = 'unlimited',
  Custom = 'custom',
}

/**
 * Workspace quota entity returned from API
 */
export interface WorkspaceQuota {
  workspace_id: string;
  organization_id: string;
  used_quota: number;
  remain_quota: number;
  /** Quota upper limit (null means unlimited) */
  quota_limit?: number | null;
  /** Whether the workspace has been configured with quota */
  configured: boolean;
  created_at?: string;
  updated_at?: string;
}

/**
 * Workspace quota list response
 */
export interface WorkspaceQuotaList {
  items: WorkspaceQuota[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

/**
 * Query params for listing workspace quotas
 */
export interface GetWorkspaceQuotasParams {
  /** Page number (minimum: 1) */
  page?: number;
  /** Number of items per page (minimum: 1, maximum: 100) */
  limit?: number;
}

/**
 * Request body for updating workspace quota
 */
export interface UpdateWorkspaceQuotaRequest {
  /** Quota type: unlimited or custom */
  quota_type: WorkspaceQuotaType;
  /** Custom quota amount (required when quota_type is custom, must be > 0) */
  quota_amount?: number;
  /** Manual override for remaining quota (must be >= 0, cannot exceed quota_limit) */
  remain_quota?: number;
}
