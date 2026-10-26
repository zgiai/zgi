export type RuntimeWorkspaceStatus = 'active' | 'disabled';
export type RuntimeCredentialStatus = 'active' | 'revoked';
export type RuntimeAgentBindingStatus = 'active' | 'disabled';

export interface RuntimeCatalogTag {
  id: string;
  name: string;
}

export interface RuntimeWorkspace {
  runtime_id: string;
  workspace_id: string;
  organization_id: string;
  managed_by_account_id: string;
  status: RuntimeWorkspaceStatus;
  mode: 'attached' | 'isolated';
  policy_json: string;
  created_at: string;
  updated_at: string;
}

export interface RuntimeCredential {
  id: string;
  runtime_workspace_id: string;
  key_prefix: string;
  name: string;
  status: RuntimeCredentialStatus;
  expires_at?: string | null;
  usage_count: number;
  last_used_at?: string | null;
  revoked_at?: string | null;
  created_by_account_id: string;
  /** What the key may call; absent means everything. */
  scopes?: RuntimeCredentialScope[];
  /** The apps the key may call; absent means every app. */
  app_keys?: string[];
  created_at: string;
  updated_at: string;
}

/** What one DigitalHuman keeps for an end user (ADR 0006). */
export interface RuntimeEndUserDigitalHuman {
  app_key: string;
  digital_human_id: string;
  name: string;
  instance?: {
    id: string;
    status: string;
    first_seen_at: string;
    last_active_at: string;
  };
  memory_count: number;
  memory: Array<{
    id: string;
    memory_key: string;
    memory_type: string;
    content: string;
    updated_at: string;
  }>;
  task_count: number;
  tasks: Array<{
    id: string;
    name: string;
    status: string;
    work_status?: string | null;
    schedule_type: string;
    created_at: string;
    last_run_at?: string | null;
  }>;
  connections: Array<{ slot_key: string; name: string; state: string }>;
}

export type RuntimeCredentialScope = 'runtime' | 'end_users.read' | 'end_users.manage';

/** An end-user export job; the JSON is downloaded once it has succeeded. */
export interface RuntimeEndUserExportJob {
  id: string;
  status: 'pending' | 'running' | 'succeeded' | 'failed';
  size_bytes: number;
  error?: string;
  created_at: string;
  finished_at?: string | null;
  expires_at: string;
}

/** What a Runtime Workspace app is: an Agent or a DigitalHuman. */
export type RuntimeAppType = 'agent' | 'digital_human';

export interface RuntimeApp {
  app_key: string;
  agent_id: string;
  app_type?: RuntimeAppType;
  workspace_id: string;
  workspace_name: string;
  name: string;
  description?: string;
  icon?: string;
  icon_type?: 'text' | 'image';
  icon_url?: string;
  version: string;
  version_uuid: string;
  published_at: string;
  status: RuntimeAgentBindingStatus;
  supports_stream: boolean;
  file_upload_enabled: boolean;
  supports_vision: boolean;
  agent_memory_enabled: boolean;
  capabilities: string[];
  tags: RuntimeCatalogTag[];
  callable?: boolean;
  unavailable_reason?: string;
}

export interface RuntimeSourceWorkspace {
  id: string;
  name: string;
  current: boolean;
}

export interface RuntimeAgentCandidateSet {
  scope: 'current' | 'authorized';
  cross_workspace_enabled: boolean;
  source_workspaces: RuntimeSourceWorkspace[];
  items: RuntimeAgentCandidate[];
  /** Set when candidates are paged. */
  page?: number;
  has_more?: boolean;
}

export interface RuntimeAgentCandidate {
  agent_id: string;
  app_type?: RuntimeAppType;
  workspace_id: string;
  workspace_name: string;
  name: string;
  description?: string;
  icon?: string;
  icon_type?: 'text' | 'image';
  icon_url?: string;
  version: string;
  version_uuid: string;
  published_at: string;
  file_upload_enabled: boolean;
  supports_vision: boolean;
  agent_memory_enabled: boolean;
  capabilities: string[];
  tags: RuntimeCatalogTag[];
}

export interface RuntimeCredentialCreation {
  credential: RuntimeCredential;
  secret: string;
}

export interface RuntimeWorkspaceCredentialInput {
  name?: string;
  expires_at?: string;
  scopes?: RuntimeCredentialScope[];
  app_keys?: string[];
  idempotency_key?: string;
}

export interface RuntimeAppCollection {
  revision: number;
  /** How many apps the external access may expose. */
  limit?: number;
  items: RuntimeApp[];
}

/** A bound app with its usage over the last 30 days and its per-app caps. */
export interface RuntimeManagedApp extends RuntimeApp {
  usage_30d: { points: number; calls: number; end_users: number };
  quota_caps: RuntimeQuotaLimit[];
}

export interface RuntimeManagedAppPage {
  revision: number;
  limit: number;
  counts: { all: number; agents: number; digital_humans: number; unavailable: number };
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
  items: RuntimeManagedApp[];
}

export interface RuntimeAppListParams {
  search?: string;
  type?: RuntimeAppType | '';
  state?: 'callable' | 'unavailable' | '';
  page?: number;
  limit?: number;
}

export interface RuntimeCandidateParams {
  scope: 'current' | 'authorized';
  sourceWorkspaceId?: string;
  search?: string;
  type?: RuntimeAppType | '';
}

/** Active keys, with how many are revoked or expired and the active-key limit. */
export interface RuntimeCredentialList {
  items: RuntimeCredential[];
  inactive: number;
  limit: number;
}

/** A page of a newest-first list. */
export interface RuntimePage<T> {
  items: T[];
  page: number;
  limit: number;
  has_more: boolean;
}

export interface RuntimeWorkspaceAgentSelectionInput {
  expected_revision: number;
  agent_ids: string[];
  digital_human_ids?: string[];
}

export type RuntimeQuotaWindow = '24h' | '7d' | '30d';
export type RuntimeEndUserStatus = 'active' | 'suspended' | 'erased';

export interface RuntimeQuotaLimit {
  window: RuntimeQuotaWindow;
  points: number;
}

export interface RuntimeEndUserQuota {
  unlimited?: boolean;
  limits?: RuntimeQuotaLimit[];
}

export interface RuntimeWindowUsage {
  window: RuntimeQuotaWindow;
  points: number;
  limit?: number;
  remaining?: number;
}

export interface RuntimeEndUserAppUsage {
  app_id: string;
  app_type?: string;
  app_key?: string;
  usage: RuntimeWindowUsage[];
}

export interface RuntimeEndUser {
  subject_id: string;
  external_user_id: string;
  status: RuntimeEndUserStatus;
  first_seen_at: string;
  last_seen_at: string;
  suspended_at?: string;
  quota_source: 'default' | 'plan' | 'custom' | 'unlimited';
  quota: RuntimeEndUserQuota;
  /** The plan held; active is false once it expired or was removed. */
  plan?: { key: string; name?: string; expires_at?: string; active: boolean };
  /** Present once the end user has been granted credits (ADR 0011). */
  credits?: RuntimeCreditBalance;
  usage: RuntimeWindowUsage[];
  apps: RuntimeEndUserAppUsage[];
}

export interface RuntimeEndUserList {
  items: RuntimeEndUser[];
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
}

export type RuntimeEndUserQuotaState = 'over' | 'near' | 'custom';
export type RuntimeEndUserSort = 'last_seen' | 'usage_24h' | 'usage_7d' | 'usage_30d' | 'quota';

export interface RuntimeEndUserListParams {
  search?: string;
  status?: RuntimeEndUserStatus | '';
  quota_state?: RuntimeEndUserQuotaState | '';
  /** A plan key, or 'none' for end users without an applying plan. */
  plan?: string;
  sort?: RuntimeEndUserSort;
  order?: 'asc' | 'desc';
  page?: number;
  limit?: number;
}

export interface RuntimeEndUserSummary {
  usage: Array<{
    window: RuntimeQuotaWindow;
    points: number;
    /** The same window just before, for comparison. */
    previous_points: number;
    end_users: number;
  }>;
  end_users: {
    total: number;
    active_7d: number;
    suspended: number;
    erased: number;
    custom_quota: number;
    over_quota: number;
    near_quota: number;
  };
  plans: Array<{ key: string; name?: string; end_users: number }>;
  top: RuntimeEndUser[];
}

export interface RuntimeCreditBalance {
  balance: number;
  debt?: number;
  lots?: Array<{ points: number; expires_at?: string }>;
}

export interface RuntimeCreditEntry {
  id: string;
  kind: 'grant' | 'usage';
  points: number;
  balance_after: number;
  external_id?: string;
  reason?: string;
  expires_at?: string;
  actor_kind?: 'account' | 'credential';
  created_at: string;
}

export interface RuntimeCreditLedger {
  credits: RuntimeCreditBalance;
  items: RuntimeCreditEntry[];
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
}

/** A negative amount takes credits back; external_id makes the grant idempotent. */
export interface RuntimeCreditGrant {
  points: number;
  external_id: string;
  reason?: string;
  expires_at?: string;
}

export interface RuntimeCreditGrantResult {
  entry: RuntimeCreditEntry;
  credits: RuntimeCreditBalance;
  duplicate: boolean;
}

export type RuntimeWebhookEvent = 'quota.near' | 'quota.exceeded' | 'credits.low' | 'plan.expired';

export interface RuntimeWebhook {
  configured: boolean;
  url?: string;
  events: RuntimeWebhookEvent[];
  credits_low_threshold: number;
  enabled: boolean;
  updated_at?: string;
}

export interface RuntimeWebhookInput {
  url: string;
  events: RuntimeWebhookEvent[];
  credits_low_threshold: number;
  enabled: boolean;
}

/** secret is present once, when the webhook is created. */
export interface RuntimeWebhookSaved {
  webhook: RuntimeWebhook;
  secret?: string;
}

export interface RuntimeWebhookDelivery {
  id: string;
  event: RuntimeWebhookEvent | 'webhook.test';
  status: 'pending' | 'delivered' | 'failed';
  attempts: number;
  last_status?: number;
  last_error?: string;
  next_attempt_at?: string;
  created_at: string;
  delivered_at?: string;
}

/** A plan of the workspace quota policy (ADR 0011). */
export interface RuntimeEndUserPlan {
  key: string;
  name?: string;
  limits: RuntimeQuotaLimit[];
  /** Replaces the per-app caps when present; empty lifts them. */
  apps?: Array<{ app_key: string; limits: RuntimeQuotaLimit[] }>;
}

export interface RuntimeUsageDay {
  date: string;
  points: number;
  /** Distinct end users with usage that day. */
  end_users: number;
  /** Metered model calls that day. */
  calls: number;
  apps: Record<string, number>;
}

export interface RuntimeUsageSeries {
  days: RuntimeUsageDay[];
  apps: Array<{ app_id: string; app_type?: string; app_key?: string }>;
}

export interface RuntimeEndUserBatchResult {
  updated: number;
  failed: string[];
}

/** A null quota returns the end user to the workspace limits; a null plan removes it. */
export interface RuntimeEndUserPatch {
  status?: 'active' | 'suspended';
  quota?: RuntimeEndUserQuota | null;
  plan?: { key: string; expires_at?: string } | null;
}

export interface RuntimeEndUserEvent {
  id: string;
  subject_id: string;
  action:
    | 'suspended'
    | 'resumed'
    | 'quota_set'
    | 'quota_reset'
    | 'plan_set'
    | 'plan_cleared'
    | 'credits_adjusted'
    | 'exported'
    | 'erased';
  actor_kind: 'account' | 'credential';
  actor_id?: string;
  details: Record<string, unknown>;
  created_at: string;
}
