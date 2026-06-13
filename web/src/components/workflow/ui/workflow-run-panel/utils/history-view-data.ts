import type { WorkflowRunNodeListItem } from '@/components/workflow/ui/workflow-run-nodes-list';
import type {
  WorkflowNodeExecution,
  WorkflowRunDetail,
  WorkflowRunStatus,
} from '@/services/types/workflow';
import { extractLlmGatewayRequest } from '@/utils/workflow/run-events';
import type { HistoryResult, WorkflowFinishedData } from '../types';

function normalizeExecutionStatus(
  status: unknown
): 'running' | 'succeeded' | 'failed' | 'stopped' | 'paused' {
  const value = typeof status === 'string' ? status.toLowerCase() : '';

  if (
    value === 'succeeded' ||
    value === 'success' ||
    value === 'completed' ||
    value === 'partial-succeeded'
  ) {
    return 'succeeded';
  }

  if (value === 'running' || value === 'in_progress' || value === 'in-progress') {
    return 'running';
  }

  if (value === 'stopped') {
    return 'stopped';
  }

  if (value === 'paused') {
    return 'paused';
  }

  return 'failed';
}

export function normalizeCanvasRunStatus(
  status: unknown
): 'idle' | 'running' | 'succeeded' | 'failed' | 'stopped' | 'paused' {
  const normalized = normalizeExecutionStatus(status);
  return normalized === 'running' ||
    normalized === 'succeeded' ||
    normalized === 'failed' ||
    normalized === 'stopped' ||
    normalized === 'paused'
    ? normalized
    : 'idle';
}

export function buildWorkflowRunSummary(
  historyDetail: WorkflowRunDetail | null | undefined
): WorkflowFinishedData | null {
  if (!historyDetail) return null;

  return {
    id: historyDetail.id,
    status: String(historyDetail.status),
    created_at: historyDetail.created_at,
    finished_at: historyDetail.finished_at,
    elapsed_time: historyDetail.elapsed_time,
    total_steps: historyDetail.total_steps,
    workflow_id: historyDetail.workflow_id,
    inputs: historyDetail.inputs,
    outputs: historyDetail.outputs,
    total_tokens: historyDetail.total_tokens,
    error: historyDetail.error,
    conversation_id: historyDetail.conversation_id,
    message_id: historyDetail.message_id,
  } satisfies WorkflowFinishedData;
}

export function buildWorkflowRunExecutionItems(
  historyNodeExecutions: WorkflowNodeExecution[] | null | undefined
): WorkflowRunNodeListItem[] {
  if (!Array.isArray(historyNodeExecutions)) return [];

  const executionIdCounts = new Map<string, number>();

  const baseItems = historyNodeExecutions.map(rec => {
    const status = normalizeExecutionStatus(rec.status);
    const baseExecutionId = rec.id || rec.node_id;
    const occurrence = executionIdCounts.get(baseExecutionId) ?? 0;
    executionIdCounts.set(baseExecutionId, occurrence + 1);

    return {
      executionId:
        occurrence === 0 ? baseExecutionId : `${baseExecutionId}:${status}:${occurrence}`,
      nodeId: rec.node_id,
      title:
        typeof rec.title === 'string' && rec.title.length > 0
          ? rec.title
          : rec.node_type || rec.node_id,
      nodeType: rec.node_type || 'unknown',
      status,
      nodeInput: rec.inputs,
      nodeOutput: rec.outputs,
      modelInput: extractLlmGatewayRequest(rec.process_data),
      processData: rec.process_data,
      executionMetadata: rec.execution_metadata,
      elapsedTime: typeof rec.elapsed_time === 'number' ? rec.elapsed_time : undefined,
      error:
        typeof rec.error === 'string' ? rec.error : rec.error ? JSON.stringify(rec.error) : null,
    };
  });

  return baseItems.map((item, index) => {
    const rec = historyNodeExecutions[index];

    if (item.nodeType === 'iteration') {
      const output = rec.outputs as unknown;
      const isObjectOutput = typeof output === 'object' && output !== null;
      const list =
        isObjectOutput && Array.isArray((output as Record<string, unknown>).output)
          ? ((output as Record<string, unknown>).output as unknown[])
          : [];

      const rounds = Array.from({ length: list.length }, (_, roundIndex) => ({
        index: roundIndex,
        nodes: [],
      })) as Array<{
        index: number;
        nodes: WorkflowRunNodeListItem[];
        elapsedTime?: number;
      }>;

      return {
        ...item,
        iterationInputs: rec.inputs,
        iterationOutputs: rec.outputs,
        iterationRounds: rounds,
        steps:
          typeof rec.execution_metadata === 'object' && rec.execution_metadata && 'steps' in rec
            ? ((rec as unknown as { steps?: number }).steps ?? list.length)
            : list.length,
      } as WorkflowRunNodeListItem;
    }

    return item;
  });
}

export function buildWorkflowRunHistoryResult(
  historyDetail: WorkflowRunDetail | null | undefined
): HistoryResult {
  const outputs = historyDetail?.outputs as unknown;

  if (outputs === null || outputs === undefined) {
    return { kind: 'empty' };
  }

  if (typeof outputs === 'string') {
    return { kind: 'text', content: outputs };
  }

  if (typeof outputs === 'number' || typeof outputs === 'boolean') {
    return { kind: 'text', content: String(outputs) };
  }

  if (typeof outputs === 'object' && outputs) {
    const record = outputs as Record<string, unknown>;
    const keys = Object.keys(record);

    if (keys.length === 1) {
      const onlyKey = keys[0];
      const value = record[onlyKey];

      if (typeof value === 'string') {
        return { kind: 'text', content: value };
      }

      if (Array.isArray(value) && value.every(item => typeof item === 'string')) {
        return { kind: 'text', content: (value as string[]).join('\n\n') };
      }
    }

    return { kind: 'json', value: outputs };
  }

  return { kind: 'empty' };
}

export function isSuccessfulWorkflowRunStatus(status: WorkflowRunStatus | string): boolean {
  const value = status.toLowerCase();
  return (
    value === 'succeeded' ||
    value === 'success' ||
    value === 'completed' ||
    value === 'partial-succeeded'
  );
}
