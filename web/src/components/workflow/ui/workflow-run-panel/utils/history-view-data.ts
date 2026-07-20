import type { WorkflowRunNodeListItem } from '@/components/workflow/ui/workflow-run-nodes-list';
import type {
  WorkflowNodeExecution,
  WorkflowRunDetail,
  WorkflowRunStatus,
} from '@/services/types/workflow';
import {
  extractLlmGatewayRequest,
  extractWorkflowRunContainerContext,
  getWorkflowRunCreatedAtMs,
  getWorkflowRunRoundDurationMap,
  getWorkflowRunRoundElapsedTime,
  sortWorkflowRunItems,
  sortWorkflowRunRounds,
} from '@/utils/workflow/run-events';
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

  interface ExecutionRecord {
    rec: WorkflowNodeExecution;
    item: WorkflowRunNodeListItem;
    owner?: { kind: 'iteration' | 'loop'; id: string; index: number };
  }
  const executionIdCounts = new Map<string, number>();

  const records: ExecutionRecord[] = historyNodeExecutions.map(rec => {
    const status = normalizeExecutionStatus(rec.status);
    const baseExecutionId = rec.node_execution_id || rec.id || rec.node_id;
    const occurrence = executionIdCounts.get(baseExecutionId) ?? 0;
    executionIdCounts.set(baseExecutionId, occurrence + 1);

    const item: WorkflowRunNodeListItem = {
      executionId:
        occurrence === 0 ? baseExecutionId : `${baseExecutionId}:${status}:${occurrence}`,
      createdAtMs:
        getWorkflowRunCreatedAtMs(rec) ??
        (typeof rec.created_at === 'string' ? Date.parse(rec.created_at) || undefined : undefined),
      receivedOrder: typeof rec.sequence === 'number' ? rec.sequence : occurrence,
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
    const context = extractWorkflowRunContainerContext(rec);
    const candidateOwner =
      context.loopId && typeof context.loopIndex === 'number'
        ? { kind: 'loop' as const, id: context.loopId, index: context.loopIndex }
        : context.iterationId && typeof context.iterationIndex === 'number'
          ? { kind: 'iteration' as const, id: context.iterationId, index: context.iterationIndex }
          : undefined;
    const owner =
      candidateOwner?.id === rec.node_id && candidateOwner.kind === item.nodeType
        ? undefined
        : candidateOwner;
    return { rec, item, owner };
  });

  const containerKeys = new Set(
    records
      .filter(record => record.item.nodeType === 'iteration' || record.item.nodeType === 'loop')
      .map(record => `${record.item.nodeType}:${record.item.nodeId}`)
  );
  const childrenByContainer = new Map<string, Map<number, ExecutionRecord[]>>();
  for (const record of records) {
    if (!record.owner) continue;
    const ownerKey = `${record.owner.kind}:${record.owner.id}`;
    if (!containerKeys.has(ownerKey)) continue;
    const rounds = childrenByContainer.get(ownerKey) ?? new Map<number, ExecutionRecord[]>();
    const children = rounds.get(record.owner.index) ?? [];
    children.push(record);
    rounds.set(record.owner.index, children);
    childrenByContainer.set(ownerKey, rounds);
  }

  const decorate = (record: ExecutionRecord): WorkflowRunNodeListItem => {
    const { item, rec } = record;
    const kind = item.nodeType === 'loop' ? 'loop' : item.nodeType === 'iteration' ? 'iteration' : null;
    if (!kind) return item;

    const roundRecords =
      childrenByContainer.get(`${kind}:${item.nodeId}`) ??
      new Map<number, ExecutionRecord[]>();
    const roundIndexes = new Set(roundRecords.keys());
    if (kind === 'iteration') {
      const outputs = rec.outputs;
      if (outputs && typeof outputs === 'object') {
        const list = (outputs as Record<string, unknown>).output;
        if (Array.isArray(list)) list.forEach((_, index) => roundIndexes.add(index));
      }
    }
    const durationMap = getWorkflowRunRoundDurationMap(rec, kind);
    const variableMap =
      kind === 'loop' && rec.execution_metadata && typeof rec.execution_metadata === 'object'
        ? (rec.execution_metadata.loop_variable_map as Record<string, unknown> | undefined)
        : undefined;
    const rounds = sortWorkflowRunRounds(
      [...roundIndexes].map(index => {
        const nodes = sortWorkflowRunItems(
          (roundRecords.get(index) ?? []).map(child => decorate(child))
        );
        return {
          index,
          nodes,
          elapsedTime: durationMap.get(index) ?? getWorkflowRunRoundElapsedTime({ nodes }),
          variables: variableMap?.[String(index)],
        };
      })
    );

    if (kind === 'iteration') {
      return {
        ...item,
        iterationInputs: rec.inputs,
        iterationOutputs: rec.outputs,
        iterationRounds: rounds.map(({ variables: _variables, ...round }) => round),
        steps: rounds.length,
      };
    }
    return {
      ...item,
      loopInputs: rec.inputs,
      loopOutputs: rec.outputs,
      loopRounds: rounds,
      steps: rounds.length,
    };
  };

  return records
    .filter(record => {
      if (!record.owner) return true;
      return !containerKeys.has(`${record.owner.kind}:${record.owner.id}`);
    })
    .map(decorate);
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
