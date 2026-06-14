export type AIChatActionJsonValue =
  | string
  | number
  | boolean
  | null
  | AIChatActionJsonValue[]
  | { [key: string]: AIChatActionJsonValue };

export type AIChatActionMetadata = Record<string, unknown>;
export type AIChatActionTimestamp = number | string;

export type AIChatActionRiskLevel = 'low' | 'medium' | 'high' | 'critical' | (string & {});

export type AIChatActionRunStatus =
  | 'planned'
  | 'needs_confirmation'
  | 'confirmed'
  | 'canceled'
  | 'running'
  | 'blocked'
  | 'completed'
  | 'failed'
  | (string & {});

export type AIChatActionStepStatus =
  | 'pending'
  | 'running'
  | 'blocked'
  | 'done'
  | 'failed'
  | (string & {});

export type AIChatActionConfirmationStatus =
  | 'not_required'
  | 'pending'
  | 'confirmed'
  | 'canceled'
  | (string & {});

export interface AIChatActionResourceRef {
  type?: string;
  id?: string;
  name?: string;
  source?: string;
  metadata?: AIChatActionMetadata;
}

export interface AIChatActionRuntimeError {
  code?: string;
  message: string;
  details?: string;
  retryable?: boolean;
  metadata?: AIChatActionMetadata;
}

export interface AIChatActionRisk {
  level: AIChatActionRiskLevel;
  summary?: string;
  reasons?: string[];
  mitigations?: string[];
  requires_confirmation?: boolean;
  metadata?: AIChatActionMetadata;
}

export interface AIChatActionPermission {
  key: string;
  label?: string;
  description?: string;
  required?: boolean;
  granted?: boolean;
  reason?: string;
}

export type AIChatActionDelegationTarget =
  | 'agent'
  | 'workflow'
  | 'skill'
  | 'file'
  | 'automation'
  | 'human'
  | 'system'
  | (string & {});

export interface AIChatActionDelegation {
  target: AIChatActionDelegationTarget;
  target_id?: string;
  title?: string;
  capability_id?: string;
  metadata?: AIChatActionMetadata;
}

export interface AIChatActionLedgerEntry {
  id: string;
  action_id?: string;
  plan_id?: string;
  step_id?: string;
  type: string;
  title?: string;
  message?: string;
  created_at?: AIChatActionTimestamp;
  actor?: string;
  metadata?: AIChatActionMetadata;
}

export interface ActionPlanRequest {
  capability_id: string;
  conversation_id?: string;
  message_id?: string;
  idempotency_key?: string;
  intent?: string;
  title?: string;
  summary?: string;
  resources?: AIChatActionResourceRef[];
  arguments?: AIChatActionMetadata;
  operation_context?: AIChatActionMetadata;
  metadata?: AIChatActionMetadata;
  requires_confirmation?: boolean;
  risk_level?: AIChatActionRiskLevel;
}

export interface ConfirmActionRequest {
  confirmed: boolean;
  reason?: string;
  metadata?: AIChatActionMetadata;
}

export interface ExecuteActionRequest {
  dry_run?: boolean;
  metadata?: AIChatActionMetadata;
}

export interface ActionCapabilityResponse {
  id: string;
  domain: string;
  action: string;
  name: string;
  description: string;
  runtime: string;
  auth_mode: string;
  risk_level: AIChatActionRiskLevel;
  requires_confirmation: boolean;
  idempotency_required: boolean;
  token_ttl_seconds?: number;
  allowed_resources?: string[];
  scopes?: string[];
  title?: string;
  kind?: string;
  status?: string;
  risk?: AIChatActionRisk;
  permissions?: AIChatActionPermission[];
  supported_resource_types?: string[];
  metadata?: AIChatActionMetadata;
}

export interface ActionStepResponse {
  id: string;
  run_id: string;
  step_index: number;
  step_key: string;
  capability_id: string;
  title: string;
  status: AIChatActionStepStatus;
  risk_level: AIChatActionRiskLevel;
  requires_confirmation: boolean;
  started_at?: AIChatActionTimestamp;
  completed_at?: AIChatActionTimestamp;
  error?: string | AIChatActionRuntimeError;
  input?: AIChatActionMetadata;
  output?: AIChatActionMetadata;
  metadata?: AIChatActionMetadata;
  description?: string;
  risk?: AIChatActionRisk;
  confirmation_id?: string;
  confirmation_status?: AIChatActionConfirmationStatus;
  permissions?: AIChatActionPermission[];
  duration_ms?: number;
  progress?: number;
  result?: AIChatActionMetadata;
  target?: {
    resource_type: string;
    resource_id: string;
    title?: string;
    href?: string;
    metadata?: AIChatActionMetadata;
  };
  delegated_to?: AIChatActionDelegation;
  created_at: AIChatActionTimestamp;
  updated_at: AIChatActionTimestamp;
}

export interface ActionRunResponse {
  id: string;
  organization_id: string;
  workspace_id?: string;
  account_id: string;
  conversation_id?: string;
  message_id?: string;
  idempotency_key?: string;
  intent: string;
  capability_id: string;
  title: string;
  summary: string;
  status: AIChatActionRunStatus;
  risk_level: AIChatActionRiskLevel;
  requires_confirmation: boolean;
  confirmation_status: AIChatActionConfirmationStatus;
  confirmed_by?: string;
  confirmed_at?: AIChatActionTimestamp;
  canceled_at?: AIChatActionTimestamp;
  error?: string | AIChatActionRuntimeError;
  resources?: AIChatActionMetadata;
  arguments?: AIChatActionMetadata;
  ledger?: AIChatActionMetadata;
  metadata?: AIChatActionMetadata;
  capability?: ActionCapabilityResponse;
  capabilities?: ActionCapabilityResponse[];
  confirmations?: AIChatActionConfirmation[];
  permissions?: AIChatActionPermission[];
  plan_id?: string;
  goal?: string;
  risk?: AIChatActionRisk;
  progress?: {
    percent?: number;
    current_step_id?: string;
    current_step_title?: string;
    completed_steps?: number;
    total_steps?: number;
  };
  current_step_id?: string;
  started_at?: AIChatActionTimestamp;
  completed_at?: AIChatActionTimestamp;
  result?: AIChatActionMetadata;
  ledger_entries?: AIChatActionLedgerEntry[];
  steps: ActionStepResponse[];
  created_at: AIChatActionTimestamp;
  updated_at: AIChatActionTimestamp;
}

export interface AIChatActionConfirmation {
  id: string;
  action_id?: string;
  step_id?: string;
  status: AIChatActionConfirmationStatus;
  title?: string;
  summary?: string;
  prompt?: string;
  risk_level?: AIChatActionRiskLevel;
  risk?: AIChatActionRisk;
  requires_confirmation?: boolean;
  requested_at?: AIChatActionTimestamp;
  confirmed_at?: AIChatActionTimestamp;
  canceled_at?: AIChatActionTimestamp;
  expires_at?: AIChatActionTimestamp;
  reason?: string;
  metadata?: AIChatActionMetadata;
}

export type AIChatActionRuntimeEventName =
  | 'action_run_snapshot'
  | 'action_run_end'
  | 'error'
  | (string & {});

export interface AIChatActionRunEndEventData {
  action_run_id?: string;
  status?: AIChatActionRunStatus;
  [key: string]: unknown;
}

export type AIChatActionRuntimeEventPayload =
  | ActionRunResponse
  | AIChatActionRunEndEventData
  | { message?: string; [key: string]: unknown };

export interface AIChatActionRuntimeSseEnvelope<
  TData extends AIChatActionRuntimeEventPayload = AIChatActionRuntimeEventPayload,
> {
  event?: AIChatActionRuntimeEventName;
  data?: TData;
}

export type AIChatActionCapability = ActionCapabilityResponse;
export type AIChatActionPlan = ActionRunResponse;
export type AIChatActionRun = ActionRunResponse;
export type AIChatActionPlanStep = ActionStepResponse;
export type AIChatActionRunStep = ActionStepResponse;
export type AIChatCreateActionPlanRequest = ActionPlanRequest;
export type AIChatActionConfirmRequest = ConfirmActionRequest;
export type AIChatExecuteActionRequest = ExecuteActionRequest;
