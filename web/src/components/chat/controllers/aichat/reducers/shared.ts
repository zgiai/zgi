import type {
  AIChatMessageFile,
  AIChatGeneratedFile,
  AIChatMessageMetadata,
  AIChatSkillInvocation,
} from '@/services/types/aichat';
import { type AIChatAgenticTimelineItem } from '@/components/chat/controllers/aichat/types';

type RedisStreamEventIdParts = {
  timestamp: number;
  sequence: number;
};

function redisStreamEventIdParts(eventId?: string | null): RedisStreamEventIdParts | null {
  if (!eventId) return null;
  const match = /^(\d+)-(\d+)(?::|$)/.exec(eventId.trim());
  if (!match) return null;

  const timestamp = Number(match[1]);
  const sequence = Number(match[2]);
  if (!Number.isSafeInteger(timestamp) || !Number.isSafeInteger(sequence)) {
    return null;
  }

  return { timestamp, sequence };
}

export function compareAIChatStreamEventIds(
  left?: string | null,
  right?: string | null
): number | null {
  if (!left || !right) return null;
  if (left === right) return 0;

  const leftParts = redisStreamEventIdParts(left);
  const rightParts = redisStreamEventIdParts(right);
  if (!leftParts || !rightParts) return null;

  if (leftParts.timestamp !== rightParts.timestamp) {
    return leftParts.timestamp > rightParts.timestamp ? 1 : -1;
  }
  if (leftParts.sequence !== rightParts.sequence) {
    return leftParts.sequence > rightParts.sequence ? 1 : -1;
  }
  return 0;
}

export function isStaleAIChatStreamEvent(
  incomingEventId?: string | null,
  lastEventId?: string | null
): boolean {
  if (!incomingEventId || !lastEventId) return false;
  const comparison = compareAIChatStreamEventIds(incomingEventId, lastEventId);
  return comparison !== null ? comparison <= 0 : incomingEventId === lastEventId;
}

export function createAIChatFileMetadata(
  files?: AIChatMessageFile[]
): AIChatMessageMetadata | undefined {
  if (!files?.length) {
    return undefined;
  }

  return {
    file_count: files.length,
    files,
  };
}

export function mergeMessageMetadata(
  existingMetadata?: AIChatMessageMetadata,
  incomingMetadata?: AIChatMessageMetadata
): AIChatMessageMetadata | undefined {
  if (!existingMetadata && !incomingMetadata) {
    return undefined;
  }

  const existingFiles = existingMetadata?.files ?? [];
  const incomingFiles = incomingMetadata?.files ?? [];
  const files = mergeByIdentity(
    existingFiles,
    incomingFiles,
    fileMetadataIdentity,
    (existing, incoming) => ({ ...existing, ...incoming })
  );
  const existingGeneratedFiles = existingMetadata?.generated_files ?? [];
  const incomingGeneratedFiles = incomingMetadata?.generated_files ?? [];
  const generatedFiles = mergeByIdentity(
    existingGeneratedFiles,
    incomingGeneratedFiles,
    generatedFileIdentity,
    (existing, incoming) => ({ ...existing, ...incoming })
  );
  const existingWorkflowRuns = existingMetadata?.workflow_runs ?? [];
  const incomingWorkflowRuns = incomingMetadata?.workflow_runs ?? [];
  const workflowRuns = mergeByIdentity(
    existingWorkflowRuns,
    incomingWorkflowRuns,
    workflowRunIdentity,
    (existing, incoming) => ({ ...existing, ...incoming })
  );
  const userInputRequest =
    incomingMetadata?.user_input_request ?? existingMetadata?.user_input_request;
  const existingSkillInvocations = visibleSkillInvocations(existingMetadata?.skill_invocations);
  const incomingSkillInvocations = visibleSkillInvocations(incomingMetadata?.skill_invocations);
  const hasSkillInvocationMetadata = Boolean(
    existingMetadata?.skill_invocations || incomingMetadata?.skill_invocations
  );
  const skillInvocations = mergeByIdentity(
    existingSkillInvocations,
    incomingSkillInvocations,
    skillInvocationIdentity,
    mergeSkillInvocationByStatus
  );
  const loadedSkillIds = uniqueStrings(
    skillInvocations
      .filter(item => item.kind === 'skill_load' && item.status !== 'error')
      .map(item => item.skill_id)
  );
  const skillNames = uniqueStrings(skillInvocations.map(item => item.skill_id));
  const toolNames = uniqueStrings(
    skillInvocations
      .filter(item => item.kind === 'tool_call')
      .map(item => item.tool_name)
      .filter((toolName): toolName is string => Boolean(toolName))
  );

  return {
    ...(existingMetadata ?? {}),
    ...(incomingMetadata ?? {}),
    ...(files.length > 0
      ? {
          file_count: files.length,
          files,
        }
      : {}),
    ...(generatedFiles.length > 0
      ? {
          generated_file_count: generatedFiles.length,
          generated_files: generatedFiles,
        }
      : {}),
    ...(workflowRuns.length > 0
      ? {
          workflow_run_count: workflowRuns.length,
          workflow_runs: workflowRuns,
        }
      : {}),
    ...(hasSkillInvocationMetadata
      ? {
          has_trace: skillInvocations.length > 0,
          skill_invocations: skillInvocations,
          selected_skill_ids: skillNames,
          loaded_skill_ids: loadedSkillIds,
          skill_step_count: skillInvocations.length,
          skill_call_count: skillInvocations.length,
          skill_names: skillNames,
          tool_call_count: skillInvocations.filter(item => item.kind === 'tool_call').length,
          tool_names: toolNames,
        }
      : {}),
    ...(userInputRequest
      ? {
          user_input_request: userInputRequest,
        }
      : {}),
  };
}

function visibleSkillInvocations(
  invocations: AIChatSkillInvocation[] | undefined
): AIChatSkillInvocation[] {
  return (invocations ?? []).filter(invocation => {
    const status = String(invocation.status ?? '').toLowerCase();
    const result =
      invocation.result && typeof invocation.result === 'object' && !Array.isArray(invocation.result)
        ? (invocation.result as Record<string, unknown>)
        : {};
    const actionType =
      invocation.action_type ||
      (typeof result.action_type === 'string' ? result.action_type : undefined);
    if (
      invocation.kind === 'skill_load' &&
      invocation.skill_id === 'console-navigator'
    ) {
      return false;
    }
    if (
      invocation.kind === 'tool_call' &&
      (status === 'approved' || status === 'allowed') &&
      Object.keys(result).length === 0
    ) {
      return false;
    }
    if (
      invocation.kind === 'client_action' &&
      (actionType === 'asset_observation' || actionType === 'route_navigation') &&
      (status === 'success' || status === 'succeeded')
    ) {
      return false;
    }
    return (
      invocation.kind !== 'guardrail' &&
      invocation.kind !== 'metadata_exposed' &&
      invocation.kind !== 'memory_planner'
    );
  });
}

function mergeByIdentity<T>(
  existing: T[],
  incoming: T[],
  identity: (item: T, index: number) => string,
  merge: (existing: T, incoming: T) => T
): T[] {
  if (existing.length === 0) return incoming;
  if (incoming.length === 0) return existing;

  const next = existing.slice();
  const indexByIdentity = new Map<string, number>();
  next.forEach((item, index) => {
    const key = identity(item, index);
    if (key) indexByIdentity.set(key, index);
  });

  incoming.forEach((item, incomingIndex) => {
    const key = identity(item, next.length + incomingIndex);
    const existingIndex = key ? indexByIdentity.get(key) : undefined;
    if (existingIndex === undefined) {
      if (key) indexByIdentity.set(key, next.length);
      next.push(item);
      return;
    }
    next[existingIndex] = merge(next[existingIndex], item);
  });

  return next;
}

function fileMetadataIdentity(file: AIChatMessageFile, index: number): string {
  return file.id || `${file.name}:${file.extension}:${file.size}:${index}`;
}

function generatedFileIdentity(file: AIChatGeneratedFile, index: number): string {
  return (
    file.correlation_id ||
    file.file_id ||
    file.upload_file_id ||
    file.tool_file_id ||
    file.source_file_id ||
    `${file.filename}:${file.extension}:${file.size}:${index}`
  );
}

function workflowRunIdentity(run: { workflow_run_id?: string; task_id?: string; id?: string }, index: number): string {
  return run.workflow_run_id || run.task_id || run.id || `workflow:${index}`;
}

function invocationRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function invocationString(value: unknown): string {
  if (typeof value === 'string') return value.trim();
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return '';
}

function skillInvocationNavigationTarget(invocation: AIChatSkillInvocation): string {
  const result = invocationRecord(invocation.result);
  const args = invocationRecord(invocation.arguments);
  const record = invocation as unknown as Record<string, unknown>;
  const href =
    invocationString(record.href) ||
    invocationString(record.loaded_href) ||
    invocationString(record.current_href) ||
    invocationString(record.target_href) ||
    invocationString(result.href) ||
    invocationString(result.loaded_href) ||
    invocationString(result.current_href) ||
    invocationString(result.target_href) ||
    invocationString(args.href);
  return href.replace(/\/+$/, '') || href;
}

type AssetOperationSemanticIdentityInput = {
  audit?: unknown;
  result?: Record<string, unknown>;
  args?: Record<string, unknown>;
  assetType?: unknown;
  effect?: unknown;
  assets?: unknown;
  actionId?: unknown;
  correlationId?: unknown;
};

function normalizeAssetOperationActionId(value: unknown): string {
  const actionId = invocationString(value);
  if (!actionId) return '';
  return actionId.startsWith('asset_observation:')
    ? actionId.slice('asset_observation:'.length)
    : actionId;
}

function assetOperationSemanticIdentity(input: AssetOperationSemanticIdentityInput): string {
  const result = input.result ?? {};
  const args = input.args ?? {};
  const audit = invocationRecord(input.audit ?? result.asset_operation_audit);
  const operationGroup = invocationRecord(result.operation_group);
  const assetType = (
    invocationString(input.assetType) ||
    invocationString(audit.asset_type) ||
    invocationString(result.asset_type) ||
    invocationString(args.asset_type)
  ).toLowerCase();
  const effect = (
    invocationString(input.effect) ||
    invocationString(audit.effect) ||
    invocationString(result.effect) ||
    invocationString(args.effect)
  ).toLowerCase();
  if (!assetType || !effect) return '';

  const correlationId =
    invocationString(input.correlationId) ||
    invocationString(audit.correlation_id) ||
    invocationString(result.correlation_id) ||
    invocationString(operationGroup.correlation_id);
  if (correlationId) return `asset_operation:${correlationId}`;

  const actionId =
    normalizeAssetOperationActionId(input.actionId) ||
    normalizeAssetOperationActionId(result.action_id) ||
    normalizeAssetOperationActionId(args.action_id);
  if (actionId) return `asset_operation:${actionId}`;

  return [
    'asset_operation',
    assetType,
    effect,
    stableMetadataValue(
      input.assets ??
        audit.assets ??
        result.assets ??
        args.assets ??
        result.item_results ??
        operationGroup.item_results ??
        {}
    ),
  ].join(':');
}

function skillInvocationGovernanceCorrelationId(invocation: AIChatSkillInvocation): string {
  const record = invocation as unknown as Record<string, unknown>;
  const governance = invocationRecord(record.governance);
  const approvalEvent = invocationRecord(record.approval_event);
  const audit = invocationRecord(record.asset_operation_audit);
  const governanceAudit = invocationRecord(governance.asset_operation_audit);
  const matchedGrant = invocationRecord(governance.matched_grant);
  const auditMatchedGrant = invocationRecord(audit.matched_grant);
  const governanceAuditMatchedGrant = invocationRecord(governanceAudit.matched_grant);
  return (
    invocationString(record.correlation_id) ||
    invocationString(record.approved_by_correlation_id) ||
    invocationString(governance.correlation_id) ||
    invocationString(governance.approved_by_correlation_id) ||
    invocationString(approvalEvent.correlation_id) ||
    invocationString(approvalEvent.approved_by_correlation_id) ||
    invocationString(audit.correlation_id) ||
    invocationString(audit.approved_by_correlation_id) ||
    invocationString(governanceAudit.correlation_id) ||
    invocationString(governanceAudit.approved_by_correlation_id) ||
    invocationString(matchedGrant.approval_correlation_id) ||
    invocationString(auditMatchedGrant.approval_correlation_id) ||
    invocationString(governanceAuditMatchedGrant.approval_correlation_id)
  );
}

export function skillInvocationSemanticIdentity(invocation: AIChatSkillInvocation): string {
  if (invocation.kind === 'intermediate_answer' && invocation.answer_id) {
    return `intermediate_answer:${invocation.answer_id}`;
  }
  if (invocation.kind === 'client_action') {
    const record = invocation as unknown as Record<string, unknown>;
    const result = invocationRecord(invocation.result);
    const actionType =
      invocation.action_type ||
      invocationString(result.action_type);
    if (actionType === 'route_navigation') {
      const href = skillInvocationNavigationTarget(invocation);
      if (href) return `navigation:route:${href.toLowerCase()}`;
    }
    if (actionType === 'asset_observation') {
      const assetOperationIdentity = assetOperationSemanticIdentity({
        audit: record.asset_operation_audit,
        result,
        args: invocationRecord(invocation.arguments),
        actionId: record.action_id,
        correlationId: record.correlation_id,
      });
      if (assetOperationIdentity) return assetOperationIdentity;
      return [
        'client_action',
        'asset_observation',
        invocationString(record.asset_type) || invocationString(result.asset_type),
        invocationString(record.effect) || invocationString(result.effect),
        stableMetadataValue(record.assets ?? result.assets ?? {}),
      ].join(':');
    }
    if (invocation.action_id) {
      return `client_action:${invocation.action_id}`;
    }
  }
  if (
    invocation.kind === 'tool_call' &&
    invocation.skill_id === 'console-navigator' &&
    invocation.tool_name === 'navigate'
  ) {
    const href = skillInvocationNavigationTarget(invocation);
    if (href) return `navigation:route:${href.toLowerCase()}`;
  }
  if (invocation.kind === 'tool_call') {
    const record = invocation as unknown as Record<string, unknown>;
    const governanceCorrelationId = skillInvocationGovernanceCorrelationId(invocation);
    if (governanceCorrelationId) {
      return [
        'tool_call_governed',
        invocation.skill_id ?? '',
        invocation.tool_name ?? '',
        governanceCorrelationId,
      ].join(':');
    }
    const assetOperationIdentity = assetOperationSemanticIdentity({
      audit: record.asset_operation_audit,
      result: invocationRecord(invocation.result),
      args: invocationRecord(invocation.arguments),
      actionId: record.action_id,
      correlationId: record.correlation_id,
    });
    if (assetOperationIdentity) return assetOperationIdentity;
  }
  if (invocation.kind === 'tool_governance') {
    const correlationId = skillInvocationGovernanceCorrelationId(invocation);
    if (correlationId) {
      return [
        'tool_governance',
        invocation.skill_id ?? '',
        invocation.tool_name ?? '',
        correlationId,
      ].join(':');
    }
  }
  return '';
}

function skillInvocationIdentity(invocation: AIChatSkillInvocation, index: number): string {
  const semanticIdentity = skillInvocationSemanticIdentity(invocation);
  if (semanticIdentity) return semanticIdentity;
  if (invocation.runtime_id) return invocation.runtime_id;
  return [
    invocation.kind ?? 'tool_call',
    invocation.skill_id ?? '',
    invocation.tool_name ?? '',
    invocation.path ?? '',
    invocation.answer_id ?? '',
    stableMetadataValue(invocation.arguments ?? {}),
    index,
  ].join(':');
}

export function mergeSkillInvocationByStatus(
  existing: AIChatSkillInvocation,
  incoming: AIChatSkillInvocation
): AIChatSkillInvocation {
  if (skillInvocationStatusRank(incoming.status) < skillInvocationStatusRank(existing.status)) {
    return { ...incoming, ...existing };
  }
  return { ...existing, ...incoming };
}

function skillInvocationStatusRank(status: string | undefined): number {
  switch (String(status ?? '').toLowerCase()) {
    case 'error':
    case 'blocked':
    case 'denied':
      return 40;
    case 'success':
    case 'succeeded':
    case 'allowed':
    case 'completed':
    case 'approved':
      return 30;
    case 'needs_approval':
    case 'waiting_approval':
    case 'waiting_client_action':
    case 'waiting_question':
      return 20;
    case 'running':
    case 'loading':
      return 10;
    default:
      return 0;
  }
}

function stableMetadataValue(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value !== 'object') return String(value);
  if (Array.isArray(value)) return `[${value.map(stableMetadataValue).join(',')}]`;
  const record = value as Record<string, unknown>;
  return `{${Object.keys(record)
    .sort()
    .map(key => `${key}:${stableMetadataValue(record[key])}`)
    .join(',')}}`;
}

function uniqueStrings(values: Array<string | undefined>): string[] {
  return Array.from(
    new Set(values.map(value => value?.trim()).filter((value): value is string => Boolean(value)))
  );
}

export function clearRuntimeMessageMetadata(
  metadata?: AIChatMessageMetadata
): AIChatMessageMetadata | undefined {
  if (!metadata) return undefined;
  const next = { ...metadata };
  delete next.sensitiveOutputBlocked;
  delete next.has_trace;
  delete next.skill_invocations;
  delete next.selected_skill_ids;
  delete next.loaded_skill_ids;
  delete next.skill_step_count;
  delete next.skill_call_count;
  delete next.skill_names;
  delete next.tool_call_count;
  delete next.tool_names;
  delete next.generated_file_count;
  delete next.generated_files;
  delete next.workflow_run_count;
  delete next.workflow_runs;
  delete next.user_input_request;
  return next;
}

export function isTransientProgressItem(item: AIChatAgenticTimelineItem): boolean {
  return (
    item.type === 'progress_text' &&
    !item.content.trim() &&
    (item.transient === true || Boolean(item.phase))
  );
}

export function removeTransientProgressItems(
  timeline: AIChatAgenticTimelineItem[] | undefined
): AIChatAgenticTimelineItem[] {
  return (timeline ?? []).filter(item => !isTransientProgressItem(item));
}
