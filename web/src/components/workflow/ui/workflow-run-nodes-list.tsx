import React, { useMemo, useState } from 'react';
import { NODE_THEMES } from '@/components/workflow/nodes/custom/config';
import { cn } from '@/lib/utils';
import { AlertTriangle, Check, ChevronDown, ChevronLeft, Copy, Filter, Loader } from 'lucide-react';
import JsonView from '@uiw/react-json-view';
import { lightTheme } from '@uiw/react-json-view/light';
import { useT } from '@/i18n';
import { formatMs } from '@/utils/format';
import MarkdownViewer from '@/components/common/markdown-viewer';
// Unified status for rendering
export type NodeRunStatus = 'running' | 'succeeded' | 'failed' | 'stopped' | 'paused';

// New item shape (pure presentational data)
export interface WorkflowRunNodeListItem {
  title: string;
  nodeId: string;
  executionId?: string;
  createdAtMs?: number;
  receivedOrder?: number;
  nodeType: string; // prefer keyof typeof NODE_THEMES, but allow string fallback for forward compatibility
  status: NodeRunStatus;
  nodeInput?: unknown;
  nodeOutput?: unknown;
  modelInput?: unknown;
  processData?: unknown;
  executionMetadata?: unknown;
  elapsedTime?: number;
  error?: string | null;
  iterationInputs?: unknown;
  iterationOutputs?: unknown;
  iterationRounds?: Array<{
    index: number;
    nodes: WorkflowRunNodeListItem[];
    elapsedTime?: number;
  }>;
  loopInputs?: unknown;
  loopOutputs?: unknown;
  loopRounds?: Array<{
    index: number;
    nodes: WorkflowRunNodeListItem[];
    elapsedTime?: number;
    variables?: unknown;
  }>;
  steps?: number;
}

// Component props: accept new shape array
interface WorkflowRunNodesListProps {
  items: WorkflowRunNodeListItem[];
  showDetail?: boolean;
  variant?: 'panel' | 'canvas';
  hideCanvasNodeChrome?: boolean;
}

interface WorkflowRunNodeGroup {
  key: string;
  executions: WorkflowRunNodeListItem[];
}

interface RuntimeLogSection {
  id: string;
  title: string;
  value: unknown;
  accent: string;
}

interface RuntimeLogPreviewRow {
  label: string;
  value: unknown;
  tone?: 'input' | 'output' | 'meta' | 'warning';
}

type RuntimeLabel = (key: string, params?: Record<string, string | number>) => string;

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

const isEmptyValue = (value: unknown): boolean => {
  if (value === null || value === undefined || value === '') return true;
  if (Array.isArray(value)) return value.length === 0;
  if (isRecord(value)) return Object.keys(value).length === 0;
  return false;
};

const pickRecordKeys = (value: unknown, keys: string[]): Record<string, unknown> | undefined => {
  if (!isRecord(value)) return undefined;
  const result: Record<string, unknown> = {};
  keys.forEach(key => {
    if (!isEmptyValue(value[key])) result[key] = value[key];
  });
  return Object.keys(result).length > 0 ? result : undefined;
};

const groupWorkflowRunItems = (items: WorkflowRunNodeListItem[]): WorkflowRunNodeGroup[] => {
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

  return order.map(key => ({ key, executions: groups.get(key) ?? [] }));
};

const getNodeSummary = (
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

const getRuntimeLogSections = (
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

const serializeForClipboard = (value: unknown): string => {
  if (typeof value === 'string') return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
};

const humanizeKey = (key: string): string =>
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

const isHiddenReadableRuntimeKey = (key: string): boolean =>
  key.startsWith('sys.') ? key !== 'sys.query' : hiddenReadableRuntimeKeys.has(key);

const getReadableRuntimeKey = (key: string, runtimeLabel: RuntimeLabel): string => {
  const runtimeKeyMap: Record<string, string> = {
    'sys.query': 'userQuestion',
    'sys.conversation_id': 'conversationId',
    'sys.dialogue_count': 'dialogueCount',
  };
  const labelKey = runtimeKeyMap[key];
  if (labelKey) return runtimeLabel(labelKey);
  return humanizeKey(key).replace(/\./g, ' ');
};

const getReadableRecordEntries = (
  value: Record<string, unknown>,
  runtimeLabel: RuntimeLabel
): Array<[string, string, unknown]> =>
  Object.entries(value)
    .filter(([key, entryValue]) => !isHiddenReadableRuntimeKey(key) && !isEmptyValue(entryValue))
    .map(([key, entryValue]) => [key, getReadableRuntimeKey(key, runtimeLabel), entryValue]);

const getByPath = (value: unknown, path: string[]): unknown => {
  let current = value;
  for (const key of path) {
    if (!isRecord(current)) return undefined;
    current = current[key];
  }
  return current;
};

const compactValue = (value: unknown, runtimeLabel: RuntimeLabel, maxLength = 180): string => {
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

const firstAvailableValue = (sources: unknown[]): unknown =>
  sources.find(value => !isEmptyValue(value));

const getCanvasPreviewRows = (
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
    default:
      push(runtimeLabel('input'), item.nodeInput, 'input');
      push(runtimeLabel('output'), item.nodeOutput, 'output');
      push(runtimeLabel('process'), item.processData, 'meta');
      break;
  }

  return rows;
};

const previewToneClass: Record<NonNullable<RuntimeLogPreviewRow['tone']>, string> = {
  input: 'border-info/15 bg-info/[0.04]',
  output: 'border-success/15 bg-success/[0.05]',
  meta: 'border-primary/15 bg-primary/[0.04]',
  warning: 'border-destructive/20 bg-destructive/[0.04]',
};

const RuntimeValuePreview: React.FC<{
  value: unknown;
  lines?: number;
  expandable?: boolean;
  runtimeLabel: RuntimeLabel;
}> = ({ value, lines = 2, expandable = false, runtimeLabel }) => {
  const [expanded, setExpanded] = useState(false);
  const textValue =
    typeof value === 'string'
      ? value
      : typeof value === 'number' || typeof value === 'boolean'
        ? String(value)
        : null;
  const normalizedTextValue = textValue?.replace(/\s+/g, ' ').trim();
  const canExpand = Boolean(expandable && normalizedTextValue && normalizedTextValue.length > 90);

  if (isRecord(value)) {
    const entries = getReadableRecordEntries(value, runtimeLabel).slice(0, 3);
    return (
      <div className="grid gap-1">
        {entries.map(([key, label, entryValue]) => (
          <div key={key} className="flex min-w-0 gap-1.5">
            <span className="shrink-0 text-muted-foreground/70">{label}:</span>
            <span className="min-w-0 truncate text-foreground/80">
              {compactValue(entryValue, runtimeLabel, expanded ? 500 : 72)}
            </span>
          </div>
        ))}
      </div>
    );
  }

  return (
    <div className="grid gap-1">
      <div
        className="break-words text-foreground/85"
        style={
          expanded
            ? { whiteSpace: 'pre-wrap' }
            : {
                display: '-webkit-box',
                WebkitLineClamp: lines,
                WebkitBoxOrient: 'vertical',
                overflow: 'hidden',
              }
        }
      >
        {expanded && textValue ? textValue : compactValue(value, runtimeLabel)}
      </div>
      {canExpand ? (
        <button
          type="button"
          className="w-fit rounded px-1.5 py-0.5 text-[10px] font-medium text-primary transition-colors hover:bg-primary/10"
          onClick={event => {
            event.stopPropagation();
            setExpanded(prev => !prev);
          }}
        >
          {expanded ? runtimeLabel('collapse') : runtimeLabel('expandAll')}
        </button>
      ) : null}
    </div>
  );
};

const RuntimeStructuredView: React.FC<{ value: unknown; runtimeLabel: RuntimeLabel }> = ({
  value,
  runtimeLabel,
}) => {
  const [showRaw, setShowRaw] = useState(false);

  if (typeof value === 'string') {
    return (
      <div className="whitespace-pre-wrap break-words px-3 py-2 text-[12px] leading-5 text-foreground/85">
        {value}
      </div>
    );
  }

  const renderReadableValue = () => {
    if (Array.isArray(value)) {
      return (
        <div className="grid gap-1.5 px-2 py-2">
          <div className="text-[11px] text-muted-foreground">
            {runtimeLabel('arrayCount', { count: value.length })}
          </div>
          {value.slice(0, 8).map((entry, index) => (
            <div key={index} className="rounded-md bg-background/70 px-2 py-1.5 text-[12px]">
              <RuntimeValuePreview value={entry} lines={3} runtimeLabel={runtimeLabel} />
            </div>
          ))}
          {value.length > 8 ? (
            <div className="px-1 text-[11px] text-muted-foreground">
              {runtimeLabel('arrayMore', { count: value.length - 8 })}
            </div>
          ) : null}
        </div>
      );
    }

    if (isRecord(value)) {
      const entries = getReadableRecordEntries(value, runtimeLabel);
      if (entries.length === 0) {
        return (
          <div className="px-3 py-2 text-[12px] leading-5 text-muted-foreground">
            {runtimeLabel('technicalFieldsHidden')}
          </div>
        );
      }
      return (
        <div className="grid gap-1.5 px-2 py-2">
          {entries.slice(0, 12).map(([key, label, entryValue]) => (
            <div
              key={key}
              className="grid gap-1 rounded-md bg-background/70 px-2 py-1.5 text-[12px] leading-5"
            >
              <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
                {label}
              </div>
              <RuntimeValuePreview value={entryValue} lines={4} runtimeLabel={runtimeLabel} />
            </div>
          ))}
          {entries.length > 12 ? (
            <div className="px-1 text-[11px] text-muted-foreground">
              {runtimeLabel('fieldsMore', { count: entries.length - 12 })}
            </div>
          ) : null}
        </div>
      );
    }

    return (
      <div className="px-3 py-2 text-[12px] leading-5 text-foreground/85">
        {compactValue(value, runtimeLabel, 320)}
      </div>
    );
  };

  return (
    <div className="grid gap-1">
      {renderReadableValue()}
      <button
        type="button"
        className="mx-2 mb-2 w-fit rounded border border-border/50 bg-background px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        onClick={event => {
          event.stopPropagation();
          setShowRaw(prev => !prev);
        }}
      >
        {showRaw ? runtimeLabel('hideRawData') : runtimeLabel('rawData')}
      </button>
      {showRaw ? (
        <JsonView
          value={value ?? {}}
          style={{ ...lightTheme, background: 'transparent' }}
          className="border-t border-border/30 p-1 px-2.5 text-[11px]"
        />
      ) : null}
    </div>
  );
};

// Render a compact list of node execution states
const WorkflowRunNodesList: React.FC<WorkflowRunNodesListProps> = ({
  items,
  showDetail = true,
  variant = 'panel',
  hideCanvasNodeChrome = false,
}) => {
  const t = useT();
  const runtimeLabel: RuntimeLabel = (key, params) =>
    t(`agents.workflow.runtimeLog.${key}` as Parameters<typeof t>[0], params);
  const isCanvasVariant = variant === 'canvas';
  const isCanvasDetailOnly = isCanvasVariant && hideCanvasNodeChrome;
  const styles = {
    running: {
      wrap: 'border-l-4 border-l-info border border-border/50 bg-card',
      dot: 'bg-info animate-pulse',
      text: 'text-info',
      label: t('agents.workflow.running'),
    },
    succeeded: {
      wrap: 'border-l-4 border-l-success border border-border/50 bg-card',
      dot: 'bg-success',
      text: 'text-success',
      label: t('agents.workflow.succeeded'),
    },
    failed: {
      wrap: 'border-l-4 border-l-destructive border border-border/50 bg-card',
      dot: 'bg-destructive',
      text: 'text-destructive',
      label: t('agents.workflow.failed'),
    },
    stopped: {
      wrap: 'border-l-4 border-l-muted-foreground border border-border/50 bg-card',
      dot: 'bg-muted-foreground',
      text: 'text-muted-foreground',
      label: t('agents.workflow.stopped'),
    },
    paused: {
      wrap: 'border-l-4 border-l-warning border border-border/50 bg-card',
      dot: 'bg-warning',
      text: 'text-warning',
      label: t('agents.workflow.paused'),
    },
  };
  const [openSet, setOpenSet] = useState<Set<string>>(() => new Set());
  const [selectedExecutionByNode, setSelectedExecutionByNode] = useState<Record<string, number>>(
    {}
  );
  const [errorOnlyByNode, setErrorOnlyByNode] = useState<Record<string, boolean>>({});
  const [copiedSection, setCopiedSection] = useState<string | null>(null);
  const groupedItems = useMemo(() => groupWorkflowRunItems(items), [items]);
  const elapsedTimeClass =
    'text-[10px] text-muted-foreground/60 tabular-nums px-1 py-0.5 tracking-tighter';

  const isOpen = (id: string) => openSet.has(id);
  const toggleOpen = (id: string) =>
    setOpenSet(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  const copySectionValue = async (id: string, value: unknown, e?: React.MouseEvent) => {
    e?.stopPropagation();
    if (typeof navigator === 'undefined' || !navigator.clipboard) return;
    await navigator.clipboard.writeText(serializeForClipboard(value));
    setCopiedSection(id);
    window.setTimeout(() => setCopiedSection(current => (current === id ? null : current)), 1200);
  };
  const [sectionOpenSet, setSectionOpenSet] = useState<Set<string>>(() => new Set());
  const isSectionOpen = (id: string, defaultOpen = false) =>
    defaultOpen ? !sectionOpenSet.has(id) : sectionOpenSet.has(id);
  const toggleSection = (id: string, e?: React.MouseEvent) => {
    e?.stopPropagation();
    setSectionOpenSet(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };
  const [containerRoundsViewSet, setContainerRoundsViewSet] = useState<Set<string>>(
    () => new Set()
  );
  const isContainerRoundsView = (id: string) => containerRoundsViewSet.has(id);
  const enterContainerRoundsView = (id: string, e?: React.MouseEvent) => {
    e?.stopPropagation();
    setContainerRoundsViewSet(prev => {
      const next = new Set(prev);
      next.add(id);
      return next;
    });
  };
  const exitContainerRoundsView = (id: string, e?: React.MouseEvent) => {
    e?.stopPropagation();
    setContainerRoundsViewSet(prev => {
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
  };

  return (
    <div className={cn('relative', isCanvasDetailOnly ? 'space-y-1.5' : 'space-y-2')}>
      {groupedItems.map(group => {
        const errorOnly = Boolean(errorOnlyByNode[group.key]);
        const visibleExecutions = errorOnly
          ? group.executions.filter(item => item.status === 'failed' || Boolean(item.error))
          : group.executions;
        const executions = visibleExecutions;
        if (executions.length === 0) return null;
        const selectedIndex = Math.min(
          selectedExecutionByNode[group.key] ?? executions.length - 1,
          Math.max(0, executions.length - 1)
        );
        const raw = executions[selectedIndex];
        if (!raw) return null;
        const itemKey = group.key;
        const executionKey = raw.executionId ?? `${raw.nodeId}-${selectedIndex}`;
        const summary = getNodeSummary(raw, runtimeLabel);
        const sections = getRuntimeLogSections(raw, runtimeLabel);
        const previewRows = isCanvasVariant ? getCanvasPreviewRows(raw, runtimeLabel) : [];
        const theme =
          raw.nodeType in NODE_THEMES
            ? NODE_THEMES[raw.nodeType as keyof typeof NODE_THEMES]
            : undefined;
        const Icon = theme?.icon;

        const statusCfg = styles[raw.status];
        const isIteration = raw.nodeType === 'iteration';
        const isLoop = raw.nodeType === 'loop';
        const isContainer = isIteration || isLoop;
        const rounds = isLoop ? (raw.loopRounds ?? []) : (raw.iterationRounds ?? []);
        const inContainerRoundsView = isContainer && isContainerRoundsView(itemKey);
        const debugDetailsId = `${itemKey}-debug-details`;
        const hasDebugDetails =
          showDetail &&
          !inContainerRoundsView &&
          (raw.modelInput !== undefined ||
            raw.nodeInput !== undefined ||
            raw.loopInputs !== undefined ||
            !isEmptyValue(raw.processData) ||
            !isEmptyValue(raw.executionMetadata));
        const debugDetailsOpen = isSectionOpen(debugDetailsId);
        const canToggle =
          !isCanvasVariant &&
          (isContainer ||
            (showDetail &&
              (sections.length > 0 ||
                raw.nodeInput !== undefined ||
                raw.nodeOutput !== undefined ||
                raw.processData !== undefined ||
                raw.executionMetadata !== undefined)));
        const shouldRenderDetailsArea = isContainer
          ? isOpen(itemKey)
          : showDetail && isOpen(itemKey);

        return (
          <div
            key={`run-item-${itemKey}`}
            className={cn(
              'rounded-lg px-2 py-2 transition-all duration-500 relative z-10 border',
              isCanvasVariant && 'rounded-md border-border/70 bg-background shadow-sm',
              isCanvasDetailOnly && 'border-border/70 bg-background/95 p-2 shadow-none',
              statusCfg.wrap,
              isCanvasDetailOnly && 'border-l border-l-border/70',
              isCanvasDetailOnly
                ? 'border-border/70 hover:border-border/70'
                : isCanvasVariant
                ? 'border-border/70 hover:border-border/80 hover:bg-background hover:shadow-md'
                : isOpen(itemKey)
                  ? 'shadow-[0_4px_12px_rgba(0,0,0,0.06),0_0_1px_rgba(0,0,0,0.1)] bg-card border-border/80 scale-[1.01]'
                  : 'border-transparent hover:border-border/40 hover:bg-muted/10 hover:shadow-sm'
            )}
          >
            {!isCanvasDetailOnly ? (
              <div
                className={cn(
                  'flex items-center gap-2',
                  canToggle ? 'cursor-pointer select-none' : 'cursor-default'
                )}
                onClick={() => canToggle && toggleOpen(itemKey)}
              >
                {/* Toggle button on the left */}
                <div className="w-3 flex items-center justify-center shrink-0">
                  {canToggle ? (
                    <ChevronDown
                      className={cn(
                        'h-3.5 w-3.5 transition-transform text-muted-foreground/40 hover:text-foreground',
                        isOpen(itemKey) ? '' : '-rotate-90'
                      )}
                    />
                  ) : null}
                </div>

                {/* Icon wrap - smaller and more refined */}
                <div
                  className={cn(
                    'w-5 h-5 flex items-center justify-center rounded text-white shrink-0 shadow-[0_1px_2px_rgba(0,0,0,0.1)] transition-all duration-300',
                    theme?.classNames.iconBg,
                    raw.status === 'paused' && 'bg-warning text-white shadow-none',
                    isOpen(itemKey) ? 'ring-2 ring-background ring-offset-1 scale-105' : ''
                  )}
                  aria-label={raw.nodeType}
                >
                  {Icon ? <Icon className="w-3 h-3" /> : null}
                </div>

                {/* Title and status */}
                <div className="flex-1 min-w-0 ml-0.5">
                  <div
                    className={cn(
                      'text-[13px] font-semibold truncate tracking-tight text-foreground/80',
                      theme?.classNames.title
                    )}
                  >
                    {raw.title}
                  </div>
                  {summary ? (
                    <div className="mt-0.5 w-fit max-w-full truncate rounded bg-primary/5 px-1.5 py-0.5 text-[10px] font-medium text-primary">
                      {summary}
                    </div>
                  ) : null}
                </div>
                {isCanvasVariant && group.executions.length > 1 ? (
                  <div className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                    {runtimeLabel('executionCount', { count: group.executions.length })}
                  </div>
                ) : null}
                <div className="flex items-center gap-1.5">
                  <span
                    className={cn(
                      'w-1.5 h-1.5 rounded-full shrink-0 shadow-[0_0_4px_rgba(0,0,0,0.1)]',
                      statusCfg.dot,
                      raw.status === 'running' && 'animate-pulse-subtle'
                    )}
                  />
                  {raw.status === 'running' ? (
                    <Loader className="h-3 w-3 animate-spin text-info" />
                  ) : raw.status === 'paused' ? null : (
                    <div className={elapsedTimeClass}>
                      {formatMs(raw?.elapsedTime ? raw.elapsedTime : 0)}
                    </div>
                  )}
                </div>
              </div>
            ) : null}
            {isCanvasVariant && previewRows.length > 0 ? (
              <div className={cn('grid gap-1.5', isCanvasDetailOnly ? 'mt-0' : 'mt-2')}>
                {previewRows.map(row => (
                  <div
                    key={`${executionKey}-${row.label}`}
                    className={cn(
                      'rounded-md border px-2 py-1.5 text-[11px] leading-4',
                      row.tone ? previewToneClass[row.tone] : 'border-border/50 bg-muted/20'
                    )}
                  >
                    <div className="mb-0.5 text-[10px] font-medium text-muted-foreground">
                      {row.label}
                    </div>
                    <RuntimeValuePreview
                      value={row.value}
                      lines={
                        row.label === runtimeLabel('reply') ||
                        row.label === runtimeLabel('replyContent')
                          ? 3
                          : 2
                      }
                      expandable
                      runtimeLabel={runtimeLabel}
                    />
                  </div>
                ))}
              </div>
            ) : null}
            {!isCanvasVariant && group.executions.length > 1 ? (
              <div className="mt-2 flex flex-wrap items-center gap-2 rounded-md border border-border/40 bg-muted/20 px-2 py-1.5">
                <span className="text-[11px] text-muted-foreground">
                  {runtimeLabel('totalCount', { count: group.executions.length })}
                </span>
                <label
                  className="ml-auto inline-flex cursor-pointer select-none items-center gap-1.5 text-[11px] text-muted-foreground"
                  onClick={e => e.stopPropagation()}
                >
                  <input
                    type="checkbox"
                    className="h-3 w-3 rounded border-border"
                    checked={errorOnly}
                    onChange={event => {
                      const checked = event.currentTarget.checked;
                      setErrorOnlyByNode(prev => ({ ...prev, [group.key]: checked }));
                      setSelectedExecutionByNode(prev => ({ ...prev, [group.key]: 0 }));
                    }}
                  />
                  <Filter className="h-3 w-3" />
                  {runtimeLabel('errorsOnly')}
                </label>
                <div className="flex max-w-full flex-wrap gap-1">
                  {executions.map((execution, index) => (
                    <button
                      key={`${group.key}-execution-${execution.executionId ?? index}`}
                      type="button"
                      className={cn(
                        'h-6 min-w-6 rounded-md border px-2 text-[11px] tabular-nums transition-colors',
                        index === selectedIndex
                          ? 'border-primary bg-primary/10 text-primary'
                          : 'border-border/60 bg-background text-muted-foreground hover:bg-muted'
                      )}
                      onClick={event => {
                        event.stopPropagation();
                        setSelectedExecutionByNode(prev => ({ ...prev, [group.key]: index }));
                      }}
                    >
                      {index + 1}
                    </button>
                  ))}
                </div>
              </div>
            ) : null}
            {!isCanvasVariant && raw.error ? (
              <div
                className="mt-2 rounded-md border border-destructive/15 bg-destructive/[0.03] px-2.5 py-2"
                title={raw.error}
              >
                <div className="mb-1 flex items-center gap-1.5 text-[11px] font-medium text-destructive/80">
                  <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
                  <span>{t('agents.workflow.errors.executionFailed')}</span>
                </div>
                <p className="text-[12px] leading-5 text-foreground/75 break-words">{raw.error}</p>
              </div>
            ) : null}

            {shouldRenderDetailsArea && (
              <div className="mt-2.5 grid gap-2 pl-4.5 min-w-0">
                {/* LLM result section */}
                {(() => {
                  if (!showDetail || raw.nodeType !== 'llm') return null;
                  const output = raw.nodeOutput as { text?: string } | null | undefined;
                  const textResult = output?.text;
                  if (!textResult) return null;
                  const resultId = `${itemKey}-result`;
                  const resultOpen = isSectionOpen(resultId, true);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(resultId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-primary/40 rounded-full" />
                          {t('agents.workflow.results')}
                        </div>
                        <div className="flex items-center gap-1">
                          <button
                            type="button"
                            className="rounded p-1 text-muted-foreground/40 hover:bg-muted hover:text-foreground"
                            onClick={e => copySectionValue(resultId, textResult, e)}
                            aria-label={runtimeLabel('copyResult')}
                          >
                            {copiedSection === resultId ? (
                              <Check className="h-3 w-3" />
                            ) : (
                              <Copy className="h-3 w-3" />
                            )}
                          </button>
                          <ChevronDown
                            className={cn(
                              'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                              resultOpen ? '' : '-rotate-90'
                            )}
                          />
                        </div>
                      </div>
                      {resultOpen && (
                        <div className="bg-muted/10 rounded-md px-3 py-2 overflow-auto max-h-80 text-[12.5px] leading-relaxed text-foreground/90 border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <MarkdownViewer content={textResult} />
                        </div>
                      )}
                    </div>
                  );
                })()}

                {hasDebugDetails ? (
                  <button
                    type="button"
                    className="ml-1 inline-flex w-fit items-center gap-1 rounded-md border border-border/40 bg-muted/20 px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                    onClick={e => toggleSection(debugDetailsId, e)}
                  >
                    <ChevronDown
                      className={cn(
                        'h-3 w-3 transition-transform',
                        debugDetailsOpen ? '' : '-rotate-90'
                      )}
                    />
                    {runtimeLabel('debugDetails')}
                  </button>
                ) : null}

                {(() => {
                  if (
                    !showDetail ||
                    inContainerRoundsView ||
                    !debugDetailsOpen ||
                    raw.nodeType !== 'llm' ||
                    raw.modelInput === undefined
                  ) {
                    return null;
                  }
                  const modelInputId = `${itemKey}-model-input`;
                  const modelInputOpen = isSectionOpen(modelInputId);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(modelInputId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-primary/40 rounded-full" />
                          {t('agents.workflow.modelInput')}
                        </div>
                        <ChevronDown
                          className={cn(
                            'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                            modelInputOpen ? '' : '-rotate-90'
                          )}
                        />
                      </div>
                      {modelInputOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.modelInput ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}

                {(() => {
                  if (
                    !showDetail ||
                    inContainerRoundsView ||
                    !debugDetailsOpen ||
                    raw.nodeInput === undefined
                  ) {
                    return null;
                  }
                  const inputId = `${itemKey}-input`;
                  const inputOpen = isSectionOpen(inputId);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(inputId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-info/40 rounded-full" />
                          {t('agents.workflow.input')}
                        </div>
                        <ChevronDown
                          className={cn(
                            'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                            inputOpen ? '' : '-rotate-90'
                          )}
                        />
                      </div>
                      {inputOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.nodeInput ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}
                {(() => {
                  if (
                    !showDetail ||
                    inContainerRoundsView ||
                    !debugDetailsOpen ||
                    raw.loopInputs === undefined
                  ) {
                    return null;
                  }
                  const loopInputId = `${itemKey}-loop-input`;
                  const loopInputOpen = isSectionOpen(loopInputId);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(loopInputId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-info/40 rounded-full" />
                          {t('agents.workflow.loopInputs')}
                        </div>
                        <ChevronDown
                          className={cn(
                            'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                            loopInputOpen ? '' : '-rotate-90'
                          )}
                        />
                      </div>
                      {loopInputOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.loopInputs ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}

                {(() => {
                  if (
                    !showDetail ||
                    inContainerRoundsView ||
                    raw.nodeOutput === undefined ||
                    raw.nodeType === 'loop'
                  ) {
                    return null;
                  }
                  const outputId = `${itemKey}-output`;
                  const outputOpen = isSectionOpen(outputId, raw.nodeType !== 'llm');
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(outputId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-success/40 rounded-full" />
                          {t('agents.workflow.output')}
                        </div>
                        <div className="flex items-center gap-1">
                          <button
                            type="button"
                            className="rounded p-1 text-muted-foreground/40 hover:bg-muted hover:text-foreground"
                            onClick={e => copySectionValue(outputId, raw.nodeOutput, e)}
                            aria-label={runtimeLabel('copyOutput')}
                          >
                            {copiedSection === outputId ? (
                              <Check className="h-3 w-3" />
                            ) : (
                              <Copy className="h-3 w-3" />
                            )}
                          </button>
                          <ChevronDown
                            className={cn(
                              'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                              outputOpen ? '' : '-rotate-90'
                            )}
                          />
                        </div>
                      </div>
                      {outputOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.nodeOutput ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}
                {(() => {
                  if (!showDetail || inContainerRoundsView || raw.loopOutputs === undefined) {
                    return null;
                  }
                  const loopOutputId = `${itemKey}-loop-output`;
                  const loopOutputOpen = isSectionOpen(loopOutputId, true);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(loopOutputId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-success/40 rounded-full" />
                          {t('agents.workflow.loopOutputs')}
                        </div>
                        <div className="flex items-center gap-1">
                          <button
                            type="button"
                            className="rounded p-1 text-muted-foreground/40 hover:bg-muted hover:text-foreground"
                            onClick={e => copySectionValue(loopOutputId, raw.loopOutputs, e)}
                            aria-label={runtimeLabel('copyOutput')}
                          >
                            {copiedSection === loopOutputId ? (
                              <Check className="h-3 w-3" />
                            ) : (
                              <Copy className="h-3 w-3" />
                            )}
                          </button>
                          <ChevronDown
                            className={cn(
                              'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                              loopOutputOpen ? '' : '-rotate-90'
                            )}
                          />
                        </div>
                      </div>
                      {loopOutputOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.loopOutputs ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}
                {(() => {
                  if (
                    !showDetail ||
                    inContainerRoundsView ||
                    !debugDetailsOpen ||
                    isEmptyValue(raw.processData)
                  ) {
                    return null;
                  }
                  const processId = `${executionKey}-process-data`;
                  const processOpen = isSectionOpen(processId);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(processId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-warning/60 rounded-full" />
                          {runtimeLabel('processInfo')}
                        </div>
                        <div className="flex items-center gap-1">
                          <button
                            type="button"
                            className="rounded p-1 text-muted-foreground/40 hover:bg-muted hover:text-foreground"
                            onClick={e => copySectionValue(processId, raw.processData, e)}
                            aria-label={runtimeLabel('copyProcessInfo')}
                          >
                            {copiedSection === processId ? (
                              <Check className="h-3 w-3" />
                            ) : (
                              <Copy className="h-3 w-3" />
                            )}
                          </button>
                          <ChevronDown
                            className={cn(
                              'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                              processOpen ? '' : '-rotate-90'
                            )}
                          />
                        </div>
                      </div>
                      {processOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.processData ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}
                {(() => {
                  if (
                    !showDetail ||
                    inContainerRoundsView ||
                    !debugDetailsOpen ||
                    isEmptyValue(raw.executionMetadata)
                  ) {
                    return null;
                  }
                  const metadataId = `${executionKey}-execution-metadata`;
                  const metadataOpen = isSectionOpen(metadataId);
                  return (
                    <div className="flex flex-col gap-0.5 ml-1 min-w-0">
                      <div
                        className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground/80 px-1 mb-1 flex items-center justify-between group cursor-pointer select-none"
                        onClick={e => toggleSection(metadataId, e)}
                      >
                        <div className="flex items-center gap-1.5">
                          <div className="w-1 h-1 bg-primary/50 rounded-full" />
                          {runtimeLabel('executionMetadata')}
                        </div>
                        <div className="flex items-center gap-1">
                          <button
                            type="button"
                            className="rounded p-1 text-muted-foreground/40 hover:bg-muted hover:text-foreground"
                            onClick={e => copySectionValue(metadataId, raw.executionMetadata, e)}
                            aria-label={runtimeLabel('copyExecutionMetadata')}
                          >
                            {copiedSection === metadataId ? (
                              <Check className="h-3 w-3" />
                            ) : (
                              <Copy className="h-3 w-3" />
                            )}
                          </button>
                          <ChevronDown
                            className={cn(
                              'h-3 w-3 transition-transform text-muted-foreground/30 group-hover:text-muted-foreground',
                              metadataOpen ? '' : '-rotate-90'
                            )}
                          />
                        </div>
                      </div>
                      {metadataOpen && (
                        <div className="bg-muted/10 rounded-md overflow-hidden border border-border/10 shadow-[inset_0_1px_4px_rgba(0,0,0,0.03)] technical-scrollbar">
                          <RuntimeStructuredView
                            value={raw.executionMetadata ?? {}}
                            runtimeLabel={runtimeLabel}
                          />
                        </div>
                      )}
                    </div>
                  );
                })()}
                {isContainer && !inContainerRoundsView && rounds.length > 0 && (
                  <button
                    type="button"
                    className="mt-1 w-full rounded-md border border-border/40 bg-muted/40 px-2 py-2 text-xs text-left text-foreground hover:bg-muted transition-colors"
                    onClick={e => enterContainerRoundsView(itemKey, e)}
                  >
                    <div className="flex items-center justify-between">
                      <span>
                        {isLoop
                          ? t('agents.workflow.loopRoundsTotal', { count: rounds.length })
                          : t('agents.workflow.iterationRoundsTotal', { count: rounds.length })}
                      </span>
                      <span className="text-muted-foreground">
                        {t('agents.workflow.viewRoundsDetails')}
                      </span>
                    </div>
                  </button>
                )}
                {isContainer && inContainerRoundsView && (
                  <button
                    type="button"
                    className="inline-flex w-fit items-center gap-1 rounded-md border border-border/40 bg-muted/30 px-2 py-1 text-xs text-foreground hover:bg-muted transition-colors"
                    onClick={e => exitContainerRoundsView(itemKey, e)}
                  >
                    <ChevronLeft className="h-3 w-3" />
                    {t('agents.workflow.backToSummary')}
                  </button>
                )}
                {isContainer && inContainerRoundsView && rounds.length > 0 && (
                  <div className="grid gap-1 bg-muted shadow-md rounded-md p-1">
                    {rounds.map(round => {
                      const roundKey = `${itemKey}-round-${round.index}`;
                      return (
                        <div
                          key={`round-${itemKey}-${round.index}`}
                          className="rounded-md border p-1 bg-background"
                        >
                          <div
                            className={cn('flex items-center justify-between cursor-pointer')}
                            onClick={() => toggleOpen(roundKey)}
                          >
                            <div className="flex items-center gap-1">
                              <ChevronDown
                                className={cn(
                                  'h-3.5 w-3.5 transition-transform',
                                  isOpen(roundKey) ? '' : '-rotate-90'
                                )}
                              />
                              <span className="text-xs font-medium text-foreground">
                                {isLoop
                                  ? t('agents.workflow.loopRound', { index: round.index + 1 })
                                  : t('agents.workflow.iterationRound', {
                                      index: round.index + 1,
                                    })}
                              </span>
                            </div>
                            <span className={elapsedTimeClass}>
                              {formatMs(round.elapsedTime ? round.elapsedTime : 0)}
                            </span>
                          </div>
                          {isOpen(roundKey) && (
                            <div className="mt-2">
                              <WorkflowRunNodesList
                                items={round.nodes}
                                showDetail={showDetail}
                                variant={variant}
                              />
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
};

export default React.memo(WorkflowRunNodesList);
