import { useCallback, useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  workflowService,
  type WorkflowRunRequest,
  type WorkflowRunSseCallbacks,
} from '@/services/workflow.service';
import { ErrorNotificationService } from '@/utils/error-notifications';

export interface UseRunWorkflowDraftStreamOptions {
  enabled?: boolean;
  onStarted?: (data: unknown) => void;
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
  invalidateKeys?: Array<readonly unknown[]>;
  onIterationStarted?: (data: unknown) => void;
  onIterationNext?: (data: unknown) => void;
  onIterationCompleted?: (data: unknown) => void;
  onLoopStarted?: (data: unknown) => void;
  onLoopNext?: (data: unknown) => void;
  onLoopCompleted?: (data: unknown) => void;
  /** Recover an established run from its durable snapshot and event tail. */
  onTransportInterrupted?: (workflowRunId: string, error: Error) => void;
}

export interface UseRunWorkflowDraftStreamReturn {
  start: (payload: WorkflowRunRequest) => Promise<void>;
  cancel: () => void;
  isStarting: boolean;
  isRunning: boolean;
}

function getWorkflowRunId(payload: unknown): string {
  const envelope =
    payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
  const data =
    envelope.data && typeof envelope.data === 'object'
      ? (envelope.data as Record<string, unknown>)
      : envelope;
  if (typeof data.id === 'string') return data.id;
  return typeof data.workflow_run_id === 'string' ? data.workflow_run_id : '';
}

export function useRunWorkflowDraftStream(
  agentId: string,
  opts: UseRunWorkflowDraftStreamOptions = {}
): UseRunWorkflowDraftStreamReturn {
  const queryClient = useQueryClient();
  const [isStarting, setIsStarting] = useState(false);
  const [isRunning, setIsRunning] = useState(false);
  const isRunningRef = useRef(false);
  const handleRef = useRef<{ close: () => void; generation: number } | null>(null);
  const requestControllerRef = useRef<AbortController | null>(null);
  const streamGenerationRef = useRef(0);
  const optsRef = useRef(opts);
  optsRef.current = opts;

  const enabled = opts.enabled !== false;

  // Sync ref with state to ensure callbacks have access to current running status
  useEffect(() => {
    isRunningRef.current = isRunning;
  }, [isRunning]);

  const cancel = useCallback(() => {
    streamGenerationRef.current += 1;
    isRunningRef.current = false;
    requestControllerRef.current?.abort();
    requestControllerRef.current = null;
    handleRef.current?.close();
    handleRef.current = null;
    setIsRunning(false);
    setIsStarting(false);
  }, []);

  const start = useCallback(
    async (payload: WorkflowRunRequest) => {
      if (!enabled) return;

      // Invalidate callbacks from the previous request before closing it. A close
      // callback may run in a later microtask after the new request has started.
      const generation = streamGenerationRef.current + 1;
      streamGenerationRef.current = generation;
      requestControllerRef.current?.abort();
      handleRef.current?.close();
      handleRef.current = null;
      const requestController = new AbortController();
      requestControllerRef.current = requestController;
      let workflowRunId = '';
      let transportRecoveryRequested = false;

      const isCurrent = () => streamGenerationRef.current === generation;
      const requestTransportRecovery = (error: Error): boolean => {
        if (!isCurrent() || transportRecoveryRequested) return transportRecoveryRequested;
        const recover = optsRef.current.onTransportInterrupted;
        if (!workflowRunId || !recover) {
          ErrorNotificationService.showNetworkError();
          return false;
        }
        transportRecoveryRequested = true;
        // The durable run-events stream owns the runtime projection from this point on.
        // Keeping the POST stream's local flag set would make a terminal snapshot look
        // running forever even after workflow_finished has been applied.
        isRunningRef.current = false;
        setIsRunning(false);
        setIsStarting(false);
        recover(workflowRunId, error);
        return true;
      };

      const callbacks: WorkflowRunSseCallbacks = {
        onWorkflowStarted: event => {
          if (!isCurrent()) return;
          workflowRunId = getWorkflowRunId(event);
          setIsRunning(true);
          optsRef.current.onStarted?.(event);
        },
        onWorkflowPaused: event => {
          if (!isCurrent()) return;
          optsRef.current.onPaused?.(event);
          setIsRunning(false);
          setIsStarting(false);
          isRunningRef.current = false;
        },
        onApprovalRequested: event => {
          if (isCurrent()) optsRef.current.onApprovalRequested?.(event);
        },
        onApprovalResultFilled: event => {
          if (isCurrent()) optsRef.current.onApprovalResultFilled?.(event);
        },
        onApprovalExpired: event => {
          if (isCurrent()) optsRef.current.onApprovalExpired?.(event);
        },
        onQuestionAnswerRequested: event => {
          if (isCurrent()) optsRef.current.onQuestionAnswerRequested?.(event);
        },
        onQuestionAnswerSubmitted: event => {
          if (isCurrent()) optsRef.current.onQuestionAnswerSubmitted?.(event);
        },
        onNodeStarted: event => {
          if (isCurrent()) optsRef.current.onNodeStarted?.(event);
        },
        onNodeFinished: event => {
          if (isCurrent()) optsRef.current.onNodeFinished?.(event);
        },
        onTextChunk: event => {
          if (isCurrent()) optsRef.current.onTextChunk?.(event);
        },
        onTextReplace: event => {
          if (isCurrent()) optsRef.current.onTextReplace?.(event);
        },
        onIterationStarted: event => {
          if (isCurrent()) optsRef.current.onIterationStarted?.(event);
        },
        onIterationNext: event => {
          if (isCurrent()) optsRef.current.onIterationNext?.(event);
        },
        onIterationCompleted: event => {
          if (isCurrent()) optsRef.current.onIterationCompleted?.(event);
        },
        onLoopStarted: event => {
          if (isCurrent()) optsRef.current.onLoopStarted?.(event);
        },
        onLoopNext: event => {
          if (isCurrent()) optsRef.current.onLoopNext?.(event);
        },
        onLoopCompleted: event => {
          if (isCurrent()) optsRef.current.onLoopCompleted?.(event);
        },
        onWorkflowFinished: async event => {
          if (!isCurrent()) return;
          try {
            optsRef.current.onFinished?.(event);
          } finally {
            if (isCurrent()) {
              setIsRunning(false);
              setIsStarting(false);
              isRunningRef.current = false;
              await queryClient.invalidateQueries({ queryKey: ['workflow-runs', agentId] });
              const invalidateKeys = optsRef.current.invalidateKeys;
              if (Array.isArray(invalidateKeys)) {
                for (const key of invalidateKeys) {
                  await queryClient.invalidateQueries({ queryKey: key });
                }
              }
            }
          }
        },
        onError: event => {
          if (!isCurrent() || transportRecoveryRequested) return;
          try {
            optsRef.current.onError?.(event);
          } finally {
            setIsRunning(false);
            setIsStarting(false);
            isRunningRef.current = false;
          }
        },
      };

      setIsStarting(true);
      isRunningRef.current = true;
      try {
        const handle = await workflowService.ssePostRunWorkflowDraft(agentId, payload, callbacks, {
          abortSignal: requestController.signal,
          suppressTransportErrorNotification: true,
          onTransportError: error => {
            requestTransportRecovery(error);
          },
          onClose: () => {
            if (!isCurrent()) return;
            handleRef.current = null;
            requestControllerRef.current = null;
            if (!isRunningRef.current || transportRecoveryRequested) return;
            requestTransportRecovery(
              new Error('The workflow execution stream was closed unexpectedly.')
            );
          },
        });
        if (!isCurrent()) {
          handle.close();
          return;
        }
        handleRef.current = { ...handle, generation };
      } catch (error) {
        if (!isCurrent()) return;
        setIsRunning(false);
        setIsStarting(false);
        isRunningRef.current = false;
        throw error;
      } finally {
        if (isCurrent()) setIsStarting(false);
      }
    },
    [agentId, enabled, queryClient]
  );

  // Auto cleanup on unmount
  useEffect(() => () => cancel(), [cancel]);

  return { start, cancel, isStarting, isRunning };
}
