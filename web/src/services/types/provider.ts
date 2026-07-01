// All fields strictly typed. English comments only.

export interface LocalizedString {
  en_US: string;
  zh_Hans: string;
}

export interface GetProvidersParams {
  is_enabled?: boolean; // Renamed from is_active to match docs
  limit?: number;
  page?: number;
}

export interface ProviderMetadata {
  i18n?: Record<
    string,
    {
      provider_name?: string;
      tagline?: string;
      description?: string; // Added description
    }
  >;
  social?: {
    twitter?: string;
    github?: string;
  } | null;
}

export interface ProviderItem {
  id: string;
  object: string; // Added: fixed to "provider"
  provider: string;
  provider_name: string;
  description?: string;
  logo_url?: string;
  website?: string;
  api_docs_url?: string;
  pricing_url?: string;
  country_code?: string;
  founded_year?: number;
  tagline?: string;
  api_base_url?: string; // Added: for custom providers
  protocol?: string; // Added: for custom providers
  provider_type: 'global' | 'custom'; // Added
  is_enabled: boolean; // Renamed from is_active to match docs
  is_available: boolean;
  model_count?: number;
  channel_count?: number;
  sort_order: number; // Added
  metadata?: ProviderMetadata;
  created_at: number; // Changed from string to number (Unix timestamp)
  updated_at: number; // Changed from string to number (Unix timestamp)
}

export interface ProviderList {
  items: ProviderItem[];
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
}

export type ProviderDetail = ProviderItem;

export interface ToggleProviderRequest {
  provider: string;
  is_enabled: boolean; // Renamed from is_active
}

export interface ToggleProviderResponse {
  is_enabled: boolean; // Renamed from is_active
  message: string;
  provider: string;
}

export interface CreateCustomProviderRequest {
  provider: string;
  provider_name: string;
  api_base_url?: string;
  logo_url?: string;
  documentation_url?: string;
  description?: string;
}

export interface UpdateCustomProviderRequest {
  provider_name?: string;
  api_base_url?: string;
  logo_url?: string;
  documentation_url?: string;
  description?: string;
  is_enabled?: boolean; // Maps to is_active in backend
  sort_order?: number;
}

export interface SyncProvidersRequest {
  force?: boolean;
  providers?: string[];
}

export interface SyncProviderResponse {
  provider: string;
  provider_display_name: string;
  status: 'success' | 'failed' | 'skipped';
  new_models: number;
  updated_models: number;
  deprecated_models: number;
  models: Array<{
    name: string;
    display_name?: string;
    status: 'new' | 'updated' | 'deprecated';
    changes?: string[];
  }>;
  duration_ms: number;
  timestamp: string;
}

export interface SyncAllProvidersResponse {
  synced_providers: number;
  new_models: number;
  updated_models: number;
  deprecated_models: number;
  duration_ms: number;
  details: SyncProviderResponse[];
  timestamp: string;
}

// ModelMeta API Types
export interface ModelMetaStatusSummary {
  upstream: number;
  local: number;
  new: number;
  updated: number;
  unchanged: number;
  local_only: number;
}

export interface ModelMetaProviderError {
  provider: string;
  error: string;
}

export interface ModelMetaStatusResponse {
  has_updates: boolean;
  degraded: boolean;
  upstream_source: string;
  checked_at: string;
  providers: ModelMetaStatusSummary;
  models: ModelMetaStatusSummary;
  provider_errors?: ModelMetaProviderError[];
}

export interface ModelMetaProviderDiffItem {
  provider: string;
  name: string;
  status: 'new' | 'updated' | 'unchanged' | 'local_only';
  changed_fields?: string[];
}

export interface ModelMetaProviderDiffResponse {
  checked_at: string;
  summary: ModelMetaStatusSummary;
  items: ModelMetaProviderDiffItem[];
}

export interface ModelMetaModelUpdateProviderItem {
  provider: string;
  name: string;
  new_models: number;
  updated_models: number;
  deprecated_models: number;
  total_remote: number;
  total_local: number;
}

export interface ModelMetaModelUpdateProvidersResponse {
  checked_at: string;
  items: ModelMetaModelUpdateProviderItem[];
  provider_errors?: ModelMetaProviderError[];
}

export interface ModelMetaSyncResult {
  provider: string;
  status: 'success' | 'partial' | 'failed';
  total_models: number;
  success_models: number;
  failed_models: number;
  new_models: number;
  updated_models: number;
  deprecated_models: number;
  skipped_models: number;
  errors?: string[];
  duration_ms: number;
}

// Field-level diff for a single model property change
export interface DiffField {
  field: string;
  old_value: unknown;
  new_value: unknown;
}

// Model change entry in diff response
export interface ModelChange {
  model: string;
  model_name: string;
  change_type: 'new' | 'updated' | 'deprecated';
  remote_data?: unknown;
  local_data?: unknown;
  diff_fields?: DiffField[];
  recommended_action: 'sync' | 'skip' | 'manual_review';
}

export interface ModelMetaDiffResponse {
  provider: string;
  checked_at: string;
  summary: {
    total_remote: number;
    total_local: number;
    new_models: number;
    updated_models: number;
    deprecated_models: number;
    unchanged_models: number;
  };
  changes: {
    new: ModelChange[];
    updated: ModelChange[];
    deprecated: ModelChange[];
  };
}
