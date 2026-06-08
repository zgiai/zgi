import type {
  AIChatAgentProgressEventData,
  AIChatConversation,
  AIChatErrorEventData,
  AIChatFileParseEndEventData,
  AIChatFileParseErrorEventData,
  AIChatFileParseStartEventData,
  AIChatGeneratedFile,
  AIChatIntermediateAnswerEventData,
  AIChatMessage,
  AIChatMessageChunkEventData,
  AIChatMessageEndEventData,
  AIChatMessageFile,
  AIChatMessageMetadata,
  AIChatMessageRetractEventData,
  AIChatMessageStartEventData,
  AIChatMemoryMutationEventData,
  AIChatSkillCallEndEventData,
  AIChatSkillCallErrorEventData,
  AIChatSkillCallStartEventData,
  AIChatSkillArtifactCreatedEventData,
  AIChatSkillInvocation,
  AIChatSkillLoadEndEventData,
  AIChatSkillLoadStartEventData,
  AIChatSkillReferenceReadEventData,
  AIChatUserInputRequestedEventData,
  AIChatWorkflowEventData,
  AIChatWorkflowNodeEventData,
  AIChatWorkflowPausedEventData,
} from '@/services/types/aichat';
import type { NodeInfo, RunStatus } from '@/components/chat/types';
import {
  SENSITIVE_OUTPUT_BLOCKED_FLAG,
  SENSITIVE_OUTPUT_BLOCKED_TOKEN,
  isSensitiveOutputBlockedValue,
} from '@/utils/model-output-filter';
import {
  DEFAULT_AICHAT_MESSAGE_PAGINATION,
  type AIChatControllerState,
  type AIChatAgenticTimelineItem,
  type AIChatMessageStartContext,
  type AIChatStreamingMessageState,
} from '@/components/chat/controllers/aichat/types';
import {
  createDraftAIChatConversation,
  createStreamingAIChatMessage,
  normalizeAIChatStatus,
  replaceAIChatConversation,
  upsertAIChatMessage,
} from '@/components/chat/utils/aichat-message';
import {
  extractLlmGatewayRequest,
  extractWorkflowRunContainerContext,
  getWorkflowRunCreatedAtMs,
  getWorkflowRunExecutionId,
  getWorkflowRunItemKey,
  getWorkflowRunRoundElapsedTime,
  sortWorkflowRunItems,
  sortWorkflowRunRounds,
} from '@/utils/workflow/run-events';
import { getNextActiveSendingState } from './selectors';

export function mergeAIChatMessages(
  currentMessages: AIChatMessage[],
  incomingMessages: AIChatMessage[]
): AIChatMessage[] {
  const byId = new Map<string, AIChatMessage>();
  currentMessages.forEach(message => byId.set(message.id, message));
  incomingMessages.forEach(message => byId.set(message.id, message));

  return Array.from(byId.values()).sort(
    (a, b) => a.created_at - b.created_at || a.id.localeCompare(b.id)
  );
}

export function removeStreamingStateByConversation(
  streamingByMessageId: Record<string, AIChatStreamingMessageState>,
  conversationId: string
): Record<string, AIChatStreamingMessageState> {
  const nextStreamingByMessageId = { ...streamingByMessageId };
  Object.values(streamingByMessageId).forEach(streaming => {
    if (streaming.conversation_id === conversationId) {
      delete nextStreamingByMessageId[streaming.message_id];
    }
  });
  return nextStreamingByMessageId;
}

function createAIChatFileMetadata(files?: AIChatMessageFile[]): AIChatMessageMetadata | undefined {
  if (!files?.length) {
    return undefined;
  }

  return {
    file_count: files.length,
    files,
  };
}

function mergeMessageMetadata(
  existingMetadata?: AIChatMessageMetadata,
  incomingMetadata?: AIChatMessageMetadata
): AIChatMessageMetadata | undefined {
  if (!existingMetadata && !incomingMetadata) {
    return undefined;
  }

  const existingFiles = existingMetadata?.files ?? [];
  const incomingFiles = incomingMetadata?.files ?? [];
  const files = incomingFiles.length > 0 ? incomingFiles : existingFiles;
  const existingGeneratedFiles = existingMetadata?.generated_files ?? [];
  const incomingGeneratedFiles = incomingMetadata?.generated_files ?? [];
  const generatedFiles =
    incomingGeneratedFiles.length > 0 ? incomingGeneratedFiles : existingGeneratedFiles;
  const existingWorkflowRuns = existingMetadata?.workflow_runs ?? [];
  const incomingWorkflowRuns = incomingMetadata?.workflow_runs ?? [];
  const workflowRuns =
    incomingWorkflowRuns.length > 0 ? incomingWorkflowRuns : existingWorkflowRuns;
  const userInputRequest =
    incomingMetadata?.user_input_request ?? existingMetadata?.user_input_request;

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
    ...(userInputRequest
      ? {
          user_input_request: userInputRequest,
        }
      : {}),
  };
}

function clearRuntimeMessageMetadata(
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

function upsertSkillInvocation(
  invocations: AIChatSkillInvocation[],
  incoming: AIChatSkillInvocation
): AIChatSkillInvocation[] {
  if (incoming.runtime_id) {
    const index = invocations.findIndex(
      invocation => invocation.runtime_id === incoming.runtime_id
    );
    if (index >= 0) {
      const next = invocations.slice();
      next[index] = {
        ...next[index],
        ...incoming,
      };
      return next;
    }
  }

  if (incoming.kind === 'intermediate_answer' && incoming.answer_id) {
    const index = invocations.findIndex(
      invocation =>
        invocation.kind === 'intermediate_answer' && invocation.answer_id === incoming.answer_id
    );
    if (index < 0) {
      return [...invocations, incoming];
    }

    const next = invocations.slice();
    next[index] = {
      ...next[index],
      ...incoming,
    };
    return next;
  }

  const next = invocations.slice();
  const incomingKind = incoming.kind ?? 'tool_call';
  const incomingToolName = incoming.tool_name ?? '';
  const incomingPath = incoming.path ?? '';
  const index = [...next].reverse().findIndex(invocation => {
    const invocationKind = invocation.kind ?? 'tool_call';
    const sameIdentity =
      invocationKind === incomingKind &&
      invocation.skill_id === incoming.skill_id &&
      (invocation.tool_name ?? '') === incomingToolName &&
      (invocation.path ?? '') === incomingPath;

    return sameIdentity && (invocation.status === 'loading' || invocation.status === 'running');
  });

  if (index < 0) {
    return [...next, incoming];
  }

  const actualIndex = next.length - 1 - index;
  next[actualIndex] = {
    ...next[actualIndex],
    ...incoming,
  };
  return next;
}

function getSkillInvocationIdentity(invocation: AIChatSkillInvocation): string {
  if (invocation.runtime_id) {
    return invocation.runtime_id;
  }
  return [
    invocation.kind ?? 'tool_call',
    invocation.skill_id ?? '',
    invocation.tool_name ?? '',
    invocation.path ?? '',
  ].join(':');
}

function isVisibleSkillInvocation(invocation: AIChatSkillInvocation): boolean {
  return invocation.kind !== 'metadata_exposed' && invocation.kind !== 'memory_planner';
}

function isTransientProgressItem(item: AIChatAgenticTimelineItem): boolean {
  return (
    item.type === 'progress_text' &&
    (item.transient === true || Boolean(item.phase && !item.content.trim()))
  );
}

function removeTransientProgressItems(
  timeline: AIChatAgenticTimelineItem[] | undefined
): AIChatAgenticTimelineItem[] {
  return (timeline ?? []).filter(item => !isTransientProgressItem(item));
}

function workflowString(value: unknown): string | undefined {
  if (typeof value === 'string' && value.trim()) return value.trim();
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return undefined;
}

function workflowRunId(payload: AIChatWorkflowEventData): string {
  return (
    workflowString(payload.workflow_run_id) ??
    workflowString(payload.task_id) ??
    workflowString(payload.id) ??
    ''
  );
}

function workflowElapsedMs(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? value
    : undefined;
}

function workflowNumber(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  return undefined;
}

function normalizeWorkflowRunTimelineStatus(status: unknown, fallback: RunStatus): RunStatus {
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
      return fallback;
  }
}

function normalizeWorkflowNodeTimelineStatus(
  status: unknown,
  finished: boolean
): NodeInfo['status'] {
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
    default:
      return finished ? 'success' : 'running';
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
    case 'iterationstart':
    case 'iteration-start':
      return 'iteration-start';
    case 'loopstart':
    case 'loop-start':
      return 'loop-start';
    case 'loopend':
    case 'loop-end':
      return 'loop-end';
    default:
      return hyphen;
  }
}

function mapWorkflowNodeTimelineItem(
  payload: AIChatWorkflowNodeEventData | AIChatWorkflowPausedEventData,
  finished: boolean
): NodeInfo {
  const nodeId =
    workflowString(payload.node_id) ??
    workflowString(payload.execution_id) ??
    workflowString(payload.id);
  const nodeType = normalizeWorkflowNodeType(payload.node_type ?? payload.type);
  const title =
    workflowString(payload.title) ??
    workflowString(payload.node_title) ??
    workflowString(payload.name) ??
    workflowString(payload.label) ??
    nodeType ??
    nodeId;
  const error = workflowString(payload.error);
  return {
    status: normalizeWorkflowNodeTimelineStatus(payload.status, finished),
    error,
    elapsedTime: workflowElapsedMs(payload.elapsed_time),
    nodeId,
    executionId: getWorkflowRunExecutionId(payload),
    createdAtMs: getWorkflowRunCreatedAtMs(payload),
    nodeType,
    title,
    data: {
      input: 'inputs' in payload ? payload.inputs : undefined,
      output: finished && 'outputs' in payload ? payload.outputs : undefined,
      modelInput: extractLlmGatewayRequest(payload),
    },
  };
}

function workflowNodeKey(node: NodeInfo): string {
  if ((node.nodeType === 'approval' || node.nodeType === 'question-answer') && node.nodeId) {
    return node.nodeId;
  }
  return getWorkflowRunItemKey(node) || [node.nodeType, node.title].filter(Boolean).join('|');
}

function upsertWorkflowNode(nodes: NodeInfo[], incoming: NodeInfo): NodeInfo[] {
  const key = workflowNodeKey(incoming);
  const index = nodes.findIndex(node => workflowNodeKey(node) === key);
  if (index < 0) return [...nodes, incoming];
  const next = nodes.slice();
  const previous = next[index];
  next[index] = {
    ...previous,
    ...incoming,
    data: {
      input: incoming.data?.input ?? previous.data?.input,
      output: incoming.data?.output ?? previous.data?.output,
      modelInput: incoming.data?.modelInput ?? previous.data?.modelInput,
    },
  };
  return next;
}

function containerKindFromEvent(eventType: string | undefined): 'iteration' | 'loop' | undefined {
  if (!eventType) return undefined;
  if (eventType.startsWith('iteration_')) return 'iteration';
  if (eventType.startsWith('loop_')) return 'loop';
  return undefined;
}

function containerKindFromNode(node: NodeInfo): 'iteration' | 'loop' | undefined {
  if (node.nodeType === 'iteration') return 'iteration';
  if (node.nodeType === 'loop') return 'loop';
  return undefined;
}

function workflowEventType(payload: AIChatWorkflowEventData): string | undefined {
  return workflowString(payload.workflow_event) ?? workflowString(payload.event);
}

function workflowRoundIndex(
  payload: AIChatWorkflowEventData,
  kind: 'iteration' | 'loop'
): number | undefined {
  const eventType = workflowEventType(payload);
  if (kind === 'iteration') {
    return workflowNumber(payload.iteration_index) ??
      (eventType === 'iteration_next' ? workflowNumber(payload.index) : undefined);
  }
  const loopIndex = workflowNumber(payload.loop_index);
  if (typeof loopIndex === 'number') return loopIndex;
  if (eventType !== 'loop_next') return undefined;
  const index = workflowNumber(payload.index);
  if (typeof index !== 'number') return undefined;
  return index > 0 ? index - 1 : index;
}

function workflowContainerNodeId(
  payload: AIChatWorkflowEventData,
  incoming: NodeInfo,
  kind: 'iteration' | 'loop'
): string | undefined {
  if (incoming.nodeType === kind && incoming.nodeId) return incoming.nodeId;
  const context = extractWorkflowRunContainerContext(payload);
  return kind === 'iteration' ? context.iterationId : context.loopId;
}

function workflowContainerTitle(
  payload: AIChatWorkflowEventData,
  incoming: NodeInfo,
  kind: 'iteration' | 'loop',
  nodeId?: string
): string {
  if (incoming.nodeType !== kind) {
    return nodeId ?? kind;
  }
  return (
    incoming.title ??
    workflowString(payload.title) ??
    workflowString(payload.node_title) ??
    nodeId ??
    kind
  );
}

function isWorkflowContainerCompletion(eventType: string | undefined, finished: boolean): boolean {
  if (finished) return true;
  switch (eventType) {
    case 'iteration_completed':
    case 'iteration_succeeded':
    case 'iteration_failed':
    case 'loop_completed':
    case 'loop_succeeded':
    case 'loop_failed':
      return true;
    default:
      return false;
  }
}

function isWorkflowContainerNextEvent(eventType: string | undefined): boolean {
  return eventType === 'iteration_next' || eventType === 'loop_next';
}

function buildWorkflowContainerNode(
  previous: NodeInfo | undefined,
  payload: AIChatWorkflowEventData,
  incoming: NodeInfo,
  kind: 'iteration' | 'loop',
  finished: boolean
): NodeInfo {
  const eventType = workflowEventType(payload);
  const nodeId = workflowContainerNodeId(payload, incoming, kind) ?? previous?.nodeId;
  const title =
    incoming.nodeType === kind
      ? workflowContainerTitle(payload, incoming, kind, nodeId)
      : previous?.title ?? nodeId ?? kind;
  const status = isWorkflowContainerCompletion(eventType, finished)
    ? incoming.status
    : ('running' as const);
  const inputs =
    incoming.nodeType === kind
      ? incoming.data?.input
      : kind === 'iteration'
        ? previous?.iterationInputs
        : previous?.loopInputs;
  const outputs = isWorkflowContainerCompletion(eventType, finished)
    ? incoming.data?.output
    : kind === 'iteration'
      ? previous?.iterationOutputs
      : previous?.loopOutputs;
  const base: NodeInfo = {
    ...(previous ?? {}),
    status,
    error: incoming.error ?? previous?.error,
    elapsedTime: incoming.elapsedTime ?? previous?.elapsedTime,
    nodeId,
    executionId: incoming.nodeType === kind ? incoming.executionId : previous?.executionId,
    createdAtMs: incoming.nodeType === kind ? incoming.createdAtMs : previous?.createdAtMs,
    nodeType: kind,
    title,
    data: {
      input: inputs,
      output: outputs,
    },
  };
  if (kind === 'iteration') {
    return {
      ...base,
      iterationInputs: inputs,
      iterationOutputs: outputs,
      iterationRounds: previous?.iterationRounds ?? [],
    };
  }
  return {
    ...base,
    loopInputs: inputs,
    loopOutputs: outputs,
    loopRounds: previous?.loopRounds ?? [],
  };
}

function upsertWorkflowContainerRound(
  container: NodeInfo,
  payload: AIChatWorkflowEventData,
  kind: 'iteration' | 'loop',
  child?: NodeInfo
): NodeInfo {
  const eventType = workflowEventType(payload);
  const explicitIndex = workflowRoundIndex(payload, kind);
  const currentRounds = kind === 'iteration' ? container.iterationRounds ?? [] : container.loopRounds ?? [];
  if (typeof explicitIndex !== 'number' && !child && !isWorkflowContainerNextEvent(eventType)) {
    return container;
  }
  const index = explicitIndex ?? currentRounds.at(-1)?.index ?? 0;
  const existingRound = currentRounds.find(round => round.index === index);
  const nextRound = existingRound
    ? { ...existingRound, nodes: existingRound.nodes ?? [] }
    : { index, nodes: [] as NodeInfo[] };

  if (child) {
    nextRound.nodes = sortWorkflowRunItems(upsertWorkflowNode(nextRound.nodes, child));
  }
  nextRound.elapsedTime =
    typeof nextRound.elapsedTime === 'number'
      ? nextRound.elapsedTime
      : getWorkflowRunRoundElapsedTime(nextRound);

  const nextRounds = sortWorkflowRunRounds(
    existingRound
      ? currentRounds.map(round => (round.index === index ? nextRound : round))
      : [...currentRounds, nextRound]
  );

  if (kind === 'iteration') {
    return { ...container, iterationRounds: nextRounds };
  }
  return { ...container, loopRounds: nextRounds };
}

function removeWorkflowContainerChildren(nodes: NodeInfo[]): NodeInfo[] {
  const childKeys = new Set<string>();
  const containerKeys = new Set<string>();
  nodes.forEach(node => {
    const rounds = [...(node.iterationRounds ?? []), ...(node.loopRounds ?? [])];
    if (rounds.length === 0) return;
    containerKeys.add(workflowNodeKey(node));
    rounds.forEach(round => {
      (round.nodes ?? []).forEach(child => childKeys.add(workflowNodeKey(child)));
    });
  });
  if (childKeys.size === 0) return nodes;
  return nodes.filter(node => containerKeys.has(workflowNodeKey(node)) || !childKeys.has(workflowNodeKey(node)));
}

function upsertWorkflowNodeWithContainers(
  nodes: NodeInfo[],
  payload: AIChatWorkflowEventData,
  incoming: NodeInfo,
  finished: boolean
): NodeInfo[] {
  const eventType = workflowEventType(payload);
  const context = extractWorkflowRunContainerContext(payload);
  const lifecycleKind = containerKindFromEvent(eventType) ?? containerKindFromNode(incoming);
  const childKind = context.loopId ? 'loop' : context.iterationId ? 'iteration' : undefined;

  if (childKind && incoming.nodeType !== childKind) {
    const containerId = childKind === 'loop' ? context.loopId : context.iterationId;
    const existingIndex = nodes.findIndex(
      node => node.nodeType === childKind && node.nodeId === containerId
    );
    const previous = existingIndex >= 0 ? nodes[existingIndex] : undefined;
    let container = buildWorkflowContainerNode(previous, payload, incoming, childKind, false);
    container = upsertWorkflowContainerRound(container, payload, childKind, incoming);
    const next = nodes.slice();
    if (existingIndex >= 0) next[existingIndex] = container;
    else next.push(container);
    return removeWorkflowContainerChildren(next);
  }

  if (lifecycleKind) {
    const containerId = workflowContainerNodeId(payload, incoming, lifecycleKind);
    const existingIndex = nodes.findIndex(
      node => node.nodeType === lifecycleKind && (!containerId || node.nodeId === containerId)
    );
    const previous = existingIndex >= 0 ? nodes[existingIndex] : undefined;
    let container = buildWorkflowContainerNode(previous, payload, incoming, lifecycleKind, finished);
    container = upsertWorkflowContainerRound(container, payload, lifecycleKind);
    const next = nodes.slice();
    if (existingIndex >= 0) next[existingIndex] = container;
    else next.push(container);
    return removeWorkflowContainerChildren(next);
  }

  return removeWorkflowContainerChildren(upsertWorkflowNode(nodes, incoming));
}

function upsertWorkflowTimelineItem(
  timeline: AIChatAgenticTimelineItem[] | undefined,
  payload: AIChatWorkflowEventData,
  eventId: string | null | undefined,
  nextStatus: RunStatus,
  node?: NodeInfo,
  approval?: Partial<AIChatWorkflowPausedEventData>,
  nodeFinished = false
): AIChatAgenticTimelineItem[] {
  const baseTimeline = removeTransientProgressItems(timeline);
  const runId = workflowRunId(payload);
  if (!runId) return baseTimeline;
  const index = baseTimeline.findIndex(
    item => item.type === 'workflow_run' && item.workflowRunId === runId
  );
  if (index < 0) {
    return [
      ...baseTimeline,
      {
        id: eventId ?? `workflow-${runId}`,
        type: 'workflow_run',
        workflowRunId: runId,
        status: nextStatus,
        elapsedTime: workflowElapsedMs(payload.elapsed_time),
        error: workflowString(payload.error),
        nodes: node ? upsertWorkflowNodeWithContainers([], payload, node, nodeFinished) : [],
        approval,
        created_at: payload.created_at,
        event_id: eventId ?? null,
      },
    ];
  }
  const next = baseTimeline.slice();
  const previous = next[index];
  if (previous.type !== 'workflow_run') return baseTimeline;
  const closedApprovalStatus =
    workflowEventType(payload) === 'approval_result_filled'
      ? 'submitted'
      : workflowEventType(payload) === 'approval_expired'
        ? 'expired'
        : undefined;
  next[index] = {
    ...previous,
    status: nextStatus,
    elapsedTime: workflowElapsedMs(payload.elapsed_time) ?? previous.elapsedTime,
    error: workflowString(payload.error) ?? previous.error,
    nodes: node
      ? upsertWorkflowNodeWithContainers(previous.nodes, payload, node, nodeFinished)
      : previous.nodes,
    approval: approval
      ? { ...(previous.approval ?? {}), ...approval }
      : closedApprovalStatus
        ? { ...(previous.approval ?? {}), status: closedApprovalStatus }
        : previous.approval,
    event_id: eventId ?? previous.event_id,
  };
  return next;
}

function upsertMemoryTimelineItem(
  timeline: AIChatAgenticTimelineItem[] | undefined,
  payload: AIChatMemoryMutationEventData,
  eventId: string | null | undefined
): AIChatAgenticTimelineItem[] {
  const baseTimeline = removeTransientProgressItems(timeline);
  const itemId =
    eventId ??
    `memory-${payload.action}-${payload.entry_id ?? 'entry'}-${payload.created_at ?? Date.now()}`;
  if (baseTimeline.some(item => 'event_id' in item && item.event_id === eventId && eventId)) {
    return baseTimeline;
  }
  return [
    ...baseTimeline,
    {
      id: itemId,
      type: 'memory_event',
      event: payload,
      created_at: payload.created_at,
      event_id: eventId ?? null,
    },
  ];
}

function upsertSkillTimelineItem(
  timeline: AIChatAgenticTimelineItem[] | undefined,
  incoming: AIChatSkillInvocation,
  eventId: string | null | undefined
): AIChatAgenticTimelineItem[] {
  const baseTimeline = removeTransientProgressItems(timeline);

  if (incoming.kind === 'intermediate_answer') {
    const existingIndex = incoming.answer_id
      ? baseTimeline.findIndex(
          item => item.type === 'intermediate_answer' && item.answer_id === incoming.answer_id
        )
      : -1;

    if (existingIndex >= 0) {
      const next = baseTimeline.slice();
      const existing = next[existingIndex];
      if (existing.type !== 'intermediate_answer') return next;

      next[existingIndex] = {
        ...existing,
        title: incoming.title ?? existing.title,
        content: incoming.message ?? existing.content,
        status: incoming.status === 'success' ? 'success' : 'streaming',
        created_at: existing.created_at ?? incoming.created_at,
        event_id: eventId ?? existing.event_id,
      };
      return next;
    }

    return [
      ...baseTimeline,
      {
        id:
          incoming.answer_id ||
          eventId ||
          `intermediate-${incoming.created_at ?? Date.now()}-${baseTimeline.length}`,
        type: 'intermediate_answer',
        answer_id: incoming.answer_id,
        title: incoming.title,
        content: incoming.message ?? '',
        status: incoming.status === 'success' ? 'success' : 'streaming',
        created_at: incoming.created_at,
        event_id: eventId ?? null,
      },
    ];
  }

  const next = baseTimeline.slice();
  const incomingIdentity = getSkillInvocationIdentity(incoming);
  const reverseIndex = [...next].reverse().findIndex(item => {
    if (item.type !== 'skill_event') return false;
    const invocation = item.invocation;
    return (
      getSkillInvocationIdentity(invocation) === incomingIdentity &&
      (invocation.status === 'loading' || invocation.status === 'running')
    );
  });

  if (reverseIndex < 0) {
    return [
      ...next,
      {
        id:
          eventId ??
          `skill-${incomingIdentity}-${incoming.created_at ?? Date.now()}-${next.length}`,
        type: 'skill_event',
        invocation: incoming,
        created_at: incoming.created_at,
        event_id: eventId ?? null,
      },
    ];
  }

  const actualIndex = next.length - 1 - reverseIndex;
  const existing = next[actualIndex];
  if (existing.type !== 'skill_event') return next;

  next[actualIndex] = {
    ...existing,
    invocation: {
      ...existing.invocation,
      ...incoming,
    },
    created_at: incoming.created_at ?? existing.created_at,
    event_id: eventId ?? existing.event_id,
  };
  return next;
}

function updateSkillInvocationMetadata(
  current: AIChatControllerState,
  conversationId: string,
  messageId: string,
  eventId: string | null | undefined,
  invocation: AIChatSkillInvocation
): AIChatControllerState {
  if (!isVisibleSkillInvocation(invocation)) {
    return current;
  }
  const messages = current.messagesByConversation[conversationId] ?? [];
  const now = Math.floor(Date.now() / 1000);
  const nextMessages = messages.map(message => {
    if (message.id !== messageId) {
      return message;
    }

    const skillInvocations = upsertSkillInvocation(
      message.metadata?.skill_invocations ?? [],
      invocation
    );

    return {
      ...message,
      metadata: {
        ...(message.metadata ?? {}),
        has_trace: true,
        skill_invocations: skillInvocations,
        selected_skill_ids: Array.from(
          new Set(
            skillInvocations
              .filter(isVisibleSkillInvocation)
              .map(item => item.skill_id)
              .filter(Boolean)
          )
        ),
        loaded_skill_ids: Array.from(
          new Set(
            skillInvocations
              .filter(item => item.kind === 'skill_load' && item.status !== 'error')
              .map(item => item.skill_id)
              .filter(Boolean)
          )
        ),
        skill_step_count: skillInvocations.filter(isVisibleSkillInvocation).length,
        skill_call_count: skillInvocations.filter(isVisibleSkillInvocation).length,
        skill_names: Array.from(
          new Set(
            skillInvocations
              .filter(isVisibleSkillInvocation)
              .map(item => item.skill_id)
              .filter(Boolean)
          )
        ),
        tool_call_count: skillInvocations.filter(item => item.kind === 'tool_call').length,
        tool_names: Array.from(
          new Set(
            skillInvocations
              .filter(item => item.kind === 'tool_call')
              .map(item => item.tool_name)
              .filter((toolName): toolName is string => Boolean(toolName))
          )
        ),
      },
      updated_at: now,
    };
  });
  const previousStreaming = current.streamingByMessageId[messageId];

  return {
    ...current,
    messagesByConversation: {
      ...current.messagesByConversation,
      [conversationId]: nextMessages,
    },
    streamingByMessageId: previousStreaming
      ? {
          ...current.streamingByMessageId,
          [messageId]: {
            ...previousStreaming,
            timeline: upsertSkillTimelineItem(previousStreaming.timeline, invocation, eventId),
            last_event_id: eventId ?? previousStreaming.last_event_id,
          },
        }
      : current.streamingByMessageId,
  };
}

export function applyAgentProgressState(
  current: AIChatControllerState,
  payload: AIChatAgentProgressEventData,
  eventId?: string | null
): AIChatControllerState {
  const content = (payload.content ?? '').trim();
  if ((!content && !payload.phase) || !payload.message_id) {
    return current;
  }

  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (!previousStreaming) {
    return current;
  }

  const timeline = previousStreaming.timeline ?? [];
  const hasSameEvent = Boolean(
    eventId && timeline.some(item => 'event_id' in item && item.event_id === eventId)
  );
  if (hasSameEvent) {
    return current;
  }
  const lastItem = timeline[timeline.length - 1];
  const isRepeatedStructuredProgress =
    lastItem?.type === 'progress_text' &&
    payload.phase &&
    lastItem.phase === payload.phase &&
    (lastItem.meta_tool_name ?? '') === (payload.meta_tool_name ?? '') &&
    (lastItem.skill_id ?? '') === (payload.skill_id ?? '') &&
    (lastItem.tool_name ?? '') === (payload.tool_name ?? '');
  const isRepeatedProgress =
    lastItem?.type === 'progress_text' && lastItem.content.trim() === content;
  if (isRepeatedStructuredProgress || isRepeatedProgress) {
    return {
      ...current,
      streamingByMessageId: {
        ...current.streamingByMessageId,
        [payload.message_id]: {
          ...previousStreaming,
          last_event_id: eventId ?? previousStreaming.last_event_id,
        },
      },
    };
  }

  const transient = Boolean(payload.phase && !content);
  const nextBaseTimeline = removeTransientProgressItems(timeline);
  const nextTimeline: AIChatAgenticTimelineItem[] = [
    ...nextBaseTimeline,
    {
      id: eventId ?? `progress-${payload.created_at ?? Date.now()}-${nextBaseTimeline.length}`,
      type: 'progress_text',
      content,
      phase: payload.phase,
      transient,
      meta_tool_name: payload.meta_tool_name,
      skill_id: payload.skill_id,
      tool_name: payload.tool_name,
      arguments_chars: payload.arguments_chars,
      created_at: payload.created_at,
      event_id: eventId ?? null,
    },
  ];

  return {
    ...current,
    streamingByMessageId: {
      ...current.streamingByMessageId,
      [payload.message_id]: {
        ...previousStreaming,
        timeline: nextTimeline,
        last_event_id: eventId ?? previousStreaming.last_event_id,
      },
    },
  };
}

export function applyIntermediateAnswerState(
  current: AIChatControllerState,
  payload: AIChatIntermediateAnswerEventData,
  eventId?: string | null
): AIChatControllerState {
  const content = payload.content ?? '';
  if ((!content && payload.done !== true) || !payload.conversation_id || !payload.message_id) {
    return current;
  }
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  const answerId =
    payload.answer_id ||
    eventId ||
    `intermediate-${payload.created_at ?? Date.now()}-${payload.index ?? 0}`;
  const previousItem = previousStreaming?.timeline?.find(
    (item): item is Extract<AIChatAgenticTimelineItem, { type: 'intermediate_answer' }> =>
      item.type === 'intermediate_answer' && item.answer_id === answerId
  );
  const nextContent = payload.delta ? `${previousItem?.content ?? ''}${content}` : content;

  return updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: 'intermediate_answer',
      answer_id: answerId,
      skill_id: '',
      title: payload.title,
      status: payload.done === false ? 'running' : 'success',
      message: nextContent,
      created_at: payload.created_at,
    }
  );
}

export function applyUserInputRequestedState(
  current: AIChatControllerState,
  payload: AIChatUserInputRequestedEventData,
  eventId?: string | null
): AIChatControllerState {
  const questions = (payload.questions ?? []).filter(question => question.question?.trim());
  if (!payload.conversation_id || !payload.message_id || questions.length === 0) {
    return current;
  }
  const request = {
    request_id: payload.request_id,
    source: payload.source,
    workflow_run_id: payload.workflow_run_id,
    node_id: payload.node_id,
    conversation_id: payload.conversation_id,
    message_id: payload.message_id,
    round: payload.round,
    questions: questions.map(question => ({
      ...question,
      question: question.question.trim(),
      options: question.options?.filter(option => option.label?.trim()),
    })),
    created_at: payload.created_at,
  };
  const messages = current.messagesByConversation[payload.conversation_id] ?? [];
  const nextMessages = messages.map(message =>
    message.id === payload.message_id
      ? {
          ...message,
          metadata: {
            ...(message.metadata ?? {}),
            user_input_request: request,
          },
          updated_at: Math.floor(Date.now() / 1000),
        }
      : message
  );
  const previousStreaming = current.streamingByMessageId[payload.message_id];

  return {
    ...current,
    messagesByConversation: {
      ...current.messagesByConversation,
      [payload.conversation_id]: nextMessages,
    },
    streamingByMessageId: previousStreaming
      ? {
          ...current.streamingByMessageId,
          [payload.message_id]: {
            ...previousStreaming,
            last_event_id: eventId ?? previousStreaming.last_event_id,
          },
        }
      : current.streamingByMessageId,
  };
}

function inferExtension(filename: string): string {
  return filename.split('.').pop()?.toLowerCase() ?? '';
}

function upsertMessageFile(
  files: AIChatMessageFile[],
  fileId: string,
  fallbackName: string,
  updater: (file: AIChatMessageFile) => AIChatMessageFile
): AIChatMessageFile[] {
  const index = files.findIndex(file => file.id === fileId);
  const baseFile: AIChatMessageFile =
    index >= 0
      ? files[index]
      : {
          id: fileId,
          name: fallbackName,
          size: 0,
          extension: inferExtension(fallbackName),
          mime_type: '',
          workspace_id: null,
          is_temporary: true,
          content_status: 'pending',
          content_chars: 0,
          content_preview: '',
          from_cache: false,
          kind: 'document',
          vision_detail: null,
          filtered_reason: null,
          parse_status: 'pending',
        };
  const nextFile = updater(baseFile);
  if (index < 0) {
    return [...files, nextFile];
  }

  const nextFiles = files.slice();
  nextFiles[index] = nextFile;
  return nextFiles;
}

function updateMessageFileMetadata(
  current: AIChatControllerState,
  conversationId: string,
  messageId: string,
  fileId: string,
  fallbackName: string,
  eventId: string | null | undefined,
  updater: (file: AIChatMessageFile) => AIChatMessageFile
): AIChatControllerState {
  const messages = current.messagesByConversation[conversationId] ?? [];
  const nextMessages = messages.map(message => {
    if (message.id !== messageId) {
      return message;
    }

    const files = upsertMessageFile(message.metadata?.files ?? [], fileId, fallbackName, updater);

    return {
      ...message,
      metadata: {
        ...(message.metadata ?? {}),
        file_count: files.length,
        files,
      },
      updated_at: Math.floor(Date.now() / 1000),
    };
  });
  const previousStreaming = current.streamingByMessageId[messageId];

  return {
    ...current,
    messagesByConversation: {
      ...current.messagesByConversation,
      [conversationId]: nextMessages,
    },
    streamingByMessageId: previousStreaming
      ? {
          ...current.streamingByMessageId,
          [messageId]: {
            ...previousStreaming,
            last_event_id: eventId ?? previousStreaming.last_event_id,
          },
        }
      : current.streamingByMessageId,
  };
}

function upsertGeneratedFile(
  files: AIChatGeneratedFile[],
  incoming: AIChatGeneratedFile
): AIChatGeneratedFile[] {
  const index = files.findIndex(file => file.file_id === incoming.file_id);
  if (index < 0) {
    return [...files, incoming];
  }

  const nextFiles = files.slice();
  nextFiles[index] = {
    ...nextFiles[index],
    ...incoming,
  };
  return nextFiles;
}

function normalizeSkillArtifactFile(
  payload: AIChatSkillArtifactCreatedEventData
): AIChatGeneratedFile | null {
  const file = payload.file;
  const fileId = file?.file_id ?? payload.file_id;
  const filename = file?.filename ?? payload.filename;
  const extension = file?.extension ?? payload.extension;
  const mimeType = file?.mime_type ?? payload.mime_type;
  const size = file?.size ?? payload.size;
  const url = file?.url ?? payload.url;

  if (
    !payload.skill_id ||
    !payload.tool_name ||
    !fileId ||
    !filename ||
    !extension ||
    !mimeType ||
    typeof size !== 'number' ||
    !url
  ) {
    return null;
  }

  return {
    artifact_type: 'file',
    skill_id: payload.skill_id,
    tool_name: payload.tool_name,
    file_id: fileId,
    filename,
    extension,
    mime_type: mimeType,
    size,
    url,
    download_url: file?.download_url ?? payload.download_url,
    transfer_method: file?.transfer_method ?? payload.transfer_method ?? 'tool_file',
    file_type: file?.file_type ?? payload.file_type,
    created_at: file?.created_at ?? payload.created_at ?? Math.floor(Date.now() / 1000),
  };
}

export function applyMemoryMutationState(
  current: AIChatControllerState,
  payload: AIChatMemoryMutationEventData,
  eventId?: string | null
): AIChatControllerState {
  if (!payload.conversation_id || !payload.message_id) {
    return current;
  }
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (!previousStreaming) {
    return current;
  }
  return {
    ...current,
    streamingByMessageId: {
      ...current.streamingByMessageId,
      [payload.message_id]: {
        ...previousStreaming,
        timeline: upsertMemoryTimelineItem(previousStreaming.timeline, payload, eventId),
        last_event_id: eventId ?? previousStreaming.last_event_id,
      },
    },
  };
}

function applyWorkflowTimelineState(
  current: AIChatControllerState,
  payload: AIChatWorkflowEventData,
  eventId: string | null | undefined,
  status: RunStatus,
  node?: NodeInfo,
  approval?: Partial<AIChatWorkflowPausedEventData>,
  nodeFinished = false
): AIChatControllerState {
  if (!payload.conversation_id || !payload.message_id || !workflowRunId(payload)) {
    return current;
  }
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (!previousStreaming) {
    return current;
  }
  return {
    ...current,
    streamingByMessageId: {
      ...current.streamingByMessageId,
      [payload.message_id]: {
        ...previousStreaming,
        timeline: upsertWorkflowTimelineItem(
          previousStreaming.timeline,
          payload,
          eventId,
          status,
          node,
          approval,
          nodeFinished
        ),
        last_event_id: eventId ?? previousStreaming.last_event_id,
      },
    },
  };
}

export function applyWorkflowStartedState(
  current: AIChatControllerState,
  payload: AIChatWorkflowEventData,
  eventId?: string | null
): AIChatControllerState {
  return applyWorkflowTimelineState(current, payload, eventId, 'running');
}

export function applyWorkflowNodeStartedState(
  current: AIChatControllerState,
  payload: AIChatWorkflowNodeEventData,
  eventId?: string | null
): AIChatControllerState {
  return applyWorkflowTimelineState(
    current,
    payload,
    eventId,
    'running',
    mapWorkflowNodeTimelineItem(payload, false)
  );
}

export function applyWorkflowNodeFinishedState(
  current: AIChatControllerState,
  payload: AIChatWorkflowNodeEventData,
  eventId?: string | null
): AIChatControllerState {
  const node = mapWorkflowNodeTimelineItem(payload, true);
  const status = node.status === 'failed' ? 'error' : 'running';
  return applyWorkflowTimelineState(current, payload, eventId, status, node, undefined, true);
}

export function applyWorkflowPausedState(
  current: AIChatControllerState,
  payload: AIChatWorkflowPausedEventData,
  eventId?: string | null
): AIChatControllerState {
  const status = normalizeWorkflowRunTimelineStatus(payload.status, 'pending_approval');
  return applyWorkflowTimelineState(
    current,
    payload,
    eventId,
    status,
    mapWorkflowNodeTimelineItem({ ...payload, status: 'paused' }, true),
    undefined,
    true
  );
}

export function applyWorkflowApprovalRequestedState(
  current: AIChatControllerState,
  payload: AIChatWorkflowPausedEventData,
  eventId?: string | null
): AIChatControllerState {
  return applyWorkflowTimelineState(current, payload, eventId, 'pending_approval', undefined, {
    approval_form_id: payload.approval_form_id,
    approval_token: payload.approval_token,
    approval_url: payload.approval_url,
    approval_form: payload.approval_form,
  });
}

export function applyWorkflowFinishedState(
  current: AIChatControllerState,
  payload: AIChatWorkflowEventData,
  eventId?: string | null
): AIChatControllerState {
  return applyWorkflowTimelineState(
    current,
    payload,
    eventId,
    normalizeWorkflowRunTimelineStatus(payload.status, 'completed')
  );
}

export function applyWorkflowFailedState(
  current: AIChatControllerState,
  payload: AIChatWorkflowEventData,
  eventId?: string | null
): AIChatControllerState {
  return applyWorkflowTimelineState(current, payload, eventId, 'error');
}

export function applySkillArtifactCreatedState(
  current: AIChatControllerState,
  payload: AIChatSkillArtifactCreatedEventData,
  eventId?: string | null
): AIChatControllerState {
  const nextGeneratedFile = normalizeSkillArtifactFile(payload);
  if (!nextGeneratedFile) {
    return current;
  }

  const messages = current.messagesByConversation[payload.conversation_id] ?? [];
  const nextMessages = messages.map(message => {
    if (message.id !== payload.message_id) {
      return message;
    }

    const generatedFiles = upsertGeneratedFile(
      message.metadata?.generated_files ?? [],
      nextGeneratedFile
    );

    return {
      ...message,
      metadata: {
        ...(message.metadata ?? {}),
        generated_file_count: generatedFiles.length,
        generated_files: generatedFiles,
      },
      updated_at: Math.floor(Date.now() / 1000),
    };
  });
  const previousStreaming = current.streamingByMessageId[payload.message_id];

  return {
    ...current,
    messagesByConversation: {
      ...current.messagesByConversation,
      [payload.conversation_id]: nextMessages,
    },
    streamingByMessageId: previousStreaming
      ? {
          ...current.streamingByMessageId,
          [payload.message_id]: {
            ...previousStreaming,
            last_event_id: eventId ?? previousStreaming.last_event_id,
          },
        }
      : current.streamingByMessageId,
  };
}

export function applyMessageStartState(
  current: AIChatControllerState,
  payload: AIChatMessageStartEventData,
  context: AIChatMessageStartContext = {},
  eventId?: string | null
): AIChatControllerState {
  const mode = context.mode ?? 'active';
  const conversation = createDraftAIChatConversation(payload.conversation_id, payload.title || '');
  conversation.current_leaf_message_id = payload.message_id;
  conversation.runtime_status = 'streaming';
  conversation.active_message_id = payload.message_id;
  const createdAt = payload.created_at ?? Math.floor(Date.now() / 1000);
  conversation.created_at = createdAt;
  conversation.updated_at = createdAt;

  const existingConversation =
    current.conversations.find(item => item.id === payload.conversation_id) ?? conversation;
  const nextConversation: AIChatConversation = {
    ...existingConversation,
    title: payload.title || existingConversation.title,
    current_leaf_message_id: payload.message_id,
    runtime_status: 'streaming',
    active_message_id: payload.message_id,
    updated_at: createdAt,
  };
  const shouldMigrateDraftConversation = Boolean(
    context.previousConversationId && context.previousConversationId !== payload.conversation_id
  );
  const baseConversations = shouldMigrateDraftConversation
    ? current.conversations.filter(item => item.id !== context.previousConversationId)
    : current.conversations;
  const messages =
    current.messagesByConversation[payload.conversation_id] ??
    (context.previousConversationId
      ? (current.messagesByConversation[context.previousConversationId] ?? [])
      : []);
  const existingMessage = messages.find(message => message.id === payload.message_id);
  const isReplace = payload.replace === true || context.resetAnswer === true;
  const createdMessage = createStreamingAIChatMessage({
    id: payload.message_id,
    conversationId: payload.conversation_id,
    parentId: payload.parent_id ?? existingMessage?.parent_id,
    query: context.query ?? existingMessage?.query ?? '',
    modelName: payload.model || context.model?.model || existingMessage?.model_name || 'unknown',
    modelProvider: context.model?.provider ?? existingMessage?.model_provider,
    createdAt: payload.created_at ?? existingMessage?.created_at,
    metadata: createAIChatFileMetadata(context.files),
  });
  const message: AIChatMessage = existingMessage
    ? {
        ...existingMessage,
        ...createdMessage,
        answer: isReplace ? '' : existingMessage.answer,
        created_at: existingMessage.created_at,
        error: undefined,
        metadata: isReplace
          ? clearRuntimeMessageMetadata(
              mergeMessageMetadata(existingMessage.metadata, createdMessage.metadata)
            )
          : mergeMessageMetadata(existingMessage.metadata, createdMessage.metadata),
        updated_at: createdAt,
      }
    : createdMessage;
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  const migratedMessages =
    shouldMigrateDraftConversation && context.previousConversationId
      ? messages.filter(message => message.conversation_id !== context.previousConversationId)
      : messages;
  const nextMessagesByConversation = {
    ...current.messagesByConversation,
    [payload.conversation_id]: upsertAIChatMessage(migratedMessages, message),
  };
  const nextMessagePaginationByConversation = {
    ...current.messagePaginationByConversation,
  };
  const nextLoadingOlderByConversation = {
    ...current.loadingOlderByConversation,
  };
  const nextRecoveringByConversation = {
    ...current.recoveringByConversation,
  };
  const nextStoppingByConversation = {
    ...current.stoppingByConversation,
  };
  const nextStreamingByMessageId = {
    ...current.streamingByMessageId,
  };
  if (shouldMigrateDraftConversation && context.previousConversationId) {
    delete nextMessagesByConversation[context.previousConversationId];
    delete nextMessagePaginationByConversation[context.previousConversationId];
    delete nextLoadingOlderByConversation[context.previousConversationId];
    delete nextRecoveringByConversation[context.previousConversationId];
    delete nextStoppingByConversation[context.previousConversationId];
    Object.values(current.streamingByMessageId).forEach(streaming => {
      if (streaming.conversation_id === context.previousConversationId) {
        delete nextStreamingByMessageId[streaming.message_id];
      }
    });
  }
  nextStreamingByMessageId[payload.message_id] = {
    conversation_id: payload.conversation_id,
    message_id: payload.message_id,
    answer: message.answer,
    status: 'streaming',
    timeline: isReplace ? [] : (previousStreaming?.timeline ?? []),
    last_event_id: eventId ?? (isReplace ? undefined : previousStreaming?.last_event_id),
    replay_base_answer: isReplace ? undefined : previousStreaming?.replay_base_answer,
    replay_offset: isReplace ? undefined : previousStreaming?.replay_offset,
    replace: isReplace || previousStreaming?.replace,
    sensitiveOutputBlocked: isReplace ? undefined : previousStreaming?.sensitiveOutputBlocked,
  };

  return {
    ...current,
    activeConversationId:
      mode === 'active' ? payload.conversation_id : current.activeConversationId,
    conversations: replaceAIChatConversation(baseConversations, nextConversation, {
      moveToTop: context.moveToTop ?? true,
    }),
    messagesByConversation: nextMessagesByConversation,
    messagePaginationByConversation: {
      ...nextMessagePaginationByConversation,
      [payload.conversation_id]: {
        ...(nextMessagePaginationByConversation[payload.conversation_id] ??
          DEFAULT_AICHAT_MESSAGE_PAGINATION),
        total: existingMessage
          ? (nextMessagePaginationByConversation[payload.conversation_id]?.total ??
            migratedMessages.length)
          : (nextMessagePaginationByConversation[payload.conversation_id]?.total ??
              migratedMessages.length) + 1,
      },
    },
    loadingOlderByConversation: nextLoadingOlderByConversation,
    recoveringByConversation: nextRecoveringByConversation,
    stoppingByConversation: nextStoppingByConversation,
    streamingByMessageId: nextStreamingByMessageId,
  };
}

export function applyFileParseStartState(
  current: AIChatControllerState,
  payload: AIChatFileParseStartEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateMessageFileMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    payload.file_id,
    payload.name,
    eventId,
    file =>
      file.id === payload.file_id
        ? {
            ...file,
            id: payload.file_id,
            name: payload.name || file.name,
            kind: payload.kind,
            content_status: file.content_status ?? 'pending',
            parse_status: 'parsing',
            error: undefined,
          }
        : { ...file, id: payload.file_id }
  );
}

export function applyFileParseEndState(
  current: AIChatControllerState,
  payload: AIChatFileParseEndEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateMessageFileMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    payload.file_id,
    payload.name,
    eventId,
    file =>
      file.id === payload.file_id
        ? {
            ...file,
            id: payload.file_id,
            name: payload.name || file.name,
            kind: payload.kind,
            content_status: payload.content_status,
            content_chars: payload.content_chars,
            from_cache: payload.from_cache,
            vision_detail: payload.vision_detail ?? null,
            filtered_reason: payload.filtered_reason ?? null,
            parse_status: 'completed',
            error: undefined,
          }
        : { ...file, id: payload.file_id }
  );
}

export function applyFileParseErrorState(
  current: AIChatControllerState,
  payload: AIChatFileParseErrorEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateMessageFileMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    payload.file_id,
    payload.name,
    eventId,
    file =>
      file.id === payload.file_id
        ? {
            ...file,
            id: payload.file_id,
            name: payload.name || file.name,
            kind: payload.kind,
            parse_status: 'error',
            error: payload.message,
          }
        : { ...file, id: payload.file_id }
  );
}

export function applySkillCallStartState(
  current: AIChatControllerState,
  payload: AIChatSkillCallStartEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: payload.kind ?? 'tool_call',
      runtime_id: payload.runtime_id,
      skill_id: payload.skill_id,
      tool_name: payload.tool_name,
      status: 'running',
      arguments: payload.arguments_summary ?? payload.arguments,
      created_at: payload.created_at,
    }
  );
}

export function applySkillCallEndState(
  current: AIChatControllerState,
  payload: AIChatSkillCallEndEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: payload.kind ?? 'tool_call',
      runtime_id: payload.runtime_id,
      skill_id: payload.skill_id,
      tool_name: payload.tool_name,
      status: payload.status ?? 'success',
      duration_ms: payload.duration_ms,
      message: payload.message,
      result: payload.result,
      created_at: payload.created_at,
    }
  );
}

export function applySkillCallErrorState(
  current: AIChatControllerState,
  payload: AIChatSkillCallErrorEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: payload.kind ?? (payload.tool_name ? 'tool_call' : 'skill_load'),
      runtime_id: payload.runtime_id,
      skill_id: payload.skill_id,
      tool_name: payload.tool_name ?? '',
      status: 'error',
      duration_ms: payload.duration_ms,
      message: payload.message,
      error: payload.message,
      created_at: payload.created_at,
    }
  );
}

export function applySkillLoadStartState(
  current: AIChatControllerState,
  payload: AIChatSkillLoadStartEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: 'skill_load',
      skill_id: payload.skill_id,
      tool_name: '',
      status: 'loading',
      created_at: payload.created_at,
    }
  );
}

export function applySkillLoadEndState(
  current: AIChatControllerState,
  payload: AIChatSkillLoadEndEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: 'skill_load',
      skill_id: payload.skill_id,
      tool_name: '',
      status: 'success',
      duration_ms: payload.duration_ms,
      created_at: payload.created_at,
    }
  );
}

export function applySkillReferenceReadState(
  current: AIChatControllerState,
  payload: AIChatSkillReferenceReadEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateSkillInvocationMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    eventId,
    {
      kind: 'reference_read',
      skill_id: payload.skill_id,
      tool_name: '',
      path: payload.path,
      status: 'success',
      duration_ms: payload.duration_ms,
      created_at: payload.created_at,
    }
  );
}

function resolveReplayChunk(
  streaming: AIChatStreamingMessageState | undefined,
  answerChunk: string
): {
  appendChunk: string;
  replayBaseAnswer?: string;
  replayOffset?: number;
} {
  const replayBaseAnswer = streaming?.replay_base_answer;
  const replayOffset = streaming?.replay_offset ?? 0;
  if (!replayBaseAnswer || replayOffset >= replayBaseAnswer.length || !answerChunk) {
    return {
      appendChunk: answerChunk,
      replayBaseAnswer,
      replayOffset: streaming?.replay_offset,
    };
  }

  const remainingBase = replayBaseAnswer.slice(replayOffset);
  const maxOverlap = Math.min(answerChunk.length, remainingBase.length);
  let overlap = 0;
  while (overlap < maxOverlap && answerChunk[overlap] === remainingBase[overlap]) {
    overlap += 1;
  }

  if (overlap === 0) {
    return { appendChunk: answerChunk };
  }

  const nextReplayOffset = replayOffset + overlap;
  return {
    appendChunk: answerChunk.slice(overlap),
    replayBaseAnswer: nextReplayOffset >= replayBaseAnswer.length ? undefined : replayBaseAnswer,
    replayOffset: nextReplayOffset >= replayBaseAnswer.length ? undefined : nextReplayOffset,
  };
}

export function applyMessageChunkState(
  current: AIChatControllerState,
  payload: AIChatMessageChunkEventData,
  eventId?: string | null
): AIChatControllerState {
  const isSensitiveBlocked =
    isSensitiveOutputBlockedValue(payload.answer) ||
    (payload as unknown as Record<string, unknown>)[SENSITIVE_OUTPUT_BLOCKED_FLAG] === true;
  const answerChunk = isSensitiveBlocked ? SENSITIVE_OUTPUT_BLOCKED_TOKEN : payload.answer || '';
  const messages = current.messagesByConversation[payload.conversation_id] ?? [];
  const existingMessage = messages.find(message => message.id === payload.message_id);
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  const { appendChunk, replayBaseAnswer, replayOffset } = isSensitiveBlocked
    ? {
        appendChunk: answerChunk,
        replayBaseAnswer: undefined,
        replayOffset: undefined,
      }
    : resolveReplayChunk(previousStreaming, answerChunk);
  const now = Math.floor(Date.now() / 1000);
  const nextMessage = existingMessage
    ? {
        ...existingMessage,
        answer: isSensitiveBlocked ? answerChunk : `${existingMessage.answer}${appendChunk}`,
        status: 'streaming' as const,
        metadata: isSensitiveBlocked
          ? {
              ...existingMessage.metadata,
              sensitiveOutputBlocked: true,
            }
          : existingMessage.metadata,
        updated_at: now,
      }
    : createStreamingAIChatMessage({
        id: payload.message_id,
        conversationId: payload.conversation_id,
        query: '',
        modelName: 'unknown',
        createdAt: now,
      });
  if (!existingMessage) {
    nextMessage.answer = appendChunk;
    if (isSensitiveBlocked) {
      nextMessage.metadata = {
        ...nextMessage.metadata,
        sensitiveOutputBlocked: true,
      };
    }
  }
  const nextStreamingAnswer = isSensitiveBlocked
    ? answerChunk
    : `${previousStreaming?.answer ?? existingMessage?.answer ?? ''}${appendChunk}`;
  let conversationChanged = false;
  const nextConversations = current.conversations.map(conversation => {
    if (conversation.id !== payload.conversation_id) return conversation;
    if (
      conversation.runtime_status === 'streaming' &&
      conversation.active_message_id === payload.message_id
    ) {
      return conversation;
    }

    conversationChanged = true;
    return {
      ...conversation,
      runtime_status: 'streaming' as const,
      active_message_id: payload.message_id,
    };
  });
  const conversations = conversationChanged ? nextConversations : current.conversations;

  return {
    ...current,
    conversations,
    messagesByConversation: {
      ...current.messagesByConversation,
      [payload.conversation_id]: upsertAIChatMessage(messages, nextMessage),
    },
    streamingByMessageId: {
      ...current.streamingByMessageId,
      [payload.message_id]: {
        conversation_id: payload.conversation_id,
        message_id: payload.message_id,
        answer: nextStreamingAnswer,
        status: 'streaming',
        timeline: removeTransientProgressItems(previousStreaming?.timeline),
        last_event_id: eventId ?? previousStreaming?.last_event_id,
        replay_base_answer: replayBaseAnswer,
        replay_offset: replayOffset,
        replace: previousStreaming?.replace,
        sensitiveOutputBlocked: isSensitiveBlocked || previousStreaming?.sensitiveOutputBlocked,
      },
    },
  };
}

function removeRetractedSuffix(answer: string, content: string, length?: number): string {
  if (!answer) {
    return answer;
  }
  if (content && answer.endsWith(content)) {
    return answer.slice(0, -content.length);
  }
  const safeLength =
    typeof length === 'number' && Number.isFinite(length) && length > 0
      ? Math.min(Math.floor(length), answer.length)
      : 0;
  if (!content && safeLength > 0) {
    return answer.slice(0, -safeLength);
  }
  return answer;
}

export function applyMessageRetractState(
  current: AIChatControllerState,
  payload: AIChatMessageRetractEventData,
  eventId?: string | null
): AIChatControllerState {
  const content = payload.content ?? '';
  if (!payload.conversation_id || !payload.message_id) {
    return current;
  }

  const messages = current.messagesByConversation[payload.conversation_id] ?? [];
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  const nextMessages = messages.map(message =>
    message.id === payload.message_id
      ? {
          ...message,
          answer: removeRetractedSuffix(message.answer, content, payload.length),
        }
      : message
  );

  return {
    ...current,
    messagesByConversation: {
      ...current.messagesByConversation,
      [payload.conversation_id]: nextMessages,
    },
    streamingByMessageId: previousStreaming
      ? {
          ...current.streamingByMessageId,
          [payload.message_id]: {
            ...previousStreaming,
            answer: removeRetractedSuffix(previousStreaming.answer, content, payload.length),
            last_event_id: eventId ?? previousStreaming.last_event_id,
          },
        }
      : current.streamingByMessageId,
  };
}

export function applyMessageEndState(
  current: AIChatControllerState,
  payload: AIChatMessageEndEventData
): AIChatControllerState {
  const endedAt = Math.floor(Date.now() / 1000);
  const messages = current.messagesByConversation[payload.conversation_id] ?? [];
  const nextMessages = messages.map(message =>
    message.id === payload.message_id
      ? {
          ...message,
          status: normalizeAIChatStatus(payload.status),
          metadata:
            message.metadata?.sensitiveOutputBlocked === true
              ? {
                  ...mergeMessageMetadata(message.metadata, payload.metadata),
                  sensitiveOutputBlocked: true,
                }
              : mergeMessageMetadata(message.metadata, payload.metadata),
          updated_at: endedAt,
        }
      : message
  );
  const previousStreaming = current.streamingByMessageId[payload.message_id];
  const nextTimeline = removeTransientProgressItems(previousStreaming?.timeline);
  const nextStreamingByMessageId = { ...current.streamingByMessageId };
  if (nextTimeline.length) {
    const terminalStatus = normalizeAIChatStatus(payload.status);
    nextStreamingByMessageId[payload.message_id] = {
      ...previousStreaming,
      timeline: nextTimeline,
      status:
        terminalStatus === 'stopped' ||
        terminalStatus === 'error' ||
        terminalStatus === 'waiting_approval' ||
        terminalStatus === 'waiting_question'
          ? terminalStatus
          : 'completed',
    };
  } else {
    delete nextStreamingByMessageId[payload.message_id];
  }

  return {
    ...current,
    conversations: current.conversations.map(conversation =>
      conversation.id === payload.conversation_id
        ? {
            ...conversation,
            runtime_status: 'idle' as const,
            active_message_id: undefined,
            current_leaf_message_id: payload.message_id,
            updated_at: endedAt,
          }
        : conversation
    ),
    messagesByConversation: {
      ...current.messagesByConversation,
      [payload.conversation_id]: nextMessages,
    },
    streamingByMessageId: nextStreamingByMessageId,
    recoveringByConversation: {
      ...current.recoveringByConversation,
      [payload.conversation_id]: false,
    },
    isSending: getNextActiveSendingState(current, payload.conversation_id, false),
  };
}

export function applyStreamErrorState(
  current: AIChatControllerState,
  payload: AIChatErrorEventData,
  fallbackConversationId: string | null
): AIChatControllerState {
  const conversationId = payload.conversation_id || fallbackConversationId;
  const messageId = payload.message_id;
  const message = payload.message || 'AIChat stream error';
  const errorMetadata =
    payload.code || payload.params
      ? {
          error_code: payload.code,
          error_params: payload.params,
        }
      : undefined;
  const messages = conversationId ? (current.messagesByConversation[conversationId] ?? []) : [];
  const erroredMessage = messageId ? messages.find(item => item.id === messageId) : undefined;
  const nextStreamingByMessageId = { ...current.streamingByMessageId };
  if (messageId) {
    delete nextStreamingByMessageId[messageId];
  }

  return {
    ...current,
    error: message,
    isSending: getNextActiveSendingState(current, conversationId, false),
    conversations: conversationId
      ? current.conversations.map(conversation =>
          conversation.id === conversationId
            ? {
                ...conversation,
                runtime_status: 'idle' as const,
                active_message_id: undefined,
                current_leaf_message_id: messageId || conversation.current_leaf_message_id,
                dialogue_count:
                  messageId && erroredMessage && !erroredMessage.parent_id
                    ? 1
                    : conversation.dialogue_count,
              }
            : conversation
        )
      : current.conversations,
    messagesByConversation: conversationId
      ? {
          ...current.messagesByConversation,
          [conversationId]:
            conversationId && messageId
              ? messages.map(item =>
                  item.id === messageId
                    ? {
                        ...item,
                        status: 'error' as const,
                        error: message,
                        metadata: errorMetadata
                          ? {
                              ...(item.metadata ?? {}),
                              ...errorMetadata,
                            }
                          : item.metadata,
                        updated_at: Math.floor(Date.now() / 1000),
                      }
                    : item
                )
              : messages,
        }
      : current.messagesByConversation,
    streamingByMessageId: nextStreamingByMessageId,
    recoveringByConversation: conversationId
      ? {
          ...current.recoveringByConversation,
          [conversationId]: false,
        }
      : current.recoveringByConversation,
  };
}
