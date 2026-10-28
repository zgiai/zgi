export type ConnectorClassification = 'connector' | 'data_source';
export type ConnectorCategory =
  | 'communication'
  | 'productivity'
  | 'information'
  | 'developer'
  | 'other';
export type ConnectorTag =
  | 'social'
  | 'messaging'
  | 'email'
  | 'enterprise'
  | 'collaboration'
  | 'documents'
  | 'tables'
  | 'workflow'
  | 'search'
  | 'web'
  | 'research'
  | 'code'
  | 'repository'
  | 'issues'
  | 'integration';
export type ConnectorReadiness = 'ready' | 'reconnect_required' | 'unavailable';
export type ConnectorCatalogStatus = 'all' | 'connected' | 'needs_attention';
export type ConnectorTargetType = 'chat' | 'agent' | 'workflow' | 'digital_human';
export type ConnectorMailUsage = 'read' | 'send' | 'read_send';
export interface ConnectorMailVerification {
  usage: ConnectorMailUsage;
  imap_status: 'not_requested' | 'verified' | 'failed';
  smtp_status: 'not_requested' | 'verified' | 'failed';
}

export interface ConnectorMailEvidence {
  version: number;
  provider: 'gmail' | 'qqmail' | 'netease163';
  identity_kind: 'authenticated_login';
  authenticated_login: string;
  credential_verified: boolean;
  imap_status: string;
  smtp_status: string;
  usage: ConnectorMailUsage;
  scope_granularity: 'not_available';
  operations: Array<{ id: string; availability: string; permission: string; resource: string }>;
  checked_at: string;
}

export type ConnectorLocalizedText = Record<string, string>;

export interface ConnectorKeyField {
  name: string;
  label: string;
  label_i18n?: ConnectorLocalizedText;
  placeholder?: string;
  placeholder_i18n?: ConnectorLocalizedText;
  help_url?: string;
  required: boolean;
  secret: true;
}

export interface ConnectorCredentialField extends Omit<ConnectorKeyField, 'secret'> {
  secret: boolean;
  type: 'string';
  enum?: string[];
  max_length: number;
}
export interface ConnectorSetupDescriptor {
  handler: string;
  methods: Array<'manual' | 'qr_create' | 'qr_update' | 'qr_connect' | 'qr_reauthorize'>;
  protocol_version?: number;
  intents?: string[];
  authentication_kind?: string;
  verification_policy?: string;
  template_id: string;
  template_digest: string;
}
export interface ConnectorPermissionTemplate {
  profile_id: string;
  profile_version: string;
  addons: {
    preset: boolean;
    scopes: { tenant: string[]; user: string[] };
    events: { items: { tenant: string[]; user: string[] } };
    callbacks: { items: string[] };
  };
  user_event_dependencies?: Array<{
    event_id: string;
    user_scope: string;
    subscription_api: string;
  }>;
}
export interface ConnectorProviderEvidence {
  mail?: ConnectorMailEvidence;
  messaging?: {
    version: number;
    provider: string;
    subject: string;
    identity: string;
    credential_verified: boolean;
    operations: Array<{ id: string; availability: string; permission: string; resource: string }>;
  };
  region: 'feishu' | 'lark';
  app_id: string;
  app_name: string;
  template_id: string;
  granted_scopes: string[];
  scope_status: 'unknown' | 'verified';
  event_configuration: 'unknown';
  events_enabled: false;
  user_authorization_enabled: false;
}
export interface ConnectorOnboardingFlow {
  protocol_version?: number;
  step?: string;
  expiry_source?: string;
  qr_payload?: string;
  allowed_actions?: Array<'cancel' | 'submit_code' | 'retry_verification'>;
  verification_phase?: 'token' | 'identity' | 'operation' | 'checkpoint' | 'commit';
  candidate_expires_at?: string;
  verification_deadline?: string;
  diagnostic_trace_id?: string;
  challenge_revision?: number;
  id: string;
  connector_id: string;
  intent:
    | 'create_app'
    | 'update_app'
    | 'resume_app'
    | 'register_bot'
    | 'login_session'
    | 'reauthorize';
  state:
    | 'starting'
    | 'awaiting_authorization'
    | 'verifying'
    | 'verification_waiting'
    | 'ready'
    | 'cancelled'
    | 'expired'
    | 'failed'
    | 'recovery_required'
    | 'pending_completion';
  region: 'feishu' | 'lark';
  revision: number;
  expires_at: string;
  retry_after: number;
  verification_url?: string;
  user_code?: string;
  result_connection_id?: string;
  error_code?: string;
  pending_setup_id?: string;
  qr_expires_at?: string;
}
/** Non-secret registration receipt; never a selectable PersonalConnection. */
export interface ConnectorPendingSetup {
  kind: 'pending_setup';
  id: string;
  connector_id: string;
  region: 'feishu' | 'lark';
  app_id: string;
  state: 'verification_required' | 'awaiting_application' | 'completed' | 'cancelled' | 'expired';
  revision: number;
  resume_expires_at: string;
  last_verification_at?: string;
  reason_code?: string;
  allowed_actions: Array<'resume_qr' | 'resume_manual' | 'cancel'>;
  resume_qr_unavailable_reason?: string;
  active_flow_id?: string;
  result_connection_id?: string;
}
export interface ConnectorPendingSetupList {
  items: ConnectorPendingSetup[];
  total_pending: number;
}
export type ConnectorOnboardingRequest =
  | { intent: 'resume_app'; pending_setup_id: string; expected_revision: number }
  | { intent: 'create_app'; region: 'feishu' | 'lark'; template_id: string }
  | { intent: 'update_app'; expected_revision: number; template_id: string }
  | { intent: 'register_bot' | 'login_session'; protocol_version: 1 }
  | { intent: 'reauthorize'; protocol_version: 1; expected_revision: number };

export interface ConnectorCapabilitySummary {
  credential_subject?: string;
  required_scopes?: string[][];
  effect?: string;
  availability?: string;
  permission?: string;
  resource?: string;
  id: string;
  name: string;
  name_i18n?: ConnectorLocalizedText;
  description: string;
  description_i18n?: ConnectorLocalizedText;
  usable: boolean;
  unavailable_reason?: string;
}

export interface ConnectorConnectionSummary {
  count: number;
  needs_attention_count?: number;
  primary_connection_id?: string;
  state?: ConnectorReadiness;
  external_display_name?: string;
}

export interface ConnectorCatalogItem {
  id: string;
  plugin_id: string;
  supported_surfaces?: ConnectorTargetType[];
  name: string;
  name_i18n?: ConnectorLocalizedText;
  description: string;
  description_i18n?: ConnectorLocalizedText;
  icon?: string;
  classification: ConnectorClassification;
  category?: ConnectorCategory;
  tags?: ConnectorTag[];
  key_fields: ConnectorKeyField[];
  extension_schema_version?: '1.1.0' | '1.2.0' | '1.3.0' | '1.4.0' | '1.5.0';
  setup_method?: string;
  setup_availability?: { enabled: boolean; methods: string[]; unavailable_reason?: string };
  credential_fields?: ConnectorCredentialField[];
  setup?: ConnectorSetupDescriptor;
  permission_template?: ConnectorPermissionTemplate;
  capabilities: ConnectorCapabilitySummary[];
  examples?: string[];
  connection_summary: ConnectorConnectionSummary;
}

export interface ConnectorCatalogResponse {
  items: ConnectorCatalogItem[];
}

export interface ConnectorTargetRef {
  type: ConnectorTargetType;
  id: string;
  binding_instance_id?: string;
  scope?: 'live' | 'draft' | 'published';
  version_id?: string;
}

export interface PersonalConnection {
  allowed_operation_ids?: string[];
  id: string;
  connector_id: string;
  name: string;
  external_account_id?: string;
  external_display_name?: string;
  state: ConnectorReadiness;
  diagnostic_code?: string;
  usable_operation_ids?: string[];
  credential_configured: true;
  credential_version?: number;
  provider_evidence?: ConnectorProviderEvidence;
  revision: number;
  last_verified_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ConnectorTargetBinding {
  id: string;
  connection_id: string;
  connector_id: string;
  target: ConnectorTargetRef;
  target_name?: string;
  status: 'active' | 'disabled' | 'needs_replacement';
  revision: number;
  authorization_mode?: 'ask_each_time' | 'direct';
  host_audiences?: ConnectorHostAudience[];
}

export type ConnectorHostAudience = 'workspace' | 'api' | 'channel';

export interface ConnectorTargetBindingListResponse {
  items: ConnectorTargetBinding[];
}

export interface CreateConnectorTargetBindingRequest {
  connection_id: string;
  target: ConnectorTargetRef;
  expected_revision?: number;
  authorization_mode?: 'ask_each_time' | 'direct';
}

export interface DetachConnectorTargetBindingRequest {
  revision: number;
}

export interface UpdateConnectorTargetBindingAuthorizationRequest {
  revision: number;
  authorization_mode: 'ask_each_time' | 'direct';
}

export interface UpdateConnectorHostDelegationRequest {
  revision: number;
  audiences: ConnectorHostAudience[];
}

export interface PersonalConnectionDetail extends PersonalConnection {
  capabilities: ConnectorCapabilitySummary[];
  bindings: ConnectorTargetBinding[];
}

/** Preferences initialize future chats; they are never runtime binding authority. */
export type ConnectorChatDefault =
  | { mode: 'none'; connection_id?: never; connection?: never; available?: never }
  | {
      mode: 'selected';
      connection_id: string;
      connection?: Pick<
        PersonalConnection,
        'id' | 'name' | 'external_display_name' | 'state' | 'usable_operation_ids'
      >;
      available: boolean;
    };

export interface ConnectorChatPreferences {
  revision: number;
  /** Missing is unset, distinct from an explicit mode: none. */
  defaults: Record<string, ConnectorChatDefault>;
}

export type SetConnectorChatPreferenceRequest = {
  expected_revision: number;
} & ({ mode: 'selected'; connection_id: string } | { mode: 'none'; connection_id?: never });

export interface PersonalConnectionListResponse {
  items: PersonalConnection[];
}

export interface RenamePersonalConnectionRequest {
  revision: number;
  name: string;
}

export interface RemovePersonalConnectionOptions {
  force?: boolean;
  impactToken?: string;
}

export interface RemovalImpact {
  connection_id: string;
  can_remove: boolean;
  impact_token?: string;
  expires_at?: string;
  affected_targets: ConnectorTargetBinding[];
}

export interface KeySetupRequest {
  mail_usage?: ConnectorMailUsage;
  /** Write-only. Callers must clear their local copy immediately after submission. */
  values: Record<string, string>;
  target?: ConnectorTargetRef;
}

export interface ReconnectKeyRequest {
  mail_usage?: ConnectorMailUsage;
  /** Write-only. Callers must clear their local copy immediately after submission. */
  values: Record<string, string>;
  /** Explicit replacement consent only when the provider cannot return a stable identity. */
  confirm_unstable_identity?: boolean;
}

/**
 * Read-only host-key probe body. It carries no Idempotency-Key and creates or
 * mutates no setup attempt or Connection. The fingerprint is optional: when it
 * is omitted the backend captures the observed host key instead of verifying a
 * supplied pin.
 */
export interface ServerOpsSSHHostKeyTestRequest {
  /** Write-only. Callers must destroy their local copy after submission. */
  values: {
    host: string;
    port: string;
    username: string;
    password?: string;
    private_key?: string;
    /** Omitted for an unencrypted key. */
    passphrase?: string;
    /** Optional canonical pin; when omitted the backend captures the host key. */
    host_key_sha256?: string;
  };
}

/** Non-secret host-key observation; never carries credential material. */
export interface ServerOpsSSHHostKeyTestResult {
  host_key_sha256: string;
  hostname: string;
  request_id: string;
}

interface KeySetupResultBase {
  attempt_id: string;
  binding_warning?: string;
}

export interface VerifyingKeySetupResult extends KeySetupResultBase {
  state: 'verifying';
}

export interface ReadyKeySetupResult extends KeySetupResultBase {
  state: 'ready';
  connection: PersonalConnection;
  attachment?: ConnectorTargetBinding;
}

export interface FailedKeySetupResult extends KeySetupResultBase {
  state: 'failed';
  error_code: string;
  retryable: boolean;
}

export interface CancelledKeySetupResult extends KeySetupResultBase {
  state: 'cancelled';
}

export type KeySetupResult =
  | VerifyingKeySetupResult
  | ReadyKeySetupResult
  | FailedKeySetupResult
  | CancelledKeySetupResult;

export interface GetConnectorCatalogParams {
  classification?: ConnectorClassification;
  category?: ConnectorCategory;
  tag?: ConnectorTag;
  query?: string;
  status?: ConnectorCatalogStatus;
}

export interface GetPersonalConnectionsParams {
  connector_id?: string;
  target_type?: ConnectorTargetType;
  target_id?: string;
}

export type ConnectorRuntimeRiskClassification = 'trusted_low' | 'high' | 'unclassified';
export type ConnectorRuntimeConfirmationRisk = Exclude<
  ConnectorRuntimeRiskClassification,
  'trusted_low'
>;

export interface ConnectorRuntimeConsequence {
  external_effect: string;
  description?: string;
  description_i18n?: ConnectorLocalizedText;
  data_egress: boolean;
  external_destination?: string;
}

/**
 * Safe display contract emitted before a consequential Connector operation.
 * Operation input and credential material are deliberately absent.
 */
export interface ConnectorRuntimeConfirmation {
  route_receipt: string;
  connector_id: string;
  connector_name: string;
  connector_name_i18n?: ConnectorLocalizedText;
  operation_id: string;
  action_name: string;
  action_name_i18n?: ConnectorLocalizedText;
  target: ConnectorTargetRef;
  consequence: ConnectorRuntimeConsequence;
  risk_classification: ConnectorRuntimeConfirmationRisk;
}

export interface ConnectorRuntimeRequiresConfirmationResult {
  state: 'requires_confirmation';
  decision_id?: string;
  error_code?: never;
  confirmation: ConnectorRuntimeConfirmation;
}

export interface ConnectorRuntimeBlockedResult {
  state: 'blocked';
  decision_id?: string;
  error_code?: never;
  confirmation?: never;
}

export interface ConnectorRuntimeCancelledResult {
  state: 'cancelled';
  decision_id?: string;
  error_code?: never;
  confirmation?: never;
}

export interface ConnectorRuntimeCompletedResult {
  state: 'completed';
  decision_id?: string;
  error_code?: never;
  confirmation?: never;
}

export interface ConnectorRuntimeFailedResult {
  state: 'failed';
  decision_id?: string;
  error_code?: string;
  confirmation?: never;
}

export interface ConnectorRuntimeUnknownResult {
  state: 'unknown';
  decision_id?: string;
  error_code?: string;
  confirmation?: never;
}

export interface ConnectorRuntimeOperationPresentation {
  schema_version: 1;
  source_kind: 'native' | 'history';
  invocation_id: string;
  connector_id?: string;
  operation_id?: string;
  display_status:
    | 'successful'
    | 'partial'
    | 'failed'
    | 'unknown'
    | 'cancelled'
    | 'unavailable'
    | 'pending';
  title?: string;
  summary?: string;
  error_code?: string;
  redacted: boolean;
  truncated: boolean;
}

export interface ConnectorRuntimeTerminalPresentation {
  connector_presentation?: ConnectorRuntimeOperationPresentation;
}

export type ConnectorRuntimeExecutionResult =
  | ConnectorRuntimeRequiresConfirmationResult
  | (ConnectorRuntimeBlockedResult & ConnectorRuntimeTerminalPresentation)
  | (ConnectorRuntimeCancelledResult & ConnectorRuntimeTerminalPresentation)
  | (ConnectorRuntimeCompletedResult & ConnectorRuntimeTerminalPresentation)
  | (ConnectorRuntimeFailedResult & ConnectorRuntimeTerminalPresentation)
  | (ConnectorRuntimeUnknownResult & ConnectorRuntimeTerminalPresentation);

export type ConnectorRuntimeConfirmationDecision = 'confirmed' | 'cancelled';

/** The request body accepted by the confirmation endpoint. */
export interface ConnectorRuntimeConfirmationCommand {
  route_receipt: string;
  decision: ConnectorRuntimeConfirmationDecision;
  remember_for_session?: boolean;
}

export type ConnectorRuntimeDecisionOutcomeState = 'cancelled' | 'completed' | 'failed' | 'unknown';

/**
 * Credential-free server response for a Connector confirmation continuation.
 * Provider output continues through the owning Chat, Agent, or Workflow events.
 */
interface ConnectorRuntimeConfirmationResponseBase {
  decision_id?: string;
  remember_for_session?: boolean;
}

export interface ConnectorRuntimeConfirmationSettledResponse
  extends ConnectorRuntimeConfirmationResponseBase {
  state: 'cancelled' | 'completed';
  error_code?: never;
}

export interface ConnectorRuntimeConfirmationErrorResponse
  extends ConnectorRuntimeConfirmationResponseBase {
  state: 'failed' | 'unknown';
  error_code?: string;
}

export type ConnectorRuntimeConfirmationResponse =
  | ConnectorRuntimeConfirmationSettledResponse
  | ConnectorRuntimeConfirmationErrorResponse;

/** Client-local result adds the consumed receipt without changing the server DTO. */
export type ConnectorRuntimeStatusResponse = { presentation?: unknown } & (
  | { resolution: 'unresolved' }
  | (ConnectorRuntimeConfirmationResponse & { resolution: 'resolved' })
);

export type ConnectorRuntimeDecisionResult = (
  | ConnectorRuntimeConfirmationResponse
  | { state: 'transport_unconfirmed' }
) & {
  route_receipt: string;
};

export interface ConnectorUnavailableReference {
  id: string;
  target: ConnectorTargetRef;
  former_reference_id: string;
  label: string;
  reason_code: string;
  status: 'needs_replacement' | 'explicitly_replaced' | 'detached';
  revision: number;
}
export interface ConnectorReferenceCommand {
  reference_id: string;
  expected_revision: number;
  action: 'replace' | 'detach';
  connection_id?: string;
  operation_id?: string;
}

export interface ConnectorConfirmationPreview {
  account_name: string;
  account_identity: string;
  kind: 'mail' | 'message' | 'resource';
  complete: boolean;
  fields: Array<{
    label: string;
    label_i18n: Record<string, string>;
    value: string;
    truncated: boolean;
    /** Presentation hint: `command` is the exact command line, shown as code. */
    format?: 'command';
  }>;
}

/** Safe OAuth status only. Provider secrets never cross this boundary. */
export interface ConnectorOAuthFlow {
  id: string;
  connector_id: string;
  status: 'pending' | 'processing' | 'select_page' | 'install_app' | 'ready' | 'failed';
  authorize_url?: string;
  install_url?: string;
  connection_id?: string;
  pages?: Array<{ id: string; name: string }>;
  error_code?: string;
  renewal_mode?: string;
  expires_at?: string;
}

/** A verified Page delivery batch; payload remains untrusted external content. */
export interface ConnectorSocialEvent {
  id: number;
  provider: string;
  kind: string;
  source_id: string;
  external_account_id: string;
  received_at: string;
  payload: Record<string, unknown>;
}
export interface ConnectorSocialEventPage {
  items: ConnectorSocialEvent[];
  next_after_id: number;
}
export interface PreparedSocialImage {
  image_data: string;
  mime_type: 'image/jpeg' | 'image/png';
  width: number;
  height: number;
  bytes: number;
}
export interface IssueConnectorBackgroundGrantRequest {
  operations: string[];
  destinations?: Record<string, string>;
  expires_at: string;
  max_invocations: number;
}

export interface ConnectorBackgroundGrant {
  id: string;
  employee_id: string;
  version_id: string;
  binding_id: string;
  connection_id: string;
  operations: string[];
  destinations: Record<string, string>;
  expires_at: string;
  max_invocations: number;
  used_invocations: number;
  revoked_at?: string;
  created_at: string;
  binding_revision: number;
  connection_revision: number;
  credential_version: number;
  contract_digest: string;
}
