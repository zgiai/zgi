import type { RuntimeLabel, RuntimeLogPreviewRow, RuntimeLogSection, WorkflowRunNodeGroup, WorkflowRunNodeListItem, NodeRunStatus } from './types';

export const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

export const isEmptyValue = (value: unknown): boolean => {
  if (value === null || value === undefined || value === '') return true;
  if (Array.isArray(value)) return value.length === 0;
  if (isRecord(value)) return Object.keys(value).length === 0;
  return false;
};

export const normalizeNodeRunStatus = (status: unknown): NodeRunStatus => {
  switch (String(status ?? '').trim().toLowerCase()) {
    case 'success':
    case 'succeeded':
    case 'completed':
      return 'succeeded';
    case 'failed':
    case 'error':
      return 'failed';
    case 'stopped':
      return 'stopped';
    case 'paused':
      return 'paused';
    case 'running':
    default:
      return 'running';
  }
};

export const getNodesElapsedTime = (nodes: WorkflowRunNodeListItem[] | undefined): number | undefined => {
  const total = (nodes ?? []).reduce((sum, node) => sum + (node.elapsedTime ?? 0), 0);
  return total > 0 ? total : undefined;
};

export const pickRecordKeys = (value: unknown, keys: string[]): Record<string, unknown> | undefined => {
  if (!isRecord(value)) return undefined;
  const result: Record<string, unknown> = {};
  keys.forEach(key => {
    if (!isEmptyValue(value[key])) result[key] = value[key];
  });
  return Object.keys(result).length > 0 ? result : undefined;
};

export const groupWorkflowRunItems = (items: WorkflowRunNodeListItem[]): WorkflowRunNodeGroup[] => {
  const order: string[] = [];
  const groups = new Map<string, WorkflowRunNodeListItem[]>();

  items.forEach(item => {
    const key = item.nodeId || item.executionId || item.title;
    if (!groups.has(key)) {
      groups.set(key, []);
      order.push(key);
    }
    groups.get(key)?.push(item);
  });

  return order.map(key => ({
    key,
    executions: normalizeWorkflowRunGroupExecutions(groups.get(key) ?? []),
  }));
};

const shouldMergeWorkflowNodeExecutions = (item: WorkflowRunNodeListItem | undefined): boolean =>
  item?.nodeType === 'approval' || item?.nodeType === 'question-answer';

const mergeWorkflowRunExecutionItems = (
  executions: WorkflowRunNodeListItem[]
): WorkflowRunNodeListItem[] => {
  if (executions.length <= 1 || !shouldMergeWorkflowNodeExecutions(executions[0])) {
    return executions;
  }
  const [first, ...rest] = executions;
  const merged = rest.reduce<WorkflowRunNodeListItem>(
    (current, item) => ({
      ...current,
      ...item,
      status: normalizeNodeRunStatus(item.status),
      error: item.error ?? current.error,
      elapsedTime: item.elapsedTime ?? current.elapsedTime,
      nodeInput: item.nodeInput ?? current.nodeInput,
      nodeOutput: item.nodeOutput ?? current.nodeOutput,
      modelInput: item.modelInput ?? current.modelInput,
      processData: item.processData ?? current.processData,
      executionMetadata: item.executionMetadata ?? current.executionMetadata,
    }),
    first
  );
  return [merged];
};

const normalizeWorkflowRunGroupExecutions = (
  executions: WorkflowRunNodeListItem[]
): WorkflowRunNodeListItem[] => mergeWorkflowRunExecutionItems(executions);

export const getNodeSummary = (
  item: WorkflowRunNodeListItem,
  runtimeLabel: RuntimeLabel
): string | null => {
  const input = isRecord(item.nodeInput) ? item.nodeInput : undefined;
  const output = isRecord(item.nodeOutput) ? item.nodeOutput : undefined;
  const meta = isRecord(item.executionMetadata) ? item.executionMetadata : undefined;

  if (item.nodeType === 'llm') {
    const model =
      meta?.resolved_model_name ??
      pickRecordKeys(item.modelInput, ['model'])?.model ??
      input?.model;
    return typeof model === 'string' ? model : null;
  }
  if (item.nodeType === 'knowledge-retrieval') {
    const topK = input?.top_k ?? input?.topK;
    return topK !== undefined ? runtimeLabel('topKSummary', { count: String(topK) }) : null;
  }
  if (item.nodeType === 'iteration') {
    const current = meta?.iteration_index;
    const total = item.steps;
    if (typeof current === 'number' && typeof total === 'number') {
      return runtimeLabel('batchSummary', { current: current + 1, total });
    }
  }
  if (item.nodeType === 'loop') {
    const round = meta?.loop_index;
    if (typeof round === 'number') return runtimeLabel('roundSummary', { round: round + 1 });
  }
  if (item.nodeType === 'variable-aggregator' && input) {
    const count = Object.keys(input).length;
    return count > 0 ? runtimeLabel('mergeSummary', { count }) : null;
  }
  if (output?.status_code !== undefined) return `HTTP ${String(output.status_code)}`;
  return null;
};

export const getRuntimeLogSections = (
  item: WorkflowRunNodeListItem,
  runtimeLabel: RuntimeLabel
): RuntimeLogSection[] => {
  const sections: RuntimeLogSection[] = [];
  const meta = item.executionMetadata;
  const processData = item.processData;

  const push = (id: string, title: string, value: unknown, accent: string) => {
    if (!isEmptyValue(value)) sections.push({ id, title, value, accent });
  };

  if (item.nodeType === 'llm') {
    const output = isRecord(item.nodeOutput) ? item.nodeOutput : undefined;
    push('result', runtimeLabel('nodeOutput'), output?.text ?? item.nodeOutput, 'bg-success/50');
    push(
      'model',
      runtimeLabel('modelInfo'),
      pickRecordKeys(meta, [
        'resolved_model_provider',
        'resolved_model_name',
        'total_tokens',
        'total_price',
        'currency',
      ]),
      'bg-primary/50'
    );
    push('prompt', runtimeLabel('modelRequest'), item.modelInput, 'bg-primary/50');
  } else {
    push('input', runtimeLabel('input'), item.nodeInput, 'bg-info/50');
    push('output', runtimeLabel('output'), item.nodeOutput, 'bg-success/50');
  }

  if (item.nodeType === 'if-else') {
    push(
      'conditions',
      runtimeLabel('conditions'),
      pickRecordKeys(item.nodeInput, ['cases', 'conditions']),
      'bg-info/50'
    );
    push('comparisons', runtimeLabel('comparisons'), processData, 'bg-warning/60');
  }
  if (item.nodeType === 'iteration') {
    push(
      'batch',
      runtimeLabel('batchInfo'),
      pickRecordKeys(meta, ['iteration_index', 'iteration_item', 'iteration_id']),
      'bg-primary/50'
    );
  }
  if (item.nodeType === 'loop') {
    push(
      'loop',
      runtimeLabel('loopInfo'),
      pickRecordKeys(meta, ['loop_index', 'loop_id', 'loop_variable_map']),
      'bg-primary/50'
    );
    push(
      'loopConditions',
      runtimeLabel('loopConditions'),
      pickRecordKeys(item.nodeInput, ['break_conditions']),
      'bg-warning/60'
    );
  }

  push('process', runtimeLabel('processInfo'), processData, 'bg-warning/60');
  push('metadata', runtimeLabel('executionMetadata'), meta, 'bg-muted-foreground/40');

  return sections;
};

export const serializeForClipboard = (value: unknown): string => {
  if (typeof value === 'string') return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
};

export const humanizeKey = (key: string): string =>
  key
    .replace(/_/g, ' ')
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .trim();

const hiddenReadableRuntimeKeys = new Set([
  'conversation_id',
  'dialogue_count',
  'execution_id',
  'node_id',
  'tenant_id',
  'user_id',
  'workflow_id',
  'workflow_run_id',
  'workspace_id',
  'sys.conversation_id',
  'sys.dialogue_count',
  'sys.user_id',
  'sys.workflow_id',
  'sys.workflow_run_id',
  'sys.workspace_id',
  'sys.tenant_id',
]);

export const isHiddenReadableRuntimeKey = (key: string): boolean =>
  key.startsWith('sys.') ? key !== 'sys.query' : hiddenReadableRuntimeKeys.has(key);

export const getReadableRuntimeKey = (key: string, runtimeLabel: RuntimeLabel): string => {
  const runtimeKeyMap: Record<string, string> = {
    'sys.query': 'userQuestion',
    'sys.conversation_id': 'conversationId',
    'sys.dialogue_count': 'dialogueCount',
  };
  const labelKey = runtimeKeyMap[key];
  if (labelKey) return runtimeLabel(labelKey);
  return humanizeKey(key).replace(/\./g, ' ');
};

export const getReadableRecordEntries = (
  value: Record<string, unknown>,
  runtimeLabel: RuntimeLabel
): Array<[string, string, unknown]> =>
  Object.entries(value)
    .filter(([key, entryValue]) => !isHiddenReadableRuntimeKey(key) && !isEmptyValue(entryValue))
    .map(([key, entryValue]) => [key, getReadableRuntimeKey(key, runtimeLabel), entryValue]);

export const getByPath = (value: unknown, path: string[]): unknown => {
  let current = value;
  for (const key of path) {
    if (!isRecord(current)) return undefined;
    current = current[key];
  }
  return current;
};

export const compactValue = (value: unknown, runtimeLabel: RuntimeLabel, maxLength = 180): string => {
  if (value === null || value === undefined || value === '') return runtimeLabel('noValue');
  if (typeof value === 'string') {
    const normalized = value.replace(/\s+/g, ' ').trim();
    return normalized.length > maxLength ? `${normalized.slice(0, maxLength)}...` : normalized;
  }
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  if (Array.isArray(value)) {
    if (value.length === 0) return runtimeLabel('emptyArray');
    const first = compactValue(value[0], runtimeLabel, 80);
    return value.length === 1
      ? first
      : runtimeLabel('arrayPreview', { count: value.length, first });
  }
  if (isRecord(value)) {
    const readableEntries = getReadableRecordEntries(value, runtimeLabel);
    if (readableEntries.length === 0) return runtimeLabel('emptyObject');
    const simpleEntries = readableEntries
      .slice(0, 3)
      .map(([, label, entryValue]) => `${label}: ${compactValue(entryValue, runtimeLabel, 48)}`);
    return simpleEntries.length > 0
      ? simpleEntries.join(' · ')
      : runtimeLabel('fieldCount', { count: readableEntries.length });
  }
  return String(value);
};

export const firstAvailableValue = (sources: unknown[]): unknown =>
  sources.find(value => !isEmptyValue(value));

export const getCanvasPreviewRows = (
  item: WorkflowRunNodeListItem,
  runtimeLabel: RuntimeLabel
): RuntimeLogPreviewRow[] => {
  const input = isRecord(item.nodeInput) ? item.nodeInput : undefined;
  const output = isRecord(item.nodeOutput) ? item.nodeOutput : undefined;
  const meta = isRecord(item.executionMetadata) ? item.executionMetadata : undefined;
  const modelInput = isRecord(item.modelInput) ? item.modelInput : undefined;
  const rows: RuntimeLogPreviewRow[] = [];
  const push = (label: string, value: unknown, tone?: RuntimeLogPreviewRow['tone']) => {
    if (!isEmptyValue(value) && rows.length < 4) rows.push({ label, value, tone });
  };

  if (item.error) {
    push(runtimeLabel('error'), item.error, 'warning');
    return rows;
  }

  switch (item.nodeType) {
    case 'start':
      push(
        runtimeLabel('userInput'),
        firstAvailableValue([input?.query, input?.question, input?.message, input]),
        'input'
      );
      break;
    case 'llm':
      push(
        runtimeLabel('reply'),
        firstAvailableValue([output?.text, output?.answer, output?.result, item.nodeOutput]),
        'output'
      );
      push(
        runtimeLabel('model'),
        firstAvailableValue([
          meta?.resolved_model_name,
          getByPath(modelInput, ['model']),
          getByPath(item.modelInput, ['model_config', 'model']),
        ]),
        'meta'
      );
      push(
        runtimeLabel('tokenCount'),
        firstAvailableValue([
          meta?.total_tokens,
          getByPath(item.nodeOutput, ['usage', 'total_tokens']),
        ]),
        'meta'
      );
      break;
    case 'answer':
    case 'end':
      push(
        runtimeLabel('replyContent'),
        firstAvailableValue([output?.answer, output?.text, output?.result, item.nodeOutput]),
        'output'
      );
      break;
    case 'knowledge-retrieval':
      push(
        runtimeLabel('query'),
        firstAvailableValue([input?.query, input?.keyword, input]),
        'input'
      );
      push(
        runtimeLabel('retrievalResults'),
        firstAvailableValue([output?.docs, output?.documents, item.nodeOutput]),
        'output'
      );
      break;
    case 'http-request':
      push(
        runtimeLabel('request'),
        firstAvailableValue([input?.url, input?.method, item.nodeInput]),
        'input'
      );
      push(
        runtimeLabel('response'),
        firstAvailableValue([output?.status_code, output?.body, item.nodeOutput]),
        'output'
      );
      break;
    case 'if-else':
      push(
        runtimeLabel('matchedBranch'),
        firstAvailableValue([output?.branch, output?.result, item.nodeOutput]),
        'output'
      );
      push(runtimeLabel('decisionBasis'), item.processData, 'meta');
      break;
    case 'iteration':
      push(runtimeLabel('currentBatch'), meta?.iteration_index, 'meta');
      push(
        runtimeLabel('currentItem'),
        firstAvailableValue([meta?.iteration_item, input?.item, item.nodeInput]),
        'input'
      );
      push(runtimeLabel('batchOutput'), item.nodeOutput, 'output');
      break;
    case 'loop':
      push(runtimeLabel('currentRound'), meta?.loop_index, 'meta');
      push(
        runtimeLabel('loopVariables'),
        firstAvailableValue([meta?.loop_variable_map, item.loopInputs, item.nodeInput]),
        'input'
      );
      push(
        runtimeLabel('loopOutput'),
        firstAvailableValue([item.loopOutputs, item.nodeOutput]),
        'output'
      );
      break;
    case 'announcement':
      {
        const announcementOutput = pickRecordKeys(item.nodeOutput, [
          'title',
          'content',
          'expiration_time',
          'url',
          'token',
        ]);
        if (announcementOutput) {
          rows.push({
            label: runtimeLabel('output'),
            value: announcementOutput,
            tone: 'output',
            maxRecordEntries: 5,
          });
        }
      }
      break;
    default:
      push(runtimeLabel('input'), item.nodeInput, 'input');
      push(runtimeLabel('output'), item.nodeOutput, 'output');
      push(runtimeLabel('process'), item.processData, 'meta');
      break;
  }

  return rows;
};

export const previewToneClass: Record<NonNullable<RuntimeLogPreviewRow['tone']>, string> = {
  input: 'border-info/15 bg-info/[0.04]',
  output: 'border-success/15 bg-success/[0.05]',
  meta: 'border-primary/15 bg-primary/[0.04]',
  warning: 'border-destructive/20 bg-destructive/[0.04]',
};

