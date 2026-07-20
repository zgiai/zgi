import type { WorkflowRunSseCallbacks } from '@/services/workflow.service';
import { normalizeWorkflowRuntimeEvent } from '../../utils/workflow/runtime-event-envelope.js';

export type WorkflowRuntimeStatus =
  | 'idle'
  | 'running'
  | 'pending_approval'
  | 'pending_question'
  | 'stopping';

export type WorkflowConnectionState = 'idle' | 'connected' | 'reconnecting' | 'disconnected';

export interface WorkflowRunSessionState {
  workflowRunId: string;
  conversationId: string | null;
  runtimeStatus: WorkflowRuntimeStatus;
  connectionState: WorkflowConnectionState;
  cursor: number;
  projectionRevision: number;
  generation: number;
  activePause: Record<string, unknown> | null;
}

export interface WorkflowConversationRuntimeState extends WorkflowRunSessionState {
  conversationId: string;
}

type WorkflowRunSessionAction =
  | { type: 'snapshot'; payload: unknown }
  | { type: 'event'; event: string; payload: unknown; sequence?: number }
  | { type: 'connection'; state: WorkflowConnectionState }
  | { type: 'generation'; generation: number }
  | { type: 'attach'; conversationId?: string | null };

function recordOf(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : {};
}

function payloadOf(value: unknown): Record<string, unknown> {
  return normalizeWorkflowRuntimeEvent(value).payload;
}

function sequenceOf(value: unknown, fallback = 0): number {
  const sequence = normalizeWorkflowRuntimeEvent(value).sequence || fallback;
  return Number.isFinite(sequence) && sequence > 0 ? sequence : 0;
}

function normalizeRuntimeStatus(value: unknown): WorkflowRuntimeStatus {
  const status = typeof value === 'string' ? value.toLowerCase() : '';
  if (status === 'pending_approval' || status === 'waiting_approval' || status === 'paused') {
    return 'pending_approval';
  }
  if (status === 'pending_question' || status === 'waiting_question') {
    return 'pending_question';
  }
  if (status === 'running' || status === 'resuming' || status === 'pending') {
    return 'running';
  }
  if (status === 'stopping') return 'stopping';
  return 'idle';
}

function pauseRuntimeStatus(payload: unknown): WorkflowRuntimeStatus {
  const data = payloadOf(payload);
  const values: string[] = [];
  const collect = (value: unknown) => {
    if (typeof value === 'string') values.push(value.toLowerCase());
    if (Array.isArray(value)) value.forEach(collect);
    if (value && typeof value === 'object') Object.values(value).forEach(collect);
  };
  collect(data.reason);
  collect(data.reasons);
  collect(data.state);
  collect(data.type);
  return values.some(value => value.includes('question')) ? 'pending_question' : 'pending_approval';
}

function isPendingPauseReason(value: unknown) {
  const reason = recordOf(value);
  const status = typeof reason.status === 'string' ? reason.status.toLowerCase() : '';
  return !status || status === 'pending' || status === 'waiting';
}

function getActionableSnapshotPause(value: unknown): Record<string, unknown> | null {
  const activePause = recordOf(value);
  if (Object.keys(activePause).length === 0) return null;
  const pause = recordOf(activePause.pause);
  const status = typeof pause.status === 'string' ? pause.status.toLowerCase() : '';
  if (status && !['paused', 'pending', 'waiting'].includes(status)) return null;
  const reasons = Array.isArray(activePause.reasons)
    ? activePause.reasons.filter(isPendingPauseReason)
    : [];
  return reasons.length > 0 ? { ...activePause, reasons } : null;
}

export function createWorkflowRunSessionState(
  workflowRunId: string,
  conversationId: string | null = null
): WorkflowRunSessionState {
  return {
    workflowRunId,
    conversationId,
    runtimeStatus: 'running',
    connectionState: 'idle',
    cursor: 0,
    projectionRevision: 0,
    generation: 0,
    activePause: null,
  };
}

export function reduceWorkflowRunSessionState(
  state: WorkflowRunSessionState,
  action: WorkflowRunSessionAction
): WorkflowRunSessionState {
  if (action.type === 'connection') {
    return { ...state, connectionState: action.state };
  }
  if (action.type === 'generation') {
    return { ...state, generation: action.generation };
  }
  if (action.type === 'attach') {
    return { ...state, conversationId: action.conversationId ?? state.conversationId };
  }
  if (action.type === 'snapshot') {
    const snapshot = payloadOf(action.payload);
    const workflowRun = recordOf(snapshot.workflow_run);
    const message = recordOf(snapshot.message);
    const workflowStatus =
      typeof workflowRun.status === 'string' ? workflowRun.status.toLowerCase() : '';
    const isTerminal = [
      'succeeded',
      'success',
      'completed',
      'failed',
      'error',
      'stopped',
      'expired',
    ].includes(workflowStatus);
    const activePause = isTerminal ? null : getActionableSnapshotPause(snapshot.active_pause);
    const runtimeStatus = activePause
      ? pauseRuntimeStatus(activePause)
      : normalizeRuntimeStatus(workflowStatus);
    return {
      ...state,
      runtimeStatus,
      activePause,
      cursor: Math.max(
        state.cursor,
        typeof snapshot.last_sequence === 'number' ? snapshot.last_sequence : 0
      ),
      projectionRevision:
        typeof message.projection_revision === 'number'
          ? message.projection_revision
          : state.projectionRevision,
    };
  }

  const sequence = sequenceOf(action.payload, action.sequence);
  if (sequence > 0 && sequence <= state.cursor) return state;
  const next = { ...state, cursor: Math.max(state.cursor, sequence) };
  switch (action.event) {
    case 'workflow_started':
    case 'workflow_resumed':
      return {
        ...next,
        runtimeStatus: 'running',
        activePause: null,
      };
    case 'workflow_paused':
      return {
        ...next,
        runtimeStatus: pauseRuntimeStatus(action.payload),
        activePause: payloadOf(action.payload),
      };
    case 'workflow_stopping':
      return { ...next, runtimeStatus: 'stopping' };
    case 'workflow_finished':
    case 'workflow_succeeded':
    case 'workflow_completed':
    case 'workflow_failed':
    case 'workflow_stopped':
      return {
        ...next,
        runtimeStatus: 'idle',
        connectionState: 'idle',
        activePause: null,
      };
    default:
      return next;
  }
}

export class WorkflowRunEventSession {
  private state: WorkflowRunSessionState;
  private listeners = new Set<(state: WorkflowRunSessionState) => void>();

  constructor(workflowRunId: string, conversationId: string | null = null) {
    this.state = createWorkflowRunSessionState(workflowRunId, conversationId);
  }

  getState(): WorkflowRunSessionState {
    return this.state;
  }

  subscribe(listener: (state: WorkflowRunSessionState) => void): () => void {
    this.listeners.add(listener);
    listener(this.state);
    return () => this.listeners.delete(listener);
  }

  dispatch(action: WorkflowRunSessionAction): WorkflowRunSessionState {
    const next = reduceWorkflowRunSessionState(this.state, action);
    if (next !== this.state) {
      this.state = next;
      this.listeners.forEach(listener => listener(next));
    }
    return this.state;
  }

  nextGeneration(): number {
    const generation = this.state.generation + 1;
    this.dispatch({ type: 'generation', generation });
    return generation;
  }

  isCurrentGeneration(generation: number): boolean {
    return this.state.generation === generation;
  }
}

interface WorkflowConversationRuntimeControllerOptions {
  stopRun?: (workflowRunId: string) => Promise<void>;
}

export class WorkflowConversationRuntimeController {
  private readonly sessionsByRunId = new Map<string, WorkflowRunEventSession>();
  private readonly runIdByConversationId = new Map<string, string>();
  private readonly options: WorkflowConversationRuntimeControllerOptions;

  constructor(options: WorkflowConversationRuntimeControllerOptions = {}) {
    this.options = options;
  }

  attachRun(conversationId: string, workflowRunId: string): WorkflowRunEventSession {
    let session = this.sessionsByRunId.get(workflowRunId);
    if (!session) {
      session = new WorkflowRunEventSession(workflowRunId, conversationId);
      this.sessionsByRunId.set(workflowRunId, session);
    } else {
      session.dispatch({ type: 'attach', conversationId });
    }
    this.runIdByConversationId.set(conversationId, workflowRunId);
    return session;
  }

  recoverRun(conversationId: string, workflowRunId: string): WorkflowRunEventSession {
    return this.attachRun(conversationId, workflowRunId);
  }

  getConversationState(conversationId: string): WorkflowConversationRuntimeState | null {
    const workflowRunId = this.runIdByConversationId.get(conversationId);
    const session = workflowRunId ? this.sessionsByRunId.get(workflowRunId) : null;
    if (!session) return null;
    return { ...session.getState(), conversationId };
  }

  async stopRun(conversationId: string): Promise<void> {
    const workflowRunId = this.runIdByConversationId.get(conversationId);
    const session = workflowRunId ? this.sessionsByRunId.get(workflowRunId) : null;
    if (!workflowRunId || !session || !this.options.stopRun) return;
    await this.options.stopRun(workflowRunId);
    session.dispatch({ type: 'event', event: 'workflow_stopping', payload: {} });
  }

  detachForeground(_conversationId: string): void {
    // Foreground selection does not own the stream. Background sessions stay connected.
  }

  disposeConversation(conversationId: string): void {
    const workflowRunId = this.runIdByConversationId.get(conversationId);
    this.runIdByConversationId.delete(conversationId);
    if (workflowRunId) this.sessionsByRunId.delete(workflowRunId);
  }
}

export function wrapWorkflowRunCallbacksWithSession(
  session: WorkflowRunEventSession,
  callbacks: WorkflowRunSseCallbacks
): WorkflowRunSseCallbacks {
  const apply = (
    name: string,
    handler?: (payload: unknown) => void,
    updateRuntime = false
  ) => (payload: unknown) => {
    const sequence = sequenceOf(payload);
    if (sequence > 0 && sequence <= session.getState().cursor) return;
    if (sequence > 0 || updateRuntime) {
      session.dispatch({ type: 'event', event: name, payload });
    }
    handler?.(payload);
  };
  return {
    ...callbacks,
    onWorkflowSnapshot: payload => {
      session.dispatch({ type: 'snapshot', payload });
      callbacks.onWorkflowSnapshot?.(payload);
    },
    onWorkflowStarted: apply('workflow_started', callbacks.onWorkflowStarted, true),
    onWorkflowResumed: apply('workflow_resumed', callbacks.onWorkflowResumed, true),
    onWorkflowPaused: apply('workflow_paused', callbacks.onWorkflowPaused, true),
    onApprovalRequested: apply('approval_requested', callbacks.onApprovalRequested),
    onApprovalResultFilled: apply('approval_result_filled', callbacks.onApprovalResultFilled),
    onApprovalExpired: apply('approval_expired', callbacks.onApprovalExpired),
    onQuestionAnswerRequested: apply(
      'question_answer_requested',
      callbacks.onQuestionAnswerRequested
    ),
    onQuestionAnswerSubmitted: apply(
      'question_answer_submitted',
      callbacks.onQuestionAnswerSubmitted
    ),
    onWorkflowFinished: apply('workflow_finished', callbacks.onWorkflowFinished, true),
    onError: apply('error', callbacks.onError),
    onNodeStarted: apply('node_started', callbacks.onNodeStarted),
    onNodeFinished: apply('node_finished', callbacks.onNodeFinished),
    onNodeRetry: apply('node_retry', callbacks.onNodeRetry),
    onAgentLog: apply('agent_log', callbacks.onAgentLog),
    onTextChunk: apply('text_chunk', callbacks.onTextChunk),
    onTextReplace: apply('text_replace', callbacks.onTextReplace),
    onMessage: apply('message', callbacks.onMessage),
    onMessageEnd: apply('message_end', callbacks.onMessageEnd),
    onIterationStarted: apply('iteration_started', callbacks.onIterationStarted),
    onIterationNext: apply('iteration_next', callbacks.onIterationNext),
    onIterationCompleted: apply('iteration_completed', callbacks.onIterationCompleted),
    onLoopStarted: apply('loop_started', callbacks.onLoopStarted),
    onLoopNext: apply('loop_next', callbacks.onLoopNext),
    onLoopCompleted: apply('loop_completed', callbacks.onLoopCompleted),
    onEventCursor: sequence => {
      session.dispatch({ type: 'event', event: 'cursor', payload: {}, sequence });
      callbacks.onEventCursor?.(sequence);
    },
  };
}
