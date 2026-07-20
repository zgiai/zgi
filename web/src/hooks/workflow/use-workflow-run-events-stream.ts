'use client';

import { useCallback, useEffect, useRef } from 'react';

import { workflowService, type WorkflowRunSseCallbacks } from '@/services/workflow.service';
import {
  WorkflowRunEventSession,
  type WorkflowConnectionState,
  wrapWorkflowRunCallbacksWithSession,
} from './workflow-runtime-controller';

export interface WorkflowRunEventsStreamParams {
  after?: number;
  include_snapshot?: boolean;
  continue_on_pause?: boolean;
}

export interface WorkflowRunEventsStreamOptions {
  onClose?: () => void;
  onConnectionStateChange?: (state: WorkflowConnectionState) => void;
  onReconnectExhausted?: () => void;
  autoReconnect?: boolean;
}

export interface UseWorkflowRunEventsStreamReturn {
  start: (
    workflowRunId: string,
    callbacks: WorkflowRunSseCallbacks,
    params?: WorkflowRunEventsStreamParams,
    options?: WorkflowRunEventsStreamOptions
  ) => Promise<void>;
  cancel: (workflowRunId?: string) => void;
  reconnect: (workflowRunId: string) => Promise<void>;
  rememberEventCursor: (workflowRunId: string, sequence: number) => void;
  getEventCursor: (workflowRunId: string) => number;
  getConnectionState: (workflowRunId: string) => WorkflowConnectionState;
  resetEventCursor: (workflowRunId?: string) => void;
}

const WORKFLOW_RECONNECT_DELAYS_MS = [800, 1600, 3200] as const;

interface ActiveRunStream {
  handle: { close: () => void } | null;
  timer: ReturnType<typeof setTimeout> | null;
  generation: number;
  callbacks: WorkflowRunSseCallbacks;
  params?: WorkflowRunEventsStreamParams;
  options?: WorkflowRunEventsStreamOptions;
}

/**
 * Streams persisted workflow events. Each run owns an independent connection, so selecting or
 * starting another conversation cannot cancel a background workflow run.
 */
export function useWorkflowRunEventsStream(): UseWorkflowRunEventsStreamReturn {
  const activeByRunRef = useRef<Record<string, ActiveRunStream>>({});
  const sessionByRunRef = useRef<Record<string, WorkflowRunEventSession>>({});
  const cursorByRunRef = useRef<Record<string, number>>({});
  const projectionRevisionByRunRef = useRef<Record<string, number>>({});
  const startRef = useRef<UseWorkflowRunEventsStreamReturn['start'] | null>(null);
  const connectRef = useRef<
    ((workflowRunId: string, generation: number, retryIndex: number) => Promise<void>) | null
  >(null);

  const getSession = useCallback((workflowRunId: string) => {
    const existing = sessionByRunRef.current[workflowRunId];
    if (existing) return existing;
    const session = new WorkflowRunEventSession(workflowRunId);
    sessionByRunRef.current[workflowRunId] = session;
    return session;
  }, []);

  const setConnectionState = useCallback(
    (workflowRunId: string, state: WorkflowConnectionState) => {
      getSession(workflowRunId).dispatch({ type: 'connection', state });
      activeByRunRef.current[workflowRunId]?.options?.onConnectionStateChange?.(state);
    },
    [getSession]
  );

  const rememberEventCursor = useCallback(
    (workflowRunId: string, sequence: number) => {
      if (!workflowRunId || !Number.isFinite(sequence) || sequence <= 0) return;
      cursorByRunRef.current[workflowRunId] = Math.max(
        cursorByRunRef.current[workflowRunId] ?? 0,
        sequence
      );
      getSession(workflowRunId).dispatch({
        type: 'event',
        event: 'cursor',
        payload: {},
        sequence,
      });
    },
    [getSession]
  );

  const getEventCursor = useCallback(
    (workflowRunId: string) => cursorByRunRef.current[workflowRunId] ?? 0,
    []
  );

  const getConnectionState = useCallback(
    (workflowRunId: string) =>
      sessionByRunRef.current[workflowRunId]?.getState().connectionState ?? 'idle',
    []
  );

  const resetEventCursor = useCallback((workflowRunId?: string) => {
    if (workflowRunId) {
      delete cursorByRunRef.current[workflowRunId];
      delete projectionRevisionByRunRef.current[workflowRunId];
      return;
    }
    cursorByRunRef.current = {};
    projectionRevisionByRunRef.current = {};
  }, []);

  const cancel = useCallback(
    (workflowRunId?: string) => {
      const runIds = workflowRunId ? [workflowRunId] : Object.keys(activeByRunRef.current);
      runIds.forEach(runId => {
        const active = activeByRunRef.current[runId];
        if (!active) return;
        if (active.timer) clearTimeout(active.timer);
        active.handle?.close();
        delete activeByRunRef.current[runId];
        getSession(runId).nextGeneration();
        setConnectionState(runId, 'idle');
      });
    },
    [getSession, setConnectionState]
  );

  const connect = useCallback(
    async (workflowRunId: string, generation: number, retryIndex: number) => {
      const active = activeByRunRef.current[workflowRunId];
      const session = getSession(workflowRunId);
      if (!active || !session.isCurrentGeneration(generation)) return;

      const rememberedCursor = cursorByRunRef.current[workflowRunId] ?? 0;
      const requestedCursor = active.params?.after ?? 0;
      const forceSnapshot = active.params?.include_snapshot === true;
      const effectiveCursor = forceSnapshot
        ? requestedCursor
        : Math.max(rememberedCursor, requestedCursor);
      const effectiveParams =
        !forceSnapshot && effectiveCursor > 0
          ? { ...active.params, after: effectiveCursor, include_snapshot: false }
          : active.params;

      const callbacks = wrapWorkflowRunCallbacksWithSession(session, active.callbacks);
      const wrappedCallbacks: WorkflowRunSseCallbacks = {
        ...callbacks,
        onWorkflowSnapshot: payload => {
          const envelope =
            payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
          const snapshot =
            envelope.data && typeof envelope.data === 'object'
              ? (envelope.data as Record<string, unknown>)
              : envelope;
          const message =
            snapshot.message && typeof snapshot.message === 'object'
              ? (snapshot.message as Record<string, unknown>)
              : null;
          projectionRevisionByRunRef.current[workflowRunId] =
            typeof message?.projection_revision === 'number' ? message.projection_revision : 0;
          callbacks.onWorkflowSnapshot?.(payload);
        },
        onMessage: payload => {
          const envelope =
            payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
          const data =
            envelope.data && typeof envelope.data === 'object'
              ? (envelope.data as Record<string, unknown>)
              : envelope;
          const revision = typeof data.answer_revision === 'number' ? data.answer_revision : null;
          if (revision !== null) {
            const current = projectionRevisionByRunRef.current[workflowRunId] ?? 0;
            if (revision <= current) return;
            // projection_revision also advances for pause/resume/status writes.
            // Event sequence is the gap detector; answer revisions only reject
            // stale/replayed checkpoints and therefore need not be contiguous.
            projectionRevisionByRunRef.current[workflowRunId] = revision;
          }
          callbacks.onMessage?.(payload);
        },
        onEventCursor: sequence => {
          rememberEventCursor(workflowRunId, sequence);
          active.callbacks.onEventCursor?.(sequence);
        },
      };

      if (retryIndex > 0) setConnectionState(workflowRunId, 'reconnecting');
      try {
        const handle = await workflowService.sseWorkflowRunEvents(workflowRunId, wrappedCallbacks, {
          params: effectiveParams,
          onClose: () => {
            const current = activeByRunRef.current[workflowRunId];
            if (!current || !session.isCurrentGeneration(generation)) return;
            current.handle = null;
            const terminal = session.getState().runtimeStatus === 'idle';
            if (terminal) {
              setConnectionState(workflowRunId, 'idle');
              current.options?.onClose?.();
              return;
            }
            const autoReconnect = current.options?.autoReconnect !== false;
            if (autoReconnect && retryIndex < WORKFLOW_RECONNECT_DELAYS_MS.length) {
              setConnectionState(workflowRunId, 'reconnecting');
              current.timer = setTimeout(() => {
                current.timer = null;
                void connectRef.current?.(workflowRunId, generation, retryIndex + 1);
              }, WORKFLOW_RECONNECT_DELAYS_MS[retryIndex]);
              return;
            }
            setConnectionState(workflowRunId, 'disconnected');
            current.options?.onReconnectExhausted?.();
            current.options?.onClose?.();
          },
        });
        const current = activeByRunRef.current[workflowRunId];
        if (!current || !session.isCurrentGeneration(generation)) {
          handle.close();
          return;
        }
        current.handle = handle;
        setConnectionState(workflowRunId, 'connected');
      } catch {
        const current = activeByRunRef.current[workflowRunId];
        if (!current || !session.isCurrentGeneration(generation)) return;
        if (retryIndex < WORKFLOW_RECONNECT_DELAYS_MS.length) {
          setConnectionState(workflowRunId, 'reconnecting');
          current.timer = setTimeout(() => {
            current.timer = null;
            void connectRef.current?.(workflowRunId, generation, retryIndex + 1);
          }, WORKFLOW_RECONNECT_DELAYS_MS[retryIndex]);
          return;
        }
        setConnectionState(workflowRunId, 'disconnected');
        current.options?.onReconnectExhausted?.();
        current.options?.onClose?.();
      }
    },
    [cancel, getSession, rememberEventCursor, setConnectionState]
  );
  connectRef.current = connect;

  const start = useCallback<UseWorkflowRunEventsStreamReturn['start']>(
    async (workflowRunId, callbacks, params, options) => {
      const existing = activeByRunRef.current[workflowRunId];
      if (existing?.timer) clearTimeout(existing.timer);
      existing?.handle?.close();

      const session = getSession(workflowRunId);
      const generation = session.nextGeneration();
      activeByRunRef.current[workflowRunId] = {
        handle: null,
        timer: null,
        generation,
        callbacks,
        params,
        options,
      };
      setConnectionState(workflowRunId, 'reconnecting');
      await connect(workflowRunId, generation, 0);
    },
    [connect, getSession, setConnectionState]
  );
  startRef.current = start;

  const reconnect = useCallback(
    async (workflowRunId: string) => {
      const active = activeByRunRef.current[workflowRunId];
      if (!active) return;
      await start(workflowRunId, active.callbacks, active.params, active.options);
    },
    [start]
  );

  useEffect(() => () => cancel(), [cancel]);

  return {
    start,
    cancel,
    reconnect,
    rememberEventCursor,
    getEventCursor,
    getConnectionState,
    resetEventCursor,
  };
}
