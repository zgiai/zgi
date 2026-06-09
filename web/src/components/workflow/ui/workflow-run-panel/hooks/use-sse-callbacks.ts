import { useMemo, useCallback, useRef } from 'react';
import { useThrottledTextStream } from '@/hooks/use-throttled-text-stream';
import { STREAM_RENDER_THROTTLE_MS } from '@/lib/config';
import type { ReactFlowInstance, Viewport } from '@xyflow/react';
import type { WorkflowRunSseCallbacks } from '@/services/workflow.service';
import { useAutoFollowNode } from './use-auto-follow-node';
import type { HistoryResult, WorkflowFinishedData } from '../types';
import type { WorkflowRunNodeListItem } from '../../workflow-run-nodes-list';
import type { WorkflowNode, WorkflowEdge } from '../../../store/type';
import type { RunGraphSnapshot } from '../../../store/helpers/history';
import { useWorkflowBillingFeedback } from '@/hooks/workflow/use-workflow-billing-feedback';
import {
  extractLlmGatewayRequest,
  extractWorkflowRunContainerContext,
  getWorkflowRunCreatedAtMs,
  getWorkflowRunExecutionId,
  getWorkflowRunItemKey,
  getWorkflowRunRoundDurationMap,
  getWorkflowRunRoundElapsedTime,
  sortWorkflowRunItems,
  sortWorkflowRunRounds,
} from '@/utils/workflow/run-events';
import {
  getSensitiveOutputTextFromPayload,
  SENSITIVE_OUTPUT_BLOCKED_TOKEN,
} from '@/utils/model-output-filter';

interface RecordLike {
  [key: string]: unknown;
}

interface UseSseCallbacksParams {
  rf: ReactFlowInstance;
  viewport: Viewport;
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  setHistorySnapshot: (runId: string, snapshot: RunGraphSnapshot) => void;
  resetRunStatus: () => void;
  setNodeRunStatus: (
    nodeId: string,
    status: 'idle' | 'running' | 'succeeded' | 'failed' | 'stopped' | 'paused'
  ) => void;
  setActiveOutputHandle: (nodeId: string, outputHandle: string | null) => void;
  resetActiveOutputHandles: () => void;
  beginRuntimeLogPopoverAutoOpen: () => void;
  finalizeRuntimeLogPopoversAfterRun: () => void;
  setAutoFollow: (enabled: boolean) => void;
  setCurrentRunningNodeId: (nodeId: string | null) => void;
  lastInputs: Record<string, unknown> | undefined;
  setRunItems: React.Dispatch<React.SetStateAction<WorkflowRunNodeListItem[]>>;
  setStreamedText: React.Dispatch<React.SetStateAction<string>>;
  setRunSummary: React.Dispatch<React.SetStateAction<WorkflowFinishedData | null>>;
  setActiveTab: (tab: 'inputs' | 'execution' | 'details' | 'results') => void;
  setFinalResult: React.Dispatch<React.SetStateAction<HistoryResult | null>>;
}

export function useSseCallbacks(params: UseSseCallbacksParams): WorkflowRunSseCallbacks {
  const {
    rf,
    viewport,
    nodes,
    edges,
    setHistorySnapshot,
    resetRunStatus,
    setNodeRunStatus,
    setActiveOutputHandle,
    resetActiveOutputHandles,
    beginRuntimeLogPopoverAutoOpen,
    finalizeRuntimeLogPopoversAfterRun,
    setAutoFollow,
    setCurrentRunningNodeId,
    lastInputs,
    setRunItems,
    setStreamedText,
    setRunSummary,
    setActiveTab,
    setFinalResult,
  } = params;

  const autoFollow = useAutoFollowNode(rf, viewport);
  const { getWorkflowRunErrorText } = useWorkflowBillingFeedback('agents');

  const throttler = useThrottledTextStream(STREAM_RENDER_THROTTLE_MS, (text: string) => {
    setStreamedText(prev => prev + text);
  });

  const isRecord = useCallback(
    (v: unknown): v is RecordLike => typeof v === 'object' && v !== null,
    []
  );
  const getSseData = useCallback(
    (payload: unknown): RecordLike | null => {
      if (!isRecord(payload)) return null;
      const maybeData = (payload as RecordLike)['data'];
      if (isRecord(maybeData)) return maybeData as RecordLike;
      return payload as RecordLike;
    },
    [isRecord]
  );
  const pickNumber = useCallback((obj: RecordLike | null, key: string): number | undefined => {
    if (!obj) return undefined;
    const v = obj[key];
    return typeof v === 'number' ? v : undefined;
  }, []);
  const pickString = useCallback((obj: RecordLike | null, key: string): string | undefined => {
    if (!obj) return undefined;
    const v = obj[key];
    return typeof v === 'string' ? v : undefined;
  }, []);
  const pickRecord = useCallback(
    (obj: RecordLike | null, key: string): RecordLike | undefined => {
      if (!obj) return undefined;
      const v = obj[key];
      return isRecord(v) ? (v as RecordLike) : undefined;
    },
    [isRecord]
  );

  interface IterationRound {
    index: number;
    nodes: WorkflowRunNodeListItem[];
    elapsedTime?: number;
  }
  interface LoopRound {
    index: number;
    nodes: WorkflowRunNodeListItem[];
    elapsedTime?: number;
    variables?: unknown;
  }
  interface IterationSession {
    nodeId: string;
    title: string;
    inputs?: unknown;
    iterationLength?: number;
    rounds: IterationRound[];
    activeIndex?: number;
    outputs?: unknown;
    elapsedTime?: number;
    error?: string | null;
  }
  const iterationSessionsRef = useRef<Map<string, IterationSession>>(new Map());
  const activeIterationRef = useRef<{ nodeId: string | null; index: number | null }>({
    nodeId: null,
    index: null,
  });
  interface LoopSession {
    nodeId: string;
    title: string;
    inputs?: unknown;
    loopLength?: number;
    rounds: LoopRound[];
    activeIndex?: number;
    outputs?: unknown;
    elapsedTime?: number;
    error?: string | null;
    steps?: number;
  }
  const loopSessionsRef = useRef<Map<string, LoopSession>>(new Map());
  const activeLoopRef = useRef<{ nodeId: string | null; index: number | null }>({
    nodeId: null,
    index: null,
  });
  const receiveOrderRef = useRef(0);
  const nextReceiveOrder = useCallback(() => {
    receiveOrderRef.current += 1;
    return receiveOrderRef.current;
  }, []);

  return useMemo<WorkflowRunSseCallbacks>(
    () => ({
      onWorkflowStarted: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          const createdAt = pickNumber(d, 'created_at');
          const inputs =
            d && isRecord(d.inputs)
              ? (d.inputs as Record<string, unknown>)
              : (lastInputs as Record<string, unknown> | undefined);
          const tokenUsage = pickRecord(d, 'token_usage');
          const totalTokens =
            pickNumber(d, 'total_tokens') ??
            (tokenUsage ? pickNumber(tokenUsage, 'total_tokens') : undefined);
          const runId = pickString(d, 'id');

          if (runId) {
            setHistorySnapshot(runId, { nodes, edges, viewport });
          }
          // Clear iteration sessions from previous runs to prevent memory buildup
          iterationSessionsRef.current.clear();
          activeIterationRef.current = { nodeId: null, index: null };
          loopSessionsRef.current.clear();
          activeLoopRef.current = { nodeId: null, index: null };
          receiveOrderRef.current = 0;

          resetRunStatus();
          beginRuntimeLogPopoverAutoOpen();
          resetActiveOutputHandles();
          setAutoFollow(true);
          setCurrentRunningNodeId(null);
          setRunItems([]);
          setStreamedText('');
          throttler.cancel();
          setRunSummary({
            id: runId ?? '',
            status: 'running',
            created_at: createdAt,
            inputs,
            total_tokens: totalTokens,
          });
          setFinalResult(null);
          setActiveTab('results');
        } catch (_e) {
          setRunItems([]);
          setStreamedText('');
          throttler.cancel();
          setRunSummary({ id: '', status: 'running', inputs: lastInputs });
          setFinalResult(null);
          setActiveTab('results');
        }
      },
      onWorkflowPaused: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          if (!d) return;
          setRunSummary(
            prev =>
              ({
                ...prev,
                status: pickString(d, 'status') ?? 'paused',
                outputs: 'outputs' in d ? (d.outputs as unknown) : prev?.outputs,
              }) as WorkflowFinishedData
          );
          setAutoFollow(false);
          setCurrentRunningNodeId(null);
          throttler.flush();
          setActiveTab('results');
        } catch (_e) {
          setCurrentRunningNodeId(null);
          setActiveTab('results');
        }
      },
      onQuestionAnswerRequested: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          const nodeId = pickString(d, 'node_id');
          if (nodeId) {
            setNodeRunStatus(nodeId, 'paused');
            setCurrentRunningNodeId(null);
          }
        } catch (e) {
          // eslint-disable-next-line no-console
          console.warn('onQuestionAnswerRequested parse error', e);
        }
      },
      onQuestionAnswerSubmitted: () => {
        setCurrentRunningNodeId(null);
      },
      onIterationStarted: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          if (!d) return;
          const nodeId = 'node_id' in d && typeof d.node_id === 'string' ? d.node_id : undefined;
          if (!nodeId) return;
          const title =
            'title' in d && typeof d.title === 'string'
              ? (d.title as string)
              : 'node_type' in d && typeof d.node_type === 'string'
                ? (d.node_type as string)
                : nodeId;
          const inputs = 'inputs' in d ? (d.inputs as unknown) : undefined;
          const meta = pickRecord(d, 'metadata');
          const iterationLength = meta ? pickNumber(meta, 'iteration_length') : undefined;
          const sessions = iterationSessionsRef.current;
          const prev = sessions.get(nodeId);
          const sess: IterationSession = {
            nodeId,
            title,
            inputs,
            iterationLength,
            rounds: prev?.rounds ?? [],
          };
          sessions.set(nodeId, sess);
          activeIterationRef.current = { nodeId, index: null };
          setNodeRunStatus(nodeId, 'running');
          setRunItems(items => {
            const exists = items.find(it => it.nodeId === nodeId);
            const base = {
              nodeId,
              title,
              nodeType: 'iteration',
              status: 'running' as const,
              iterationInputs: inputs,
              iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                index: r.index,
                nodes: sortWorkflowRunItems(r.nodes),
              })),
            };
            return exists
              ? items.map(it => (it.nodeId === nodeId ? { ...it, ...base } : it))
              : [...items, base as WorkflowRunNodeListItem];
          });
        } catch (_e) {
          //
        }
      },
      onIterationNext: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          if (!d) return;
          const nodeId = 'node_id' in d && typeof d.node_id === 'string' ? d.node_id : undefined;
          const index = 'index' in d && typeof d.index === 'number' ? (d.index as number) : null;
          if (!nodeId || index === null) return;
          const sessions = iterationSessionsRef.current;
          const sess = sessions.get(nodeId) ?? {
            nodeId,
            title: 'title' in d && typeof d.title === 'string' ? (d.title as string) : nodeId,
            rounds: [],
          };
          const hasRound = sess.rounds.some(r => r.index === index);
          if (!hasRound) {
            sess.rounds.push({ index, nodes: [] });
          }
          sess.activeIndex = index;
          sessions.set(nodeId, sess);
          activeIterationRef.current = { nodeId, index };
          setRunItems(items =>
            items.map(it =>
              it.nodeId === nodeId
                ? {
                    ...it,
                    nodeType: 'iteration',
                    status: 'running',
                    iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                      index: r.index,
                      nodes: sortWorkflowRunItems(r.nodes),
                    })),
                  }
                : it
            )
          );
        } catch (_e) {
          //
        }
      },
      onLoopStarted: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          if (!d) return;
          const nodeId = 'node_id' in d && typeof d.node_id === 'string' ? d.node_id : undefined;
          if (!nodeId) return;
          const title =
            'title' in d && typeof d.title === 'string'
              ? (d.title as string)
              : 'node_type' in d && typeof d.node_type === 'string'
                ? (d.node_type as string)
                : nodeId;
          const inputs = 'inputs' in d ? (d.inputs as unknown) : undefined;
          const meta = pickRecord(d, 'metadata');
          const loopLength = meta ? pickNumber(meta, 'loop_length') : undefined;
          const sessions = loopSessionsRef.current;
          const prev = sessions.get(nodeId);
          const sess: LoopSession = {
            nodeId,
            title,
            inputs,
            loopLength,
            rounds: prev?.rounds ?? [],
          };
          sessions.set(nodeId, sess);
          activeLoopRef.current = { nodeId, index: null };
          setNodeRunStatus(nodeId, 'running');
          setRunItems(items => {
            const exists = items.find(it => it.nodeId === nodeId);
            const base = {
              nodeId,
              title,
              nodeType: 'loop',
              status: 'running' as const,
              loopInputs: inputs,
              loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                index: r.index,
                nodes: sortWorkflowRunItems(r.nodes),
              })),
            };
            return exists
              ? items.map(it => (it.nodeId === nodeId ? { ...it, ...base } : it))
              : [...items, base as WorkflowRunNodeListItem];
          });
        } catch (_e) {
          //
        }
      },
      onLoopNext: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          if (!d) return;
          const nodeId = 'node_id' in d && typeof d.node_id === 'string' ? d.node_id : undefined;
          const index = 'index' in d && typeof d.index === 'number' ? (d.index as number) : null;
          if (!nodeId || index === null) return;
          const sessions = loopSessionsRef.current;
          const sess = sessions.get(nodeId) ?? {
            nodeId,
            title: 'title' in d && typeof d.title === 'string' ? (d.title as string) : nodeId,
            rounds: [],
          };
          const hasRound = sess.rounds.some(r => r.index === index);
          if (!hasRound) {
            sess.rounds.push({ index, nodes: [] });
          }
          sess.activeIndex = index;
          sessions.set(nodeId, sess);
          activeLoopRef.current = { nodeId, index };
          setRunItems(items =>
            items.map(it =>
              it.nodeId === nodeId
                ? {
                    ...it,
                    nodeType: 'loop',
                    status: 'running',
                    loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                      index: r.index,
                      nodes: sortWorkflowRunItems(r.nodes),
                    })),
                  }
                : it
            )
          );
        } catch (_e) {
          //
        }
      },
      onNodeStarted: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          if (!d) return;
          const id = 'node_id' in d && typeof d.node_id === 'string' ? d.node_id : undefined;
          if (!id) return;
          const title =
            'title' in d && typeof d.title === 'string'
              ? (d.title as string)
              : 'node_type' in d && typeof d.node_type === 'string'
                ? (d.node_type as string)
                : id;
          const nodeType =
            'node_type' in d && typeof d.node_type === 'string'
              ? (d.node_type as string)
              : 'unknown';
          setNodeRunStatus(id, 'running');
          setActiveOutputHandle(id, null);
          setCurrentRunningNodeId(id);
          try {
            autoFollow(id);
          } catch {
            /* no-op */
          }
          const containerContext = extractWorkflowRunContainerContext(d);
          const { loopId, loopIndex, iterationId, iterationIndex } = containerContext;
          const executionId = getWorkflowRunExecutionId(d);
          const createdAtMs = getWorkflowRunCreatedAtMs(d);
          const receivedOrder = nextReceiveOrder();
          if (loopId) {
            const session = loopSessionsRef.current.get(loopId);
            if (session) {
              const targetIndex =
                typeof loopIndex === 'number'
                  ? loopIndex
                  : (session.activeIndex ?? activeLoopRef.current.index);
              if (typeof targetIndex === 'number') {
                let round = session.rounds.find(r => r.index === targetIndex);
                if (!round) {
                  round = { index: targetIndex, nodes: [] };
                  session.rounds.push(round);
                }
                const nodeInput = 'inputs' in d ? (d.inputs as unknown) : undefined;
                const child = {
                  nodeId: id,
                  executionId,
                  createdAtMs,
                  receivedOrder,
                  title,
                  nodeType,
                  status: 'running' as const,
                  nodeInput,
                };
                const childKey = getWorkflowRunItemKey(child);
                const existingIndex = round.nodes.findIndex(
                  p => getWorkflowRunItemKey(p) === childKey
                );
                if (existingIndex >= 0) {
                  const existing = round.nodes[existingIndex];
                  round.nodes[existingIndex] = {
                    ...existing,
                    ...child,
                    createdAtMs: existing.createdAtMs ?? child.createdAtMs,
                    receivedOrder: existing.receivedOrder ?? child.receivedOrder,
                  };
                } else {
                  round.nodes.push(child);
                }
                session.activeIndex = targetIndex;
                loopSessionsRef.current.set(loopId, { ...session });
                activeLoopRef.current = { nodeId: loopId, index: targetIndex };
                setRunItems(items =>
                  items.map(it =>
                    it.nodeId === loopId
                      ? {
                          ...it,
                          loopRounds: sortWorkflowRunRounds(session.rounds).map(r => ({
                            index: r.index,
                            nodes: sortWorkflowRunItems(r.nodes),
                            elapsedTime: r.elapsedTime,
                          })),
                        }
                      : it
                  )
                );
                return;
              }
            }
          }
          const active = activeIterationRef.current;
          const targetIterationId = iterationId ?? active.nodeId;
          const targetIterationIndex =
            typeof iterationIndex === 'number' ? iterationIndex : active.index;
          if (targetIterationId !== null && targetIterationIndex !== null) {
            const session = iterationSessionsRef.current.get(targetIterationId);
            if (session) {
              let round = session.rounds.find(r => r.index === targetIterationIndex);
              if (!round) {
                round = { index: targetIterationIndex, nodes: [] };
                session.rounds.push(round);
              }
              if (round) {
                const nodeInput = 'inputs' in d ? (d.inputs as unknown) : undefined;
                const child = {
                  nodeId: id,
                  executionId,
                  createdAtMs,
                  receivedOrder,
                  title,
                  nodeType,
                  status: 'running' as const,
                  nodeInput,
                };
                const childKey = getWorkflowRunItemKey(child);
                const existingIndex = round.nodes.findIndex(
                  p => getWorkflowRunItemKey(p) === childKey
                );
                if (existingIndex >= 0) {
                  const existing = round.nodes[existingIndex];
                  round.nodes[existingIndex] = {
                    ...existing,
                    ...child,
                    createdAtMs: existing.createdAtMs ?? child.createdAtMs,
                    receivedOrder: existing.receivedOrder ?? child.receivedOrder,
                  };
                } else {
                  round.nodes.push(child);
                }
                session.activeIndex = targetIterationIndex;
                iterationSessionsRef.current.set(targetIterationId, { ...session });
                setRunItems(items =>
                  items.map(it =>
                    it.nodeId === targetIterationId
                      ? {
                          ...it,
                          iterationRounds: sortWorkflowRunRounds(session.rounds).map(r => ({
                            index: r.index,
                            nodes: sortWorkflowRunItems(r.nodes),
                          })),
                        }
                      : it
                  )
                );
                return;
              }
            }
          }
          const nodeInput = 'inputs' in d ? (d.inputs as unknown) : undefined;
          setRunItems((prev: WorkflowRunNodeListItem[]) => {
            const exists = prev.find(p => p.nodeId === id);
            if (exists) {
              return prev.map(p =>
                p.nodeId === id ? { ...p, status: 'running', title, nodeType, nodeInput } : p
              );
            }
            const item = {
              nodeId: id,
              executionId,
              createdAtMs,
              receivedOrder,
              title,
              nodeType,
              status: 'running',
              nodeInput,
            } as WorkflowRunNodeListItem;
            return [...prev, item];
          });
        } catch (e) {
          // eslint-disable-next-line no-console
          console.warn('onNodeStarted parse error', e);
        }
      },
      onNodeFinished: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          if (!d) return;
          const id =
            'node_id' in d && typeof d.node_id === 'string' ? (d.node_id as string) : undefined;
          if (!id) return;
          const title =
            'title' in d && typeof d.title === 'string'
              ? (d.title as string)
              : 'node_type' in d && typeof d.node_type === 'string'
                ? (d.node_type as string)
                : id;
          const nodeType =
            'node_type' in d && typeof d.node_type === 'string'
              ? (d.node_type as string)
              : 'unknown';
          const rawStatus =
            'status' in d && typeof d.status === 'string' ? (d.status as string) : undefined;
          const status: 'succeeded' | 'failed' | 'stopped' | 'paused' =
            rawStatus === 'succeeded' ||
            rawStatus === 'success' ||
            rawStatus === 'completed' ||
            rawStatus === 'partial-succeeded'
              ? 'succeeded'
              : rawStatus === 'paused'
                ? 'paused'
                : rawStatus === 'stopped'
                  ? 'stopped'
                  : 'failed'; // Covers 'exception', 'failed', and unknown states

          const elapsedTimeMs =
            'elapsed_time' in d && typeof d.elapsed_time === 'number'
              ? Math.max(0, d.elapsed_time as number)
              : 0;
          const nodeInput = 'inputs' in d ? (d.inputs as unknown) : undefined;
          const nodeOutput = 'outputs' in d ? (d.outputs as unknown) : undefined;
          const modelInput = extractLlmGatewayRequest(d);
          const processData = 'process_data' in d ? (d.process_data as unknown) : undefined;
          const executionMetadata =
            'execution_metadata' in d ? (d.execution_metadata as unknown) : undefined;
          const error = 'error' in d && typeof d.error === 'string' ? (d.error as string) : null;
          const outputHandle =
            'output_handle' in d && typeof d.output_handle === 'string'
              ? (d.output_handle as string)
              : '';

          // If status is exception/failed but no error message provided, we still mark it as failed
          // The UI should handle failed state gracefully even without error message
          setNodeRunStatus(id, status);
          setActiveOutputHandle(id, outputHandle.length > 0 ? outputHandle : null);
          if (status === 'failed' || status === 'stopped' || status === 'paused') {
            setCurrentRunningNodeId(null);
          }
          const containerContext = extractWorkflowRunContainerContext(d);
          const { loopId, loopIndex, iterationId, iterationIndex } = containerContext;
          const executionId = getWorkflowRunExecutionId(d);
          const createdAtMs = getWorkflowRunCreatedAtMs(d);
          const receivedOrder = nextReceiveOrder();
          if (loopId) {
            const session = loopSessionsRef.current.get(loopId);
            if (session) {
              const targetIndex =
                typeof loopIndex === 'number'
                  ? loopIndex
                  : (session.activeIndex ?? activeLoopRef.current.index);
              if (typeof targetIndex === 'number') {
                let round = session.rounds.find(r => r.index === targetIndex);
                if (!round) {
                  round = { index: targetIndex, nodes: [] };
                  session.rounds.push(round);
                }
                const child = {
                  nodeId: id,
                  executionId,
                  createdAtMs,
                  receivedOrder,
                  title,
                  nodeType,
                  status,
                  elapsedTime: elapsedTimeMs,
                  nodeInput,
                  nodeOutput,
                  modelInput,
                  processData,
                  executionMetadata,
                  error,
                };
                const childKey = getWorkflowRunItemKey(child);
                const existingIndex = round.nodes.findIndex(
                  p => getWorkflowRunItemKey(p) === childKey || p.nodeId === id
                );
                if (existingIndex >= 0) {
                  const existing = round.nodes[existingIndex];
                  round.nodes[existingIndex] = {
                    ...existing,
                    ...child,
                    createdAtMs: existing.createdAtMs ?? child.createdAtMs,
                    receivedOrder: existing.receivedOrder ?? child.receivedOrder,
                  };
                } else {
                  round.nodes.push(child);
                }
                session.activeIndex = targetIndex;
                loopSessionsRef.current.set(loopId, { ...session });
                activeLoopRef.current = { nodeId: loopId, index: targetIndex };
                setRunItems(items =>
                  items.map(it =>
                    it.nodeId === loopId
                      ? {
                          ...it,
                          loopRounds: sortWorkflowRunRounds(session.rounds).map(r => ({
                            index: r.index,
                            nodes: sortWorkflowRunItems(r.nodes),
                            elapsedTime: r.elapsedTime,
                          })),
                        }
                      : it
                  )
                );
                return;
              }
            }
          }
          const active = activeIterationRef.current;
          const targetIterationId = iterationId ?? active.nodeId;
          const targetIterationIndex =
            typeof iterationIndex === 'number' ? iterationIndex : active.index;
          if (targetIterationId !== null && targetIterationIndex !== null) {
            const session = iterationSessionsRef.current.get(targetIterationId);
            if (session) {
              let round = session.rounds.find(r => r.index === targetIterationIndex);
              if (!round) {
                round = { index: targetIterationIndex, nodes: [] };
                session.rounds.push(round);
              }
              if (round) {
                const child = {
                  nodeId: id,
                  executionId,
                  createdAtMs,
                  receivedOrder,
                  title,
                  nodeType,
                  status,
                  elapsedTime: elapsedTimeMs,
                  nodeInput,
                  nodeOutput,
                  modelInput,
                  processData,
                  executionMetadata,
                  error,
                };
                const childKey = getWorkflowRunItemKey(child);
                const exists = round.nodes.some(p => getWorkflowRunItemKey(p) === childKey);
                if (exists) {
                  round.nodes = round.nodes.map(p =>
                    getWorkflowRunItemKey(p) === childKey
                      ? {
                          ...p,
                          ...child,
                          createdAtMs: p.createdAtMs ?? child.createdAtMs,
                          receivedOrder: p.receivedOrder ?? child.receivedOrder,
                        }
                      : p
                  );
                } else {
                  round.nodes.push(child);
                }
                session.activeIndex = targetIterationIndex;
                iterationSessionsRef.current.set(targetIterationId, { ...session });
                setRunItems(items =>
                  items.map(it =>
                    it.nodeId === targetIterationId
                      ? {
                          ...it,
                          iterationRounds: sortWorkflowRunRounds(session.rounds).map(r => ({
                            index: r.index,
                            nodes: sortWorkflowRunItems(r.nodes),
                          })),
                        }
                      : it
                  )
                );
                return;
              }
            }
          }
          setRunItems(prev => {
            const exists = prev.find(p => p.nodeId === id);
            if (exists) {
              return prev.map(p =>
                p.nodeId === id
                  ? {
                      ...p,
                      title,
                      nodeType,
                      status,
                      elapsedTime: elapsedTimeMs,
                      nodeInput,
                      nodeOutput,
                      modelInput,
                      processData,
                      executionMetadata,
                      error,
                    }
                  : p
              );
            }
            const item = {
              nodeId: id,
              title,
              nodeType,
              status,
              executionId,
              createdAtMs,
              receivedOrder,
              elapsedTime: elapsedTimeMs,
              nodeInput,
              nodeOutput,
              modelInput,
              processData,
              executionMetadata,
              error,
            };
            return [...prev, item];
          });
        } catch (e) {
          // eslint-disable-next-line no-console
          console.warn('onNodeFinished parse error', e);
        }
      },
      onIterationCompleted: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          if (!d) return;
          const nodeId =
            'node_id' in d && typeof d.node_id === 'string' ? (d.node_id as string) : undefined;
          if (!nodeId) return;
          const elapsedTimeMs =
            'elapsed_time' in d && typeof d.elapsed_time === 'number'
              ? Math.max(0, d.elapsed_time as number)
              : 0;
          const error = 'error' in d && typeof d.error === 'string' ? (d.error as string) : null;
          const outputs = 'outputs' in d ? (d.outputs as unknown) : undefined;
          const steps =
            'steps' in d && typeof d.steps === 'number' ? (d.steps as number) : undefined;
          const sessions = iterationSessionsRef.current;
          const sess = sessions.get(nodeId);
          if (sess) {
            const roundDurations = getWorkflowRunRoundDurationMap(d, 'iteration');
            sess.elapsedTime = elapsedTimeMs;
            sess.error = error;
            sess.outputs = outputs;
            sess.rounds = sess.rounds.map(r => ({
              ...r,
              elapsedTime: roundDurations.get(r.index) ?? getWorkflowRunRoundElapsedTime(r),
            }));
            sessions.set(nodeId, sess);
          }
          setNodeRunStatus(nodeId, error ? 'failed' : 'succeeded');
          activeIterationRef.current = { nodeId: null, index: null };
          setRunItems(items =>
            items.map(it =>
              it.nodeId === nodeId
                ? {
                    ...it,
                    status: error ? 'failed' : 'succeeded',
                    elapsedTime: elapsedTimeMs,
                    iterationOutputs: outputs,
                    steps,
                    iterationRounds: sortWorkflowRunRounds(sess?.rounds || []).map(r => ({
                      index: r.index,
                      nodes: sortWorkflowRunItems(r.nodes),
                      elapsedTime: r.elapsedTime,
                    })),
                  }
                : it
            )
          );
        } catch (_e) {
          //
        }
      },
      onLoopCompleted: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          if (!d) return;
          const nodeId =
            'node_id' in d && typeof d.node_id === 'string' ? (d.node_id as string) : undefined;
          if (!nodeId) return;
          const elapsedTimeMs =
            'elapsed_time' in d && typeof d.elapsed_time === 'number'
              ? Math.max(0, d.elapsed_time as number)
              : 0;
          const statusRaw = pickString(d, 'status');
          const status: 'succeeded' | 'failed' | 'stopped' =
            statusRaw === 'succeeded' ||
            statusRaw === 'success' ||
            statusRaw === 'completed' ||
            statusRaw === 'partial-succeeded'
              ? 'succeeded'
              : statusRaw === 'stopped'
                ? 'stopped'
                : 'failed';
          const error = 'error' in d && typeof d.error === 'string' ? (d.error as string) : null;
          const outputs = 'outputs' in d ? (d.outputs as unknown) : undefined;
          const steps =
            'steps' in d && typeof d.steps === 'number' ? (d.steps as number) : undefined;
          const execMeta = pickRecord(d, 'execution_metadata');
          const variableMap =
            execMeta && isRecord(execMeta.loop_variable_map)
              ? (execMeta.loop_variable_map as RecordLike)
              : undefined;
          const roundDurations = getWorkflowRunRoundDurationMap(d, 'loop');
          const sessions = loopSessionsRef.current;
          const sess = sessions.get(nodeId);
          if (sess) {
            sess.elapsedTime = elapsedTimeMs;
            sess.error = error;
            sess.outputs = outputs;
            sess.steps = steps;
            sess.rounds = sess.rounds.map(r => {
              const variables = variableMap?.[String(r.index)];
              return {
                ...r,
                elapsedTime: roundDurations.get(r.index) ?? getWorkflowRunRoundElapsedTime(r),
                variables: variables ?? r.variables,
              };
            });
            sessions.set(nodeId, sess);
          }
          setNodeRunStatus(nodeId, status);
          activeLoopRef.current = { nodeId: null, index: null };
          setRunItems(items =>
            items.map(it =>
              it.nodeId === nodeId
                ? {
                    ...it,
                    status,
                    elapsedTime: elapsedTimeMs,
                    loopOutputs: outputs,
                    steps,
                    loopRounds: sortWorkflowRunRounds(sess?.rounds || []).map(r => ({
                      index: r.index,
                      nodes: sortWorkflowRunItems(r.nodes),
                      elapsedTime: r.elapsedTime,
                      variables: r.variables,
                    })),
                  }
                : it
            )
          );
        } catch (_e) {
          //
        }
      },
      onWorkflowFinished: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          if (!d) return;
          const tokenUsage = pickRecord(d, 'token_usage');
          const errorObj = pickRecord(d, 'error');
          setRunSummary(prev => {
            const startAt = prev?.created_at; // preserve start time strictly from workflow_started
            return {
              ...prev,
              // Explicitly keep created_at from start event; do not override with any end payload
              created_at: startAt,
              status: pickString(d, 'status') ?? 'unknown',
              finished_at: pickNumber(d, 'finished_at'),
              elapsed_time: pickNumber(d, 'elapsed_time'),
              total_steps: pickNumber(d, 'total_steps'),
              outputs: 'outputs' in d ? (d.outputs as unknown) : undefined,
              total_tokens:
                pickNumber(d, 'total_tokens') ??
                (tokenUsage ? pickNumber(tokenUsage, 'total_tokens') : undefined),
              error: (errorObj as unknown) ?? ('error' in d ? d.error : undefined),
            } as WorkflowFinishedData;
          });
          const outputs = 'outputs' in d ? (d.outputs as unknown) : undefined;
          let result: HistoryResult | null = null;
          if (outputs === null || outputs === undefined) {
            result = { kind: 'empty' };
          } else if (typeof outputs === 'string') {
            result = { kind: 'text', content: outputs };
          } else if (typeof outputs === 'number' || typeof outputs === 'boolean') {
            result = { kind: 'text', content: String(outputs) };
          } else if (typeof outputs === 'object') {
            const rec = outputs as Record<string, unknown>;
            const keys = Object.keys(rec);
            if (keys.length === 1) {
              const v = rec[keys[0]];
              if (typeof v === 'string' || typeof v === 'number') {
                result = { kind: 'text', content: String(v) };
              } else if (Array.isArray(v) && (v as unknown[]).every(x => typeof x === 'string')) {
                result = { kind: 'text', content: (v as string[]).join('\n\n') };
              } else {
                result = { kind: 'json', value: outputs };
              }
            } else {
              result = { kind: 'json', value: outputs };
            }
          } else {
            result = { kind: 'empty' };
          }
          setFinalResult(result);
          setAutoFollow(false);
          setCurrentRunningNodeId(null);
          finalizeRuntimeLogPopoversAfterRun();
          throttler.flush();
        } catch (e) {
          // eslint-disable-next-line no-console
          console.warn('onWorkflowFinished parse error', e);
        }
      },
      onError: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          if (!d) return;
          const id =
            'node_id' in d && typeof d.node_id === 'string' ? (d.node_id as string) : undefined;
          if (!id) return;
          const title =
            'title' in d && typeof d.title === 'string'
              ? (d.title as string)
              : 'node_type' in d && typeof d.node_type === 'string'
                ? (d.node_type as string)
                : id;
          const nodeType =
            'node_type' in d && typeof d.node_type === 'string'
              ? (d.node_type as string)
              : 'unknown';
          const errorMsg = getWorkflowRunErrorText('error' in d ? d.error : undefined) ?? 'Error';
          setNodeRunStatus(id, 'failed');
          setRunItems((prev: WorkflowRunNodeListItem[]) => {
            const exists = prev.find(p => p.nodeId === id);
            if (exists) {
              return prev.map(p =>
                p.nodeId === id ? { ...p, title, nodeType, status: 'failed', error: errorMsg } : p
              );
            }
            const item = {
              nodeId: id,
              title,
              nodeType,
              status: 'failed',
              error: errorMsg,
            } as WorkflowRunNodeListItem;
            return [...prev, item];
          });
          throttler.flush();
        } catch (e) {
          // eslint-disable-next-line no-console
          console.warn('onError parse error', e);
        }
      },
      onTextReplace: (payload: unknown) => {
        throttler.cancel();
        const text = getSensitiveOutputTextFromPayload(payload) ?? SENSITIVE_OUTPUT_BLOCKED_TOKEN;
        setStreamedText(text);
        setFinalResult({ kind: 'text', content: text });
        setActiveTab('results');
      },
      onTextChunk: (payload: unknown) => {
        try {
          const d = getSseData(payload);
          const text = pickString(d, 'text');
          if (typeof text === 'string' && text.length > 0) {
            throttler.append(text);
          }
        } catch (e) {
          // eslint-disable-next-line no-console
          console.warn('onTextChunk parse error', e);
        }
      },
      onMessage: (payload: unknown) => {
        try {
          const text = getSensitiveOutputTextFromPayload(payload);
          if (typeof text === 'string' && text.length > 0) {
            throttler.append(text);
          }
        } catch (e) {
          // eslint-disable-next-line no-console
          console.warn('onMessage parse error', e);
        }
      },
      onMessageEnd: () => {
        throttler.flush();
      },
    }),
    [
      viewport,
      nodes,
      edges,
      setHistorySnapshot,
      resetRunStatus,
      setNodeRunStatus,
      setActiveOutputHandle,
      resetActiveOutputHandles,
      beginRuntimeLogPopoverAutoOpen,
      finalizeRuntimeLogPopoversAfterRun,
      setAutoFollow,
      setCurrentRunningNodeId,
      lastInputs,
      setRunItems,
      setStreamedText,
      setRunSummary,
      setActiveTab,
      setFinalResult,
      autoFollow,
      getSseData,
      throttler,
      nextReceiveOrder,
      isRecord,
      pickNumber,
      pickString,
      pickRecord,
      getWorkflowRunErrorText,
    ]
  );
}
