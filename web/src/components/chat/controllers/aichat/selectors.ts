import type {
  AIChatConversation,
  AIChatMessage,
  AIChatSkillInvocation,
  AIChatToolGovernanceDecisionEventData,
  AIChatWorkflowRunMetadata,
  AIChatWorkflowRunNodeMetadata,
} from '@/services/types/aichat';
import {
  DEFAULT_AICHAT_MESSAGE_PAGINATION,
  type AIChatAgenticTimelineItem,
  type AIChatControllerState,
  type AIChatPagination,
  type AIChatStreamingMessageState,
} from '@/components/chat/controllers/aichat/types';
import {
  buildChatBranchNavigationByMessageId,
  buildChatMessageById,
  getCurrentChatPathIds,
  materializeChatMessages,
  type ChatMessageTopology,
} from '@/components/chat/utils/message-tree';
import { upsertAIChatMessage } from '@/components/chat/utils/aichat-message';
import type { NodeInfo, RunStatus } from '@/components/chat/types';
import {
  governanceCorrelationIdFromInvocation,
  isPendingToolGovernanceInvocation,
} from './governance';

const EMPTY_AICHAT_MESSAGES: AIChatMessage[] = [];

type PersistedSkillInvocation = AIChatSkillInvocation & {
  arguments_summary?: Record<string, unknown> | null;
};

function normalizeSkillInvocation(invocation: AIChatSkillInvocation): AIChatSkillInvocation {
  const persisted = invocation as PersistedSkillInvocation;
  return {
    ...invocation,
    status: invocation.status === 'loaded' ? 'success' : invocation.status,
    arguments: invocation.arguments ?? persisted.arguments_summary,
  };
}

function isVisibleSkillInvocation(invocation: AIChatSkillInvocation): boolean {
  const status = String(invocation.status ?? '').toLowerCase();
  const result =
    invocation.result && typeof invocation.result === 'object' && !Array.isArray(invocation.result)
      ? (invocation.result as Record<string, unknown>)
      : {};
  const actionType =
    invocation.action_type ||
    (typeof result.action_type === 'string' ? result.action_type : undefined);
  if (invocation.kind === 'guardrail') {
    return false;
  }
  if (
    invocation.kind === 'skill_load' &&
    invocation.skill_id === 'console-navigator'
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
    invocation.kind !== 'metadata_exposed' &&
    invocation.kind !== 'memory_planner' &&
    invocation.kind !== 'planner_feedback' &&
    invocation.kind !== 'user_input_request'
  );
}

function isTerminalGovernedSkillInvocation(invocation: AIChatSkillInvocation): boolean {
  if (!invocation.governance || invocation.kind === 'tool_governance') return false;
  const status = String(invocation.status ?? '').toLowerCase();
  return (
    status === 'success' ||
    status === 'succeeded' ||
    status === 'allowed' ||
    status === 'error' ||
    status === 'blocked' ||
    status === 'denied'
  );
}

function toolGovernanceEventFromInvocation(
  message: AIChatMessage,
  invocation: AIChatSkillInvocation
): AIChatToolGovernanceDecisionEventData {
  const governance = invocation.governance ?? undefined;
  const approvalEvent = governance?.approval_event;
  const approvalResult = governance?.approval_result;
  const status = governance?.status ?? invocation.status;
  return {
    conversation_id: message.conversation_id,
    message_id: message.id,
    skill_id: invocation.skill_id || governance?.manifest?.skill_id,
    tool_name: invocation.tool_name,
    status,
    decision: status,
    duration_ms: invocation.duration_ms,
    created_at: invocation.created_at,
    execution_status: invocation.status,
    execution_error:
      invocation.status === 'error' ? (invocation.error ?? invocation.message) : undefined,
    execution_message: invocation.message,
    execution_duration_ms: invocation.duration_ms,
    execution_result: invocation.result,
    governance,
    correlation_id: governanceCorrelationIdFromInvocation(invocation),
    requires_approval: governance?.requires_approval,
    reason: governance?.reason,
    risk_level: governance?.manifest?.risk_level ?? approvalEvent?.risk_level,
    effect: governance?.manifest?.effect ?? approvalEvent?.effect,
    asset_type: governance?.manifest?.asset_type ?? approvalEvent?.asset_type,
    asset_operation_audit:
      invocation.asset_operation_audit ??
      governance?.asset_operation_audit ??
      (governance?.model_feedback?.asset_operation_audit as
        | AIChatToolGovernanceDecisionEventData['asset_operation_audit']
        | undefined),
    approval_status:
      governance?.approval_status ??
      (approvalResult?.approval_status as AIChatToolGovernanceDecisionEventData['approval_status']),
    approval_event: approvalEvent,
    matched_grant: governance?.matched_grant,
    approval_result: approvalResult,
    model_feedback:
      governance?.model_feedback ??
      (approvalResult?.model_feedback as Record<string, unknown> | undefined),
    session_grant: approvalResult?.session_grant as Record<string, unknown> | undefined,
  };
}

function workflowString(value: unknown): string | undefined {
  if (typeof value === 'string' && value.trim()) return value.trim();
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return undefined;
}

function workflowElapsedMs(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : undefined;
}

function workflowRunId(run: AIChatWorkflowRunMetadata): string {
  return (
    workflowString(run.workflow_run_id) ??
    workflowString(run.task_id) ??
    workflowString(run.id) ??
    ''
  );
}

function normalizeWorkflowRunStatus(status: unknown): RunStatus {
  switch (String(status ?? '').toLowerCase()) {
    case 'running':
      return 'running';
    case 'paused':
    case 'pending_approval':
      return 'pending_approval';
    case 'pending_question':
      return 'pending_question';
    case 'succeeded':
    case 'success':
    case 'completed':
      return 'completed';
    case 'stopped':
      return 'stopped';
    case 'expired':
      return 'expired';
    case 'failed':
    case 'error':
      return 'error';
    default:
      return 'running';
  }
}

function isWorkflowApprovalClosed(run: AIChatWorkflowRunMetadata): boolean {
  const approvalStatus = String(run.approval?.status ?? '').toLowerCase();
  return Boolean(
    run.approval_result ||
      run.approval_expired ||
      approvalStatus === 'submitted' ||
      approvalStatus === 'approved' ||
      approvalStatus === 'rejected' ||
      approvalStatus === 'expired'
  );
}

function normalizePersistedWorkflowRunStatus(run: AIChatWorkflowRunMetadata): RunStatus {
  const status = normalizeWorkflowRunStatus(run.status);
  if (status === 'pending_approval' && isWorkflowApprovalClosed(run)) {
    return run.approval_expired ? 'expired' : 'running';
  }
  return status;
}

function normalizeWorkflowNodeStatus(status: unknown): NodeInfo['status'] {
  switch (String(status ?? '').toLowerCase()) {
    case 'failed':
    case 'error':
    case 'exception':
      return 'failed';
    case 'paused':
    case 'pending_approval':
      return 'paused';
    case 'success':
    case 'succeeded':
    case 'completed':
      return 'success';
    case 'stopped':
      return 'stopped';
    case 'partial-succeeded':
      return 'partial-succeeded';
    case 'running':
      return 'running';
    default:
      return 'running';
  }
}

function normalizeWorkflowNodeType(value: unknown): string | undefined {
  const raw = workflowString(value);
  if (!raw) return undefined;
  const hyphen = raw.replace(/_/g, '-').toLowerCase();
  switch (hyphen) {
    case 'database':
      return 'call-database';
    case 'http':
    case 'http-request':
      return 'http-request';
    case 'assign':
    case 'assigner':
      return 'assigner';
    default:
      return hyphen;
  }
}

function sumWorkflowNodeElapsedMs(nodes: NodeInfo[]): number | undefined {
  const total = nodes.reduce((sum, node) => sum + (node.elapsedTime ?? 0), 0);
  return total > 0 ? total : undefined;
}

function mapPersistedWorkflowNode(node: AIChatWorkflowRunNodeMetadata): NodeInfo {
  const nodeId =
    workflowString(node.node_id) ?? workflowString(node.execution_id) ?? workflowString(node.id);
  const nodeType = normalizeWorkflowNodeType(node.node_type ?? node.type);
  const iterationRounds = (node.iteration_rounds ?? []).map((round, index) => {
    const nodes = (round.nodes ?? []).map(mapPersistedWorkflowNode);
    return {
      index: typeof round.index === 'number' ? round.index : index,
      elapsedTime: workflowElapsedMs(round.elapsed_time) ?? sumWorkflowNodeElapsedMs(nodes),
      nodes,
    };
  });
  const loopRounds = (node.loop_rounds ?? []).map((round, index) => {
    const nodes = (round.nodes ?? []).map(mapPersistedWorkflowNode);
    return {
      index: typeof round.index === 'number' ? round.index : index,
      elapsedTime: workflowElapsedMs(round.elapsed_time) ?? sumWorkflowNodeElapsedMs(nodes),
      variables: round.variables,
      nodes,
    };
  });
  return {
    status: normalizeWorkflowNodeStatus(node.status),
    error: workflowString(node.error),
    elapsedTime: workflowElapsedMs(node.elapsed_time),
    nodeId,
    nodeType,
    title:
      workflowString(node.title) ??
      workflowString(node.node_title) ??
      workflowString(node.name) ??
      workflowString(node.label) ??
      nodeType ??
      nodeId,
    data: {
      input: node.inputs,
      output: node.outputs,
    },
    iterationInputs: node.iteration_inputs,
    iterationOutputs: node.iteration_outputs,
    iterationRounds,
    loopInputs: node.loop_inputs,
    loopOutputs: node.loop_outputs,
    loopRounds,
    steps: typeof node.steps === 'number' ? node.steps : undefined,
  };
}

function workflowTimelineFromMessage(message: AIChatMessage): AIChatAgenticTimelineItem[] {
  const runs = message.metadata?.workflow_runs ?? [];
  return runs
    .map((run, index): AIChatAgenticTimelineItem | null => {
      const runId = workflowRunId(run);
      if (!runId) return null;
      return {
        id: `history-workflow-${message.id}-${runId}-${index}`,
        type: 'workflow_run',
        workflowRunId: runId,
        status: normalizePersistedWorkflowRunStatus(run),
        elapsedTime: workflowElapsedMs(run.elapsed_time),
        error: workflowString(run.error),
        nodes: (run.nodes ?? []).map(mapPersistedWorkflowNode),
        approval: run.approval,
        created_at: run.created_at,
      };
    })
    .filter((item): item is AIChatAgenticTimelineItem => item !== null);
}

export function hasRunningMessageState(
  streamingByMessageId: Record<string, AIChatStreamingMessageState>,
  conversationId: string
): boolean {
  return Object.values(streamingByMessageId).some(
    streaming => streaming.conversation_id === conversationId && streaming.status === 'streaming'
  );
}

export function shouldTreatConversationAsRunning(
  state: AIChatControllerState,
  conversationId: string | null | undefined
): boolean {
  if (!conversationId) return false;
  const conversation = state.conversations.find(item => item.id === conversationId);

  return Boolean(
    conversation?.runtime_status === 'streaming' ||
      state.recoveringByConversation[conversationId] ||
      state.stoppingByConversation[conversationId] ||
      hasRunningMessageState(state.streamingByMessageId, conversationId)
  );
}

export function getNextActiveSendingState(
  state: AIChatControllerState,
  conversationId: string | null | undefined,
  isSending: boolean
): boolean {
  return state.activeConversationId === conversationId ? isSending : state.isSending;
}

export function mergeSelectedMessagesWithStreamingState(
  conversation: AIChatConversation,
  incomingMessages: AIChatMessage[],
  current: AIChatControllerState
): AIChatMessage[] {
  const messageId = conversation.active_message_id;
  if (conversation.runtime_status !== 'streaming' || !messageId) {
    return incomingMessages;
  }

  const streamingState = current.streamingByMessageId[messageId];
  const localMessages = current.messagesByConversation[conversation.id] ?? [];
  const localMessage = localMessages.find(message => message.id === messageId);
  if (!localMessage) {
    return incomingMessages;
  }

  const serverMessage = incomingMessages.find(message => message.id === messageId);
  const shouldPreferServer =
    !streamingState?.last_event_id &&
    localMessage.status !== 'streaming' &&
    localMessage.answer.length < (serverMessage?.answer.length ?? 0);
  if (shouldPreferServer) {
    return incomingMessages;
  }

  const preservedMessage: AIChatMessage = {
    ...(serverMessage ?? localMessage),
    answer: localMessage.answer,
    status: 'streaming',
    updated_at: Math.max(serverMessage?.updated_at ?? 0, localMessage.updated_at),
  };

  return upsertAIChatMessage(incomingMessages, preservedMessage);
}

export function timelineFromAIChatMessage(message: AIChatMessage): AIChatAgenticTimelineItem[] {
  const invocations = (message.metadata?.skill_invocations ?? [])
    .filter(isVisibleSkillInvocation)
    .map(normalizeSkillInvocation);
  const governanceCorrelationIds = new Set(
    invocations
      .filter(invocation => invocation.kind === 'tool_governance')
      .map(governanceCorrelationIdFromInvocation)
      .filter((correlationId): correlationId is string => Boolean(correlationId))
  );

  const skillTimeline = invocations.map((invocation, index): AIChatAgenticTimelineItem => {
    const correlationId = governanceCorrelationIdFromInvocation(invocation) ?? String(index);
    const hasGovernance = Boolean(invocation.governance);
    const shouldRenderAsGovernanceDecision =
      invocation.kind === 'tool_governance' ||
      isTerminalGovernedSkillInvocation(invocation) ||
      (hasGovernance && !governanceCorrelationIds.has(correlationId)) ||
      (isPendingToolGovernanceInvocation(invocation) &&
        !governanceCorrelationIds.has(correlationId));

    if (shouldRenderAsGovernanceDecision) {
      return {
        id: `history-governance-${message.id}-${correlationId}`,
        type: 'tool_governance_decision',
        event: toolGovernanceEventFromInvocation(message, invocation),
        created_at: invocation.created_at,
      };
    }
    if (invocation.kind === 'intermediate_answer' && invocation.message) {
      return {
        id: `history-intermediate-${message.id}-${invocation.answer_id ?? index}`,
        type: 'intermediate_answer',
        answer_id: invocation.answer_id,
        title: invocation.title,
        content: invocation.message,
        status: invocation.status === 'success' ? 'success' : undefined,
        created_at: invocation.created_at,
      };
    }
    return {
      id: `history-skill-${message.id}-${index}`,
      type: 'skill_event',
      invocation,
      created_at: invocation.created_at,
    };
  });

  return sortTimelineItems(
    dedupeTimelineItems([...skillTimeline, ...workflowTimelineFromMessage(message)])
  );
}

function sortTimelineItems(timeline: AIChatAgenticTimelineItem[]): AIChatAgenticTimelineItem[] {
  return [...timeline].sort((left, right) => {
    const leftAt = left.created_at ?? Number.MAX_SAFE_INTEGER;
    const rightAt = right.created_at ?? Number.MAX_SAFE_INTEGER;
    return leftAt - rightAt || left.id.localeCompare(right.id);
  });
}

function stableTimelineValue(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value !== 'object') return String(value);
  if (Array.isArray(value)) {
    return `[${value.map(stableTimelineValue).join(',')}]`;
  }
  const record = value as Record<string, unknown>;
  return `{${Object.keys(record)
    .sort()
    .map(key => `${key}:${stableTimelineValue(record[key])}`)
    .join(',')}}`;
}

function timelineString(value: unknown): string {
  if (typeof value === 'string') return value.trim();
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return '';
}

function timelineRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function skillInvocationNavigationTarget(invocation: AIChatSkillInvocation): string {
  const result = timelineRecord(invocation.result);
  const args = timelineRecord(invocation.arguments);
  const record = invocation as unknown as Record<string, unknown>;
  const href =
    timelineString(record.href) ||
    timelineString(record.loaded_href) ||
    timelineString(record.current_href) ||
    timelineString(record.target_href) ||
    timelineString(result.href) ||
    timelineString(result.loaded_href) ||
    timelineString(result.current_href) ||
    timelineString(result.target_href) ||
    timelineString(args.href);
  return href.replace(/\/+$/, '') || href;
}

function governanceCorrelationId(value: unknown): string {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return '';
  const record = value as Record<string, unknown>;
  const governance =
    record.governance && typeof record.governance === 'object' && !Array.isArray(record.governance)
      ? (record.governance as Record<string, unknown>)
      : undefined;
  const approvalEvent =
    record.approval_event &&
    typeof record.approval_event === 'object' &&
    !Array.isArray(record.approval_event)
      ? (record.approval_event as Record<string, unknown>)
      : undefined;
  const audit =
    record.asset_operation_audit &&
    typeof record.asset_operation_audit === 'object' &&
    !Array.isArray(record.asset_operation_audit)
      ? (record.asset_operation_audit as Record<string, unknown>)
      : undefined;
  return (
    timelineString(record.correlation_id) ||
    timelineString(governance?.correlation_id) ||
    timelineString(approvalEvent?.correlation_id) ||
    timelineString(audit?.correlation_id)
  );
}

function timelineSkillInvocationIdentity(invocation: AIChatSkillInvocation): string {
  if (invocation.kind === 'client_action') {
    const record = invocation as unknown as Record<string, unknown>;
    const result = timelineRecord(invocation.result);
    const actionType = (
      timelineString(invocation.action_type) ||
      timelineString(result.action_type)
    ).toLowerCase();
    if (actionType === 'route_navigation') {
      const href = skillInvocationNavigationTarget(invocation);
      if (href) return `skill:navigation:route:${href.toLowerCase()}`;
    }
    if (actionType === 'asset_observation') {
      return [
        'skill',
        'client_action',
        'asset_observation',
        timelineString(record.asset_type) || timelineString(result.asset_type),
        timelineString(record.effect) || timelineString(result.effect),
        stableTimelineValue(record.assets ?? result.assets ?? {}),
      ].join(':');
    }
    if (invocation.action_id) {
      return `skill:client_action:${invocation.action_id}`;
    }
  }
  if (
    invocation.kind === 'tool_call' &&
    invocation.skill_id === 'console-navigator' &&
    invocation.tool_name === 'navigate'
  ) {
    const href = skillInvocationNavigationTarget(invocation);
    if (href) return `skill:navigation:route:${href.toLowerCase()}`;
  }
  if (invocation.kind === 'tool_governance') {
    const correlationId = governanceCorrelationId(invocation);
    if (correlationId) return `skill:tool_governance:${correlationId}`;
  }

  const argumentsKey = stableTimelineValue(invocation.arguments ?? {});
  return [
    'skill',
    invocation.kind ?? 'tool_call',
    invocation.skill_id ?? '',
    invocation.tool_name ?? '',
    invocation.path ?? '',
    invocation.answer_id ?? '',
    argumentsKey,
  ].join(':');
}

function timelineSkillInvocationBaseIdentity(invocation: AIChatSkillInvocation): string {
  return [
    'skill-base',
    invocation.kind ?? 'tool_call',
    invocation.skill_id ?? '',
    invocation.tool_name ?? '',
    invocation.path ?? '',
    invocation.answer_id ?? '',
  ].join(':');
}

function timelineItemIdentity(item: AIChatAgenticTimelineItem): string {
  switch (item.type) {
    case 'progress_text': {
      if (item.phase === 'client_action' || item.phase === 'client_action_result') {
        const result = timelineRecord(item.result);
        const actionType = (
          timelineString(item.action_type) ||
          timelineString(result.action_type)
        ).toLowerCase();
        if (actionType === 'route_navigation') {
          const href =
            timelineString(result.href) ||
            timelineString(result.loaded_href) ||
            timelineString(result.current_href) ||
            timelineString(result.target_href);
          if (href) {
            return `progress:client_action:route_navigation:${href.replace(/\/+$/, '').toLowerCase()}`;
          }
        }
        if (actionType === 'asset_observation') {
          return [
            'progress',
            'client_action',
            'asset_observation',
            timelineString(item.asset_type) || timelineString(result.asset_type),
            timelineString(item.effect) || timelineString(result.effect),
            stableTimelineValue(item.assets ?? result.assets ?? {}),
          ].join(':');
        }
      }
      return [
        'progress',
        item.phase ?? '',
        item.skill_id ?? '',
        item.tool_name ?? '',
        item.content.trim().replace(/\s+/g, ' '),
      ].join(':');
    }
    case 'skill_event': {
      return timelineSkillInvocationIdentity(item.invocation);
    }
    case 'intermediate_answer':
      return ['intermediate', item.answer_id ?? item.id].join(':');
    case 'memory_event':
      return ['memory', item.event_id ?? item.id].join(':');
    case 'tool_governance_decision':
      return ['governance', governanceCorrelationId(item.event) || item.id].join(':');
    case 'workflow_run':
      return ['workflow', item.workflowRunId].join(':');
  }
}

function timelineItemRank(item: AIChatAgenticTimelineItem): number {
  if (item.type === 'skill_event') {
    const status = String(item.invocation.status ?? '').toLowerCase();
    if (status === 'error' || status === 'blocked' || status === 'denied') return 40;
    if (status === 'success' || status === 'succeeded' || status === 'allowed') return 30;
    if (status === 'needs_approval' || status === 'waiting_client_action') return 20;
    if (status === 'running' || status === 'loading') return 10;
  }
  if (item.type === 'tool_governance_decision') {
    const status = String(
      item.event.approval_status ?? item.event.status ?? item.event.decision ?? ''
    ).toLowerCase();
    if (status === 'rejected' || status === 'denied' || status === 'error') return 40;
    if (status === 'approved' || status === 'success' || status === 'allowed') return 30;
    if (status === 'needs_approval') return 20;
  }
  if (item.type === 'intermediate_answer' && item.status === 'success') return 30;
  return item.event_id ? 5 : 0;
}

function preferTimelineItem(
  existing: AIChatAgenticTimelineItem,
  incoming: AIChatAgenticTimelineItem
): AIChatAgenticTimelineItem {
  const existingRank = timelineItemRank(existing);
  const incomingRank = timelineItemRank(incoming);
  if (incomingRank > existingRank) return incoming;
  if (incomingRank < existingRank) return existing;
  if ((incoming.created_at ?? 0) >= (existing.created_at ?? 0)) return incoming;
  return existing;
}

function pendingSkillTimelineBaseIdentity(item: AIChatAgenticTimelineItem): string {
  if (item.type !== 'skill_event') return '';
  const rank = timelineItemRank(item);
  if (rank <= 0 || rank >= 30) return '';
  return timelineSkillInvocationBaseIdentity(item.invocation);
}

export function dedupeTimelineItems(
  timeline: AIChatAgenticTimelineItem[] | undefined
): AIChatAgenticTimelineItem[] {
  const items = (timeline ?? []).filter(item => {
    if (item.type !== 'skill_event') return true;
    return item.invocation.kind !== 'guardrail';
  });
  if (items.length <= 1) return items;

  const indexByIdentity = new Map<string, number>();
  const pendingIndexByBaseIdentity = new Map<string, number>();
  const out: AIChatAgenticTimelineItem[] = [];
  for (const item of items) {
    const identity = timelineItemIdentity(item);
    const existingIndex = indexByIdentity.get(identity);
    if (existingIndex !== undefined) {
      const previousPendingBaseIdentity = pendingSkillTimelineBaseIdentity(out[existingIndex]);
      out[existingIndex] = preferTimelineItem(out[existingIndex], item);
      if (
        previousPendingBaseIdentity &&
        !pendingSkillTimelineBaseIdentity(out[existingIndex])
      ) {
        pendingIndexByBaseIdentity.delete(previousPendingBaseIdentity);
      }
      continue;
    }

    const baseIdentity =
      item.type === 'skill_event' ? timelineSkillInvocationBaseIdentity(item.invocation) : '';
    const pendingIndex = baseIdentity ? pendingIndexByBaseIdentity.get(baseIdentity) : undefined;
    if (pendingIndex !== undefined) {
      out[pendingIndex] = preferTimelineItem(out[pendingIndex], item);
      indexByIdentity.set(timelineItemIdentity(out[pendingIndex]), pendingIndex);
      if (!pendingSkillTimelineBaseIdentity(out[pendingIndex])) {
        pendingIndexByBaseIdentity.delete(baseIdentity);
      }
      continue;
    }

    indexByIdentity.set(identity, out.length);
    const pendingBaseIdentity = pendingSkillTimelineBaseIdentity(item);
    if (pendingBaseIdentity) {
      pendingIndexByBaseIdentity.set(pendingBaseIdentity, out.length);
    }
    out.push(item);
  }
  return out;
}

export function mergeRuntimeTimelineWithMessageTimeline(
  messageTimeline: AIChatAgenticTimelineItem[],
  runtimeTimeline?: AIChatAgenticTimelineItem[]
): AIChatAgenticTimelineItem[] {
  if (!runtimeTimeline?.length) {
    return messageTimeline;
  }
  if (!messageTimeline.length) {
    return runtimeTimeline;
  }

  const messageByIdentity = new Map(
    messageTimeline.map(item => [timelineItemIdentity(item), item] as const)
  );
  const seen = new Set<string>();
  const merged = runtimeTimeline.map(item => {
    const identity = timelineItemIdentity(item);
    seen.add(identity);
    const messageItem = messageByIdentity.get(identity);
    return item.type === 'progress_text' || !messageItem
      ? item
      : preferTimelineItem(messageItem, item);
  });

  messageTimeline.forEach(item => {
    const identity = timelineItemIdentity(item);
    if (!seen.has(identity)) {
      merged.push(item);
    }
  });

  return sortTimelineItems(dedupeTimelineItems(merged));
}

export function seedStreamingTimelineFromMessages(
  conversation: AIChatConversation,
  messages: AIChatMessage[],
  streamingByMessageId: Record<string, AIChatStreamingMessageState>
): Record<string, AIChatStreamingMessageState> {
  const messageId = conversation.active_message_id;
  if (conversation.runtime_status !== 'streaming' || !messageId) {
    return streamingByMessageId;
  }
  const message = messages.find(item => item.id === messageId);
  if (!message) {
    return streamingByMessageId;
  }
  const timeline = timelineFromAIChatMessage(message);
  if (timeline.length === 0) {
    return streamingByMessageId;
  }
  const previous = streamingByMessageId[messageId];
  if (previous?.timeline?.length) {
    return {
      ...streamingByMessageId,
      [messageId]: {
        ...previous,
        timeline: mergeRuntimeTimelineWithMessageTimeline(timeline, previous.timeline),
      },
    };
  }

  return {
    ...streamingByMessageId,
    [messageId]: {
      conversation_id: conversation.id,
      message_id: messageId,
      answer: previous?.answer ?? message.answer,
      status: 'streaming',
      timeline,
      last_event_id: previous?.last_event_id,
      replay_base_answer: previous?.replay_base_answer,
      replay_offset: previous?.replay_offset,
      replace: previous?.replace,
      sensitiveOutputBlocked: previous?.sensitiveOutputBlocked,
    },
  };
}

export function isTerminalReplaceableMessageStatus(status: AIChatMessage['status']): boolean {
  return status === 'completed' || status === 'stopped' || status === 'error';
}

export function canReplaceRootMessage(
  conversation: AIChatConversation | null | undefined,
  message: AIChatMessage | null | undefined,
  messages: AIChatMessage[]
): boolean {
  if (!conversation || !message) return false;
  return (
    conversation.runtime_status === 'idle' &&
    conversation.dialogue_count === 1 &&
    !message.parent_id &&
    conversation.current_leaf_message_id === message.id &&
    isTerminalReplaceableMessageStatus(message.status) &&
    messages.length === 1
  );
}

export function selectActiveConversation(state: AIChatControllerState): AIChatConversation | null {
  return state.activeConversationId
    ? (state.conversations.find(item => item.id === state.activeConversationId) ?? null)
    : null;
}

export function selectActiveMessages(state: AIChatControllerState): AIChatMessage[] {
  return state.activeConversationId
    ? (state.messagesByConversation[state.activeConversationId] ?? EMPTY_AICHAT_MESSAGES)
    : EMPTY_AICHAT_MESSAGES;
}

export function selectActiveMessagePagination(state: AIChatControllerState): AIChatPagination {
  return state.activeConversationId
    ? (state.messagePaginationByConversation[state.activeConversationId] ??
        DEFAULT_AICHAT_MESSAGE_PAGINATION)
    : DEFAULT_AICHAT_MESSAGE_PAGINATION;
}

export function selectIsLoadingOlderMessages(state: AIChatControllerState): boolean {
  return state.activeConversationId
    ? Boolean(state.loadingOlderByConversation[state.activeConversationId])
    : false;
}

export function selectIsRecoveringMessages(state: AIChatControllerState): boolean {
  return state.activeConversationId
    ? Boolean(state.recoveringByConversation[state.activeConversationId])
    : false;
}

export function selectIsStopping(state: AIChatControllerState): boolean {
  return state.activeConversationId
    ? Boolean(state.stoppingByConversation[state.activeConversationId])
    : false;
}

export function selectDisplayMessageIds(
  activeConversation: AIChatConversation | null,
  messageTopology: ChatMessageTopology
): string[] {
  return getCurrentChatPathIds(activeConversation, messageTopology);
}

export function selectDisplayMessages(
  displayMessageIds: string[],
  messages: AIChatMessage[]
): AIChatMessage[] {
  return materializeChatMessages(displayMessageIds, buildChatMessageById(messages));
}

export function selectBranchNavigationByMessageId(
  displayMessageIds: string[],
  messageTopology: ChatMessageTopology
) {
  return buildChatBranchNavigationByMessageId(displayMessageIds, messageTopology);
}
