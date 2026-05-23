export type MarketplacePluginCategory = 'tool' | 'extension' | 'integration';

export interface InstalledPlugin {
  version_id: string;
  [key: string]: unknown;
}

export interface UninstallResult {
  result: string;
}

export interface MarketplacePluginDeveloper {
  id: string;
  organization_name: string;
  organization_slug: string;
  logo_url?: string;
  is_verified: boolean;
}

export interface MarketplaceBrandingSettings {
  official_logo_url?: string;
  blue_v_icon_url?: string;
  yellow_v_icon_url?: string;
  feedback_enabled?: boolean;
  feedback_url?: string;
  upload_application_enabled?: boolean;
  upload_application_url?: string;
}

export type MarketplacePluginFeedbackRequestType = 'existing_official' | 'missing_plugin' | 'other';

export interface SubmitMarketplacePluginFeedbackRequest {
  request_type: MarketplacePluginFeedbackRequestType;
  plugin_id?: string;
  content: string;
  submitter_id?: string;
  submitter_name?: string;
  submitter_email?: string;
  submitter_organization_id?: string;
  submitter_organization_name?: string;
}

export interface MarketplacePluginVersion {
  id: string;
  version: string;
  status: string;
  published_at: string;
}

export interface MarketplacePluginVersionDetail {
  id: string;
  plugin_id: string;
  version: string;
  changelog?: string;
  package_url?: string;
  package_size?: number;
  package_checksum?: string;
  manifest?: {
    name?: string;
    version?: string;
    author?: string;
    description?: string;
    tools?: string[];
    [key: string]: unknown;
  };
  status: string;
  published_at: string;
  created_at: string;
  updated_at: string;
}

export interface MarketplacePluginVersionListResponse {
  items: MarketplacePluginVersionDetail[];
  total: number;
  page: number;
  page_size: number;
}

export interface MarketplacePluginDisplayMetadata {
  name: string;
  short_description: string;
  description: string;
  tags: string[];
  official_labels: string[];
}

export interface MarketplacePlugin {
  id: string;
  developer_id: string;
  unique_identifier: string;
  name: string;
  short_description?: string;
  description: string;
  icon: string;
  category: MarketplacePluginCategory;
  tags: string[];
  official_labels?: string[];
  locale?: string;
  default_metadata?: MarketplacePluginDisplayMetadata;
  localized?: MarketplacePluginDisplayMetadata;
  download_count: number;
  rating: number;
  rating_count: number;
  is_official: boolean;
  is_featured: boolean;
  created_at: string;
  updated_at: string;
  developer: MarketplacePluginDeveloper;
  latest_version: MarketplacePluginVersion;
}

export interface MarketplacePluginListResponse {
  items: MarketplacePlugin[];
  total: number;
  page: number;
  page_size: number;
}
