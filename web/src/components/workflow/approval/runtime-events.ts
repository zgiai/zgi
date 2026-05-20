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
    nodeTitle: pickString(record, 'node_title') || pickString(record, 'title') || form?.node_title || '',
    token: pickString(record, 'token') || pickString(approvalForm, 'token') || form?.token || '',
    form,
    actionId: pickString(record, 'action_id') || null,
  };
}

export function parseApprovalPausedEvent(payload: unknown): ParsedApprovalPausedEvent {
  const root = getPayloadRecord(payload);
  if (!root) return { isApproval: false, token: '', form: null, nodeIds: [], entries: [] };

  const nodeType = typeof root.node_type === 'string' ? root.node_type : '';
  const reasons = Array.isArray(root.reasons) ? root.reasons : [];
  const nodeIds = new Set<string>();
  const entries: ParsedApprovalRuntimeEntry[] = [];
  const hasApprovalReason = reasons.some(reason => {
    if (!reason || typeof reason !== 'object') return false;
    const record = reason as Record<string, unknown>;
    if (typeof record.node_id === 'string' && record.node_id.trim().length > 0) {
      nodeIds.add(record.node_id);
    }
    const isApproval = record.type === 'approval_required' || typeof record.form_id === 'string';
    if (isApproval) entries.push(createParsedApprovalEntry(record));
    return isApproval;
  });
  const pausedNodes = Array.isArray(root.paused_nodes) ? root.paused_nodes : [];
  pausedNodes.forEach(nodeId => {
    if (typeof nodeId === 'string' && nodeId.trim().length > 0) {
      nodeIds.add(nodeId);
    }
  });
  if (nodeType === 'approval' && typeof root.node_id === 'string' && root.node_id.trim().length > 0) {
    nodeIds.add(root.node_id);
    if (entries.length === 0) entries.push(createParsedApprovalEntry(root));
  }
  const isApproval = nodeType === 'approval' || hasApprovalReason;

  return { isApproval, token: '', form: null, nodeIds: Array.from(nodeIds), entries };
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
