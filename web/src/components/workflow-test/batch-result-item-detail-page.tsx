'use client';

import * as React from 'react';
import Link from 'next/link';
import { ArrowLeft, ChevronLeft, ChevronRight, Copy, ExternalLink, RefreshCcw } from 'lucide-react';
import { toast } from 'sonner';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import {
  useWorkflowTestBatchItems,
  useWorkflowTestBatches,
  useWorkflowTestScenarios,
} from '@/hooks/workflow-test/use-workflow-test';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { formatWorkflowElapsedMs } from '@/utils/format';
import type { WorkflowTestBatchItem } from '@/services/types/workflow-test';
import { useWorkflowDraft } from '@/hooks/workflow/use-workflow';
import { NODE_THEMES } from '@/components/workflow/nodes/custom/config';
import { cn } from '@/lib/utils';
import { formatQuestionTypeLabel } from './question-type';

interface BatchResultItemDetailPageProps {
  agentId: string;
  batchId: string;
  itemId: string;
  agentName?: string;
}

const SHOW_RAW_DATA_ACTIONS = false;

function itemStatusClass(status: string) {
  if (status === 'passed') return 'bg-emerald-50 text-emerald-700 border-emerald-200';
  if (status === 'failed') return 'bg-red-50 text-red-700 border-red-200';
  if (status === 'review') return 'bg-amber-50 text-amber-700 border-amber-200';
  if (status === 'running') return 'bg-blue-50 text-blue-700 border-blue-200';
  return 'bg-slate-100 text-slate-500 border-slate-200';
}

function itemStatusLabel(status: string, none: string) {
  const map: Record<string, string> = {
    pending: '待开始',
    running: '执行中',
    passed: '通过',
    failed: '不通过',
    review: '需复核',
    canceled: '已取消',
  };
  return map[status] || status || none;
}

function stringifyOutput(outputs: Record<string, unknown>, none: string) {
  if (!outputs || Object.keys(outputs).length === 0) {
    return none;
  }
  const preferredKeys = ['answer', 'text', 'result', 'output'];
  for (const key of preferredKeys) {
    const value = outputs[key];
    if (typeof value === 'string' && value.trim()) {
      return value;
    }
  }
  return JSON.stringify(outputs, null, 2);
}

function stringifyJson(value: unknown, none: string) {
  if (value === undefined || value === null || value === '') {
    return none;
  }
  return JSON.stringify(value, null, 2);
}

function hasExecutionFailure(outputs: Record<string, unknown>) {
  if (typeof outputs.error === 'string' && outputs.error.trim()) {
    return true;
  }
  if (outputs.status === 'failed') {
    return true;
  }
  const nodeErrors = outputs.node_errors;
  if (nodeErrors && typeof nodeErrors === 'object' && Object.keys(nodeErrors).length > 0) {
    return true;
  }
  return false;
}

function formatResponseTime(item: WorkflowTestBatchItem, none: string) {
  const outputs = item.outputs || {};
  const elapsedKeys = ['elapsed_time', 'elapsed_ms', 'duration_ms', 'latency_ms', 'response_time_ms'];
  for (const key of elapsedKeys) {
    const value = outputs[key];
    if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
      return formatWorkflowElapsedMs(value);
    }
  }
  return none;
}

function normalizeJudgeScore(value: number) {
  if (!Number.isFinite(value)) {
    return 0;
  }
  return Math.max(0, Math.min(1, value));
}

function deriveJudgeScore(item: WorkflowTestBatchItem, outputs: Record<string, unknown>) {
  if (item.error || hasExecutionFailure(outputs)) {
    return 0;
  }
  const confidence = normalizeJudgeScore(item.judge_confidence || 0);
  switch (item.status) {
    case 'passed':
      return 4 + confidence;
    case 'review':
      return 2.5 + (1 - confidence);
    case 'failed':
      return 1.5 * (1 - confidence);
    default:
      return 0;
  }
}

function formatJudgeScore(value: number) {
  const score = Math.max(0, Math.min(5, value));
  const rounded = Math.round(score * 10) / 10;
  return `${Number.isInteger(rounded) ? rounded.toFixed(0) : rounded.toFixed(1)} / 5`;
}

function localizeWorkflowTestError(
  message: string,
  t: (key:
    | 'errors.judgeModelRequired'
    | 'errors.judgeNotConfigured'
    | 'errors.judgeEmptyResult'
    | 'errors.judgeFailed'
    | 'errors.judgeFailedSuggestion'
    | 'errors.judgeManualReviewSuggestion'
    | 'errors.judgeConfigureSuggestion') => string
) {
  const normalized = message.trim().toLowerCase();
  if (!normalized) return message;
  if (
    normalized.includes('judge failed: model field is required') ||
    normalized.includes('model field is required')
  ) {
    return t('errors.judgeModelRequired');
  }
  if (normalized.includes('judge is not configured')) {
    return t('errors.judgeNotConfigured');
  }
  if (normalized.includes('judge returned empty result')) {
    return t('errors.judgeEmptyResult');
  }
  if (
    normalized === 'ai scoring failed; review manually or rerun the test' ||
    normalized === 'ai 评分失败，请人工复核或重新测试。'
  ) {
    return t('errors.judgeFailedSuggestion');
  }
  if (
    normalized === 'review this result manually' ||
    normalized === '请人工复核本次结果。'
  ) {
    return t('errors.judgeManualReviewSuggestion');
  }
  if (
    normalized === 'configure ai scoring and rerun, or review manually' ||
    normalized === '请配置 ai 评分能力后重新执行，或人工复核本次结果。'
  ) {
    return t('errors.judgeConfigureSuggestion');
  }
  if (normalized === 'judge failed') {
    return t('errors.judgeFailed');
  }
  if (normalized.startsWith('judge failed: ')) {
    return `${t('errors.judgeFailed')}：${message.trim().slice('judge failed: '.length)}`;
  }
  return message;
}

interface ExecutionNodeSnapshot {
  id: string;
  status: string;
  error: string;
  startTime: string;
  endTime: string;
}

function extractExecutionNodes(outputs: Record<string, unknown>): ExecutionNodeSnapshot[] {
  const nodeResults = outputs?.node_results;
  if (!nodeResults || typeof nodeResults !== 'object') {
    return [];
  }
  return Object.entries(nodeResults as Record<string, unknown>)
    .map(([id, raw]) => {
      const value = raw as Record<string, unknown>;
      return {
        id,
        status: typeof value.status === 'string' ? value.status : 'unknown',
        error: typeof value.error === 'string' ? value.error : '',
        startTime: typeof value.startTime === 'string' ? value.startTime : '',
        endTime: typeof value.endTime === 'string' ? value.endTime : '',
      };
    });
}

interface WorkflowDraftNodeMeta {
  id: string;
  title: string;
  type: string;
}

function getNodeDataString(node: unknown, key: string) {
  if (!node || typeof node !== 'object') return '';
  const data = (node as { data?: unknown }).data;
  if (!data || typeof data !== 'object') return '';
  const value = (data as Record<string, unknown>)[key];
  return typeof value === 'string' ? value.trim() : '';
}

function buildWorkflowDraftNodeMetaMap(draft: { graph?: { nodes?: unknown[] } } | undefined) {
  const nodes = draft?.graph?.nodes;
  const result = new Map<string, WorkflowDraftNodeMeta>();
  if (!Array.isArray(nodes)) {
    return result;
  }
  nodes.forEach(node => {
    if (!node || typeof node !== 'object') return;
    const idValue = (node as { id?: unknown }).id;
    if (typeof idValue !== 'string' || !idValue) return;
    const type = getNodeDataString(node, 'type');
    result.set(idValue, {
      id: idValue,
      title: getNodeDataString(node, 'title') || type || idValue,
      type,
    });
  });
  return result;
}

function buildWorkflowDraftNodeOrderMap(draft: { graph?: { nodes?: unknown[] } } | undefined) {
  const nodes = draft?.graph?.nodes;
  const result = new Map<string, number>();
  if (!Array.isArray(nodes)) {
    return result;
  }
  nodes.forEach((node, index) => {
    if (!node || typeof node !== 'object') return;
    const idValue = (node as { id?: unknown }).id;
    if (typeof idValue !== 'string' || !idValue) return;
    result.set(idValue, index);
  });
  return result;
}

function getNodeId(node: unknown) {
  if (!node || typeof node !== 'object') return '';
  const idValue = (node as { id?: unknown }).id;
  return typeof idValue === 'string' ? idValue : '';
}

function getEdgeEndpoint(edge: unknown, key: 'source' | 'target') {
  if (!edge || typeof edge !== 'object') return '';
  const value = (edge as Record<string, unknown>)[key];
  return typeof value === 'string' ? value : '';
}

function buildWorkflowDraftTraversalOrderMap(draft: { graph?: { nodes?: unknown[]; edges?: unknown[] } } | undefined) {
  const nodes = draft?.graph?.nodes;
  const edges = draft?.graph?.edges;
  const result = new Map<string, number>();
  if (!Array.isArray(nodes) || !Array.isArray(edges) || nodes.length === 0) {
    return result;
  }

  const nodeIds = new Set(nodes.map(getNodeId).filter(Boolean));
  const adjacency = new Map<string, string[]>();
  edges.forEach(edge => {
    const source = getEdgeEndpoint(edge, 'source');
    const target = getEdgeEndpoint(edge, 'target');
    if (!source || !target || !nodeIds.has(source) || !nodeIds.has(target)) return;
    const next = adjacency.get(source) || [];
    next.push(target);
    adjacency.set(source, next);
  });

  const startNode = nodes.find(node => getNodeDataString(node, 'type') === 'start') || nodes[0];
  const startNodeId = getNodeId(startNode);
  const queue = startNodeId ? [startNodeId] : [];
  const visited = new Set<string>();

  while (queue.length > 0) {
    const id = queue.shift();
    if (!id || visited.has(id)) continue;
    visited.add(id);
    result.set(id, result.size);
    const targets = adjacency.get(id) || [];
    targets.forEach(target => {
      if (!visited.has(target)) {
        queue.push(target);
      }
    });
  }

  nodes.forEach(node => {
    const id = getNodeId(node);
    if (id && !result.has(id)) {
      result.set(id, result.size);
    }
  });
  return result;
}

function sortExecutionNodesByWorkflowOrder(
  executionNodes: ExecutionNodeSnapshot[],
  nodeOrderById: Map<string, number>
) {
  return [...executionNodes].sort((a, b) => {
    const aOrder = nodeOrderById.get(a.id);
    const bOrder = nodeOrderById.get(b.id);
    if (aOrder !== undefined && bOrder !== undefined) {
      return aOrder - bOrder;
    }
    if (aOrder !== undefined) return -1;
    if (bOrder !== undefined) return 1;
    return a.startTime.localeCompare(b.startTime);
  });
}

function fallbackExecutionStepMeta(nodeId: string): WorkflowDraftNodeMeta {
  if (!nodeId) return { id: nodeId, title: nodeId, type: '' };
  if (nodeId.includes('start')) return { id: nodeId, title: '开始节点', type: 'start' };
  if (nodeId.includes('answer') || nodeId.includes('reply')) return { id: nodeId, title: '回复生成', type: 'answer' };
  if (nodeId.includes('llm')) return { id: nodeId, title: 'LLM', type: 'llm' };
  if (nodeId.includes('branch')) return { id: nodeId, title: '分支节点', type: 'if-else' };
  return { id: nodeId, title: nodeId, type: '' };
}

function buildTurnTitle(index: number) {
  return `第 ${index + 1} 轮`;
}

interface TurnResultSnapshot {
  turnIndex: number;
  content: string;
  workflowRunId: string;
  outputs: Record<string, unknown>;
}

interface ConversationTurnSnapshot extends TurnResultSnapshot {
  answer: string;
}

function extractTurnResultSnapshots(outputs: Record<string, unknown>): TurnResultSnapshot[] {
  const rawTurnResults = outputs.turn_results;
  if (!Array.isArray(rawTurnResults)) {
    return [];
  }

  return rawTurnResults
    .map((raw, index) => {
      if (!raw || typeof raw !== 'object') {
        return null;
      }
      const value = raw as Record<string, unknown>;
      const outputsValue = value.outputs;
      return {
        turnIndex: typeof value.turn_index === 'number' ? value.turn_index : index + 1,
        content: typeof value.content === 'string' ? value.content : '',
        workflowRunId: typeof value.workflow_run_id === 'string' ? value.workflow_run_id : '',
        outputs:
          outputsValue && typeof outputsValue === 'object' && !Array.isArray(outputsValue)
            ? (outputsValue as Record<string, unknown>)
            : {},
      };
    })
    .filter((item): item is TurnResultSnapshot => item !== null);
}

function outputAnswer(outputs: Record<string, unknown>) {
  if (hasExecutionFailure(outputs)) {
    return '';
  }
  const direct = stringifyOutput(outputs, '');
  if (direct.trim()) {
    return direct;
  }
  const nestedOutputs = outputs.outputs;
  if (nestedOutputs && typeof nestedOutputs === 'object' && !Array.isArray(nestedOutputs)) {
    return stringifyOutput(nestedOutputs as Record<string, unknown>, '');
  }
  return '';
}

function buildConversationTurnSnapshots(
  outputs: Record<string, unknown>,
  item: WorkflowTestBatchItem
): ConversationTurnSnapshot[] {
  const turnResults = extractTurnResultSnapshots(outputs);
  if (turnResults.length > 0) {
    return turnResults.map((turn, index) => {
      const fallbackTurn = item.case_snapshot.turns?.[index];
      return {
        ...turn,
        content: turn.content || fallbackTurn?.content || item.case_snapshot.content || '',
        answer: outputAnswer(turn.outputs),
      };
    });
  }

  const answer = outputAnswer(outputs);
  if (!answer.trim()) {
    return [];
  }
  const firstTurn = item.case_snapshot.turns?.[0];
  return [
    {
      turnIndex: 1,
      content: firstTurn?.content || item.case_snapshot.content || '',
      workflowRunId: item.workflow_run_id || '',
      outputs,
      answer,
    },
  ];
}

interface RawViewPayload {
  title: string;
  content: Record<string, unknown> | unknown[] | string | number | boolean | null;
}

export function BatchResultItemDetailPage({
  agentId,
  batchId,
  itemId,
  agentName,
}: BatchResultItemDetailPageProps) {
  const t = useT('agents.workflowTest.detail');
  const commonT = useT('agents.workflowTest.common');
  const typeT = useT('agents.workflowTest.questionTypes');
  const { data: workflowDraft } = useWorkflowDraft(agentId);
  const {
    data: batchesData,
    isLoading: batchesLoading,
    error: batchesError,
    refetch: refetchBatches,
  } = useWorkflowTestBatches(agentId);
  const {
    data: itemsData,
    isLoading: itemsLoading,
    error: itemsError,
    refetch: refetchItems,
  } = useWorkflowTestBatchItems(agentId, batchId);
  const { data: scenariosData } = useWorkflowTestScenarios(agentId);

  const batches = batchesData?.data?.items ?? [];
  const batch = batches.find(item => item.id === batchId);
  const items = itemsData?.data?.items ?? [];
  const itemIndex = items.findIndex(item => item.id === itemId);
  const selectedItem = itemIndex >= 0 ? items[itemIndex] : null;
  const previousItem = itemIndex > 0 ? items[itemIndex - 1] : null;
  const nextItem = itemIndex >= 0 && itemIndex < items.length - 1 ? items[itemIndex + 1] : null;
  const [rawView, setRawView] = React.useState<RawViewPayload | null>(null);
  const isLoading = batchesLoading || itemsLoading;
  const error = batchesError || itemsError;
  const scenarioNameById = React.useMemo(
    () => new Map((scenariosData?.data?.items ?? []).map(scenario => [scenario.id, scenario.name])),
    [scenariosData]
  );
  const draftNodeMetaById = React.useMemo(
    () => buildWorkflowDraftNodeMetaMap(workflowDraft),
    [workflowDraft]
  );
  const draftNodeOrderById = React.useMemo(
    () => {
      const traversalOrder = buildWorkflowDraftTraversalOrderMap(workflowDraft);
      return traversalOrder.size > 0 ? traversalOrder : buildWorkflowDraftNodeOrderMap(workflowDraft);
    },
    [workflowDraft]
  );
  const getScenarioName = React.useCallback(
    (item: WorkflowTestBatchItem) => {
      const scenarioId = item.case_snapshot.scenario_id;
      return scenarioId ? scenarioNameById.get(scenarioId) || commonT('none') : commonT('none');
    },
    [commonT, scenarioNameById]
  );
  const itemPosition = itemIndex >= 0 ? `${itemIndex + 1} / ${items.length}` : '';
  const rawViewText = stringifyJson(rawView?.content, commonT('none'));
  const copyRawView = React.useCallback(async () => {
    if (!rawViewText || rawViewText === commonT('none')) {
      return;
    }
    try {
      await navigator.clipboard.writeText(rawViewText);
      toast.success(t('rawCopied'));
    } catch {
      toast.error(t('rawCopyFailed'));
    }
  }, [commonT, rawViewText, t]);

  if (isLoading) {
    return (
      <div className="min-h-full bg-slate-50 p-8">
        <div className="mx-auto max-w-[1600px] space-y-6">
          <Skeleton className="h-48 rounded-2xl" />
          <Skeleton className="h-[720px] rounded-2xl" />
        </div>
      </div>
    );
  }

  if (error || !batch || !selectedItem) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="w-full max-w-xl">
          <Alert variant="destructive">
            <AlertTitle>{t('loadFailedTitle')}</AlertTitle>
            <AlertDescription>
              {error ? getErrorMessage(error) || t('loadFailedFallback') : t('notFound')}
            </AlertDescription>
          </Alert>
          <Button
            className="mt-4"
            onClick={() => {
              void refetchBatches();
              void refetchItems();
            }}
          >
            <RefreshCcw className="mr-2 size-4" />
            {t('retry')}
          </Button>
        </div>
      </div>
    );
  }

  const outputs = selectedItem.outputs || {};
  const executionNodes = sortExecutionNodesByWorkflowOrder(
    extractExecutionNodes(outputs),
    draftNodeOrderById
  );
  const conversationTurns = buildConversationTurnSnapshots(outputs, selectedItem);
  const judgeScore = deriveJudgeScore(selectedItem, outputs);
  const judgeScoreText = formatJudgeScore(judgeScore);
  const questionSnapshot = selectedItem.case_snapshot.content || commonT('none');
  const expectedResult = selectedItem.case_snapshot.expected_result || commonT('none');
  const reasonText =
    (selectedItem.error ? localizeWorkflowTestError(selectedItem.error, t) : '') ||
    (selectedItem.judge_reason ? localizeWorkflowTestError(selectedItem.judge_reason, t) : '') ||
    (selectedItem.status === 'passed' ? t('passedReasonFallback') : commonT('none'));
  const judgeReasonText = selectedItem.judge_reason
    ? localizeWorkflowTestError(selectedItem.judge_reason, t)
    : commonT('none');
  const judgeSuggestionText = selectedItem.judge_suggestion
    ? localizeWorkflowTestError(selectedItem.judge_suggestion, t)
    : '';
  const reasonTitle = selectedItem.status === 'passed' && !selectedItem.error ? t('passConclusion') : t('issueReason');
  const reasonToneClass =
    selectedItem.status === 'passed' && !selectedItem.error
      ? 'border-emerald-200 bg-emerald-50 text-emerald-800'
      : 'border-red-200 bg-red-50 text-red-800';
  const reasonTitleClass =
    selectedItem.status === 'passed' && !selectedItem.error ? 'text-emerald-600' : 'text-red-500';
  const openRawView = (title: string, content: RawViewPayload['content']) => setRawView({ title, content });

  return (
    <div className="min-h-full bg-slate-50 px-8 py-8">
      <div className="mx-auto flex max-w-[1600px] flex-col gap-6">
        <div className="text-sm text-slate-500">
          {t('breadcrumb', { agentName: agentName || commonT('agentFallback') })}
        </div>

        <Card className="rounded-2xl border-slate-200">
          <CardContent className="p-6">
            <div className="mb-4 flex items-center justify-between gap-4">
              <Button variant="ghost" size="sm" asChild className="px-0 text-slate-600">
                <Link href={`/console/agents/${agentId}/batch-test/${batchId}`}>
                  <ArrowLeft className="mr-2 size-4" />
                  {t('back')}
                </Link>
              </Button>
              <div className="flex items-center gap-3">
                {itemPosition ? <div className="text-sm text-slate-500">{itemPosition}</div> : null}
                <div className="flex items-center gap-2">
                  {previousItem ? (
                    <Button variant="outline" size="sm" asChild>
                      <Link href={`/console/agents/${agentId}/batch-test/${batchId}/items/${previousItem.id}`}>
                        <ChevronLeft className="mr-1 size-4" />
                        {t('previousItem')}
                      </Link>
                    </Button>
                  ) : (
                    <Button variant="outline" size="sm" disabled>
                      <ChevronLeft className="mr-1 size-4" />
                      {t('previousItem')}
                    </Button>
                  )}
                  {nextItem ? (
                    <Button variant="outline" size="sm" asChild>
                      <Link href={`/console/agents/${agentId}/batch-test/${batchId}/items/${nextItem.id}`}>
                        {t('nextItem')}
                        <ChevronRight className="ml-1 size-4" />
                      </Link>
                    </Button>
                  ) : (
                    <Button variant="outline" size="sm" disabled>
                      {t('nextItem')}
                      <ChevronRight className="ml-1 size-4" />
                    </Button>
                  )}
                </div>
              </div>
            </div>

            <div className="flex flex-wrap items-start justify-between gap-4">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-3">
                  <h1 className="text-2xl font-semibold text-slate-950">{selectedItem.case_snapshot.content}</h1>
                  <Badge className={itemStatusClass(selectedItem.status)}>
                    {itemStatusLabel(selectedItem.status, commonT('none'))}
                  </Badge>
                </div>
                <div className="mt-3 flex flex-wrap gap-2 text-sm text-slate-500">
                  <Badge variant="outline">{getScenarioName(selectedItem)}</Badge>
                  <Badge variant="outline">
                    {formatQuestionTypeLabel(selectedItem.case_snapshot.question_type, typeT)}
                  </Badge>
                  <Badge variant="outline">{formatResponseTime(selectedItem, commonT('none'))}</Badge>
                </div>
              </div>
            </div>

            <div className="mt-5 grid grid-cols-1 gap-4 rounded-2xl border border-slate-200 bg-slate-50 p-4 text-sm text-slate-700 md:grid-cols-3">
              <div>
                <div className="text-slate-500">{t('table.scenario')}</div>
                <div className="mt-1 font-semibold text-slate-950">{getScenarioName(selectedItem)}</div>
              </div>
              <div>
                <div className="text-slate-500">{t('table.testResult')}</div>
                <div className="mt-1">
                  <Badge className={itemStatusClass(selectedItem.status)}>
                    {itemStatusLabel(selectedItem.status, commonT('none'))}
                  </Badge>
                </div>
              </div>
              <div>
                <div className="text-slate-500">{t('table.responseTime')}</div>
                <div className="mt-1 font-semibold text-slate-950">
                  {formatResponseTime(selectedItem, commonT('none'))}
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="rounded-2xl border-slate-200">
          <CardContent className="space-y-6 p-6">
            <section className="space-y-3">
              <h2 className="text-lg font-semibold text-slate-950">{t('questionSnapshot')}</h2>
              <div className="rounded-2xl border border-slate-200 bg-white p-4 text-sm text-slate-800">
                {questionSnapshot}
              </div>
              <div className="rounded-2xl border border-slate-200 bg-white p-4 text-sm text-slate-800">
                <div className="mb-2 text-xs font-medium text-slate-500">{t('expectedResult')}</div>
                <div className="whitespace-pre-wrap">{expectedResult}</div>
              </div>
              <div className="flex items-center justify-between gap-3">
                <h2 className="text-lg font-semibold text-slate-950">{t('conversationContent')}</h2>
                {SHOW_RAW_DATA_ACTIONS ? (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => openRawView(t('rawOutput'), outputs as Record<string, unknown>)}
                  >
                    <ExternalLink className="mr-2 size-4" />
                    {t('rawData')}
                  </Button>
                ) : null}
              </div>
              {conversationTurns.length > 0 ? (
                <div className="space-y-4">
                  {conversationTurns.map((turn, index) => {
                    const turnSnapshot = selectedItem.case_snapshot.turns?.[index];
                    return (
                      <div key={`${turn.workflowRunId || 'turn'}-${turn.turnIndex}`} className="rounded-2xl border border-slate-200 bg-slate-50 p-5">
                        <div className="text-base font-medium text-slate-950">
                          {buildTurnTitle(index)}
                        </div>
                        <div className="mt-3 space-y-3">
                          <div className="rounded-xl border border-slate-200 bg-white p-4">
                            <div className="mb-2 text-xs font-medium text-slate-500">{t('userQuestion')}</div>
                            <div className="whitespace-pre-wrap text-sm text-slate-800">
                              {turn.content || turnSnapshot?.content || commonT('none')}
                            </div>
                          </div>
                          <div className="rounded-xl border border-slate-200 bg-white p-4">
                            <div className="mb-2 text-xs font-medium text-slate-500">{t('agentReply')}</div>
                            <div className="whitespace-pre-wrap text-sm text-slate-800">
                              {turn.answer || commonT('none')}
                            </div>
                          </div>
                          {SHOW_RAW_DATA_ACTIONS ? (
                            <button
                              type="button"
                              onClick={() => openRawView(`第 ${turn.turnIndex} 轮输出`, turn.outputs)}
                              className="text-left text-xs text-slate-500 underline-offset-4 hover:underline"
                            >
                              {t('viewTurnOutputRaw')}
                            </button>
                          ) : null}
                        </div>
                      </div>
                    );
                  })}
                </div>
              ) : (
                <div className="rounded-2xl border border-slate-200 bg-slate-50 p-6 text-sm text-slate-500">
                  {t('conversationSnapshotFallback')}
                </div>
              )}
            </section>

            <section className="space-y-3">
              <h2 className="text-lg font-semibold text-slate-950">{t('judgeDetail')}</h2>
              <div className="grid gap-4 lg:grid-cols-[160px_minmax(0,1fr)]">
                <div className="rounded-2xl border border-slate-200 bg-white p-4">
                  <div className="text-sm text-slate-500">{t('judgeScore')}</div>
                  <div
                    className={cn(
                      'mt-3 text-3xl font-semibold',
                      selectedItem.status === 'passed' ? 'text-emerald-600' : 'text-slate-700'
                    )}
                  >
                    {judgeScoreText}
                  </div>
                </div>
                <div className="rounded-2xl border border-slate-200 bg-white p-4 text-sm text-slate-800">
                  <div className="text-slate-500">{t('judgeOpinion')}</div>
                  <div className="mt-2 whitespace-pre-wrap">{judgeReasonText}</div>
                  {judgeSuggestionText ? (
                    <div className="mt-4 rounded-xl bg-amber-50 p-4 text-amber-800">
                      <div className="font-medium">{t('suggestionLabel')}</div>
                      <div className="mt-1 whitespace-pre-wrap">{judgeSuggestionText}</div>
                    </div>
                  ) : null}
                </div>
              </div>
            </section>

            <section className="space-y-3">
              <h2 className="text-lg font-semibold text-slate-950">{t('executionRecord')}</h2>
              <div className={cn('rounded-2xl border p-4 text-sm', reasonToneClass)}>
                <div className={cn('text-xs', reasonTitleClass)}>{reasonTitle}</div>
                <div className="mt-1 font-medium">{reasonText}</div>
              </div>
              <div className="rounded-2xl border border-slate-200 bg-white p-4">
                <div className="text-xs text-slate-500">{t('executionPath')}</div>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  {executionNodes.length > 0 ? (
                    executionNodes.map((node, index) => {
                      const meta = draftNodeMetaById.get(node.id) || fallbackExecutionStepMeta(node.id);
                      const theme =
                        meta.type in NODE_THEMES
                          ? NODE_THEMES[meta.type as keyof typeof NODE_THEMES]
                          : undefined;
                      const Icon = theme?.icon;
                      return (
                        <React.Fragment key={node.id}>
                          <div className="inline-flex items-center gap-2 rounded-full border border-slate-200 bg-white px-3 py-1 text-sm font-medium text-slate-700">
                            {Icon ? (
                              <span
                                className={cn(
                                  'flex size-5 items-center justify-center rounded-md',
                                  theme?.classNames.iconBg
                                )}
                              >
                                <Icon className="size-3" />
                              </span>
                            ) : null}
                            <span>{meta.title}</span>
                          </div>
                          {index < executionNodes.length - 1 ? <span className="text-slate-300">›</span> : null}
                        </React.Fragment>
                      );
                    })
                  ) : (
                    <span className="text-sm text-slate-500">{commonT('none')}</span>
                  )}
                </div>
              </div>
            </section>
          </CardContent>
        </Card>
      </div>

      <Dialog open={Boolean(rawView)} onOpenChange={open => !open && setRawView(null)}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>{rawView?.title || '原始数据'}</DialogTitle>
          </DialogHeader>
          <DialogBody>
            <pre className="max-h-[70vh] overflow-auto whitespace-pre-wrap rounded-2xl border border-slate-200 bg-slate-50 p-5 text-xs text-slate-700">
              {rawViewText}
            </pre>
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={copyRawView} disabled={!rawView}>
              <Copy className="mr-2 size-4" />
              {t('copyRawData')}
            </Button>
            <Button variant="outline" onClick={() => setRawView(null)}>
              {t('close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
