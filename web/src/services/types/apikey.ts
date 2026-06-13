/**
 * API Key status
 */
export enum ApiKeyStatus {
  Active = 'active',
  Inactive = 'inactive',
  Revoked = 'revoked',
}

/**
 * Quota type for API Key creation
 */
export enum ApiKeyQuotaType {
  Unlimited = 'unlimited',
  Custom = 'custom',
}

/**
 * API Key item in list response
 */
export interface ApiKeyItem {
  id: string;
  key?: string;
  key_masked: string;
  name: string;
  status: ApiKeyStatus;
  created_at: string;
  updated_at: string;
  /** Last accessed time in ISO 8601 format */
  accessed_at?: string;
  /** Expiration time in RFC3339 format, null means never expires */
  expires_at?: string | null;
  used_quota: number;
  remain_quota: number;
  /** Quota upper limit (null means unlimited) */
  quota_limit?: number | null;
  model_limits_enabled: boolean;
  /** Model limits - array of allowed model names */
  model_limits?: string[];
  /** IP whitelist (comma separated) */
  allow_ips?: string;
}

/**
 * API Key detail
 */
export type ApiKeyDetail = ApiKeyItem;

/**
 * API Key list response data
 */
export interface ApiKeyList {
  items: ApiKeyItem[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

/**
 * Get API Keys request params
 */
export interface GetApiKeysParams {
  /** Page number (minimum: 1) */
  page?: number;
  /** Number of items per page (minimum: 1, maximum: 100) */
  limit?: number;
  /** Search term for API key name */
  search?: string;
  /** Filter by status */
  status?: ApiKeyStatus;
}

/**
 * Create API Key request
 */
export interface CreateApiKeyRequest {
  /** Token name */
  name: string;
  /** Number of tokens to create */
  count?: number;
  /** Quota setting: unlimited or custom */
  quota_type?: ApiKeyQuotaType;
  /** Custom quota amount (required when quota_type is custom) */
  quota_amount?: number;
  /** Allow access to all models */
  allow_all_models?: boolean;
  /** Specified model names */
  model_names?: string[];
  /** IP whitelist (CIDR format) */
  allow_ips?: string;
  /** Expiration time in ISO 8601 format */
  expires_at?: string;
}

/**
 * Create API Key response
 */
export interface CreateApiKeyResponse {
  keys: ApiKeyItem[];
  count: number;
  message?: string;
}

/**
 * Update API Key request
 * Matches LLMAPIKeyUpdateRequest from backend
 */
export interface UpdateApiKeyRequest {
  /** Key name */
  name?: string;
  /** API key status */
  status?: ApiKeyStatus;
  /** Quota upper limit */
  quota_limit?: number;
  /** Remaining quota */
  remain_quota?: number;
  /** Whether to enable model limits */
  model_limits_enabled?: boolean;
  /** Specified model name list */
  model_limits?: string[];
  /** IP whitelist (CIDR format) */
  allow_ips?: string;
  /** Expiration time in ISO 8601 format */
  expires_at?: string;
}

/**
 * API Key validation result
 */
export interface ApiKeyValidateResult {
  valid: boolean;
  key_id?: string;
  key_name?: string;
  expires_at?: string;
  message?: string;
}
