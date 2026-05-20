'use client';

import { useCallback, useEffect, useRef } from 'react';

import { workflowService, type WorkflowRunSseCallbacks } from '@/services/workflow.service';

export interface WorkflowRunEventsStreamParams {
  after?: number;
  include_snapshot?: boolean;
  continue_on_pause?: boolean;
}

export interface UseWorkflowRunEventsStreamReturn {
  start: (
    workflowRunId: string,
    callbacks: WorkflowRunSseCallbacks,
    params?: WorkflowRunEventsStreamParams,
    options?: { onClose?: () => void }
  ) => Promise<void>;
  cancel: () => void;
}

/**
 * @hook useWorkflowRunEventsStream
 * @category Workflow
 * @status Beta
 * @description Streams persisted workflow run events with sequence resume support.
 */
export function useWorkflowRunEventsStream(): UseWorkflowRunEventsStreamReturn {
  const handleRef = useRef<{ close: () => void } | null>(null);

  const cancel = useCallback(() => {
    handleRef.current?.close();
    handleRef.current = null;
  }, []);

  const start = useCallback(
    async (
      workflowRunId: string,
      callbacks: WorkflowRunSseCallbacks,
      params?: WorkflowRunEventsStreamParams,
      options?: { onClose?: () => void }
    ) => {
      cancel();
      const handle = await workflowService.sseWorkflowRunEvents(workflowRunId, callbacks, {
        params,
        onClose: () => {
          handleRef.current = null;
          options?.onClose?.();
        },
      });
      handleRef.current = handle;
    },
    [cancel]
  );

  useEffect(() => () => cancel(), [cancel]);

  return { start, cancel };
}
