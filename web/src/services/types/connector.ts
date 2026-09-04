export type ConnectorClassification = 'connector' | 'data_source';
export type ConnectorReadiness = 'ready' | 'reconnect_required' | 'unavailable';
export type ConnectorCatalogStatus = 'all' | 'connected' | 'needs_attention';
export type ConnectorTargetType = 'chat' | 'agent' | 'workflow';

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
  allowed_actions?: Array<'cancel' | 'submit_code'>;
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
  name: string;
  name_i18n?: ConnectorLocalizedText;
  description: string;
  description_i18n?: ConnectorLocalizedText;
  icon?: string;
  classification: ConnectorClassification;
  key_fields: ConnectorKeyField[];
  extension_schema_version?: '1.1.0' | '1.2.0' | '1.3.0';
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
}

export interface ConnectorTargetBindingListResponse {
  items: ConnectorTargetBinding[];
}

export interface CreateConnectorTargetBindingRequest {
  connection_id: string;
  target: ConnectorTargetRef;
  expected_revision?: number;
}

export interface DetachConnectorTargetBindingRequest {
  revision: number;
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
  /** Write-only. Callers must clear their local copy immediately after submission. */
  values: Record<string, string>;
  target?: ConnectorTargetRef;
}

export interface ReconnectKeyRequest {
  /** Write-only. Callers must clear their local copy immediately after submission. */
  values: Record<string, string>;
  /** Explicit replacement consent only when the provider cannot return a stable identity. */
  confirm_unstable_identity?: boolean;
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

export type ConnectorRuntimeExecutionResult =
  | ConnectorRuntimeRequiresConfirmationResult
  | ConnectorRuntimeBlockedResult
  | ConnectorRuntimeCancelledResult
  | ConnectorRuntimeCompletedResult
  | ConnectorRuntimeFailedResult
  | ConnectorRuntimeUnknownResult;

export type ConnectorRuntimeConfirmationDecision = 'confirmed' | 'cancelled';

/** The complete, one-shot request body accepted by the confirmation endpoint. */
export interface ConnectorRuntimeConfirmationCommand {
  route_receipt: string;
  decision: ConnectorRuntimeConfirmationDecision;
}

export type ConnectorRuntimeDecisionOutcomeState = 'cancelled' | 'completed' | 'failed' | 'unknown';

/**
 * Credential-free server response for a Connector confirmation continuation.
 * Provider output continues through the owning Chat, Agent, or Workflow events.
 */
interface ConnectorRuntimeConfirmationResponseBase {
  decision_id?: string;
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
export type ConnectorRuntimeStatusResponse =
  | { resolution: 'unresolved' }
  | (ConnectorRuntimeConfirmationResponse & { resolution: 'resolved' });

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
