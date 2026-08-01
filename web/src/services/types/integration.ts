export type IntegrationCredentialSource = 'platform' | 'organization' | 'account';
export type IntegrationAuthType = 'platform' | 'api_key' | 'oauth' | 'oauth2' | (string & {});
export type IntegrationAuthIdentityKind = 'user' | 'application' | 'channel' | 'service';
export type IntegrationAuthAcquisitionStrategy = 'browser_redirect' | 'manual_form' | 'none';
export type IntegrationAuthLifecycleStrategy =
  | 'static'
  | 'oauth_refresh'
  | 'exchange_on_demand'
  | 'signed_request';
export type IntegrationRequestAuthStrategy =
  | 'none'
  | 'bearer_header'
  | 'api_key_header'
  | 'api_key_query'
  | 'basic_header'
  | 'oauth1_signature'
  | 'webhook_url'
  | 'provider_custom';
export type IntegrationConnectionStatus = 'pending' | 'active' | 'invalid' | 'disabled';
export type IntegrationApprovalPolicy = 'inherit' | 'always_ask';
export type IntegrationLocalizedText = Record<string, string>;
export type IntegrationLocalizedLabelMap = Record<string, IntegrationLocalizedText>;

export type IntegrationCredentialFieldType =
  | 'text'
  | 'password'
  | 'textarea'
  | 'select'
  | 'boolean';

export interface IntegrationCredentialFieldOption {
  label: string;
  label_i18n?: IntegrationLocalizedText;
  value: string;
}

export interface IntegrationProviderCredentialField {
  key: string;
  label?: string;
  label_i18n?: IntegrationLocalizedText;
  description?: string;
  description_i18n?: IntegrationLocalizedText;
  placeholder?: string;
  placeholder_i18n?: IntegrationLocalizedText;
  input?: 'text' | 'password' | 'textarea' | 'select' | 'url';
  required?: boolean;
  secret?: boolean;
  options?: IntegrationCredentialFieldOption[];
}

/**
 * A normalized field descriptor. Providers may return this compact shape, or
 * a JSON Schema object through `credential_schema`; the UI supports both.
 */
export interface IntegrationCredentialField {
  name: string;
  label?: string;
  label_i18n?: IntegrationLocalizedText;
  description?: string;
  description_i18n?: IntegrationLocalizedText;
  placeholder?: string;
  placeholder_i18n?: IntegrationLocalizedText;
  type?: IntegrationCredentialFieldType;
  required?: boolean;
  secret?: boolean;
  default_value?: string | boolean;
  options?: IntegrationCredentialFieldOption[];
  storage?: 'credentials' | 'config';
}

export interface IntegrationJSONSchemaProperty {
  type?: 'string' | 'boolean' | 'number' | 'integer';
  title?: string;
  title_i18n?: IntegrationLocalizedText;
  description?: string;
  description_i18n?: IntegrationLocalizedText;
  format?: string;
  writeOnly?: boolean;
  default?: string | number | boolean;
  enum?: Array<string | number | boolean>;
  enumNames?: string[];
  'x-secret'?: boolean;
  'x-storage'?: 'credentials' | 'config';
  'x-placeholder'?: string;
  'x-placeholder-i18n'?: IntegrationLocalizedText;
}

export interface IntegrationCredentialSchema {
  type?: 'object';
  fields?: IntegrationCredentialField[];
  properties?: Record<string, IntegrationJSONSchemaProperty>;
  required?: string[];
}

export type IntegrationAuthSetupStepAction =
  | 'open_console'
  | 'open_documentation'
  | 'copy_callback_url';

export type IntegrationAuthSetupNoticeLevel = 'info' | 'warning';

export interface IntegrationAuthSetupStep {
  id: string;
  title: string;
  title_i18n?: IntegrationLocalizedText;
  description?: string;
  description_i18n?: IntegrationLocalizedText;
  action?: IntegrationAuthSetupStepAction;
}

export interface IntegrationAuthSetupNotice {
  id: string;
  level: IntegrationAuthSetupNoticeLevel;
  text: string;
  text_i18n?: IntegrationLocalizedText;
}

export interface IntegrationAuthSetupGuide {
  console_url?: string;
  documentation_url?: string;
  steps?: IntegrationAuthSetupStep[];
  notices?: IntegrationAuthSetupNotice[];
}

export interface IntegrationAuthDefinition {
  id: string;
  type: IntegrationAuthType;
  label: string;
  label_i18n?: IntegrationLocalizedText;
  description?: string;
  description_i18n?: IntegrationLocalizedText;
  available: boolean;
  credential_source?: IntegrationCredentialSource;
  fields?: IntegrationProviderCredentialField[];
  credential_schema?: IntegrationCredentialSchema;
  oauth?: IntegrationOAuthAuthMetadata;
  setup_guide?: IntegrationAuthSetupGuide;
  /**
   * Provider-declared, composable authentication behavior. These fields are
   * optional while older provider catalogs are rolling forward; the UI uses
   * the method type and credential owner as a conservative compatibility
   * fallback and never branches on a provider ID.
   */
  identity_kind?: IntegrationAuthIdentityKind;
  acquisition_strategy?: IntegrationAuthAcquisitionStrategy;
  lifecycle_strategy?: IntegrationAuthLifecycleStrategy;
  request_auth_strategy?: IntegrationRequestAuthStrategy;
}

/**
 * Browser-safe OAuth capability metadata. Provider endpoints and client
 * secrets remain server-side; the catalog only reports which flows are ready.
 */
export interface IntegrationOAuthAuthMetadata {
  connect_enabled: boolean;
  reconnect_enabled?: boolean;
  scope_upgrade_enabled?: boolean;
  client_configured: boolean;
  /**
   * Stable provider-level OAuth application configuration. Personal and
   * organization auth methods may intentionally share this identifier.
   */
  client_config_id?: string;
  client_config_source?: 'deployment' | 'organization' | 'none';
  provider_setup_url?: string;
  default_action_ids?: string[];
  client_fields?: Array<IntegrationCredentialField | IntegrationProviderCredentialField>;
}

export type IntegrationOAuthFlowIntent = 'connect' | 'reconnect' | 'scope_upgrade';
export type IntegrationOAuthFlowStatus =
  | 'pending'
  | 'authorizing'
  | 'exchanging'
  | 'succeeded'
  | 'failed'
  | 'expired'
  | 'cancelled';

export interface StartIntegrationOAuthFlowRequest {
  integration_id: string;
  auth_method_id: string;
  credential_source: Extract<IntegrationCredentialSource, 'organization' | 'account'>;
  intent: IntegrationOAuthFlowIntent;
  connection_name?: string;
  /**
   * Internal reference for reconnect/upgrade requests. It is never rendered,
   * copied into popup messages, or returned as reader-facing text.
   */
  connection_id?: string;
  requested_action_ids?: string[];
  return_path?: string;
}

export interface IntegrationOAuthFlow {
  /** Opaque public reference. Neither value may be the database UUID. */
  flow_ref?: string;
  flow_id?: string;
  authorization_url?: string;
  status: IntegrationOAuthFlowStatus;
  integration_id?: string;
  auth_method_id?: string;
  credential_source?: Extract<IntegrationCredentialSource, 'organization' | 'account'>;
  intent?: IntegrationOAuthFlowIntent;
  expires_at: string;
  next_poll_after_ms?: number;
  error_code?: string | null;
  retryable?: boolean;
  connection_name?: string | null;
  usage_rules_required?: boolean;
  ai_chat_available?: boolean;
}

export interface IntegrationOAuthFlowStartResponse extends IntegrationOAuthFlow {
  authorization_url: string;
}

export interface IntegrationOAuthClientConfig {
  integration_id: string;
  auth_method_id: string;
  configured: boolean;
  source: 'deployment' | 'organization' | 'none';
  revision?: number;
  client_id_masked?: string | null;
  has_client_secret: boolean;
  callback_url?: string | null;
  provider_setup_url?: string | null;
  config?: Record<string, unknown>;
  updated_at?: string | null;
}

export interface IntegrationOAuthClientConfigImpact {
  dependent_connections: number;
  active_connections: number;
  pending_flows: number;
  pending_revocations: number;
  can_remove: boolean;
}

export type IntegrationOAuthRecoveryResolutionCode =
  | 'provider_access_removed'
  | 'token_confirmed_expired';

/**
 * Secret-free administrator remediation item. `operation_ref` is an opaque
 * API handle used only when acknowledging the item and must never be rendered.
 */
export interface IntegrationOAuthRecoveryOperation {
  operation_ref: string;
  integration_id: string;
  auth_method_id: string;
  reason_code: string;
  attempts: number;
  created_at: string;
  failed_at: string;
}

export interface IntegrationOAuthRecoveryStatus {
  pending_revocations: number;
  manual_action_required: number;
  failed_revocations: number;
  unresolved_dead_letters: number;
  remediation_operations: IntegrationOAuthRecoveryOperation[];
}

export interface AcknowledgeIntegrationOAuthRecoveryRequest {
  resolution_code: IntegrationOAuthRecoveryResolutionCode;
}

export interface AcknowledgeIntegrationOAuthRecoveryResponse {
  acknowledged: boolean;
}

export interface UpdateIntegrationOAuthClientConfigRequest {
  revision?: number;
  client_id?: string;
  client_secret?: string;
  config?: Record<string, unknown>;
}

export interface IntegrationActionDefinition {
  id: string;
  tool_name?: string;
  name: string;
  name_i18n?: IntegrationLocalizedText;
  description?: string;
  description_i18n?: IntegrationLocalizedText;
  effect?: string;
  risk_level?: string;
  data_egress?: boolean;
  external_destination?: string;
  required_scopes?: string[];
  required_any_scopes?: string[];
  preferred_scopes?: string[];
  /**
   * Restricts an action to explicit provider authentication methods. Missing
   * or empty means the action is compatible with every method.
   */
  supported_auth_method_ids?: string[];
  scope_labels_i18n?: IntegrationLocalizedLabelMap;
  supported_callers?: Array<'aichat' | 'agent' | 'workflow' | 'api' | (string & {})>;
  preparation_hints?: IntegrationActionPreparationHint[];
  default_policy?: {
    enabled: boolean;
    approval_policy?: IntegrationApprovalPolicy | string;
    data_egress_allowed: boolean;
  };
}

export interface IntegrationActionPreparationHint {
  action_id: string;
  relation: 'resolve_target' | 'inspect' | (string & {});
  target_arguments?: string[];
  result_paths?: string[];
  description: string;
  description_i18n?: IntegrationLocalizedText;
}

export type IntegrationProviderScopeCategory = 'identity' | 'lifecycle' | 'provider' | 'internal';
export type IntegrationProviderScopeAccess =
  | 'unknown'
  | 'read'
  | 'write'
  | 'manage'
  | 'identity'
  | 'session';

export interface IntegrationProviderScopeDefinition {
  id: string;
  label: string;
  label_i18n?: IntegrationLocalizedText;
  description?: string;
  description_i18n?: IntegrationLocalizedText;
  category: IntegrationProviderScopeCategory;
  access: IntegrationProviderScopeAccess;
  broad: boolean;
}

export type IntegrationCapabilityAvailability =
  | 'available'
  | 'needs_connection'
  | 'needs_scope'
  | 'needs_permission'
  | 'disabled_by_policy'
  | 'data_egress_blocked';

export interface IntegrationActionCapability extends IntegrationActionDefinition {
  enabled: boolean;
  approval_policy: IntegrationApprovalPolicy;
  data_egress_allowed: boolean;
  availability: IntegrationCapabilityAvailability;
  compatible_connection_count: number;
}

export interface IntegrationProviderCapabilitySummary {
  total: number;
  read: number;
  write: number;
  available: number;
  needs_attention: number;
}

export interface IntegrationProviderCapabilities {
  integration_id: string;
  summary: IntegrationProviderCapabilitySummary;
  actions: IntegrationActionCapability[];
}

export interface IntegrationHealthProbeDefinition {
  supported: boolean;
  may_incur_cost: boolean;
  description?: string;
  description_i18n?: IntegrationLocalizedText;
}

export type IntegrationProviderHealthState =
  | 'ready'
  | 'configured'
  | 'setup_required'
  | 'degraded'
  | 'unavailable'
  | 'unknown';

export interface IntegrationCatalogConnectionSummary {
  total?: number;
  active?: number;
  invalid?: number;
  disabled?: number;
  healthy?: number;
  degraded?: number;
  unhealthy?: number;
  unknown?: number;
  auth_required?: number;
  scope_drifted?: number;
  default_connection_id?: string | null;
}

export interface IntegrationCatalogItem {
  id: string;
  integration_id?: string;
  driver_id: string;
  name: string;
  name_i18n?: IntegrationLocalizedText;
  description?: string;
  description_i18n?: IntegrationLocalizedText;
  author?: string;
  category?: string;
  categories?: string[];
  category_labels_i18n?: IntegrationLocalizedLabelMap;
  icon?: string;
  tags?: string[];
  tag_labels_i18n?: IntegrationLocalizedLabelMap;
  docs_url?: string;
  documentation_url?: string;
  documentation_url_i18n?: Record<string, string>;
  enabled: boolean;
  catalog_revision?: string;
  auth?: IntegrationAuthDefinition[];
  health_probe?: IntegrationHealthProbeDefinition;
  scopes?: IntegrationProviderScopeDefinition[];
  credential_schema?: IntegrationCredentialSchema;
  credential_sources?: IntegrationCredentialSource[];
  auth_types?: IntegrationAuthType[];
  platform_credentials_configured?: boolean;
  connection_test_may_incur_cost?: boolean;
  connection_summary?: IntegrationCatalogConnectionSummary;
  health_state?: IntegrationProviderHealthState;
  actions: IntegrationActionDefinition[];
}

export interface IntegrationCatalogResponse {
  data?: IntegrationCatalogItem[];
  items?: IntegrationCatalogItem[];
}

export type IntegrationConnectionHealthState =
  | 'ready'
  | 'testing'
  | 'degraded'
  | 'expired'
  | 'revoked'
  | 'error'
  | 'disabled'
  | 'unknown';

export interface IntegrationConnectionCapabilityPermission {
  action_id: string;
  name: string;
  name_i18n?: IntegrationLocalizedText;
  description?: string;
  description_i18n?: IntegrationLocalizedText;
  effect: string;
  risk_level: string;
  availability?: 'ready' | 'scope_upgrade_required' | 'permission_missing';
  can_upgrade?: boolean;
  scope_satisfied: boolean;
  required_scopes?: string[];
  required_any_scopes?: string[];
  preferred_scopes?: string[];
  missing_scope_ids?: string[];
}

export interface IntegrationConnectionProviderPermission {
  id: string;
  label: string;
  label_i18n?: IntegrationLocalizedText;
  description?: string;
  description_i18n?: IntegrationLocalizedText;
  category: IntegrationProviderScopeCategory | '';
  access: IntegrationProviderScopeAccess;
  broad: boolean;
  known: boolean;
}

export interface IntegrationConnectionPermissionSummary {
  adapted_capabilities: IntegrationConnectionCapabilityPermission[] | null;
  identity_permissions: IntegrationConnectionProviderPermission[] | null;
  lifecycle_permissions: IntegrationConnectionProviderPermission[] | null;
  provider_permissions: IntegrationConnectionProviderPermission[] | null;
  unknown_permissions: IntegrationConnectionProviderPermission[] | null;
  missing_permissions: IntegrationConnectionProviderPermission[] | null;
  provider_scopes_reported: boolean;
  has_broad_permissions: boolean;
}

export interface IntegrationConnection {
  id: string;
  organization_id?: string;
  integration_id: string;
  driver_id: string;
  name: string;
  credential_source: IntegrationCredentialSource;
  auth_type: IntegrationAuthType;
  auth_method_id: string;
  owner_account_id?: string | null;
  credential_configured?: boolean;
  config?: Record<string, unknown>;
  status: IntegrationConnectionStatus;
  health_status: 'healthy' | 'degraded' | 'unhealthy' | 'unknown';
  auth_status: 'valid' | 'reconnect_required' | 'expired' | 'unknown';
  scope_status: 'verified' | 'drifted' | 'unknown';
  attention_code?:
    | 'reconnect_required'
    | 'scope_update_required'
    | 'billing_required'
    | 'provider_incident'
    | 'admin_check_required'
    | null;
  missing_required_scopes?: string[] | null;
  is_default: boolean;
  account_id?: string | null;
  display_name?: string | null;
  granted_scopes?: string[] | null;
  permission_summary?: IntegrationConnectionPermissionSummary | null;
  credential_version?: number;
  revision?: number;
  last_tested_at?: string | null;
  last_health_checked_at?: string | null;
  last_healthy_at?: string | null;
  last_runtime_success_at?: string | null;
  last_runtime_failure_at?: string | null;
  scope_checked_at?: string | null;
  consecutive_failures?: number;
  health_revision?: number;
  token_expires_at?: string | null;
  refresh_token_expires_at?: string | null;
  next_token_refresh_at?: string | null;
  last_error_code?: string | null;
  expires_at?: string | null;
  created_at?: string;
  updated_at?: string;
}

export interface IntegrationConnectionListResponse {
  data?: IntegrationConnection[];
  items?: IntegrationConnection[];
  page?: number;
  limit?: number;
  page_size?: number;
  total?: number;
  has_more?: boolean;
}

/**
 * Per-account, per-workspace routing preference for AIChat. Preferences never
 * grant access: the API rechecks connection grants and health before every use.
 */
export interface AIChatIntegrationPreference {
  id?: string;
  organization_id?: string;
  account_id?: string;
  workspace_id?: string | null;
  integration_id: string;
  selected_connection_ids: string[];
  preferred_connection_id: string;
  revision?: number;
  created_at?: string;
  updated_at?: string;
}

export interface AIChatIntegrationPreferenceListResponse {
  items: AIChatIntegrationPreference[];
}

export interface AIChatIntegrationPreferenceInput {
  integration_id: string;
  selected_connection_ids: string[];
  preferred_connection_id: string;
}

export interface ReplaceAIChatIntegrationPreferencesRequest {
  items: AIChatIntegrationPreferenceInput[];
}

export type IntegrationConnectionGrantPrincipalType = 'organization' | 'workspace' | 'account';
export type IntegrationConnectionGrantAccessMode = 'read' | 'write';

/**
 * Secret-free connection authorization returned by the management API.
 * Raw resource constraints are deliberately omitted: the UI does not offer an
 * editor for that policy dimension and must not silently overwrite it. The API
 * exposes capability flags so constrained grants remain visibly read-only.
 */
export interface IntegrationConnectionGrant {
  id: string;
  organization_id: string;
  connection_id: string;
  principal_type: IntegrationConnectionGrantPrincipalType;
  principal_id?: string | null;
  principal_display_name?: string | null;
  principal_state?: 'active' | 'missing';
  has_resource_constraints: boolean;
  editable: boolean;
  access_mode: IntegrationConnectionGrantAccessMode;
  allowed_action_ids: string[];
  revision: number;
  created_by?: string | null;
  updated_by?: string | null;
  created_at: string;
  updated_at: string;
}

export interface IntegrationConnectionGrantListResponse {
  items: IntegrationConnectionGrant[];
}

export interface SaveIntegrationConnectionGrantRequest {
  revision: number;
  principal_type: IntegrationConnectionGrantPrincipalType;
  principal_id?: string | null;
  access_mode: IntegrationConnectionGrantAccessMode;
  allowed_action_ids: string[];
}

export type IntegrationConnectionHealthSource =
  | 'manual'
  | 'scheduled'
  | 'runtime'
  | 'oauth_refresh';
export type IntegrationConnectionHealthCheckKind = 'full' | 'auth' | 'scope' | 'passive';
export type IntegrationConnectionHealthClassification =
  | 'success'
  | 'auth_invalid'
  | 'oauth_expired'
  | 'scope_drift'
  | 'access_denied'
  | 'budget_exhausted'
  | 'rate_limited'
  | 'transient'
  | 'provider_incident'
  | 'ignored';
export type IntegrationConnectionHealthStatus = 'healthy' | 'degraded' | 'unhealthy' | 'unknown';
export type IntegrationConnectionAuthStatus =
  | 'valid'
  | 'reconnect_required'
  | 'expired'
  | 'unknown';
export type IntegrationConnectionScopeStatus = 'verified' | 'drifted' | 'unknown';

export interface IntegrationConnectionHealthEvent {
  id: string;
  organization_id: string;
  connection_id: string;
  integration_id: string;
  driver_id: string;
  source: IntegrationConnectionHealthSource;
  check_kind: IntegrationConnectionHealthCheckKind;
  classification: IntegrationConnectionHealthClassification;
  reason_code?: string | null;
  health_status_after: IntegrationConnectionHealthStatus;
  auth_status_after: IntegrationConnectionAuthStatus;
  scope_status_after: IntegrationConnectionScopeStatus;
  attention_code_after?: string | null;
  credential_version: number;
  health_revision: number;
  execution_id?: string | null;
  actor_id?: string | null;
  provider_request_id?: string | null;
  provider_error_code?: string | null;
  provider_http_status?: number | null;
  latency_ms: number;
  retry_after_at?: string | null;
  granted_scopes?: string[];
  added_scopes?: string[];
  removed_scopes?: string[];
  missing_scopes?: string[];
  applied: boolean;
  observed_at: string;
  created_at: string;
}

export interface IntegrationConnectionHealthEventListResponse {
  items: IntegrationConnectionHealthEvent[];
  page: number;
  page_size: number;
  total: number;
  has_more: boolean;
}

export interface CreateIntegrationConnectionRequest {
  integration_id: string;
  driver_id: string;
  name: string;
  credential_source: IntegrationCredentialSource;
  auth_type: IntegrationAuthType;
  auth_method_id: string;
  credentials?: Record<string, string>;
  config?: Record<string, unknown>;
  expires_at?: string | null;
}

export interface UpdateIntegrationConnectionRequest {
  revision: number;
  name?: string;
  disabled?: boolean;
  credentials?: Record<string, string>;
  config?: Record<string, unknown>;
  expires_at?: string | null;
  clear_expires_at?: boolean;
}

export interface IntegrationConnectionTestResult {
  connection: IntegrationConnection;
  profile?: Record<string, unknown> | null;
  may_incur_cost: boolean;
}

export interface IntegrationConnectionDeleteImpact {
  connection_id: string;
  bound_agent_count: number;
  can_delete: boolean;
}

export interface IntegrationActionPolicy {
  integration_id: string;
  action_id: string;
  enabled: boolean;
  approval_policy: IntegrationApprovalPolicy;
  data_egress_allowed: boolean;
  name?: string;
  effect?: string;
  risk_level?: string;
  data_egress?: boolean;
  external_destination?: string;
  updated_at?: string;
}

export interface IntegrationActionPolicyResponse {
  revision: string;
  integration_id?: string;
  items: IntegrationActionPolicy[];
  policies?: IntegrationActionPolicy[];
}

export interface UpdateIntegrationActionPoliciesRequest {
  revision: string;
  policies: Array<
    Pick<
      IntegrationActionPolicy,
      'action_id' | 'enabled' | 'approval_policy' | 'data_egress_allowed'
    >
  >;
}

export type IntegrationExecutionStatus = 'running' | 'succeeded' | 'failed' | 'timed_out';

export interface IntegrationExecution {
  id: string;
  organization_id?: string;
  workspace_id?: string | null;
  account_id?: string | null;
  app_id?: string | null;
  conversation_id?: string | null;
  message_id?: string | null;
  integration_id: string;
  driver_id: string;
  action_id: string;
  connection_id?: string | null;
  invoke_from: string;
  status: IntegrationExecutionStatus;
  provider_request_id?: string | null;
  provider_error_code?: string | null;
  provider_http_status?: number | null;
  retry_after_at?: string | null;
  duration_ms?: number;
  cost_usd?: number | null;
  result_count?: number;
  attempt_count?: number;
  error_code?: string | null;
  created_at: string;
  updated_at?: string;
}

export interface IntegrationExecutionListResponse {
  data?: IntegrationExecution[];
  items?: IntegrationExecution[];
  page?: number;
  limit?: number;
  page_size?: number;
  total?: number;
  has_more?: boolean;
}

export interface GetIntegrationExecutionsParams {
  page?: number;
  limit?: number;
  integration_id?: string;
  connection_id?: string;
  action_id?: string;
  status?: IntegrationExecutionStatus;
}

export interface AgentIntegrationConnectionBinding {
  connection_id: string;
  integration_id: string;
  access_mode: 'read' | 'write';
  allowed_action_ids: string[];
}

export interface AgentIntegrationConnectionCandidate {
  connection_id: string;
  integration_id: string;
  driver_id: string;
  auth_method_id?: string;
  name: string;
  status: IntegrationConnectionStatus;
  credential_source: IntegrationCredentialSource;
  is_default: boolean;
  selected?: boolean;
  access_mode?: 'read' | 'write';
  allowed_action_ids?: string[];
  available_access_mode?: 'read' | 'write';
  available_action_ids: string[];
  updated_at?: number;
}

export interface AgentIntegrationConnectionCandidatesResponse {
  agent_id?: string;
  workspace_id?: string;
  query?: string;
  integration_id?: string;
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
  include_selected?: boolean;
  count?: number;
  data: AgentIntegrationConnectionCandidate[];
}
