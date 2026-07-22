import type { ApprovalRuntimeForm as ApprovalRuntimeFormData } from '@/services/approval.service';

export interface ParsedApprovalRuntimeEntry {
  formId: string;
  nodeId: string;
  nodeTitle: string;
  token: string;
  form: ApprovalRuntimeFormData | null;
  actionId: string | null;
}

export interface ParsedApprovalPausedEvent {
  isApproval: boolean;
  token: string;
  form: ApprovalRuntimeFormData | null;
  nodeIds: string[];
  entries: ParsedApprovalRuntimeEntry[];
}

export interface ParsedApprovalRuntimeFormEvent {
  token: string;
  form: ApprovalRuntimeFormData | null;
  formId: string;
  nodeId: string;
  nodeTitle: string;
  actionId: string | null;
}

export interface WorkflowSnapshotPauseEvent {
  [key: string]: unknown;
  event: 'workflow_paused';
  sequence?: number;
  data: Record<string, unknown>;
}

function getPayloadRecord(payload: unknown): Record<string, unknown> | null {
  if (!payload || typeof payload !== 'object') return null;
  const record = payload as Record<string, unknown>;
  const data = record.data;
  return data && typeof data === 'object' ? (data as Record<string, unknown>) : record;
}

function pickString(record: Record<string, unknown> | null, key: string): string {
  const value = record?.[key];
  return typeof value === 'string' ? value : '';
}

function createParsedApprovalEntry(
  record: Record<string, unknown> | null,
  form: ApprovalRuntimeFormData | null = null
): ParsedApprovalRuntimeEntry {
  const approvalForm =
    record?.approval_form && typeof record.approval_form === 'object'
      ? (record.approval_form as Record<string, unknown>)
      : null;
  return {
    formId: pickString(record, 'form_id') || pickString(approvalForm, 'id') || form?.id || '',
    nodeId: pickString(record, 'node_id') || form?.node_id || '',
    nodeTitle:
      pickString(record, 'node_title') || pickString(record, 'title') || form?.node_title || '',
    token: pickString(record, 'token') || pickString(approvalForm, 'token') || form?.token || '',
    form,
    actionId: pickString(record, 'action_id') || null,
  };
}

function isPendingPauseReason(reason: unknown): reason is Record<string, unknown> {
  if (!reason || typeof reason !== 'object') return false;
  const status = pickString(reason as Record<string, unknown>, 'status').toLowerCase();
  return !status || status === 'pending' || status === 'waiting';
}

function isActionablePause(pause: Record<string, unknown>) {
  const status = pickString(pause, 'status').toLowerCase();
  return !status || status === 'paused' || status === 'pending' || status === 'waiting';
}

export function parseApprovalPausedEvent(payload: unknown): ParsedApprovalPausedEvent {
  const root = getPayloadRecord(payload);
  if (!root) return { isApproval: false, token: '', form: null, nodeIds: [], entries: [] };

  const nodeType = typeof root.node_type === 'string' ? root.node_type : '';
  const reasons = Array.isArray(root.reasons) ? root.reasons.filter(isPendingPauseReason) : [];
  const nodeIds = new Set<string>();
  const entries: ParsedApprovalRuntimeEntry[] = [];
  const hasApprovalReason = reasons.some(reason => {
    if (!reason || typeof reason !== 'object') return false;
    const record = reason as Record<string, unknown>;
    const isApproval = record.type === 'approval_required' || typeof record.form_id === 'string';
    if (isApproval) {
      if (typeof record.node_id === 'string' && record.node_id.trim().length > 0) {
        nodeIds.add(record.node_id);
      }
      entries.push(createParsedApprovalEntry(record, normalizeApprovalRuntimeForm(record)));
    }
    return isApproval;
  });
  if (
    nodeType === 'approval' &&
    typeof root.node_id === 'string' &&
    root.node_id.trim().length > 0
  ) {
    nodeIds.add(root.node_id);
    if (entries.length === 0) entries.push(createParsedApprovalEntry(root));
  }
  // Legacy single-reason events may only carry paused_nodes. Only use that fallback
  // when the event itself is explicitly typed as approval; V2 mixed pauses derive
  // node ownership from their typed reasons above.
  if (nodeType === 'approval' && reasons.length === 0) {
    const pausedNodes = Array.isArray(root.paused_nodes) ? root.paused_nodes : [];
    pausedNodes.forEach(nodeId => {
      if (typeof nodeId === 'string' && nodeId.trim().length > 0) {
        nodeIds.add(nodeId);
      }
    });
  }
  const isApproval = nodeType === 'approval' || hasApprovalReason;

  return { isApproval, token: '', form: null, nodeIds: Array.from(nodeIds), entries };
}

/**
 * Rebuilds the public paused event from a V2 workflow snapshot.
 *
 * A snapshot cursor already covers every event up to `last_sequence`, so consumers must
 * restore the active interaction from `active_pause` instead of waiting for an earlier
 * approval/question event to be replayed.
 */
export function createWorkflowSnapshotPauseEvent(
  payload: unknown
): WorkflowSnapshotPauseEvent | null {
  const snapshot = getPayloadRecord(payload);
  if (!snapshot) return null;
  const activePause =
    snapshot.active_pause && typeof snapshot.active_pause === 'object'
      ? (snapshot.active_pause as Record<string, unknown>)
      : null;
  if (!activePause) return null;

  const pause =
    activePause.pause && typeof activePause.pause === 'object'
      ? (activePause.pause as Record<string, unknown>)
      : {};
  if (!isActionablePause(pause)) return null;
  const reasons = Array.isArray(activePause.reasons)
    ? activePause.reasons.filter(isPendingPauseReason)
    : [];
  if (reasons.length === 0) return null;
  const sequence =
    typeof snapshot.last_sequence === 'number'
      ? snapshot.last_sequence
      : getApprovalEventSequence(
          payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {}
        );

  return {
    event: 'workflow_paused',
    ...(typeof sequence === 'number' ? { sequence } : {}),
    data: {
      ...pause,
      status: 'paused',
      reasons,
      paused_nodes: reasons
        .map(reason =>
          reason && typeof reason === 'object' ? (reason as Record<string, unknown>).node_id : null
        )
        .filter((nodeId): nodeId is string => typeof nodeId === 'string' && nodeId.length > 0),
    },
  };
}

export function normalizeApprovalRuntimeForm(value: unknown): ApprovalRuntimeFormData | null {
  if (!value || typeof value !== 'object') return null;
  const record = value as Record<string, unknown>;
  if (!Array.isArray(record.fields) || !Array.isArray(record.actions)) return null;

  const formId =
    typeof record.id === 'string'
      ? record.id
      : typeof record.form_id === 'string'
        ? record.form_id
        : '';
  const expiresAt =
    typeof record.expiration_at === 'number'
      ? record.expiration_at
      : typeof record.expires_at === 'number'
        ? record.expires_at
        : undefined;

  return {
    ...(record as unknown as ApprovalRuntimeFormData),
    id: formId,
    token: typeof record.token === 'string' ? record.token : '',
    node_id: typeof record.node_id === 'string' ? record.node_id : '',
    node_title: typeof record.node_title === 'string' ? record.node_title : '',
    content: typeof record.content === 'string' ? record.content : '',
    fields: record.fields as ApprovalRuntimeFormData['fields'],
    actions: record.actions as ApprovalRuntimeFormData['actions'],
    expiration_at: expiresAt,
  };
}

export function parseApprovalRequestedEvent(payload: unknown): ParsedApprovalRuntimeFormEvent {
  const root = getPayloadRecord(payload);
  const form = normalizeApprovalRuntimeForm(root);
  const entry = createParsedApprovalEntry(root, form);
  return {
    token: entry.token,
    form,
    formId: entry.formId,
    nodeId: entry.nodeId,
    nodeTitle: entry.nodeTitle,
    actionId: entry.actionId,
  };
}

export function parseApprovalResultFilledEvent(payload: unknown): ParsedApprovalRuntimeFormEvent {
  const root = getPayloadRecord(payload);
  const entry = createParsedApprovalEntry(root);
  return {
    token: entry.token,
    form: null,
    formId: entry.formId,
    nodeId: entry.nodeId,
    nodeTitle: entry.nodeTitle,
    actionId: entry.actionId,
  };
}

export function parseApprovalExpiredEvent(payload: unknown): ParsedApprovalRuntimeFormEvent {
  const root = getPayloadRecord(payload);
  const entry = createParsedApprovalEntry(root);
  return {
    token: entry.token,
    form: null,
    formId: entry.formId,
    nodeId: entry.nodeId,
    nodeTitle: entry.nodeTitle,
    actionId: null,
  };
}

export function getApprovalEventSequence(event: { [key: string]: unknown }): number | null {
  const sequence = event.sequence ?? event.sequence_number;
  if (typeof sequence === 'number') return sequence;
  const data = event.data;
  if (data && typeof data === 'object') {
    const payload = data as Record<string, unknown>;
    const payloadSequence = payload.sequence ?? payload.sequence_number;
    if (typeof payloadSequence === 'number') return payloadSequence;
  }
  return null;
}
