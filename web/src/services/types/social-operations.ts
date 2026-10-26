import type { CreateDigitalHumanTaskRequest, DigitalHumanTask } from './digital-human';

export type SocialPlatform = 'x' | 'linkedin' | 'facebook';
export type SocialCapability =
  | 'account'
  | 'post'
  | 'readback'
  | 'comment'
  | 'inbox'
  | 'metrics'
  | 'images';
export type SocialCapabilityMode = 'required' | 'optional';

export interface SocialOperationsScope {
  organization_id: string;
  workspace_id: string;
  owner_id: string;
  employee_id: string;
}

export interface SocialOperationsSource {
  url?: string;
  artifact_id?: string;
}

export interface SocialOperationsPlatformBinding {
  platform: SocialPlatform;
  connector_id: string;
  connection_id: string;
  account_id: string;
  max_daily_publications: number;
  capabilities: Array<{
    capability: SocialCapability;
    mode: SocialCapabilityMode;
  }>;
}

export interface SocialOperationsCadence {
  daily_content_cron: string;
  inbox_cron: string;
  seven_day_sample_cron: string;
  weekly_review_cron: string;
}

export interface SocialOperationsTeamHandoff {
  connector_id: string;
  connection_id: string;
  operation_id: string;
  destination: string;
}

export interface SocialOperationsResearchBinding {
  connector_id: string;
  connection_id: string;
}

export interface SocialOperationsSettings {
  product: { name: string; description: string };
  audience: string;
  publication_language: string;
  internal_review_language: string;
  sources: SocialOperationsSource[];
  brand_artifact_refs: string[];
  research: SocialOperationsResearchBinding;
  platforms: SocialOperationsPlatformBinding[];
  timezone: string;
  cadence: SocialOperationsCadence;
  team_handoff: SocialOperationsTeamHandoff;
}

export interface SocialOperationsConfiguration
  extends Omit<SocialOperationsSettings, 'brand_artifact_refs'> {
  brand_artifact_refs?: string[];
  scope: SocialOperationsScope;
  employee_version_id: string;
  revision: number;
}

export interface SocialOperationsConfigurationRecord {
  id: string;
  organization_id: string;
  workspace_id: string;
  owner_id: string;
  employee_id: string;
  kind: 'configuration';
  key: 'current';
  revision: number;
  artifact_id: string;
  updated_at: string;
}

export interface SocialOperationsConfigurationResponse {
  configuration: SocialOperationsConfiguration | null;
  record: SocialOperationsConfigurationRecord | null;
}

export interface SocialOperationsVerifiedAccountsResponse {
  items: Array<{
    connector_id: string;
    connection_id: string;
    accounts: Array<{ id: string; label: string }>;
    verified_at: string;
  }>;
}

export interface SocialOperationsImageUploadResponse {
  artifact: {
    id: string;
    organization_id: string;
    workspace_id: string;
    owner_id: string;
    employee_id: string;
    digest: string;
    mime: string;
    size: number;
    created_at: string;
  };
}

export interface SaveSocialOperationsConfigurationRequest {
  expected_revision: number;
  configuration: SocialOperationsSettings;
}

export type SocialOperationsCapabilityStatus =
  | 'ready'
  | 'missing'
  | 'unsupported'
  | 'optional_skipped';

export interface SocialOperationsCapabilityFinding {
  capability: SocialCapability;
  status: SocialOperationsCapabilityStatus;
  required: boolean;
  required_operations: string[];
  missing_operations?: string[];
  reason: string;
}

export interface SocialOperationsPreflightReport {
  scope: SocialOperationsScope;
  published_employee_version_id: string;
  configuration_revision: number;
  ready: boolean;
  prerequisites?: Array<{ key: string; ready: boolean; reason: string }>;
  platforms: Array<{
    platform: SocialPlatform;
    connector_id: string;
    connection_id: string;
    account_id: string;
    ready: boolean;
    reason: string;
    findings: SocialOperationsCapabilityFinding[];
  }>;
  handoff: SocialOperationsTeamHandoff & { ready: boolean; reason: string };
}

export interface SocialOperationsRunBudget {
  max_pages: number;
  max_records: number;
  max_connector_calls: number;
  max_tokens: number;
  max_safe_read_attempts: number;
}

export interface SocialOperationsPlannedTask {
  key: string;
  stages: string[];
  budget: SocialOperationsRunBudget;
  consumes_record_kinds: string[];
  produces_record_kinds: string[];
  updates_records: Array<{ kind: string; key: string }>;
  request: CreateDigitalHumanTaskRequest;
}

export interface SocialOperationsSetupPlan {
  scope: SocialOperationsScope;
  employee_version_id: string;
  configuration_revision: number;
  tasks: SocialOperationsPlannedTask[];
}

export interface SocialOperationsProvisioningResult {
  configuration_record: SocialOperationsConfigurationRecord;
  preflight: SocialOperationsPreflightReport;
  plan: SocialOperationsSetupPlan;
  tasks: Array<{ key: string; operation_id: string; task: DigitalHumanTask }>;
}

export interface SetupSocialOperationsRequest {
  expected_configuration_revision: number;
  expected_employee_version_id: string;
}
