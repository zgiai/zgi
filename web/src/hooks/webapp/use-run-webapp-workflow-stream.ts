import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { WebAppRunRequest, WebAppRunSseCallbacks } from '@/services/types/webapp';
import { WebAppService } from '@/services/webapp.service';
import { workflowService } from '@/services/workflow.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { emitWebAppOffline, isWebAppOfflineError } from '@/utils/webapp/errors';
import { ErrorNotificationService } from '@/utils/error-notifications';

export interface UseRunWebAppWorkflowStreamOptions {
  enabled?: boolean;
  /** Agent ID for stop API - required for stop functionality */
  agentId?: string;
  onStarted?: (data: unknown) => void;
  onResumed?: (data: unknown) => void;
  onPaused?: (data: unknown) => void;
  onApprovalRequested?: (data: unknown) => void;
  onApprovalResultFilled?: (data: unknown) => void;
  onApprovalExpired?: (data: unknown) => void;
  onQuestionAnswerRequested?: (data: unknown) => void;
  onQuestionAnswerSubmitted?: (data: unknown) => void;
  onNodeStarted?: (data: unknown) => void;
  onNodeFinished?: (data: unknown) => void;
  onTextChunk?: (data: unknown) => void;
  onTextReplace?: (data: unknown) => void;
  onFinished?: (data: unknown) => void;
  onError?: (data: unknown) => void;
  onMessage?: (data: unknown) => void;
  onMessageEnd?: (data: unknown) => void;
  onIterationStarted?: (data: unknown) => void;
  onIterationNext?: (data: unknown) => void;
  onIterationCompleted?: (data: unknown) => void;
  onLoopStarted?: (data: unknown) => void;
  onLoopNext?: (data: unknown) => void;
  onLoopCompleted?: (data: unknown) => void;
}

export interface UseRunWebAppWorkflowStreamReturn {
  start: (
    payload: WebAppRunRequest,
    perRunCallbacks?: WebAppRunSseCallbacks,
    opts?: {
      abortSignal?: AbortSignal;
      onTransportInterrupted?: (workflowRunId: string, error: Error) => void;
    }
  ) => Promise<void>;
  cancel: () => void;
  stop: () => Promise<void>;
  isStarting: boolean;
  isRunning: boolean;
  isStopping: boolean;
}

/**
 * Run a published workflow (webapp) via SSE, consistent with the draft run hook semantics.
 */
export function useRunWebAppWorkflowStream(
  versionUuid: string,
  opts: UseRunWebAppWorkflowStreamOptions = {}
): UseRunWebAppWorkflowStreamReturn {
  const t = useT('agents');
  const [isStarting, setIsStarting] = useState(false);
  const [isRunning, setIsRunning] = useState(false);
  const [isStopping, setIsStopping] = useState(false);
  const isRunningRef = useRef(false);
  const handlesRef = useRef<Map<string, { close: () => void }>>(new Map());
  const taskIdRef = useRef<string | null>(null);

  const enabled = opts.enabled !== false;
  const agentId = opts.agentId;

  // Use a ref to store the latest options to avoid dependency cycle
  const optsRef = useRef(opts);
  useEffect(() => {
    optsRef.current = opts;
  });

  useEffect(() => {
    isRunningRef.current = isRunning;
  }, [isRunning]);

  const callbacks: WebAppRunSseCallbacks = useMemo(
    () => ({
      onWorkflowStarted: payload => {
        setIsRunning(true);
        isRunningRef.current = true;
        // Extract task_id from payload for stop functionality
        // Try multiple possible field names: task_id, id, workflow_run_id
        const data = (payload as { data?: unknown })?.data ?? payload;
        const taskId =
          (data as { task_id?: string })?.task_id ||
          (data as { id?: string })?.id ||
          (data as { workflow_run_id?: string })?.workflow_run_id;
        if (taskId) {
          taskIdRef.current = taskId;
        }
        optsRef.current.onStarted?.(payload);
      },
      onNodeStarted: payload => optsRef.current.onNodeStarted?.(payload),
      onNodeFinished: payload => optsRef.current.onNodeFinished?.(payload),
      onWorkflowResumed: payload => {
        setIsRunning(true);
        setIsStarting(false);
        isRunningRef.current = true;
        optsRef.current.onResumed?.(payload);
      },
      onWorkflowPaused: payload => {
        try {
          optsRef.current.onPaused?.(payload);
        } finally {
          setIsRunning(false);
          setIsStarting(false);
          isRunningRef.current = false;
        }
      },
      onApprovalRequested: payload => optsRef.current.onApprovalRequested?.(payload),
      onApprovalResultFilled: payload => optsRef.current.onApprovalResultFilled?.(payload),
      onApprovalExpired: payload => optsRef.current.onApprovalExpired?.(payload),
      onQuestionAnswerRequested: payload => optsRef.current.onQuestionAnswerRequested?.(payload),
      onQuestionAnswerSubmitted: payload => optsRef.current.onQuestionAnswerSubmitted?.(payload),
      onTextChunk: payload => optsRef.current.onTextChunk?.(payload),
      onTextReplace: payload => optsRef.current.onTextReplace?.(payload),
      onWorkflowFinished: payload => {
        try {
          optsRef.current.onFinished?.(payload);
        } finally {
          setIsRunning(false);
          setIsStarting(false);
          setIsStopping(false);
          isRunningRef.current = false;
          taskIdRef.current = null;
        }
      },
      onError: payload => {
        try {
          if (isWebAppOfflineError(payload)) {
            emitWebAppOffline();
          }
          optsRef.current.onError?.(payload);
        } finally {
          setIsRunning(false);
          setIsStarting(false);
          isRunningRef.current = false;
          setIsStopping(false);
          taskIdRef.current = null;
        }
      },
      onMessage: payload => optsRef.current.onMessage?.(payload),
      onMessageEnd: payload => optsRef.current.onMessageEnd?.(payload),
      onIterationStarted: payload => optsRef.current.onIterationStarted?.(payload),
      onIterationNext: payload => optsRef.current.onIterationNext?.(payload),
      onIterationCompleted: payload => optsRef.current.onIterationCompleted?.(payload),
      onLoopStarted: payload => optsRef.current.onLoopStarted?.(payload),
      onLoopNext: payload => optsRef.current.onLoopNext?.(payload),
      onLoopCompleted: payload => optsRef.current.onLoopCompleted?.(payload),
    }),
    []
  );

  const cancel = useCallback(() => {
    isRunningRef.current = false;
    handlesRef.current.forEach(handle => handle.close());
    handlesRef.current.clear();
    setIsRunning(false);
    setIsStarting(false);
  }, []);

  const start = useCallback(
    async (
      payload: WebAppRunRequest,
      perRunCallbacks?: WebAppRunSseCallbacks,
      opts?: {
        abortSignal?: AbortSignal;
        onTransportInterrupted?: (workflowRunId: string, error: Error) => void;
      }
    ) => {
      if (!enabled) return;
      const requestKey =
        payload.conversation_id || `draft:${Date.now()}:${Math.random().toString(36).slice(2)}`;
      let workflowRunId = '';
      let terminalReceived = false;
      let gracefulStreamBoundary = false;
      let transportRecoveryRequested = false;
      let suppressNextTransportErrorCallback = false;
      const captureWorkflowRunId = (eventPayload: unknown): void => {
        const data =
          eventPayload && typeof eventPayload === 'object' && 'data' in eventPayload
            ? ((eventPayload as { data?: unknown }).data ?? eventPayload)
            : eventPayload;
        if (!data || typeof data !== 'object') return;
        const record = data as Record<string, unknown>;
        const candidate = [record.task_id, record.id, record.workflow_run_id].find(
          value => typeof value === 'string' && value.length > 0
        );
        if (typeof candidate !== 'string') return;
        workflowRunId = candidate;
        taskIdRef.current = candidate;
      };
      const requestTransportRecovery = (error: Error): boolean => {
        if (terminalReceived || transportRecoveryRequested) return transportRecoveryRequested;
        const recover = opts?.onTransportInterrupted;
        if (!workflowRunId || !recover) return false;
        transportRecoveryRequested = true;
        isRunningRef.current = false;
        setIsRunning(false);
        setIsStarting(false);
        recover(workflowRunId, error);
        return true;
      };
      setIsStarting(true);
      isRunningRef.current = true;
      try {
        const merged: WebAppRunSseCallbacks = {
          onWorkflowStarted: p => {
            captureWorkflowRunId(p);
            callbacks.onWorkflowStarted?.(p);
            perRunCallbacks?.onWorkflowStarted?.(p);
          },
          onNodeStarted: p => {
            captureWorkflowRunId(p);
            callbacks.onNodeStarted?.(p);
            perRunCallbacks?.onNodeStarted?.(p);
          },
          onNodeFinished: p => {
            callbacks.onNodeFinished?.(p);
            perRunCallbacks?.onNodeFinished?.(p);
          },
          onWorkflowResumed: p => {
            captureWorkflowRunId(p);
            callbacks.onWorkflowResumed?.(p);
            perRunCallbacks?.onWorkflowResumed?.(p);
          },
          onWorkflowPaused: p => {
            captureWorkflowRunId(p);
            gracefulStreamBoundary = true;
            callbacks.onWorkflowPaused?.(p);
            perRunCallbacks?.onWorkflowPaused?.(p);
          },
          onApprovalRequested: p => {
            captureWorkflowRunId(p);
            callbacks.onApprovalRequested?.(p);
            perRunCallbacks?.onApprovalRequested?.(p);
          },
          onApprovalResultFilled: p => {
            callbacks.onApprovalResultFilled?.(p);
            perRunCallbacks?.onApprovalResultFilled?.(p);
          },
          onApprovalExpired: p => {
            callbacks.onApprovalExpired?.(p);
            perRunCallbacks?.onApprovalExpired?.(p);
          },
          onQuestionAnswerRequested: p => {
            captureWorkflowRunId(p);
            callbacks.onQuestionAnswerRequested?.(p);
            perRunCallbacks?.onQuestionAnswerRequested?.(p);
          },
          onQuestionAnswerSubmitted: p => {
            callbacks.onQuestionAnswerSubmitted?.(p);
            perRunCallbacks?.onQuestionAnswerSubmitted?.(p);
          },
          onTextChunk: p => {
            callbacks.onTextChunk?.(p);
            perRunCallbacks?.onTextChunk?.(p);
          },
          onTextReplace: p => {
            callbacks.onTextReplace?.(p);
            perRunCallbacks?.onTextReplace?.(p);
          },
          onWorkflowFinished: p => {
            captureWorkflowRunId(p);
            gracefulStreamBoundary = true;
            terminalReceived = true;
            callbacks.onWorkflowFinished?.(p);
            perRunCallbacks?.onWorkflowFinished?.(p);
          },
          onError: p => {
            if (suppressNextTransportErrorCallback || transportRecoveryRequested) {
              suppressNextTransportErrorCallback = false;
              return;
            }
            terminalReceived = true;
            callbacks.onError?.(p);
            perRunCallbacks?.onError?.(p);
          },
          onMessage: p => {
            callbacks.onMessage?.(p);
            perRunCallbacks?.onMessage?.(p);
          },
          onMessageEnd: p => {
            callbacks.onMessageEnd?.(p);
            perRunCallbacks?.onMessageEnd?.(p);
          },
          onIterationStarted: p => {
            callbacks.onIterationStarted?.(p);
            perRunCallbacks?.onIterationStarted?.(p);
          },
          onIterationNext: p => {
            callbacks.onIterationNext?.(p);
            perRunCallbacks?.onIterationNext?.(p);
          },
          onIterationCompleted: p => {
            callbacks.onIterationCompleted?.(p);
            perRunCallbacks?.onIterationCompleted?.(p);
          },
          onLoopStarted: p => {
            callbacks.onLoopStarted?.(p);
            perRunCallbacks?.onLoopStarted?.(p);
          },
          onLoopNext: p => {
            callbacks.onLoopNext?.(p);
            perRunCallbacks?.onLoopNext?.(p);
          },
          onLoopCompleted: p => {
            callbacks.onLoopCompleted?.(p);
            perRunCallbacks?.onLoopCompleted?.(p);
          },
        };
        const handle = await WebAppService.ssePostRun(versionUuid, payload, merged, {
          abortSignal: opts?.abortSignal,
          suppressTransportErrorNotification: true,
          onTransportError: error => {
            const recoveryStarted = requestTransportRecovery(error);
            if (gracefulStreamBoundary || recoveryStarted) {
              // ssePost invokes the application onError callback immediately after
              // onTransportError. Ignore that single transport callback because a
              // paused run is already durable and an interrupted active run is now
              // owned by the recovery stream.
              suppressNextTransportErrorCallback = true;
              return;
            }
            ErrorNotificationService.showNetworkError();
          },
          onClose: () => {
            handlesRef.current.delete(requestKey);
            if (terminalReceived || transportRecoveryRequested || !workflowRunId) return;
            requestTransportRecovery(
              new Error('The workflow execution stream was closed unexpectedly.')
            );
          },
        });
        handlesRef.current.set(requestKey, handle);
      } catch (error) {
        if (isWebAppOfflineError(error)) {
          emitWebAppOffline();
        }
        setIsRunning(false);
        setIsStarting(false);
        isRunningRef.current = false;
        throw error;
      } finally {
        setIsStarting(false);
      }
    },
    [versionUuid, callbacks, enabled]
  );

  // Stop running workflow via API using agentId
  const stop = useCallback(async () => {
    const taskId = taskIdRef.current;
    if (!taskId || !agentId) {
      return;
    }
    setIsStopping(true);
    try {
      await workflowService.stopWorkflowTask(agentId, taskId);
    } catch (error) {
      toast.error(t('workflow.stopFailed'));
      setIsStopping(false);
      throw error;
    }
  }, [agentId, t]);

  // Auto cleanup on unmount
  useEffect(() => () => cancel(), [cancel]);

  return { start, cancel, stop, isStarting, isRunning, isStopping };
}
