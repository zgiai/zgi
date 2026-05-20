/**
 * @util extractLlmGatewayRequest
 * @description Extract the LLM gateway request payload from workflow node event/process data objects.
 */
export function extractLlmGatewayRequest(source: unknown): unknown {
  const record = toRecord(source);
  if (!record) return undefined;

  const processData = toRecord(record['process_data']) ?? record;
  return processData?.['llm_gateway_request'];
}

export interface WorkflowRunContainerContext {
  iterationId?: string;
  iterationIndex?: number;
  loopId?: string;
  loopIndex?: number;
}

export interface WorkflowRunOrderedItem {
  createdAtMs?: number;
  receivedOrder?: number;
  elapsedTime?: number;
}

export interface WorkflowRunExecutionItem extends WorkflowRunOrderedItem {
  executionId?: string;
  nodeId?: string;
  nodeType?: string;
  title?: string;
}

/**
 * @util extractWorkflowRunContainerContext
 * @description Extract container ownership from a workflow SSE node payload.
 */
export function extractWorkflowRunContainerContext(source: unknown): WorkflowRunContainerContext {
  const record = toRecord(source);
  const execMeta = toRecord(record?.['execution_metadata']);

  return {
    iterationId: pickString(record, 'iteration_id') ?? pickString(execMeta, 'iteration_id'),
    iterationIndex:
      pickNumber(record, 'iteration_index') ?? pickNumber(execMeta, 'iteration_index'),
    loopId: pickString(record, 'loop_id') ?? pickString(execMeta, 'loop_id'),
    loopIndex: pickNumber(record, 'loop_index') ?? pickNumber(execMeta, 'loop_index'),
  };
}

/**
 * @util getWorkflowRunExecutionId
 * @description Return the stable node execution instance id from an SSE payload.
 */
export function getWorkflowRunExecutionId(source: unknown): string | undefined {
  const record = toRecord(source);
  return (
    pickString(record, 'execution_id') ??
    pickNumberString(record, 'execution_id') ??
    pickString(record, 'id') ??
    pickNumberString(record, 'id')
  );
}

/**
 * @util getWorkflowRunCreatedAtMs
 * @description Return a comparable millisecond timestamp from an SSE payload when available.
 */
export function getWorkflowRunCreatedAtMs(source: unknown): number | undefined {
  const record = toRecord(source);
  const createdAtMs = pickNumber(record, 'created_at_ms');
  if (typeof createdAtMs === 'number') return createdAtMs;

  const createdAt = pickNumber(record, 'created_at');
  if (typeof createdAt !== 'number') return undefined;
  return createdAt > 1_000_000_000_000 ? createdAt : createdAt * 1000;
}

/**
 * @util getWorkflowRunItemKey
 * @description Build a stable matching key for node executions in parallel container rounds.
 */
export function getWorkflowRunItemKey(item: WorkflowRunExecutionItem): string {
  return item.executionId || item.nodeId || [item.nodeType, item.title].filter(Boolean).join('|');
}

/**
 * @util sortWorkflowRunItems
 * @description Sort workflow run items by backend timestamp, then frontend receive order.
 */
export function sortWorkflowRunItems<T extends WorkflowRunOrderedItem>(items: T[]): T[] {
  return [...items].sort((a, b) => {
    const aTime = a.createdAtMs;
    const bTime = b.createdAtMs;
    if (typeof aTime === 'number' && typeof bTime === 'number' && aTime !== bTime) {
      return aTime - bTime;
    }
    if (typeof aTime === 'number' && typeof bTime !== 'number') return -1;
    if (typeof aTime !== 'number' && typeof bTime === 'number') return 1;

    const aOrder = a.receivedOrder ?? 0;
    const bOrder = b.receivedOrder ?? 0;
    return aOrder - bOrder;
  });
}

/**
 * @util sortWorkflowRunRounds
 * @description Sort container rounds by their iteration or loop index.
 */
export function sortWorkflowRunRounds<T extends { index: number }>(rounds: T[]): T[] {
  return [...rounds].sort((a, b) => a.index - b.index);
}

/**
 * @util getWorkflowRunRoundElapsedTime
 * @description Resolve a container round duration from the sum of child node durations.
 */
export function getWorkflowRunRoundElapsedTime(round: { nodes: WorkflowRunOrderedItem[] }): number {
  return round.nodes.reduce(
    (total, node) => total + (typeof node.elapsedTime === 'number' ? node.elapsedTime : 0),
    0
  );
}

function toRecord(value: unknown): Record<string, unknown> | undefined {
  if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }

  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed) return undefined;
    try {
      return toRecord(JSON.parse(trimmed));
    } catch {
      return undefined;
    }
  }

  return undefined;
}

function pickString(record: Record<string, unknown> | undefined, key: string): string | undefined {
  const value = record?.[key];
  return typeof value === 'string' ? value : undefined;
}

function pickNumber(record: Record<string, unknown> | undefined, key: string): number | undefined {
  const value = record?.[key];
  return typeof value === 'number' ? value : undefined;
}

function pickNumberString(
  record: Record<string, unknown> | undefined,
  key: string
): string | undefined {
  const value = record?.[key];
  return typeof value === 'number' ? String(value) : undefined;
}
