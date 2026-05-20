import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  workflowService,
  type WorkflowRunRequest,
  type WorkflowRunSseCallbacks,
} from '@/services/workflow.service';

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
}

export interface UseRunWorkflowDraftStreamReturn {
  start: (payload: WorkflowRunRequest) => Promise<void>;
  cancel: () => void;
  isStarting: boolean;
  isRunning: boolean;
}

export function useRunWorkflowDraftStream(
  agentId: string,
  opts: UseRunWorkflowDraftStreamOptions = {}
): UseRunWorkflowDraftStreamReturn {
  const queryClient = useQueryClient();
  const [isStarting, setIsStarting] = useState(false);
  const [isRunning, setIsRunning] = useState(false);
  const isRunningRef = useRef(false);
  const handleRef = useRef<{ close: () => void } | null>(null);

  const enabled = opts.enabled !== false;

  // Sync ref with state to ensure callbacks have access to current running status
  useEffect(() => {
    isRunningRef.current = isRunning;
  }, [isRunning]);

  // Stable callbacks wrapper
  const callbacks: WorkflowRunSseCallbacks = useMemo(
    () => ({
      onWorkflowStarted: payload => {
        setIsRunning(true);
        opts.onStarted?.(payload);
      },
      onWorkflowPaused: payload => {
        opts.onPaused?.(payload);
        setIsRunning(false);
        setIsStarting(false);
        isRunningRef.current = false;
      },
      onApprovalRequested: payload => opts.onApprovalRequested?.(payload),
      onApprovalResultFilled: payload => opts.onApprovalResultFilled?.(payload),
      onApprovalExpired: payload => opts.onApprovalExpired?.(payload),
      onQuestionAnswerRequested: payload => opts.onQuestionAnswerRequested?.(payload),
      onQuestionAnswerSubmitted: payload => opts.onQuestionAnswerSubmitted?.(payload),
      onNodeStarted: payload => opts.onNodeStarted?.(payload),
      onNodeFinished: payload => opts.onNodeFinished?.(payload),
      onTextChunk: payload => opts.onTextChunk?.(payload),
      onTextReplace: payload => opts.onTextReplace?.(payload),
      onIterationStarted: payload => opts.onIterationStarted?.(payload),
      onIterationNext: payload => opts.onIterationNext?.(payload),
      onIterationCompleted: payload => opts.onIterationCompleted?.(payload),
      onLoopStarted: payload => opts.onLoopStarted?.(payload),
      onLoopNext: payload => opts.onLoopNext?.(payload),
      onLoopCompleted: payload => opts.onLoopCompleted?.(payload),
      onWorkflowFinished: async payload => {
        try {
          opts.onFinished?.(payload);
        } finally {
          setIsRunning(false);
          setIsStarting(false);
          // Sync ref immediately to prevent onClose from triggering error
          isRunningRef.current = false;
          // Invalidate default workflow runs list
          await queryClient.invalidateQueries({ queryKey: ['workflow-runs', agentId] });
          // Invalidate additional keys if provided
          if (Array.isArray(opts.invalidateKeys)) {
            for (const key of opts.invalidateKeys) {
              await queryClient.invalidateQueries({ queryKey: key });
            }
          }
        }
      },
      onError: payload => {
        try {
          opts.onError?.(payload);
        } finally {
          // Reset running state on error to prevent infinite loading
          setIsRunning(false);
          setIsStarting(false);
          // Sync ref immediately to prevent onClose from triggering error
          isRunningRef.current = false;
        }
      },
    }),
    [agentId, opts, queryClient]
  );

  const cancel = useCallback(() => {
    // Mark as not running immediately to prevent onClose from triggering error
    isRunningRef.current = false;
    handleRef.current?.close();
    handleRef.current = null;
    setIsRunning(false);
    setIsStarting(false);
  }, []);

  const start = useCallback(
    async (payload: WorkflowRunRequest) => {
      if (!enabled) return;
      // Close previous stream if exists
      handleRef.current?.close();

      setIsStarting(true);
      // Mark as running immediately for onClose check
      isRunningRef.current = true;
      try {
        const handle = await workflowService.ssePostRunWorkflowDraft(agentId, payload, callbacks, {
          onClose: () => {
            // If stream closes but we still think we are running, it means
            // the connection was lost unexpectedly (e.g. network error)
            if (isRunningRef.current) {
              const errPayload = {
                error: 'Connection Closed',
                message: 'The workflow execution stream was closed unexpectedly.',
              };
              opts.onError?.(errPayload);
              setIsRunning(false);
              setIsStarting(false);
            }
          },
        });
        handleRef.current = handle;
      } catch (error) {
        // Reset state on error
        setIsRunning(false);
        setIsStarting(false);
        isRunningRef.current = false;
        throw error;
      } finally {
        setIsStarting(false);
      }
    },
    [agentId, callbacks, enabled, opts]
  );

  // Auto cleanup on unmount
  useEffect(() => () => cancel(), [cancel]);

  return { start, cancel, isStarting, isRunning };
}
