import { useEffect, useMemo } from 'react';
import { useWorkflowStore } from '@/components/workflow/store';
import type { WorkflowRunNodeListItem } from '@/components/workflow/ui/workflow-run-nodes-list';
import { useWorkflowRunDetail, useWorkflowRunNodeExecutions } from '@/hooks';
import type { WorkflowFinishedData, HistoryResult } from '../types';
import {
  buildWorkflowRunExecutionItems,
  buildWorkflowRunHistoryResult,
  buildWorkflowRunSummary,
  normalizeCanvasRunStatus,
} from '../utils/history-view-data';

interface UseHistoryViewParams {
  agentId: string;
  open: boolean;
  isHistory: boolean;
  selectedRunId?: string | null;
}

export function useHistoryView({ agentId, open, isHistory, selectedRunId }: UseHistoryViewParams) {
  const {
    detail: historyDetail,
    isLoading: isLoadingHistoryDetail,
    isFetching: isFetchingHistoryDetail,
    error: detailError,
  } = useWorkflowRunDetail(
    { agentId, runId: selectedRunId || null },
    { enabled: open && isHistory, staleTime: 60_000, refetchOnWindowFocus: false }
  );

  const {
    records: historyNodeExecutions,
    isLoading: isLoadingHistoryNodes,
    isFetching: isFetchingHistoryNodes,
    error: nodeExecutionsError,
  } = useWorkflowRunNodeExecutions(
    { agentId, runId: selectedRunId },
    { enabled: open && isHistory, staleTime: 60_000, refetchOnWindowFocus: false }
  );

  const loading = Boolean(
    isLoadingHistoryDetail ||
      isFetchingHistoryDetail ||
      isLoadingHistoryNodes ||
      isFetchingHistoryNodes
  );

  const summary: WorkflowFinishedData | null = useMemo(() => {
    return buildWorkflowRunSummary(historyDetail);
  }, [historyDetail]);

  const graph = historyDetail?.graph as
    | { nodes?: unknown[]; edges?: unknown[]; viewport?: { x: number; y: number; zoom: number } }
    | undefined;

  const items: WorkflowRunNodeListItem[] = useMemo(() => {
    return buildWorkflowRunExecutionItems(historyNodeExecutions);
  }, [historyNodeExecutions]);

  const result: HistoryResult = useMemo(() => {
    return buildWorkflowRunHistoryResult(historyDetail);
  }, [historyDetail]);

  // Apply statuses to canvas when in history mode
  useEffect(() => {
    if (!open || !isHistory) return;
    const resetRunStatus = useWorkflowStore.getState().resetRunStatus;
    const setNodeRunStatus = useWorkflowStore.getState().setNodeRunStatus;
    const setActiveOutputHandle = useWorkflowStore.getState().setActiveOutputHandle;
    resetRunStatus();
    if (!Array.isArray(historyNodeExecutions) || historyNodeExecutions.length === 0) return;
    for (const rec of historyNodeExecutions) {
      const status = normalizeCanvasRunStatus(rec?.status);
      if (rec?.node_id) setNodeRunStatus(rec.node_id, status);
      if (rec?.node_id && typeof rec.output_handle === 'string' && rec.output_handle.length > 0) {
        setActiveOutputHandle(rec.node_id, rec.output_handle);
      }
    }
  }, [open, isHistory, selectedRunId, historyNodeExecutions]);

  return {
    loading,
    summary,
    items,
    result,
    graph,
    detailError,
    nodeExecutionsError,
  };
}
