import type {
  AIChatConversation,
  AIChatMessage,
  AIChatSkillInvocation,
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
  return (
    invocation.kind !== 'metadata_exposed' &&
    invocation.kind !== 'memory_planner' &&
    invocation.kind !== 'user_input_request'
  );
}

function workflowString(value: unknown): string | undefined {
  if (typeof value === 'string' && value.trim()) return value.trim();
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return undefined;
}

function workflowElapsedMs(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? value
    : undefined;
}

function workflowRunId(run: AIChatWorkflowRunMetadata): string {
  return workflowString(run.workflow_run_id) ?? workflowString(run.task_id) ?? workflowString(run.id) ?? '';
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

  const skillTimeline = invocations.map((invocation, index): AIChatAgenticTimelineItem => {
    if (invocation.kind === 'intermediate_answer' && invocation.message) {
      return {
        id: `history-intermediate-${message.id}-${index}`,
        type: 'intermediate_answer',
        title: invocation.title,
        content: invocation.message,
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

  return [...skillTimeline, ...workflowTimelineFromMessage(message)].sort((left, right) => {
    const leftAt = left.created_at ?? Number.MAX_SAFE_INTEGER;
    const rightAt = right.created_at ?? Number.MAX_SAFE_INTEGER;
    return leftAt - rightAt || left.id.localeCompare(right.id);
  });
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
    return streamingByMessageId;
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
    ? state.conversations.find(item => item.id === state.activeConversationId) ?? null
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
