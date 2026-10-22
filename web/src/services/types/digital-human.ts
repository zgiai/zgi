import type { FileUploadSelection } from '@/services/upload.service';
import type { AgentRuntimeRunDetail, AgentRuntimeStep } from './agent-runtime-log';

export type DigitalHumanStatus = 'draft' | 'active' | 'disabled';
export type DigitalHumanVersionState = 'draft' | 'published';
export type DigitalHumanRunStatus =
  | 'queued'
  | 'running'
  | 'waiting'
  | 'canceling'
  | 'succeeded'
  | 'failed'
  | 'canceled'
  | 'unknown';

/**
 * Why a durable run is parked in `waiting`. The backend constrains this to the
 * runtime message statuses that suspend a turn: a governance approval, a
 * question for the operator, or an external page action.
 */
export type DigitalHumanRunWaitingReason =
  | 'waiting_approval'
  | 'waiting_question'
  | 'waiting_budget'
  | 'waiting_client_action';

export interface DigitalHumanChannelResponsePolicy {
  intermediate_messages_enabled?: boolean;
  acknowledgement_enabled?: boolean;
}

export interface DigitalHumanTriggerPolicy extends Record<string, unknown> {
  manual?: boolean;
  channel_response?: DigitalHumanChannelResponsePolicy;
}

export interface DigitalHumanPresentationPolicy {
  home_title: string;
  opening_statement: string;
  input_placeholder: string;
  suggested_questions: string[];
}

/** Application opt-in only; deployment settings own the environment policy. */
export interface DigitalHumanExecutionRuntimeConfig {
  enabled: boolean;
}

/** Server-owned admission state for the DigitalHuman Linux toggle. */
export interface DigitalHumanExecutionRuntimeCapability {
  available: boolean;
  can_enable: boolean;
  reason?: 'disabled' | 'organization_not_enabled' | 'runtime_unavailable' | 'entry_not_enabled' | string;
  entry_capabilities?: Record<string, {
    available: boolean;
    reason?: string;
  }>;
}

export interface DigitalHumanDefinition {
  id: string;
  web_app_id: string;
  organization_id: string;
  workspace_id: string;
  name: string;
  avatar?: string;
  icon_type: 'text' | 'image';
  icon: string;
  icon_url?: string;
  description?: string;
  status: DigitalHumanStatus;
  autonomy_level: 'assisted' | 'supervised' | 'autonomous';
  visibility: 'private' | 'workspace';
  draft_version_id: string;
  published_version_id?: string | null;
  revision: number;
  created_at: string;
  updated_at: string;
}

export interface DigitalHumanVersion {
  file_upload_policy?: FileUploadSelection | null;
  id: string;
  digital_human_id: string;
  state: DigitalHumanVersionState;
  version_number?: number | null;
  name: string;
  description: string;
  lock_version: number;
  revision: string;
  identity_name: string;
  persona: string;
  goals: string[];
  rules: string[];
  default_model: string;
  memory_policy?: Record<string, unknown>;
  trigger_policy?: DigitalHumanTriggerPolicy;
  presentation_policy?: DigitalHumanPresentationPolicy;
  execution_runtime?: DigitalHumanExecutionRuntimeConfig | null;
  published_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface DigitalHumanDetail {
  definition: DigitalHumanDefinition;
  draft: DigitalHumanVersion;
  published?: DigitalHumanVersion | null;
  /** Draft configuration or resource bindings differ from the publication. */
  has_unpublished_changes?: boolean;
  execution_runtime_capability?: DigitalHumanExecutionRuntimeCapability | null;
}

export interface DigitalHumanVersionList {
  items: DigitalHumanVersion[];
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
}

export type DigitalHumanVersionRestoreBindingChange = 'added' | 'removed' | 'updated';

export interface DigitalHumanVersionRestoreBindingImpact {
  change: DigitalHumanVersionRestoreBindingChange;
  type: DigitalHumanBindingType;
  resource_id: string;
  parent_resource_id?: string;
  display_name?: string;
}

export interface DigitalHumanVersionRestorePreview {
  version_id: string;
  version_number: number;
  is_current: boolean;
  will_discard_unpublished_changes: boolean;
  changed_fields: string[];
  binding_changes: DigitalHumanVersionRestoreBindingImpact[];
  bindings: DigitalHumanResourceBinding[];
  impact_token: string;
}

export interface RestoreDigitalHumanVersionRequest {
  version_id: string;
  expected_revision: number;
  expected_draft_lock_version: number;
  impact_token: string;
}

export interface DigitalHumanList {
  items: DigitalHumanDefinition[];
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
}

export interface CreateDigitalHumanRequest {
  file_upload_policy?: FileUploadSelection | null;
  name: string;
  avatar?: string;
  icon_type?: 'text' | 'image';
  icon?: string;
  description?: string;
  autonomy_level: DigitalHumanDefinition['autonomy_level'];
  visibility: DigitalHumanDefinition['visibility'];
  identity_name?: string;
  persona: string;
  goals?: string[];
  rules: string[];
  default_model: string;
  memory_policy?: Record<string, unknown>;
  trigger_policy?: DigitalHumanTriggerPolicy;
  presentation_policy?: DigitalHumanPresentationPolicy;
  execution_runtime?: DigitalHumanExecutionRuntimeConfig | null;
}

export interface UpdateDigitalHumanRequest {
  expected_revision: number;
  name?: string;
  avatar?: string;
  icon_type?: 'text' | 'image';
  icon?: string;
  description?: string;
  /**
   * Publication status. `disabled` takes a published digital human offline
   * so it can no longer be run or reached, and is the prerequisite for
   * deleting it; `active` brings it back online.
   */
  status?: DigitalHumanDefinition['status'];
  autonomy_level?: DigitalHumanDefinition['autonomy_level'];
  visibility?: DigitalHumanDefinition['visibility'];
}

export interface UpdateDigitalHumanDraftRequest {
  file_upload_policy?: FileUploadSelection | null;
  expected_lock_version: number;
  identity_name?: string;
  persona?: string;
  goals?: string[];
  rules?: string[];
  default_model?: string;
  memory_policy?: Record<string, unknown>;
  trigger_policy?: DigitalHumanTriggerPolicy;
  presentation_policy?: DigitalHumanPresentationPolicy;
  /** Omitted/null leaves the draft unchanged; enabled:false clears the opt-in. */
  execution_runtime?: DigitalHumanExecutionRuntimeConfig | null;
}

export interface DigitalHumanAccessSettings {
  webapp_enabled: boolean;
  api_enabled: boolean;
}

export interface DigitalHumanAPIKey {
  id: string;
  digital_human_id: string;
  key_prefix: string;
  name: string;
  status: 'active' | 'revoked';
  expires_at?: string | null;
  usage_count: number;
  last_used_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface DigitalHumanAPIKeyList {
  api_keys: DigitalHumanAPIKey[];
  total: number;
}

export interface DigitalHumanAPIKeyCreated extends DigitalHumanAPIKey {
  api_key: string;
}

export interface DigitalHumanWebAppConfig {
  file_upload_policy?: FileUploadSelection | null;
  digital_human_id: string;
  web_app_id: string;
  name: string;
  description: string;
  icon: string;
  icon_type: 'text' | 'image';
  icon_url: string;
  home_title: string;
  opening_statement: string;
  input_placeholder: string;
  suggested_questions: string[];
  anonymous_chat_enabled: boolean;
  login_required: boolean;
  file_upload_enabled: boolean;
  file_upload_requires_login: boolean;
  relationship_memory_enabled: boolean;
  runtime_input_enabled: boolean;
  version: string;
  version_uuid: string;
}

export interface PublishDigitalHumanRequest {
  expected_revision: number;
  expected_draft_lock_version: number;
  name?: string;
  description?: string;
}

export interface DigitalHumanPersonaTemplate {
  id: string;
  name: string;
  persona: string;
  goals: string[];
  rules: string[];
  revision: number;
  created_at: string;
  updated_at: string;
}

export interface DigitalHumanPersonaTemplateList {
  items: DigitalHumanPersonaTemplate[];
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
}

export interface CreateDigitalHumanPersonaTemplateRequest {
  name: string;
  persona: string;
  goals: string[];
  rules: string[];
}

export interface UpdateDigitalHumanPersonaTemplateRequest {
  expected_revision: number;
  name?: string;
  persona?: string;
  goals?: string[];
  rules?: string[];
}

export type DigitalHumanBindingType =
  | 'skill'
  | 'knowledge_dataset'
  | 'database'
  | 'database_table'
  | 'workflow'
  | 'integration_connection'
  | 'connector';

export interface DigitalHumanResourceBindingInput {
  /**
   * Existing row in this digital human's current draft; omitted for a new
   * resource. Sending it lets the server retain row identity instead of
   * re-authorizing the binding.
   */
  id?: string;
  type: DigitalHumanBindingType;
  resource_id: string;
  parent_resource_id?: string;
  display_name?: string;
  access_mode: 'read' | 'write' | 'execute';
  resource_revision: string;
  /**
   * Opaque server-side reference. Connector bindings must leave it empty so no
   * credential value or handle ever travels through the browser DTO.
   */
  credential_ref?: string;
  metadata: Record<string, unknown>;
}

export interface DigitalHumanResourceBinding
  extends Omit<DigitalHumanResourceBindingInput, 'type'> {
  id: string;
  binding_type: DigitalHumanBindingType;
  binding_revision: string;
}

export interface DraftResourceBindings {
  /** Draft generation; changes at publication while lock versions stay local to it. */
  draft_version_id?: string;
  draft_lock_version: number;
  items: DigitalHumanResourceBinding[];
}

export interface ReplaceDigitalHumanResourceBindingsRequest {
  expected_draft_lock_version: number;
  expected_draft_version_id?: string;
  items: DigitalHumanResourceBindingInput[];
}

export interface DigitalHumanResourceCandidate {
  type: DigitalHumanBindingType;
  resource_id: string;
  parent_resource_id?: string;
  display_name: string;
  description?: string;
  resource_revision: string;
  access_modes: Array<'read' | 'write' | 'execute'>;
  metadata: Record<string, unknown>;
  status: string;
  available: boolean;
}

export interface DigitalHumanResourceCandidateList {
  items: DigitalHumanResourceCandidate[];
  total: number;
}

export interface DigitalHumanRun {
  id: string;
  digital_human_id: string;
  digital_human_version_id: string;
  idempotency_key: string;
  invocation_event: Record<string, unknown>;
  run_spec: Record<string, unknown>;
  run_spec_revision: string;
  execution_snapshot: Record<string, unknown>;
  execution_snapshot_revision: string;
  execution_trace: Record<string, unknown>;
  query: string;
  status: DigitalHumanRunStatus;
  /**
   * Present only while `status` is `waiting`; says what the run is parked on so
   * the console can label it instead of showing a generic pending state.
   */
  waiting_reason?: DigitalHumanRunWaitingReason;
  /**
   * Opaque server-side receipt for a `waiting_approval` run. Connector
   * confirmation continues to use this receipt-scoped path.
   */
  waiting_receipt?: string | null;
  conversation_id?: string | null;
  message_id?: string | null;
  /** Where the run was asked from: console, webapp, api, task, channel, preview. */
  surface?: string;
  /** Who asked for the run; console readers only. */
  requester?: DigitalHumanTaskRequester;
  result_summary?: string;
  error_code?: string;
  error_message?: string;
  cancel_requested_at?: string | null;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface DigitalHumanRunList {
  items: DigitalHumanRun[];
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
}

export interface DigitalHumanRunRuntimeLog {
  detail?: AgentRuntimeRunDetail | null;
  steps: AgentRuntimeStep[];
  /**
   * Server-authored, credential-free Connector confirmation presentation.
   * Present only while the run is durably `waiting_approval` and the projection
   * matched the run, message, conversation, and waiting receipt. Deliberately
   * `unknown`: the console must pass it through the strict DigitalHuman parser
   * before rendering consent UI.
   */
  pending_connector_confirmation?: unknown;
  /** Inline workflow approval data for a run paused on a generic approval node. */
  pending_workflow_approval?: {
    approval_token: string;
    approval_form: unknown;
  };
}

export interface ManualDigitalHumanInvocationRequest {
  request_id: string;
  query: string;
  conversation_id?: string;
  parent_message_id?: string;
  file_ids?: string[];
}

export interface ManualDigitalHumanInvocationResponse {
  run: DigitalHumanRun;
  created: boolean;
}

export type DigitalHumanMemoryClass = 'self' | 'business';
export type DigitalHumanMemoryType =
  | 'identity'
  | 'experience'
  | 'decision'
  | 'fact'
  | 'open_loop'
  | 'reflection';
export type DigitalHumanMemoryScopeType =
  | 'workspace'
  | 'project'
  | 'customer'
  | 'campaign'
  | 'channel';
export type DigitalHumanMemoryStatus = 'active' | 'invalidated' | 'archived';

export interface DigitalHumanMemoryEvidence {
  type: 'run' | 'action' | 'tool' | 'artifact' | 'message' | 'manual';
  id: string;
  digest?: string;
}

export interface DigitalHumanMemory {
  id: string;
  digital_human_id: string;
  memory_key: string;
  memory_class: DigitalHumanMemoryClass;
  memory_type: DigitalHumanMemoryType;
  scope_type?: DigitalHumanMemoryScopeType | '';
  scope_id?: string;
  content: string;
  content_digest: string;
  visibility: 'workspace' | 'restricted';
  status: DigitalHumanMemoryStatus;
  confidence: number;
  revision: number;
  config_revision: string;
  memory_epoch: number;
  evidence: DigitalHumanMemoryEvidence[];
  source_principal_id?: string;
  last_operation_id: string;
  undoable_until?: string | null;
  undo_consumed: boolean;
  valid_from?: string | null;
  valid_to?: string | null;
  created_at: string;
  updated_at: string;
}

export interface DigitalHumanMemoryState {
  memory_epoch: number;
  config_revision: string;
  auto_extraction_enabled: boolean;
  extraction_cutoff_at?: string | null;
}

export interface DigitalHumanMemoryList {
  items: DigitalHumanMemory[];
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
  state: DigitalHumanMemoryState;
}

export interface DigitalHumanMemoryListParams {
  page?: number;
  page_size?: number;
  search?: string;
  memory_class?: DigitalHumanMemoryClass;
  memory_type?: DigitalHumanMemoryType;
  scope_type?: DigitalHumanMemoryScopeType;
  scope_id?: string;
  status?: DigitalHumanMemoryStatus;
  source?: string;
}

// Moves one memory between self memory and workspace business memory.
export interface DigitalHumanMemoryRescopeRequest {
  operation_id: string;
  expected_revision: number;
  config_revision: string;
  memory_epoch: number;
  memory_class: 'self' | 'business';
  scope?: { type: 'workspace'; id: string };
}

export interface DigitalHumanMemoryProposalRequest {
  operation_id: string;
  key: string;
  action: 'upsert' | 'clear';
  memory_class: DigitalHumanMemoryClass;
  memory_type: DigitalHumanMemoryType;
  scope?: { type: DigitalHumanMemoryScopeType; id: string };
  content?: string;
  expected_revision: number;
  supersedes_id?: string;
  expected_superseded_revision?: number;
  config_revision: string;
  memory_epoch: number;
  evidence?: DigitalHumanMemoryEvidence[];
  visibility?: 'workspace' | 'restricted';
  confidence?: number;
  source_context: {
    private_conversation: boolean;
    promoted_by_authorized_actor: boolean;
  };
  admin_approved: boolean;
  occurred_at: string;
}

export interface DigitalHumanMemoryMutationResult {
  memory?: DigitalHumanMemory;
  event: { operation_id: string; action: string; result: string; reason_code?: string };
  replayed: boolean;
}

export interface DigitalHumanMemoryRepairResult {
  status: 'repaired';
  memory_count: number;
  embedding_provider?: string;
  embedding_model?: string;
}

export type DigitalHumanChannelInstallationStatus = 'active' | 'disabled';
export type DigitalHumanChannelEventStatus = 'received' | 'ignored' | 'accepted' | 'failed';
export type DigitalHumanChannelDeliveryStatus =
  | 'pending'
  | 'processing'
  | 'succeeded'
  | 'failed'
  | 'unknown'
  | 'dismissed';

export interface DigitalHumanChannelInstallation {
  id: string;
  organization_id: string;
  workspace_id: string;
  provider: 'feishu' | (string & {});
  connector_id?: string;
  connection_id: string;
  credential_source?: 'embedded' | 'personal_connection' | 'legacy' | (string & {});
  connection_revision?: number;
  connection_credential_version?: number;
  credential_mode: 'embedded' | 'connection';
  identity_verified: boolean;
  name: string;
  external_tenant_id?: string;
  external_bot_id?: string;
  webhook_url?: string;
  receiver?: {
    mode: 'stream' | 'websocket' | 'webhook' | 'unknown' | (string & {});
    state:
      | 'not_applicable'
      | 'pending'
      | 'connecting'
      | 'connected'
      | 'reconnecting'
      | 'disconnected'
      | 'error';
    error_code?: string;
    last_event_at?: string;
    updated_at?: string;
  };
  default_subject_type?: 'digital_human';
  default_subject_id?: string;
  status: DigitalHumanChannelInstallationStatus;
  revision: number;
  created_at: string;
  updated_at: string;
  channel_context?: {
    status:
      | 'ready'
      | 'runtime_verification'
      | 'missing_message_history'
      | 'missing_member_names'
      | 'not_supported'
      | 'temporarily_unavailable';
    message_history_available: boolean;
    member_names_available: boolean;
  };
}

export interface CreateDigitalHumanChannelInstallationRequest {
  provider: string;
  connector_id?: string;
  connection_id?: string;
  credentials?: Record<string, string>;
  name: string;
}

export interface UpdateDigitalHumanChannelCredentialRequest {
  expected_revision: number;
  credentials: Record<string, string>;
}

export interface UpdateDigitalHumanChannelInstallationRequest {
  expected_revision: number;
  name?: string;
  status?: DigitalHumanChannelInstallationStatus;
  connector_id?: string;
  connection_id?: string;
}

export interface DigitalHumanChannelInstallationVerification {
  status: 'verified' | 'failed';
  credential_verified: boolean;
  bot_identity_verified: boolean;
  channel_context: NonNullable<DigitalHumanChannelInstallation['channel_context']>;
  checked_at: string;
  failure?: {
    code:
      | 'credentials_invalid'
      | 'access_denied'
      | 'configuration_invalid'
      | 'rate_limited'
      | 'temporarily_unavailable'
      | 'response_invalid'
      | 'provider_rejected'
      | (string & {});
    reason_code?: string;
    provider_error_code?: string;
    provider_request_id?: string;
    provider_http_status?: number;
    provider_invalid_field?: string;
    retryable: boolean;
  };
}

export type DigitalHumanChannelConnectionTestStatus =
  | 'configuration_failed'
  | 'awaiting_message'
  | 'message_received'
  | 'replying'
  | 'succeeded'
  | 'failed'
  | 'expired';

export type DigitalHumanChannelConnectionTestConversationType = 'direct' | 'group';

export interface DigitalHumanChannelConnectionCapability {
  id:
    | 'credentials'
    | 'bot_identity'
    | 'direct_inbound'
    | 'group_mention'
    | 'message_history'
    | 'member_names'
    | 'reply_delivery'
    | (string & {});
  status: 'pending' | 'passed' | 'failed';
  failure_code?: string;
  required_permissions?: string[];
}

export interface DigitalHumanChannelConnectionTest {
  id?: string;
  status: DigitalHumanChannelConnectionTestStatus;
  readiness: 'pending' | 'available' | 'partial' | 'unavailable';
  conversation_type: DigitalHumanChannelConnectionTestConversationType;
  verification?: DigitalHumanChannelInstallationVerification;
  test_code?: string;
  test_message?: string;
  started_at?: string;
  expires_at?: string;
  message_received: boolean;
  reply_delivered: boolean;
  inbound_event_id?: string;
  delivery_id?: string;
  failure_code?: string;
  capabilities: DigitalHumanChannelConnectionCapability[];
}

export interface DigitalHumanChannelInboundEvent {
  id: string;
  installation_id: string;
  provider?: string;
  binding_id?: string | null;
  provider_event_id: string;
  provider_message_id: string;
  external_conversation_id: string;
  sender_external_id: string;
  conversation_type?: 'direct' | 'group' | '';
  content_preview?: string;
  mention_only?: boolean;
  event_type: string;
  payload_digest: string;
  status: DigitalHumanChannelEventStatus;
  reason_code?: string;
  invocation_run_id?: string | null;
  received_at: string;
  processed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface DigitalHumanChannelDelivery {
  id: string;
  installation_id: string;
  binding_id: string;
  inbound_event_id?: string;
  run_id: string;
  kind?: 'acknowledgement' | 'progress' | 'final';
  provider: string;
  connection_id: string;
  subject_type: 'digital_human';
  subject_id: string;
  external_conversation_id: string;
  external_thread_id?: string;
  payload: { text?: string };
  payload_digest: string;
  status: DigitalHumanChannelDeliveryStatus;
  attempt_count: number;
  available_at: string;
  claimed_at?: string | null;
  delivered_at?: string | null;
  provider_message_id?: string;
  provider_request_id?: string;
  error_code?: string;
  error_message?: string;
  dismissed_at?: string | null;
  dismissed_by?: string | null;
  created_at: string;
  updated_at: string;
}

export interface DigitalHumanChannelPage<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
}

export type DigitalHumanTaskStatus =
  | 'proposed'
  | 'pending'
  | 'in_progress'
  | 'waiting'
  | 'blocked'
  | 'completed'
  | 'canceled';
export type DigitalHumanTaskPriority = 'low' | 'normal' | 'high' | 'urgent';
export type DigitalHumanTaskScheduleType = 'manual' | 'once' | 'cron';
export type DigitalHumanTaskOwnerKind = 'digital_human' | 'user_subject' | 'conversation';
export type DigitalHumanTaskVisibility = 'requester' | 'conversation' | 'workspace' | 'restricted';

export interface DigitalHumanTaskPendingChange {
  conversation_id: string;
  message_id: string;
}

export interface DigitalHumanTaskSchedule {
  type: DigitalHumanTaskScheduleType;
  run_at?: string;
  cron?: string;
  timezone?: string;
}

export interface DigitalHumanTaskSchedulePreviewRequest {
  type: 'once' | 'cron';
  local_run_at?: string;
  cron?: string;
  timezone: string;
}

export interface DigitalHumanTaskSchedulePreview {
  schedule: DigitalHumanTaskSchedule;
  next_run_at?: string;
}

export interface DigitalHumanTask {
  id: string;
  digital_human_id: string;
  owner_kind: DigitalHumanTaskOwnerKind;
  visibility?: DigitalHumanTaskVisibility;
  /** False when the task is visible to the reader but requested by someone else. */
  can_change?: boolean;
  /** Visibilities this reader may set; empty when they cannot change it. */
  visibility_options?: DigitalHumanTaskVisibility[];
  // Set in the personal view when a proposed change to this task waits for
  // the reader's approval in one of their conversations.
  pending_change?: DigitalHumanTaskPendingChange;
  /** The newest final report of the task. */
  latest_report?: DigitalHumanTaskReportSummary;
  /** Team channels that also receive this task's reports. */
  notify_channel_ids?: string[];
  /** The requester manages the team channels; a task manager may only remove them. */
  notify_channel_access?: 'manage' | 'remove';
  /** An integrator's own identifier, given when an API task is created. */
  client_reference?: string;
  /** Who asked for the task; only the task center returns it. */
  requester?: DigitalHumanTaskRequester;
  name: string;
  goal: string;
  success_criteria: string[];
  work_status: DigitalHumanTaskStatus;
  lifecycle_status: 'draft' | 'active' | 'paused' | 'completed' | 'archived';
  priority: DigitalHumanTaskPriority;
  schedule: DigitalHumanTaskSchedule;
  delivery_mode: 'task_only' | 'source_conversation';
  delivery_target?: 'task_center' | 'source_conversation';
  delivery_fallback?: 'task_center';
  execution_timeout_seconds: number;
  max_attempts: number;
  next_run_at?: string;
  last_run_at?: string;
  last_run_status?: string;
  source: string;
  source_conversation_type?: 'direct' | 'group';
  revision: number;
  proposal_expires_at?: string;
  confirmed_at?: string;
  created_at: string;
  updated_at: string;
}

// `visitor` is a landing page visitor who has not signed in; only runs have them.
export type DigitalHumanTaskRequesterKind = 'member' | 'api_user' | 'channel' | 'visitor';

/**
 * Who asked for a task. API end users are named by their instance label,
 * because the API's user identifier is not kept; channel requesters have no ID.
 */
export interface DigitalHumanTaskRequester {
  kind: DigitalHumanTaskRequesterKind;
  id?: string;
  name?: string;
  email?: string;
}

export interface DigitalHumanTaskRequesterPage {
  items: Array<DigitalHumanTaskRequester & { count: number }>;
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
}

export interface DigitalHumanTaskList {
  items: DigitalHumanTask[];
  counts: Partial<Record<DigitalHumanTaskStatus, number>>;
  successful_run_count: number;
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
}

export interface CreateDigitalHumanTaskRequest {
  name: string;
  goal: string;
  success_criteria?: string[];
  priority?: DigitalHumanTaskPriority;
  schedule: DigitalHumanTaskSchedule;
  delivery_mode?: 'task_only' | 'source_conversation';
  execution_timeout_seconds?: number;
  max_attempts?: number;
  notify_channel_ids?: string[];
}

export type UpdateDigitalHumanTaskRequest = Partial<CreateDigitalHumanTaskRequest> & {
  expected_revision: number;
  // Only task center managers may change visibility.
  visibility?: DigitalHumanTaskVisibility;
};

export interface DigitalHumanTaskRun {
  id: string;
  task_id: string;
  digital_human_run_id?: string;
  trigger_source: string;
  attempt: number;
  status: string;
  error_summary?: string;
  result_summary?: string;
  result_delivery_status?:
    | 'pending'
    | 'processing'
    | 'succeeded'
    | 'failed'
    | 'unknown'
    | 'dismissed';
  result_delivery_error?: string;
  scheduled_for: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
}

export type DigitalHumanTaskReportOutcome = 'done' | 'partial' | 'not_done' | 'failed';
export type DigitalHumanTaskReportReviewState = 'new' | 'accepted' | 'rework_requested';
export type DigitalHumanTaskCriterionStatus = 'met' | 'not_met' | 'unknown';

export interface DigitalHumanTaskReportSummary {
  id: string;
  outcome: DigitalHumanTaskReportOutcome;
  summary: string;
  review_state: DigitalHumanTaskReportReviewState;
  created_at: string;
}

/** What one task run hands back (ADR 0008). */
export type DigitalHumanArtifactKind = 'document' | 'file' | 'image' | 'table';

/** One immutable version of an artifact (ADR 0009). */
export interface DigitalHumanArtifactVersion {
  id: string;
  artifact_id: string;
  number: number;
  file_name?: string;
  mime_type: string;
  size: number;
  note?: string;
  source_kind: 'run' | 'task_report' | 'manual';
  source_task_id?: string;
  source_run_id?: string;
  source_report_id?: string;
  created_at: string;
}

/** A deliverable the digital human kept, with its current version. */
export interface DigitalHumanArtifact {
  id: string;
  kind: DigitalHumanArtifactKind;
  title: string;
  visibility: 'owner' | 'team' | 'application';
  owner_kind: 'account' | 'api_end_user' | 'workspace' | 'application';
  owner_id: string;
  source_task_id?: string;
  current_version: number;
  created_at: string;
  updated_at: string;
  /** Set when the artifact is archived out of the default list. */
  archived_at?: string;
  /** Whether the reader may archive or restore it. */
  can_manage?: boolean;
  latest?: DigitalHumanArtifactVersion;
}

export interface DigitalHumanArtifactList {
  items: DigitalHumanArtifact[];
  total: number;
  page: number;
  page_size: number;
}

export interface DigitalHumanArtifactParams {
  scope?: 'mine';
  kind?: DigitalHumanArtifactKind;
  q?: string;
  task_id?: string;
  /** Archived artifacts are left out unless asked for. */
  archived?: 'archived' | 'all';
  page?: number;
  page_size?: number;
}

/** A file a task report carries, downloaded through the report. */
export interface DigitalHumanTaskReportFile {
  id: string;
  name: string;
  mime_type: string;
  size: number;
}

export interface DigitalHumanTaskReportOrigin {
  kind: 'conversation' | 'channel';
  conversation_id?: string;
  conversation_type?: 'direct' | 'group' | string;
  provider?: string;
}

export interface DigitalHumanTaskReport {
  /** Files the run produced that the report carries. */
  files?: DigitalHumanTaskReportFile[];
  id: string;
  digital_human_id: string;
  task_id: string;
  task_name: string;
  task_run_id: string;
  digital_human_run_id?: string;
  attempt: number;
  outcome: DigitalHumanTaskReportOutcome;
  summary: string;
  body?: string;
  criteria: Array<{ criterion: string; status: DigitalHumanTaskCriterionStatus; note?: string }>;
  issues?: string;
  source: 'report_tool' | 'system';
  final: boolean;
  review_state: DigitalHumanTaskReportReviewState;
  review_feedback?: string;
  reviewed_at?: string;
  can_review: boolean;
  created_at: string;
  /** Who asked for the report's task; task center readers only. */
  requester?: DigitalHumanTaskRequester;
  /** The reader holds the task, and is the one who supplements this report. */
  requested_by_reader?: boolean;
  /** Where the task was asked for, to follow the report up there. */
  origin?: DigitalHumanTaskReportOrigin;
}

export interface DigitalHumanTaskReportList {
  items: DigitalHumanTaskReport[];
  total: number;
  page: number;
  page_size: number;
}

export interface DigitalHumanTaskReportParams {
  task_id?: string;
  // Task center only: who asked for the report's task.
  'requester_kind[]'?: DigitalHumanTaskRequesterKind[];
  requester_id?: string;
  'review_state[]'?: DigitalHumanTaskReportReviewState[];
  'outcome[]'?: DigitalHumanTaskReportOutcome[];
  page?: number;
  page_size?: number;
}

export interface ReviewDigitalHumanTaskReportRequest {
  action: 'accept' | 'rework';
  feedback?: string;
}

export interface DigitalHumanTaskActivity {
  id: string;
  actor_type: string;
  actor_ref: string;
  action: string;
  summary: string;
  changes?: Record<string, unknown>;
  created_at: string;
}

export interface DigitalHumanTaskPage<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
}
