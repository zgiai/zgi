export type DigitalHumanStatus = 'draft' | 'active' | 'disabled';
export type DigitalHumanVersionState = 'draft' | 'published';
export type DigitalHumanRunStatus =
  | 'queued'
  | 'running'
  | 'canceling'
  | 'succeeded'
  | 'failed'
  | 'canceled'
  | 'unknown';

export interface DigitalHumanChannelResponsePolicy {
  intermediate_messages_enabled?: boolean;
  acknowledgement_enabled?: boolean;
}

export interface DigitalHumanTriggerPolicy extends Record<string, unknown> {
  manual?: boolean;
  channel_response?: DigitalHumanChannelResponsePolicy;
}

export interface DigitalHumanDefinition {
  id: string;
  organization_id: string;
  workspace_id: string;
  name: string;
  avatar?: string;
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
  id: string;
  digital_human_id: string;
  state: DigitalHumanVersionState;
  version_number?: number | null;
  lock_version: number;
  revision: string;
  persona: string;
  rules: string[];
  default_model: string;
  memory_policy?: Record<string, unknown>;
  trigger_policy?: DigitalHumanTriggerPolicy;
  published_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface DigitalHumanDetail {
  definition: DigitalHumanDefinition;
  draft: DigitalHumanVersion;
  published?: DigitalHumanVersion | null;
}

export interface DigitalHumanList {
  items: DigitalHumanDefinition[];
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
}

export interface CreateDigitalHumanRequest {
  name: string;
  avatar?: string;
  description?: string;
  autonomy_level: DigitalHumanDefinition['autonomy_level'];
  visibility: DigitalHumanDefinition['visibility'];
  persona: string;
  rules: string[];
  default_model: string;
  memory_policy?: Record<string, unknown>;
  trigger_policy?: DigitalHumanTriggerPolicy;
}

export interface UpdateDigitalHumanRequest {
  expected_revision: number;
  name?: string;
  avatar?: string;
  description?: string;
  autonomy_level?: DigitalHumanDefinition['autonomy_level'];
  visibility?: DigitalHumanDefinition['visibility'];
}

export interface UpdateDigitalHumanDraftRequest {
  expected_lock_version: number;
  persona?: string;
  rules?: string[];
  default_model?: string;
  memory_policy?: Record<string, unknown>;
  trigger_policy?: DigitalHumanTriggerPolicy;
}

export interface PublishDigitalHumanRequest {
  expected_revision: number;
  expected_draft_lock_version: number;
}

export interface VersionRef {
  kind: string;
  id: string;
  version?: string;
  revision?: string;
}

export interface DigitalHumanRoleTemplate {
  id: string;
  name: string;
  description?: string;
  draft_version_id: string;
  published_version_id?: string | null;
  revision: number;
  created_at: string;
  updated_at: string;
}

export interface DigitalHumanRoleTemplateVersion {
  id: string;
  role_template_id: string;
  state: DigitalHumanVersionState;
  version_number?: number | null;
  lock_version: number;
  revision: string;
  goals: string[];
  responsibilities: string[];
  capabilities: VersionRef[];
  sop_refs: VersionRef[];
  knowledge_refs: VersionRef[];
  policy: Record<string, unknown>;
  evaluation_refs: VersionRef[];
}

export interface DigitalHumanRoleTemplateDetail {
  template: DigitalHumanRoleTemplate;
  draft: DigitalHumanRoleTemplateVersion;
  published?: DigitalHumanRoleTemplateVersion | null;
}

export interface DigitalHumanRoleTemplateList {
  items: DigitalHumanRoleTemplate[];
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
}

export interface CreateDigitalHumanRoleTemplateRequest {
  name: string;
  description?: string;
  goals: string[];
  responsibilities: string[];
  capabilities: VersionRef[];
  sop_refs: VersionRef[];
  knowledge_refs: VersionRef[];
  policy: Record<string, unknown>;
  evaluation_refs: VersionRef[];
}

export interface DigitalHumanRoleAssignmentInput {
  role_template_version_id: string;
  parameters: Record<string, unknown>;
  override_policy: Record<string, unknown>;
  effective_from?: string;
  effective_to?: string;
}

export interface DigitalHumanRoleAssignment {
  id: string;
  role_template_version_id: string;
  parameters: Record<string, unknown>;
  override_policy: Record<string, unknown>;
}

export interface DraftRoleAssignments {
  draft_lock_version: number;
  items: DigitalHumanRoleAssignment[];
}

export type DigitalHumanBindingType =
  | 'skill'
  | 'knowledge_dataset'
  | 'database'
  | 'database_table'
  | 'workflow'
  | 'integration_connection';

export interface DigitalHumanResourceBindingInput {
  type: DigitalHumanBindingType;
  resource_id: string;
  parent_resource_id?: string;
  display_name?: string;
  access_mode: 'read' | 'write' | 'execute';
  resource_revision: string;
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
  draft_lock_version: number;
  items: DigitalHumanResourceBinding[];
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
  conversation_id?: string | null;
  message_id?: string | null;
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

export interface ManualDigitalHumanInvocationRequest {
  request_id: string;
  query: string;
  conversation_id?: string;
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

export type DigitalHumanChannelInstallationStatus = 'active' | 'disabled';
export type DigitalHumanChannelConversationType = 'direct' | 'group';
export type DigitalHumanChannelWakePolicy = 'direct_and_mention' | 'mention_only';
export type DigitalHumanChannelEventStatus = 'received' | 'ignored' | 'accepted' | 'failed';
export type DigitalHumanChannelDeliveryStatus =
  | 'pending'
  | 'processing'
  | 'succeeded'
  | 'failed'
  | 'unknown';

export interface DigitalHumanChannelInstallationUsage {
  digital_human_id: string;
  digital_human_name: string;
  binding_count: number;
}

export interface DigitalHumanChannelInstallation {
  id: string;
  organization_id: string;
  workspace_id: string;
  provider: 'feishu' | (string & {});
  connection_id: string;
  name: string;
  external_tenant_id?: string;
  external_bot_id?: string;
  default_subject_type?: 'digital_human';
  default_subject_id?: string;
  status: DigitalHumanChannelInstallationStatus;
  revision: number;
  binding_count?: number;
  used_by?: DigitalHumanChannelInstallationUsage[];
  created_at: string;
  updated_at: string;
}

export interface CreateDigitalHumanChannelInstallationRequest {
  provider: 'feishu';
  connection_id: string;
  name: string;
}

export interface UpdateDigitalHumanChannelInstallationRequest {
  expected_revision: number;
  name?: string;
  status?: DigitalHumanChannelInstallationStatus;
}

export interface DigitalHumanExternalConversationMapping {
  id: string;
  installation_id: string;
  external_conversation_id: string;
  conversation_type: DigitalHumanChannelConversationType;
  local_conversation_id?: string | null;
  last_event_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface DigitalHumanChannelBinding {
  id: string;
  installation_id: string;
  conversation_mapping_id: string;
  subject_type: 'digital_human';
  subject_id: string;
  wake_policy: DigitalHumanChannelWakePolicy;
  enabled: boolean;
  revision: number;
  created_at: string;
  updated_at: string;
}

export interface DigitalHumanChannelBindingView {
  binding: DigitalHumanChannelBinding;
  mapping: DigitalHumanExternalConversationMapping;
}

export interface DigitalHumanChannelConversationDiscovery {
  installation_id: string;
  external_conversation_id: string;
  conversation_type: DigitalHumanChannelConversationType;
  sender_external_id: string;
  event_count: number;
  last_event_at: string;
}

export interface CreateDigitalHumanChannelBindingRequest {
  installation_id: string;
  external_conversation_id: string;
  conversation_type: DigitalHumanChannelConversationType;
  wake_policy: DigitalHumanChannelWakePolicy;
}

export interface DigitalHumanChannelInboundEvent {
  id: string;
  installation_id: string;
  binding_id?: string | null;
  provider_event_id: string;
  provider_message_id: string;
  external_conversation_id: string;
  sender_external_id: string;
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
  inbound_event_id: string;
  run_id: string;
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
