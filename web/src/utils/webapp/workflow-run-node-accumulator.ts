import type { NodeInfo } from '@/components/chat/types';
import {
  extractWorkflowRunContainerContext,
  getWorkflowRunCreatedAtMs,
  getWorkflowRunExecutionId,
  getWorkflowRunItemKey,
  getWorkflowRunRoundDurationMap,
  getWorkflowRunRoundElapsedTime,
  sortWorkflowRunItems,
  sortWorkflowRunRounds,
} from '@/utils/workflow/run-events';
import { mapNode, unwrap } from '@/utils/webapp/run-mappers';

interface WorkflowRunNodeAccumulatorCallbacks {
  onNodeStarted?: (node: NodeInfo) => void;
  onNodeFinished?: (node: NodeInfo) => void;
}

interface ContainerRound {
  index: number;
  nodes: NodeInfo[];
  elapsedTime?: number;
  variables?: unknown;
}

interface ContainerSession {
  nodeId?: string;
  nodeType: 'iteration' | 'loop';
  title: string;
  inputs?: unknown;
  outputs?: unknown;
  elapsedTime?: number;
  error?: string;
  rounds: ContainerRound[];
  activeIndex: number | null;
}

export interface WorkflowRunNodeAccumulator {
  reset: () => void;
  replaceSnapshot: (nodes: NodeInfo[]) => void;
  onNodeStarted: (payload: unknown) => void;
  onNodeFinished: (payload: unknown) => void;
  onIterationStarted: (payload: unknown) => void;
  onIterationNext: (payload: unknown) => void;
  onIterationCompleted: (payload: unknown) => void;
  onLoopStarted: (payload: unknown) => void;
  onLoopNext: (payload: unknown) => void;
  onLoopCompleted: (payload: unknown) => void;
}

function stringField(source: Record<string, unknown>, key: string): string | undefined {
  return typeof source[key] === 'string' ? (source[key] as string) : undefined;
}

function numberField(source: Record<string, unknown>, key: string): number | undefined {
  return typeof source[key] === 'number' ? (source[key] as number) : undefined;
}

function containerIdentity(payload: unknown, kind: 'iteration' | 'loop') {
  const source = unwrap(payload);
  const context = extractWorkflowRunContainerContext(source);
  const nodeId = stringField(source, 'node_id') ?? context.containerId;
  const nodeType = context.containerType ?? kind;
  return {
    source,
    nodeId,
    nodeType,
    title: stringField(source, 'title') || nodeType,
    key: nodeId || stringField(source, 'title') || nodeType,
  };
}

function childNode(payload: unknown, finished: boolean, receivedOrder: number): NodeInfo {
  const source = unwrap(payload);
  const mapped = mapNode(payload, finished);
  return {
    ...mapped,
    executionId: getWorkflowRunExecutionId(source),
    createdAtMs: getWorkflowRunCreatedAtMs(source),
    receivedOrder,
    data: {
      input: mapped.data?.input,
      output: finished ? mapped.data?.output : undefined,
      modelInput: mapped.data?.modelInput,
    },
  };
}

function ensureRound(session: ContainerSession, index: number): ContainerRound {
  let round = session.rounds.find(item => item.index === index);
  if (!round) {
    round = { index, nodes: [] };
    session.rounds.push(round);
  }
  return round;
}

function upsertRoundNode(round: ContainerRound, node: NodeInfo): void {
  const key = getWorkflowRunItemKey(node);
  const index = round.nodes.findIndex(item => getWorkflowRunItemKey(item) === key);
  if (index < 0) {
    round.nodes.push(node);
    return;
  }
  const previous = round.nodes[index];
  round.nodes[index] = {
    ...previous,
    ...node,
    createdAtMs: previous.createdAtMs ?? node.createdAtMs,
    receivedOrder: previous.receivedOrder ?? node.receivedOrder,
    data: {
      input: node.data?.input ?? previous.data?.input,
      output: node.data?.output ?? previous.data?.output,
      modelInput: node.data?.modelInput ?? previous.data?.modelInput,
    },
  };
}

export function createWorkflowRunNodeAccumulator(
  callbacks: WorkflowRunNodeAccumulatorCallbacks
): WorkflowRunNodeAccumulator {
  const iterationSessions = new Map<string, ContainerSession>();
  const loopSessions = new Map<string, ContainerSession>();
  let activeIteration: { nodeId: string | null; index: number | null } = {
    nodeId: null,
    index: null,
  };
  let activeLoop: { nodeId: string | null; index: number | null } = {
    nodeId: null,
    index: null,
  };
  let receivedOrder = 0;

  const reset = () => {
    iterationSessions.clear();
    loopSessions.clear();
    activeIteration = { nodeId: null, index: null };
    activeLoop = { nodeId: null, index: null };
    receivedOrder = 0;
  };

  const replaceSnapshot = (nodes: NodeInfo[]) => {
    reset();

    const visit = (node: NodeInfo) => {
      const kind = node.nodeType === 'iteration' || node.nodeType === 'loop' ? node.nodeType : null;
      if (kind) {
        const rounds = (kind === 'iteration' ? node.iterationRounds : node.loopRounds) ?? [];
        const session: ContainerSession = {
          nodeId: node.nodeId,
          nodeType: kind,
          title: node.title || node.nodeId || kind,
          inputs: kind === 'iteration' ? node.iterationInputs : node.loopInputs,
          outputs: kind === 'iteration' ? node.iterationOutputs : node.loopOutputs,
          elapsedTime: node.elapsedTime,
          error: node.error,
          rounds: rounds.map(round => ({
            index: round.index,
            nodes: round.nodes,
            elapsedTime: round.elapsedTime,
            variables: 'variables' in round ? round.variables : undefined,
          })),
          activeIndex: null,
        };
        const key = node.nodeId || session.title;
        const sessions = kind === 'iteration' ? iterationSessions : loopSessions;
        sessions.set(key, session);

        if (node.status === 'running' || node.status === 'paused') {
          const lastRound = sortWorkflowRunRounds(session.rounds).at(-1);
          session.activeIndex = lastRound?.index ?? null;
          if (kind === 'iteration') {
            activeIteration = { nodeId: key, index: session.activeIndex };
          } else {
            activeLoop = { nodeId: key, index: session.activeIndex };
          }
        }

        session.rounds.forEach(round => round.nodes.forEach(visit));
      }
    };

    nodes.forEach(visit);
  };

  const emitContainer = (session: ContainerSession, finished: boolean) => {
    const rounds = sortWorkflowRunRounds(session.rounds).map(round => ({
      index: round.index,
      nodes: sortWorkflowRunItems(round.nodes),
      elapsedTime: round.elapsedTime,
      variables: round.variables,
    }));
    const common: NodeInfo = {
      status: finished ? (session.error ? 'failed' : 'success') : 'running',
      nodeId: session.nodeId,
      nodeType: session.nodeType,
      title: session.title,
      elapsedTime: session.elapsedTime,
      error: session.error,
      steps: rounds.length,
    };
    if (session.nodeType === 'iteration') {
      const node: NodeInfo = {
        ...common,
        iterationInputs: session.inputs,
        iterationOutputs: session.outputs,
        iterationRounds: rounds.map(({ variables: _variables, ...round }) => round),
      };
      (finished ? callbacks.onNodeFinished : callbacks.onNodeStarted)?.(node);
      return;
    }
    const node: NodeInfo = {
      ...common,
      loopInputs: session.inputs,
      loopOutputs: session.outputs,
      loopRounds: rounds,
    };
    (finished ? callbacks.onNodeFinished : callbacks.onNodeStarted)?.(node);
  };

  const handleContainerLifecycle = (
    payload: unknown,
    node: NodeInfo,
    finished: boolean
  ): boolean => {
    const kind = node.nodeType === 'iteration' || node.nodeType === 'loop' ? node.nodeType : null;
    if (!kind) return false;

    const source = unwrap(payload);
    const context = extractWorkflowRunContainerContext(source);
    // A container node can itself be a child of another container. In that case it must still be
    // projected into the parent round rather than consumed as a root container lifecycle update.
    if (context.containerId && context.containerId !== node.nodeId) return false;

    const key = node.nodeId || node.title || kind;
    const sessions = kind === 'iteration' ? iterationSessions : loopSessions;
    const session = sessions.get(key);
    if (!session) return false;

    session.nodeId = node.nodeId ?? session.nodeId;
    session.title = node.title || session.title;
    session.inputs = node.data?.input ?? session.inputs;
    session.outputs = node.data?.output ?? session.outputs;
    session.elapsedTime = node.elapsedTime ?? session.elapsedTime;
    session.error = node.error ?? session.error;
    sessions.set(key, session);

    if (finished) {
      if (kind === 'iteration') activeIteration = { nodeId: null, index: null };
      else activeLoop = { nodeId: null, index: null };
    }
    emitContainer(session, finished);
    return true;
  };

  const handleChild = (payload: unknown, finished: boolean) => {
    const source = unwrap(payload);
    const node = childNode(payload, finished, ++receivedOrder);
    if (handleContainerLifecycle(payload, node, finished)) return;
    const { loopId, loopIndex, iterationId, iterationIndex } =
      extractWorkflowRunContainerContext(source);

    const targetLoopId = loopId ?? activeLoop.nodeId ?? undefined;
    const targetLoopIndex =
      typeof loopIndex === 'number' ? loopIndex : (activeLoop.index ?? undefined);
    if (targetLoopId && typeof targetLoopIndex === 'number') {
      const session = loopSessions.get(targetLoopId) ?? {
        nodeId: targetLoopId,
        nodeType: 'loop' as const,
        title: targetLoopId,
        rounds: [],
        activeIndex: targetLoopIndex,
      };
      loopSessions.set(targetLoopId, session);
      upsertRoundNode(ensureRound(session, targetLoopIndex), node);
      session.activeIndex = targetLoopIndex;
      activeLoop = { nodeId: targetLoopId, index: targetLoopIndex };
      emitContainer(session, false);
      return;
    }

    const targetIterationId = iterationId ?? activeIteration.nodeId ?? undefined;
    const targetIterationIndex =
      typeof iterationIndex === 'number' ? iterationIndex : (activeIteration.index ?? undefined);
    if (targetIterationId && typeof targetIterationIndex === 'number') {
      const session = iterationSessions.get(targetIterationId) ?? {
        nodeId: targetIterationId,
        nodeType: 'iteration' as const,
        title: targetIterationId,
        rounds: [],
        activeIndex: targetIterationIndex,
      };
      iterationSessions.set(targetIterationId, session);
      upsertRoundNode(ensureRound(session, targetIterationIndex), node);
      session.activeIndex = targetIterationIndex;
      activeIteration = { nodeId: targetIterationId, index: targetIterationIndex };
      emitContainer(session, false);
      return;
    }

    (finished ? callbacks.onNodeFinished : callbacks.onNodeStarted)?.(node);
  };

  const startContainer = (payload: unknown, kind: 'iteration' | 'loop') => {
    const { source, nodeId, nodeType, title, key } = containerIdentity(payload, kind);
    const session: ContainerSession = {
      nodeId,
      nodeType,
      title,
      inputs: source.inputs,
      rounds: [],
      activeIndex: null,
    };
    (kind === 'iteration' ? iterationSessions : loopSessions).set(key, session);
    if (kind === 'iteration') activeIteration = { nodeId: key, index: null };
    else activeLoop = { nodeId: key, index: null };
    emitContainer(session, false);
  };

  const nextContainerRound = (payload: unknown, kind: 'iteration' | 'loop') => {
    const { nodeId, nodeType, title, key } = containerIdentity(payload, kind);
    const source = unwrap(payload);
    const sessions = kind === 'iteration' ? iterationSessions : loopSessions;
    const session = sessions.get(key) ?? {
      nodeId,
      nodeType,
      title,
      rounds: [],
      activeIndex: null,
    };
    session.nodeId = nodeId ?? session.nodeId;
    session.title = title || session.title;
    const containerContext = extractWorkflowRunContainerContext(source);
    const index =
      containerContext.roundIndex ??
      numberField(source, kind === 'iteration' ? 'iteration_index' : 'loop_index') ??
      numberField(source, 'index') ??
      0;
    ensureRound(session, index);
    session.activeIndex = index;
    sessions.set(key, session);
    if (kind === 'iteration') activeIteration = { nodeId: key, index };
    else activeLoop = { nodeId: key, index };
    emitContainer(session, false);
  };

  const completeContainer = (payload: unknown, kind: 'iteration' | 'loop') => {
    const { source, nodeId, nodeType, title, key } = containerIdentity(payload, kind);
    const sessions = kind === 'iteration' ? iterationSessions : loopSessions;
    const session = sessions.get(key) ?? {
      nodeId,
      nodeType,
      title,
      rounds: [],
      activeIndex: null,
    };
    session.nodeId = nodeId ?? session.nodeId;
    session.title = title || session.title;
    session.inputs = source.inputs ?? session.inputs;
    session.elapsedTime = numberField(source, 'elapsed_time');
    session.error = mapNode(payload, true).error;
    session.outputs = source.outputs;
    const durations = getWorkflowRunRoundDurationMap(source, kind);
    const variableMap =
      kind === 'loop' && source.execution_metadata && typeof source.execution_metadata === 'object'
        ? ((source.execution_metadata as Record<string, unknown>).loop_variable_map as
            | Record<string, unknown>
            | undefined)
        : undefined;
    session.rounds = session.rounds.map(round => ({
      ...round,
      elapsedTime: durations.get(round.index) ?? getWorkflowRunRoundElapsedTime(round),
      variables: variableMap?.[String(round.index)] ?? round.variables,
    }));
    sessions.set(key, session);
    if (kind === 'iteration') activeIteration = { nodeId: null, index: null };
    else activeLoop = { nodeId: null, index: null };
    emitContainer(session, true);
  };

  return {
    reset,
    replaceSnapshot,
    onNodeStarted: payload => handleChild(payload, false),
    onNodeFinished: payload => handleChild(payload, true),
    onIterationStarted: payload => startContainer(payload, 'iteration'),
    onIterationNext: payload => nextContainerRound(payload, 'iteration'),
    onIterationCompleted: payload => completeContainer(payload, 'iteration'),
    onLoopStarted: payload => startContainer(payload, 'loop'),
    onLoopNext: payload => nextContainerRound(payload, 'loop'),
    onLoopCompleted: payload => completeContainer(payload, 'loop'),
  };
}
