import type {
  AIChatWorkflowEventData,
  AIChatWorkflowNodeEventData,
  AIChatWorkflowPausedEventData
} from '@/services/types/aichat';
import type {
  NodeInfo,
  RunStatus
} from '@/components/chat/types';
import {
  type AIChatControllerState,
  type AIChatAgenticTimelineItem
} from '@/components/chat/controllers/aichat/types';
import {
  extractLlmGatewayRequest,
  extractWorkflowRunContainerContext,
  getWorkflowRunCreatedAtMs,
  getWorkflowRunExecutionId,
  getWorkflowRunItemKey,
  getWorkflowRunRoundDurationMap,
  getWorkflowRunRoundElapsedTime,
  sortWorkflowRunItems,
  sortWorkflowRunRounds
} from '@/utils/workflow/run-events';
import { removeTransientProgressItems } from './shared';

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

function applyWorkflowContainerRoundDurations(
  container: NodeInfo,
  payload: AIChatWorkflowEventData,
  kind: 'iteration' | 'loop'
): NodeInfo {
  const durations = getWorkflowRunRoundDurationMap(payload, kind);
  if (durations.size === 0) return container;
  const rounds = kind === 'iteration' ? container.iterationRounds ?? [] : container.loopRounds ?? [];
  const durationRounds: Array<{
    index: number;
    nodes: NodeInfo[];
    elapsedTime?: number;
    variables?: unknown;
  }> = Array.from(durations.entries()).map(([index, elapsedTime]) => {
    const existing = rounds.find(round => round.index === index);
    return {
      ...(existing ?? { index, nodes: [] }),
      elapsedTime,
    };
  });
  const nextRounds = sortWorkflowRunRounds(
    durationRounds.concat(rounds.filter(round => !durations.has(round.index)))
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
    container = applyWorkflowContainerRoundDurations(container, payload, childKind);
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
    container = applyWorkflowContainerRoundDurations(container, payload, lifecycleKind);
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
