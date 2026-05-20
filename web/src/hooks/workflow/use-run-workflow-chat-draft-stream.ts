import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { workflowService, type WorkflowRunSseCallbacks } from '@/services/workflow.service';
import type { ChatAttachment } from '@/components/chat/types';
import type {
  WorkflowFinishedSseData,
  WorkflowMessageEndSseData,
  WorkflowMessageSseData,
  WorkflowNodeFinishedSseData,
  WorkflowNodeStartedSseData,
  WorkflowPausedSseData,
  WorkflowStartedSseData,
  QuestionAnswerRequestedSseData,
  QuestionAnswerSubmittedSseData,
} from '@/services/types/workflow';

export interface UseRunWorkflowChatDraftStreamOptions {
  enabled?: boolean;
  onWorkflowStarted?: (data: WorkflowStartedEvent) => void;
  onApprovalRequested?: (data: unknown) => void;
  onApprovalResultFilled?: (data: unknown) => void;
  onApprovalExpired?: (data: unknown) => void;
  onQuestionAnswerRequested?: (data: QuestionAnswerRequestedSseData | unknown) => void;
  onQuestionAnswerSubmitted?: (data: QuestionAnswerSubmittedSseData | unknown) => void;
  onMessage?: (data: ChatMessageEvent) => void;
  onTextReplace?: (data: unknown) => void;
  onNodeStarted?: (data: NodeStartedEvent) => void;
  onNodeFinished?: (data: NodeFinishedEvent) => void;
  onMessageEnd?: (data: MessageEndEvent) => void;
  onWorkflowPaused?: (data: WorkflowPausedEvent) => void;
  onWorkflowFinished?: (data: WorkflowFinishedEvent) => void;
  onError?: (data: unknown) => void;
  onIterationStarted?: (data: unknown) => void;
  onIterationNext?: (data: unknown) => void;
  onIterationCompleted?: (data: unknown) => void;
  onLoopStarted?: (data: unknown) => void;
  onLoopNext?: (data: unknown) => void;
  onLoopCompleted?: (data: unknown) => void;
}

export interface UseRunWorkflowChatDraftStreamReturn {
  start: (payload: {
    query: string;
    conversation_id?: string;
    history_window_size?: number;
    files?: ChatAttachment[];
    inputs?: Record<string, unknown>;
  }) => Promise<void>;
  cancel: () => void;
  /** Stop the running workflow by calling the stop API and closing the stream */
  stop: () => Promise<void>;
  isStarting: boolean;
  isRunning: boolean;
  isStopping: boolean;
  /** Current workflow run ID from workflow_started event */
  workflowRunId: string | null;
}

export function useRunWorkflowChatDraftStream(
  agentId: string,
  opts: UseRunWorkflowChatDraftStreamOptions = {}
): UseRunWorkflowChatDraftStreamReturn {
  const [isStarting, setIsStarting] = useState(false);
  const [isRunning, setIsRunning] = useState(false);
  const [isStopping, setIsStopping] = useState(false);
  const [workflowRunId, setWorkflowRunId] = useState<string | null>(null);
  const handleRef = useRef<{ close: () => void } | null>(null);

  const enabled = opts.enabled !== false;

  const callbacks: WorkflowRunSseCallbacks = useMemo(
    () => ({
      onWorkflowStarted: payload => {
        setIsRunning(true);
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          const ev = data as WorkflowStartedEvent;
          // Store workflow run ID for stop functionality
          if (ev.id) {
            setWorkflowRunId(ev.id);
          }
          opts.onWorkflowStarted?.(ev);
        } catch {
          // swallow
        }
      },
      onNodeStarted: payload => {
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          const ev = data as NodeStartedEvent;
          opts.onNodeStarted?.(ev);
        } catch {
          // swallow
        }
      },
      onApprovalRequested: payload => {
        const data = (payload as { data?: unknown })?.['data'] ?? payload;
        opts.onApprovalRequested?.(data);
      },
      onApprovalResultFilled: payload => {
        const data = (payload as { data?: unknown })?.['data'] ?? payload;
        opts.onApprovalResultFilled?.(data);
      },
      onApprovalExpired: payload => {
        const data = (payload as { data?: unknown })?.['data'] ?? payload;
        opts.onApprovalExpired?.(data);
      },
      onQuestionAnswerRequested: payload => {
        const data = (payload as { data?: unknown })?.['data'] ?? payload;
        opts.onQuestionAnswerRequested?.(data);
      },
      onQuestionAnswerSubmitted: payload => {
        const data = (payload as { data?: unknown })?.['data'] ?? payload;
        opts.onQuestionAnswerSubmitted?.(data);
      },
      onNodeFinished: payload => {
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          const ev = data as NodeFinishedEvent;

          // Normalize status
          const rawStatus = ev.status as string;
          const status =
            rawStatus === 'succeeded' || rawStatus === 'success' || rawStatus === 'completed'
              ? 'succeeded'
              : rawStatus === 'paused'
                ? 'paused'
              : rawStatus === 'stopped'
                ? 'stopped'
                : 'failed'; // Covers 'exception', 'failed', and unknown states

          opts.onNodeFinished?.({
            ...ev,
            status,
          });
        } catch {
          // swallow
        }
      },
      onIterationStarted: payload => {
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          opts.onIterationStarted?.(data);
        } catch {
          // swallow
        }
      },
      onIterationNext: payload => {
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          opts.onIterationNext?.(data);
        } catch {
          // swallow
        }
      },
      onIterationCompleted: payload => {
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          opts.onIterationCompleted?.(data);
        } catch {
          // swallow
        }
      },
      onLoopStarted: payload => {
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          opts.onLoopStarted?.(data);
        } catch {
          // swallow
        }
      },
      onLoopNext: payload => {
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          opts.onLoopNext?.(data);
        } catch {
          // swallow
        }
      },
      onLoopCompleted: payload => {
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          opts.onLoopCompleted?.(data);
        } catch {
          // swallow
        }
      },
      onMessage: payload => {
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          const ev = data as { event?: string } & ChatMessageEvent;
          opts.onMessage?.(ev as ChatMessageEvent);
        } catch {
          // swallow
        }
      },
      onTextReplace: payload => opts.onTextReplace?.(payload),
      onMessageEnd: payload => {
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          const ev = data as MessageEndEvent;
          opts.onMessageEnd?.(ev);
        } catch {
          // swallow
        }
      },
      onWorkflowPaused: payload => {
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          const ev = data as WorkflowPausedEvent;
          opts.onWorkflowPaused?.(ev);
        } finally {
          setIsRunning(false);
        }
      },
      onWorkflowFinished: payload => {
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          const ev = data as WorkflowFinishedEvent;
          opts.onWorkflowFinished?.(ev);
        } finally {
          setIsRunning(false);
          setWorkflowRunId(null);
        }
      },
      onError: payload => {
        try {
          opts.onError?.(payload);
        } finally {
          setIsRunning(false);
          setIsStopping(false);
          setWorkflowRunId(null);
        }
      },
    }),
    [opts]
  );

  const cancel = useCallback(() => {
    handleRef.current?.close();
    handleRef.current = null;
    setIsRunning(false);
    setWorkflowRunId(null);
  }, []);

  const stop = useCallback(async () => {
    const runId = workflowRunId;
    if (!runId) {
      return;
    }
    setIsStopping(true);
    try {
      await workflowService.stopWorkflowTask(agentId, runId);
      setIsRunning(false);
      setWorkflowRunId(null);
      opts.onWorkflowFinished?.({
        id: runId,
        workflow_run_id: runId,
        status: 'stopped',
        created_at: Date.now() / 1000,
        finished_at: Date.now() / 1000,
        elapsed_time: 0,
      });
      handleRef.current?.close();
      handleRef.current = null;
    } catch (err) {
      console.error('Failed to stop workflow task:', err);
    } finally {
      setIsStopping(false);
    }
  }, [agentId, opts, workflowRunId]);

  const start = useCallback(
    async (payload: {
      query: string;
      conversation_id?: string;
      history_window_size?: number;
      files?: ChatAttachment[];
      inputs?: Record<string, unknown>;
    }) => {
      if (!enabled) return;
      handleRef.current?.close();
      setIsStarting(true);
      try {
        const handle = await workflowService.ssePostRunWorkflowChatDraft(
          agentId,
          payload,
          callbacks
        );
        handleRef.current = handle;
      } finally {
        setIsStarting(false);
      }
    },
    [agentId, callbacks, enabled]
  );

  useEffect(() => () => cancel(), [cancel]);

  return { start, cancel, stop, isStarting, isRunning, isStopping, workflowRunId };
}

// Shared SSE payload types. Nested workflow/node events intentionally do not
// include `event` because the stream client unwraps `payload.data` first.
export type WorkflowStartedEvent = WorkflowStartedSseData;
export type NodeStartedEvent = WorkflowNodeStartedSseData;
export type NodeFinishedEvent = WorkflowNodeFinishedSseData;
export type ChatMessageEvent = WorkflowMessageSseData;
export type WorkflowPausedEvent = WorkflowPausedSseData;
export type WorkflowFinishedEvent = WorkflowFinishedSseData;
export type MessageEndEvent = WorkflowMessageEndSseData;
