'use client';

import * as React from 'react';
import Link from 'next/link';
import { ArrowLeft, ChevronLeft, ChevronRight, Copy, ExternalLink, RefreshCcw } from 'lucide-react';
import { toast } from 'sonner';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import {
  useWorkflowTestBatchItems,
  useWorkflowTestBatches,
  useWorkflowTestScenarios,
} from '@/hooks/workflow-test/use-workflow-test';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { formatWorkflowElapsedMs } from '@/utils/format';
import type {
  WorkflowTestAnalysis,
  WorkflowTestBatchItem,
  WorkflowTestCheckResult,
  WorkflowTestEvaluationAssertion,
  WorkflowTestEvaluationSchema,
} from '@/services/types/workflow-test';
import { useWorkflowDraft } from '@/hooks/workflow/use-workflow';
import { NODE_THEMES } from '@/components/workflow/nodes/custom/config';
import { cn } from '@/lib/utils';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { formatQuestionTypeLabel } from './question-type';

interface BatchResultItemDetailPageProps {
  agentId: string;
  batchId: string;
  itemId: string;
  agentName?: string;
}

const SHOW_RAW_DATA_ACTIONS = false;
const BUSINESS_OUTPUT_TEXT_KEYS = [
  'answer',
  'text',
  'summary',
  'result',
  'output',
  'content',
  'message',
  'value',
];
const RUNTIME_OUTPUT_KEYS = new Set([
  'elapsed_time',
  'elapsed_ms',
  'duration_ms',
  'latency_ms',
  'response_time_ms',
  'node_results',
  'node_errors',
  'status',
  'task_id',
  'workflow_run_id',
  'workflow_trace',
  'workflow_test_analysis',
  'check_results',
  'turn_results',
  'turn_count',
  'conversation_id',
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value));
}

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

function diagnosisStatusLabel(status: string) {
  const map: Record<string, string> = {
    passed: '通过',
    failed: '不通过',
    review: '需复核',
  };
  return map[status] || status || '需复核';
}

function diagnosisStatusClass(status: string) {
  if (status === 'passed') return 'border-emerald-200 bg-emerald-50 text-emerald-700';
  if (status === 'failed') return 'border-red-200 bg-red-50 text-red-700';
  return 'border-amber-200 bg-amber-50 text-amber-700';
}

function workflowTestAnalysis(outputs: Record<string, unknown>): WorkflowTestAnalysis | null {
  const raw = outputs.workflow_test_analysis;
  if (!isRecord(raw) || !isRecord(raw.summary)) {
    return null;
  }
  const trace = isRecord(raw.trace) ? raw.trace : {};
  const comparisons = isRecord(raw.comparisons) ? raw.comparisons : {};
  const nodes = Array.isArray(trace.nodes) ? trace.nodes : [];
  const checks = Array.isArray(comparisons.checks) ? comparisons.checks : [];
  const turns = Array.isArray(comparisons.turns) ? comparisons.turns : [];
  const suggestions = Array.isArray(raw.suggestions) ? raw.suggestions : [];
  const evaluationSchema = isRecord(raw.evaluation_schema)
    ? (raw.evaluation_schema as unknown as WorkflowTestEvaluationSchema)
    : undefined;
  return {
    mode: typeof raw.mode === 'string' ? raw.mode : 'task',
    evaluation_schema: evaluationSchema,
    trace: { nodes: nodes.filter(isRecord) as unknown as WorkflowTestAnalysis['trace']['nodes'] },
    comparisons: {
      overall: isRecord(comparisons.overall)
        ? (comparisons.overall as WorkflowTestAnalysis['comparisons']['overall'])
        : undefined,
      checks: checks.filter(isRecord) as unknown as WorkflowTestAnalysis['comparisons']['checks'],
      turns: turns.filter(isRecord) as unknown as WorkflowTestAnalysis['comparisons']['turns'],
    },
    summary: raw.summary as unknown as WorkflowTestAnalysis['summary'],
    suggestions: suggestions.filter(isRecord) as unknown as WorkflowTestAnalysis['suggestions'],
  };
}

function businessOutputText(outputs: Record<string, unknown>): string {
  const nested = isRecord(outputs.outputs) ? outputs.outputs : null;
  if (nested) {
    const nestedText: string = businessOutputText(nested);
    if (nestedText) return nestedText;
    const nestedPayload = businessOutputPayload(nested);
    if (Object.keys(nestedPayload).length > 0) return JSON.stringify(nestedPayload, null, 2);
  }
  for (const key of BUSINESS_OUTPUT_TEXT_KEYS) {
    const value = outputs[key];
    if (typeof value === 'string' && value.trim()) {
      return value.trim();
    }
  }
  const payload = businessOutputPayload(outputs);
  if (Object.keys(payload).length > 0) {
    return JSON.stringify(payload, null, 2);
  }
  return '';
}

function businessOutputPayload(outputs: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(outputs).filter(([key, value]) => {
      if (!key.trim() || RUNTIME_OUTPUT_KEYS.has(key)) return false;
      return value !== undefined && value !== null && value !== '';
    })
  );
}

function stringifyOutput(outputs: Record<string, unknown>, none: string) {
  if (!outputs || Object.keys(outputs).length === 0) {
    return none;
  }
  return businessOutputText(outputs) || none;
}

function WorkflowOutput({ content, none }: { content: string; none: string }) {
  if (!content.trim()) {
    return <>{none}</>;
  }
  return (
    <div className="prose prose-sm max-w-none break-words text-slate-800 prose-p:my-2 prose-ul:my-2 prose-ol:my-2 prose-li:my-1 prose-strong:text-slate-900">
      <MarkdownViewer content={content} />
    </div>
  );
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
  const elapsedKeys = [
    'elapsed_time',
    'elapsed_ms',
    'duration_ms',
    'latency_ms',
    'response_time_ms',
  ];
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
  const analysis = workflowTestAnalysis(outputs);
  const summary = analysis?.summary;
  if (analysis?.mode === 'task' && summary && summary.total > 0) {
    if (typeof summary.reference_score === 'number' && Number.isFinite(summary.reference_score)) {
      return summary.reference_score;
    }
    const covered = summary.passed + summary.review * 0.5;
    return (covered / summary.total) * 5;
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
  t: (
    key:
      | 'errors.judgeModelRequired'
      | 'errors.judgeNotConfigured'
      | 'errors.judgeEmptyResult'
      | 'errors.judgeFailed'
      | 'errors.judgeFailedSuggestion'
      | 'errors.judgeManualReviewSuggestion'
      | 'errors.judgeConfigureSuggestion'
      | 'errors.modelUnavailable'
  ) => string
) {
  const normalized = message.trim().toLowerCase();
  if (!normalized) return message;
  if (
    normalized.includes('all providers failed') ||
    normalized.includes('current user api does not support http call') ||
    normalized.includes('upstream service error') ||
    normalized.includes('no provider available') ||
    normalized.includes('model unavailable') ||
    normalized.includes('当前评分模型不可用')
  ) {
    return t('errors.modelUnavailable');
  }
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
  if (normalized === 'review this result manually' || normalized === '请人工复核本次结果。') {
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
    return `${t('errors.judgeFailed')}：${translateMissingFieldPhrases(message.trim().slice('judge failed: '.length))}`;
  }
  return translateMissingFieldPhrases(message);
}

function translateMissingFieldPhrases(value: string) {
  return value
    .replace(/missing field:\s*date/gi, '缺失字段：日期')
    .replace(/missing field:\s*related party/gi, '缺失字段：相关方')
    .replace(/missing fields:\s*date/gi, '缺失字段：日期')
    .replace(/missing fields:\s*related party/gi, '缺失字段：相关方');
}

function workflowTestCheckKey(check: WorkflowTestCheckResult, index: number) {
  const parts = [check.id, check.type, check.label, check.severity, String(index)]
    .map(part => String(part || '').trim())
    .filter(Boolean);
  return parts.length > 0 ? parts.join('::') : `check::${index}`;
}

function isGenericCheckLabel(value: string) {
  return /^第\s*\d+\s*轮期望$/.test(value.trim());
}

function relevantChecks(analysis: WorkflowTestAnalysis | null) {
  const checks = analysis?.comparisons?.checks ?? [];
  return checks
    .filter(check => check.status === 'failed' || check.status === 'review')
    .sort((left, right) => {
      const leftScore =
        (left.status === 'failed' ? 10 : 0) + (left.severity === 'critical' ? 5 : 0);
      const rightScore =
        (right.status === 'failed' ? 10 : 0) + (right.severity === 'critical' ? 5 : 0);
      return rightScore - leftScore;
    });
}

function checkCountByStatus(checks: WorkflowTestCheckResult[]) {
  return checks.reduce(
    (counts, check) => {
      if (check.status === 'passed') counts.passed += 1;
      else if (check.status === 'failed') counts.failed += 1;
      else counts.review += 1;
      return counts;
    },
    { passed: 0, failed: 0, review: 0 }
  );
}

function taskGoalTypeLabel(value: string | undefined) {
  const map: Record<string, string> = {
    extract: '信息抽取',
    classify: '分类判断',
    transform: '内容转换',
    analyze: '分析总结',
    decision: '决策判断',
    action: '动作执行',
    general: '通用任务',
  };
  return map[String(value || '').trim()] || value || '通用任务';
}

function taskAssertionTypeLabel(value: string | undefined) {
  const map: Record<string, string> = {
    must_include: '必须覆盖',
    must_not_include: '禁止出现',
    missing_policy: '缺失处理',
    fact_present: '事实字段',
    missing_field_marked: '缺失字段',
    state_present: '状态结论',
    format: '格式要求',
    source_grounding: '输入依据',
    action_result: '执行结果',
    semantic_match: '语义满足',
  };
  return map[String(value || '').trim()] || value || '评价断言';
}

function taskAssertionSeverityLabel(value: string | undefined) {
  const map: Record<string, string> = {
    critical: '关键',
    normal: '普通',
    hint: '提示',
  };
  return map[String(value || '').trim()] || value || '普通';
}

function taskAssertionText(assertion: WorkflowTestEvaluationAssertion) {
  const description = typeof assertion.description === 'string' ? assertion.description.trim() : '';
  if (description) return translateMissingFieldPhrases(description);
  const values = Array.isArray(assertion.values) ? assertion.values.filter(Boolean).join('、') : '';
  return translateMissingFieldPhrases(values || taskAssertionTypeLabel(assertion.type));
}

function compactTaskSchemaAssertions(schema: WorkflowTestEvaluationSchema | undefined) {
  return Array.isArray(schema?.assertions)
    ? (schema.assertions.filter(isRecord) as unknown as WorkflowTestEvaluationAssertion[])
    : [];
}

function readableAnalysisIssue(analysis: WorkflowTestAnalysis | null) {
  const firstProblem = relevantChecks(analysis)[0];
  if (firstProblem?.evidence?.trim()) {
    return translateMissingFieldPhrases(firstProblem.evidence.trim());
  }
  const issue = analysis?.summary?.main_issue;
  if (typeof issue !== 'string') return '';
  const trimmed = issue.trim();
  return trimmed && !isGenericCheckLabel(trimmed) ? translateMissingFieldPhrases(trimmed) : '';
}

function workflowOptimizationText(check: WorkflowTestCheckResult) {
  const label = translateMissingFieldPhrases(
    isGenericCheckLabel(check.label) ? '回复生成策略' : check.label || '相关节点'
  );
  if (check.status === 'failed') {
    return `${label}：请调整工作流中的提示词、分支条件或知识引用，确保回复覆盖可验证的业务要点。`;
  }
  return `${label}：当前仅需复核，不应直接作为工作流修改建议。`;
}

function hasDisplayValue(value: unknown) {
  if (value === undefined || value === null || value === '') return false;
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === 'object') return Object.keys(value as Record<string, unknown>).length > 0;
  return true;
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
  return Object.entries(nodeResults as Record<string, unknown>).map(([id, raw]) => {
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

function buildWorkflowDraftTraversalOrderMap(
  draft: { graph?: { nodes?: unknown[]; edges?: unknown[] } } | undefined
) {
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
  if (nodeId.includes('answer') || nodeId.includes('reply'))
    return { id: nodeId, title: '回复生成', type: 'answer' };
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
  expected: string;
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
        expected: fallbackTurn?.expected_result || item.case_snapshot.expected_result || '',
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
      expected: firstTurn?.expected_result || item.case_snapshot.expected_result || '',
    },
  ];
}

function compactText(value: string) {
  return value.replace(/\s+/g, ' ').trim();
}

function uniqueTexts(values: string[]) {
  const seen = new Set<string>();
  return values
    .map(value => value.trim())
    .filter(value => {
      if (!value) return false;
      const key = compactText(value).toLowerCase();
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
}

function buildRecommendationTexts(
  judgeSuggestionText: string,
  analysis: WorkflowTestAnalysis | null,
  none: string
) {
  if (analysis?.mode === 'task' && analysis.summary?.status === 'passed') {
    return [];
  }
  const values: string[] = [];
  const normalizedJudgeSuggestion = judgeSuggestionText.trim().toLowerCase();
  if (
    judgeSuggestionText &&
    judgeSuggestionText !== none &&
    !normalizedJudgeSuggestion.includes('ai 评分') &&
    !normalizedJudgeSuggestion.includes('人工复核') &&
    !normalizedJudgeSuggestion.includes('ai scoring')
  ) {
    values.push(translateMissingFieldPhrases(judgeSuggestionText));
  }
  analysis?.suggestions.forEach(suggestion => {
    const target =
      typeof suggestion.target === 'string'
        ? translateMissingFieldPhrases(suggestion.target.trim())
        : '';
    const content =
      typeof suggestion.content === 'string'
        ? translateMissingFieldPhrases(suggestion.content.trim())
        : '';
    if (!content) return;
    if (
      content.includes('AI 评估') ||
      content.includes('人工复核') ||
      content.includes('检查点判断')
    )
      return;
    values.push(target && !isGenericCheckLabel(target) ? `${target}: ${content}` : content);
  });
  relevantChecks(analysis)
    .filter(check => check.status === 'failed')
    .slice(0, 3)
    .forEach(check => {
      values.push(workflowOptimizationText(check));
    });
  return uniqueTexts(values).slice(0, 5);
}

function analysisMainIssue(analysis: WorkflowTestAnalysis | null) {
  return readableAnalysisIssue(analysis);
}

function failedExecutionNodes(nodes: ExecutionNodeSnapshot[]) {
  return nodes.filter(node => node.status === 'failed' || Boolean(node.error));
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
  const draftNodeOrderById = React.useMemo(() => {
    const traversalOrder = buildWorkflowDraftTraversalOrderMap(workflowDraft);
    return traversalOrder.size > 0 ? traversalOrder : buildWorkflowDraftNodeOrderMap(workflowDraft);
  }, [workflowDraft]);
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
  const analysis = workflowTestAnalysis(outputs);
  const checks = analysis?.comparisons.checks ?? [];
  const checkCounts = checkCountByStatus(checks);
  const taskEvaluationSchema = analysis?.mode === 'task' ? analysis.evaluation_schema : undefined;
  const taskEvaluationAssertions = compactTaskSchemaAssertions(taskEvaluationSchema);
  const visibleTaskEvaluationAssertions = taskEvaluationAssertions.slice(0, 4);
  const problemChecks = relevantChecks(analysis);
  const primaryProblemChecks = problemChecks.slice(0, 3);
  const executionNodes = sortExecutionNodesByWorkflowOrder(
    extractExecutionNodes(outputs),
    draftNodeOrderById
  );
  const conversationTurns = buildConversationTurnSnapshots(outputs, selectedItem);
  const judgeScore = deriveJudgeScore(selectedItem, outputs);
  const judgeScoreText = formatJudgeScore(judgeScore);
  const questionSnapshot = selectedItem.case_snapshot.content || commonT('none');
  const expectedResult = selectedItem.case_snapshot.expected_result || commonT('none');
  const primaryReasonText =
    (selectedItem.error ? localizeWorkflowTestError(selectedItem.error, t) : '') ||
    (selectedItem.judge_reason ? localizeWorkflowTestError(selectedItem.judge_reason, t) : '') ||
    (selectedItem.status === 'passed' ? t('passedReasonFallback') : commonT('none'));
  const judgeReasonText = selectedItem.judge_reason
    ? localizeWorkflowTestError(selectedItem.judge_reason, t)
    : commonT('none');
  const judgeSuggestionText = selectedItem.judge_suggestion
    ? localizeWorkflowTestError(selectedItem.judge_suggestion, t)
    : '';
  const recommendations =
    selectedItem.status === 'passed'
      ? []
      : buildRecommendationTexts(judgeSuggestionText, analysis, commonT('none'));
  const diagnosisIssue = analysisMainIssue(analysis);
  const displayedDiagnosisStatus =
    selectedItem.status === 'passed' && analysis?.mode === 'task'
      ? 'passed'
      : analysis?.summary.status;
  const failedNodes = failedExecutionNodes(executionNodes);
  const reasonText = primaryReasonText;
  const reasonTitle =
    selectedItem.status === 'passed' && !selectedItem.error
      ? t('passConclusion')
      : t('issueReason');
  const reasonToneClass =
    selectedItem.status === 'passed' && !selectedItem.error
      ? 'border-emerald-200 bg-emerald-50 text-emerald-800'
      : 'border-red-200 bg-red-50 text-red-800';
  const reasonTitleClass =
    selectedItem.status === 'passed' && !selectedItem.error ? 'text-emerald-600' : 'text-red-500';
  const openRawView = (title: string, content: RawViewPayload['content']) =>
    setRawView({ title, content });

  return (
    <div className="min-h-full bg-slate-50 px-8 py-8">
      <div className="mx-auto flex max-w-[1600px] flex-col gap-6">
        <div className="text-sm text-slate-500">
          {t('breadcrumb', { agentName: agentName || commonT('agentFallback') })}
        </div>

        <Card className="rounded-2xl border-slate-200">
          <CardContent className="space-y-8 p-6">
            <div className="flex items-center justify-between gap-4">
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
                      <Link
                        href={`/console/agents/${agentId}/batch-test/${batchId}/items/${previousItem.id}`}
                      >
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
                      <Link
                        href={`/console/agents/${agentId}/batch-test/${batchId}/items/${nextItem.id}`}
                      >
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

            <section className="space-y-3">
              <div className="flex flex-wrap items-center gap-3">
                <h1 className="text-2xl font-semibold text-slate-950">
                  {selectedItem.case_snapshot.content}
                </h1>
                <Badge className={itemStatusClass(selectedItem.status)}>
                  {itemStatusLabel(selectedItem.status, commonT('none'))}
                </Badge>
              </div>
              <div className="flex flex-wrap gap-2 text-sm text-slate-500">
                <Badge variant="outline">{getScenarioName(selectedItem)}</Badge>
                <Badge variant="outline">
                  {formatQuestionTypeLabel(selectedItem.case_snapshot.question_type, typeT)}
                </Badge>
                <Badge variant="outline">{formatResponseTime(selectedItem, commonT('none'))}</Badge>
              </div>
            </section>

            <section className="grid gap-4 lg:grid-cols-[220px_minmax(0,1fr)]">
              <div className={cn('rounded-2xl border p-5', itemStatusClass(selectedItem.status))}>
                <div className="text-sm opacity-80">{t('table.testResult')}</div>
                <div className="mt-2 text-2xl font-semibold">
                  {itemStatusLabel(selectedItem.status, commonT('none'))}
                </div>
                <div className="mt-4 text-sm opacity-90">{t('judgeScore')}</div>
                <div className="mt-1 text-xl font-semibold">{judgeScoreText}</div>
              </div>
              <div className="rounded-2xl border border-slate-200 bg-white p-5">
                <div className="mb-2 text-sm font-medium text-slate-500">
                  {selectedItem.status === 'passed' && !selectedItem.error
                    ? t('passConclusion')
                    : t('issueReason')}
                </div>
                <div className="whitespace-pre-wrap text-base font-medium text-slate-950">
                  {primaryReasonText}
                </div>
                {diagnosisIssue ? (
                  <div className="mt-3 rounded-xl bg-slate-50 px-4 py-3 text-sm text-slate-600">
                    {diagnosisIssue}
                  </div>
                ) : null}
              </div>
            </section>

            <section className="space-y-3">
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
                  {conversationTurns.map((turn, index) => (
                    <div
                      key={`${turn.workflowRunId || 'turn'}-${turn.turnIndex}`}
                      className="rounded-2xl border border-slate-200 bg-white p-5"
                    >
                      <div className="flex items-center justify-between gap-3">
                        <div className="text-base font-semibold text-slate-950">
                          {buildTurnTitle(index)}
                        </div>
                        {analysis?.comparisons.turns?.[index] ? (
                          <Badge
                            className={diagnosisStatusClass(
                              analysis.comparisons.turns[index].status
                            )}
                          >
                            {diagnosisStatusLabel(analysis.comparisons.turns[index].status)}
                          </Badge>
                        ) : null}
                      </div>
                      <div className="mt-4 grid gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
                        <div className="space-y-3">
                          <div className="rounded-xl border border-slate-200 bg-slate-50 p-4">
                            <div className="mb-2 text-xs font-medium text-slate-500">
                              {t('userQuestion')}
                            </div>
                            <div className="whitespace-pre-wrap text-sm text-slate-800">
                              {turn.content || questionSnapshot}
                            </div>
                          </div>
                          {turn.expected || expectedResult ? (
                            <div className="rounded-xl border border-slate-200 bg-slate-50 p-4">
                              <div className="mb-2 text-xs font-medium text-slate-500">
                                期望重点
                              </div>
                              <div className="whitespace-pre-wrap text-sm text-slate-700">
                                {turn.expected || expectedResult}
                              </div>
                            </div>
                          ) : null}
                        </div>
                        <div className="rounded-xl border border-slate-200 bg-slate-50 p-4">
                          <div className="mb-2 text-xs font-medium text-slate-500">
                            {t('agentReply')}
                          </div>
                          <div className="max-h-96 overflow-auto text-sm leading-6 text-slate-800">
                            <WorkflowOutput content={turn.answer} none={commonT('none')} />
                          </div>
                        </div>
                      </div>
                      {SHOW_RAW_DATA_ACTIONS ? (
                        <button
                          type="button"
                          onClick={() => openRawView(`第 ${turn.turnIndex} 轮输出`, turn.outputs)}
                          className="mt-3 text-left text-xs text-slate-500 underline-offset-4 hover:underline"
                        >
                          {t('viewTurnOutputRaw')}
                        </button>
                      ) : null}
                    </div>
                  ))}
                </div>
              ) : (
                <div className="grid gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
                  <div className="space-y-3">
                    <div className="rounded-2xl border border-slate-200 bg-white p-4">
                      <div className="mb-2 text-xs font-medium text-slate-500">
                        {t('userQuestion')}
                      </div>
                      <div className="whitespace-pre-wrap text-sm text-slate-800">
                        {questionSnapshot}
                      </div>
                    </div>
                    <div className="rounded-2xl border border-slate-200 bg-white p-4">
                      <div className="mb-2 text-xs font-medium text-slate-500">期望重点</div>
                      <div className="whitespace-pre-wrap text-sm text-slate-800">
                        {expectedResult}
                      </div>
                    </div>
                  </div>
                  <div className="rounded-2xl border border-slate-200 bg-white p-4">
                    <div className="mb-2 text-xs font-medium text-slate-500">{t('agentReply')}</div>
                    <div className="max-h-96 overflow-auto text-sm leading-6 text-slate-800">
                      <WorkflowOutput content={outputAnswer(outputs)} none={commonT('none')} />
                    </div>
                  </div>
                </div>
              )}
            </section>

            <section className="space-y-3">
              <h2 className="text-lg font-semibold text-slate-950">{t('judgeDetail')}</h2>
              <div className="grid gap-4 lg:grid-cols-[180px_minmax(0,1fr)]">
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
                  <div className="mt-3">
                    <Badge className={itemStatusClass(selectedItem.status)}>
                      {itemStatusLabel(selectedItem.status, commonT('none'))}
                    </Badge>
                  </div>
                </div>
                <div className="rounded-2xl border border-slate-200 bg-white p-4 text-sm text-slate-800">
                  <div className="text-slate-500">{t('judgeOpinion')}</div>
                  <div className="mt-2 whitespace-pre-wrap">{judgeReasonText}</div>
                </div>
              </div>
            </section>

            {recommendations.length > 0 ? (
              <section className="space-y-3">
                <h2 className="text-lg font-semibold text-slate-950">工作流优化建议</h2>
                <div className="rounded-2xl border border-amber-200 bg-amber-50 p-5 text-sm text-amber-950">
                  <div className="mb-3 text-amber-800">
                    以下建议面向工作流配置、节点提示词、分支条件和知识引用，不是要求修改测试问题。
                  </div>
                  <div className="space-y-3">
                    {recommendations.map((recommendation, index) => (
                      <div key={`${recommendation}-${index}`} className="flex gap-3">
                        <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-amber-100 text-xs font-semibold text-amber-700">
                          {index + 1}
                        </span>
                        <div className="whitespace-pre-wrap leading-6">{recommendation}</div>
                      </div>
                    ))}
                  </div>
                </div>
              </section>
            ) : null}

            <section className="space-y-3">
              <h2 className="text-lg font-semibold text-slate-950">{t('executionRecord')}</h2>
              <div className="rounded-2xl border border-slate-200 bg-white p-4">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div>
                    <div className="text-sm font-medium text-slate-950">{t('executionPath')}</div>
                    <div className="mt-1 text-xs text-slate-500">
                      {failedNodes.length > 0
                        ? `${failedNodes.length} 个节点需要关注`
                        : displayedDiagnosisStatus
                          ? `诊断结果：${diagnosisStatusLabel(displayedDiagnosisStatus)}`
                          : commonT('none')}
                    </div>
                  </div>
                  {displayedDiagnosisStatus ? (
                    <Badge className={diagnosisStatusClass(displayedDiagnosisStatus)}>
                      {diagnosisStatusLabel(displayedDiagnosisStatus)}
                    </Badge>
                  ) : null}
                </div>

                <div className="mt-4 flex flex-wrap items-center gap-2">
                  {executionNodes.length > 0 ? (
                    executionNodes.map((node, index) => {
                      const meta =
                        draftNodeMetaById.get(node.id) || fallbackExecutionStepMeta(node.id);
                      const theme =
                        meta.type in NODE_THEMES
                          ? NODE_THEMES[meta.type as keyof typeof NODE_THEMES]
                          : undefined;
                      const Icon = theme?.icon;
                      return (
                        <React.Fragment key={node.id}>
                          <div
                            className={cn(
                              'inline-flex items-center gap-2 rounded-full border px-3 py-1 text-sm font-medium',
                              node.status === 'failed' || node.error
                                ? 'border-red-200 bg-red-50 text-red-700'
                                : 'border-slate-200 bg-white text-slate-700'
                            )}
                          >
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
                          {index < executionNodes.length - 1 ? (
                            <span className="text-slate-300">-&gt;</span>
                          ) : null}
                        </React.Fragment>
                      );
                    })
                  ) : (
                    <span className="text-sm text-slate-500">{commonT('none')}</span>
                  )}
                </div>

                {taskEvaluationSchema ? (
                  <div className="mt-4 rounded-xl border border-slate-200 bg-slate-50 p-4">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <div className="text-sm font-medium text-slate-950">任务评价标准</div>
                        <div className="mt-1 text-xs text-slate-500">
                          评分按任务输入和最终输出判断，不要求任务工作流像对话一样追问或保留上下文。
                        </div>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        <Badge variant="outline">
                          {taskGoalTypeLabel(taskEvaluationSchema.goal_type)}
                        </Badge>
                        {taskEvaluationSchema.source_grounding ? (
                          <Badge variant="outline">依据输入输出</Badge>
                        ) : null}
                      </div>
                    </div>

                    {taskEvaluationSchema.primary_objective ? (
                      <div className="mt-3 rounded-lg bg-white px-3 py-2 text-sm text-slate-700">
                        {taskEvaluationSchema.primary_objective}
                      </div>
                    ) : null}

                    {visibleTaskEvaluationAssertions.length > 0 ? (
                      <div className="mt-3 grid gap-2 lg:grid-cols-2">
                        {visibleTaskEvaluationAssertions.map((assertion, index) => (
                          <div
                            key={`${assertion.id || assertion.type || 'assertion'}-${index}`}
                            className="rounded-lg border border-slate-200 bg-white p-3 text-sm"
                          >
                            <div className="flex flex-wrap items-center gap-2">
                              <Badge variant="outline">
                                {taskAssertionTypeLabel(assertion.type)}
                              </Badge>
                              {assertion.severity ? (
                                <span className="text-xs text-slate-500">
                                  {taskAssertionSeverityLabel(assertion.severity)}
                                </span>
                              ) : null}
                            </div>
                            <div className="mt-2 line-clamp-2 text-slate-800">
                              {taskAssertionText(assertion)}
                            </div>
                          </div>
                        ))}
                      </div>
                    ) : null}

                    <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-600">
                      {taskEvaluationSchema.allowed_extra_types?.length ? (
                        <span className="rounded-full bg-white px-2 py-1">
                          允许合理附加：{taskEvaluationSchema.allowed_extra_types.join('、')}
                        </span>
                      ) : null}
                      {taskEvaluationSchema.missing_policy?.mode ? (
                        <span className="rounded-full bg-white px-2 py-1">
                          缺失策略：允许标注业务缺失，禁止误报技术失败
                        </span>
                      ) : null}
                    </div>

                    {taskEvaluationAssertions.length > visibleTaskEvaluationAssertions.length ? (
                      <details className="mt-3">
                        <summary className="cursor-pointer text-sm font-medium text-slate-700">
                          查看全部评价断言（{taskEvaluationAssertions.length}）
                        </summary>
                        <div className="mt-2 space-y-2">
                          {taskEvaluationAssertions
                            .slice(visibleTaskEvaluationAssertions.length)
                            .map((assertion, index) => (
                              <div
                                key={`${assertion.id || assertion.type || 'assertion-more'}-${index}`}
                                className="rounded-lg bg-white px-3 py-2 text-sm text-slate-700"
                              >
                                <span className="mr-2 font-medium text-slate-900">
                                  {taskAssertionTypeLabel(assertion.type)}
                                </span>
                                {taskAssertionText(assertion)}
                              </div>
                            ))}
                        </div>
                      </details>
                    ) : null}
                  </div>
                ) : null}

                {checks.length ? (
                  <div className="mt-4 rounded-xl border border-slate-200 bg-slate-50 p-4">
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <div className="text-sm font-medium text-slate-950">检查点摘要</div>
                      <div className="flex flex-wrap gap-2 text-xs">
                        <Badge className="border-emerald-200 bg-emerald-50 text-emerald-700">
                          通过 {checkCounts.passed}
                        </Badge>
                        <Badge className="border-red-200 bg-red-50 text-red-700">
                          不通过 {checkCounts.failed}
                        </Badge>
                        <Badge className="border-amber-200 bg-amber-50 text-amber-700">
                          需复核 {checkCounts.review}
                        </Badge>
                      </div>
                    </div>

                    {primaryProblemChecks.length > 0 ? (
                      <div className="mt-3 grid gap-2">
                        {primaryProblemChecks.map((check, index) => (
                          <div
                            key={workflowTestCheckKey(check, index)}
                            className="rounded-lg border border-slate-200 bg-white p-3 text-sm"
                          >
                            <div className="flex flex-wrap items-center justify-between gap-2">
                              <div className="font-medium text-slate-900">
                                {isGenericCheckLabel(check.label)
                                  ? '回复是否覆盖本轮期望'
                                  : check.label}
                              </div>
                              <Badge className={diagnosisStatusClass(check.status)}>
                                {diagnosisStatusLabel(check.status)}
                              </Badge>
                            </div>
                            {check.evidence ? (
                              <div className="mt-2 line-clamp-2 text-slate-600">
                                {check.evidence}
                              </div>
                            ) : null}
                          </div>
                        ))}
                      </div>
                    ) : null}

                    <details className="mt-3">
                      <summary className="cursor-pointer text-sm font-medium text-slate-700">
                        查看全部检查点明细（{checks.length}）
                      </summary>
                      <div className="mt-3 max-h-96 space-y-2 overflow-auto pr-1">
                        {checks.map((check, index) => (
                          <div
                            key={workflowTestCheckKey(check, index)}
                            className="rounded-lg border border-slate-200 bg-white p-3 text-sm"
                          >
                            <div className="flex flex-wrap items-center justify-between gap-2">
                              <div className="font-medium text-slate-900">
                                {isGenericCheckLabel(check.label)
                                  ? '回复是否覆盖本轮期望'
                                  : check.label}
                              </div>
                              <Badge className={diagnosisStatusClass(check.status)}>
                                {diagnosisStatusLabel(check.status)}
                              </Badge>
                            </div>
                            {check.evidence ? (
                              <div className="mt-2 text-slate-600">{check.evidence}</div>
                            ) : null}
                          </div>
                        ))}
                      </div>
                    </details>
                  </div>
                ) : null}

                {analysis?.trace.nodes.length ? (
                  <details className="mt-4 rounded-xl border border-slate-200 bg-slate-50 p-3">
                    <summary className="cursor-pointer text-sm font-medium text-slate-900">
                      工程排障信息：节点输入输出
                    </summary>
                    <div className="mt-2 text-xs text-slate-500">
                      仅在运行日志记录了可展示数据时显示。空白不代表节点没有执行，可能是该节点未产出前端可展示字段或日志未保存详细输入输出。
                    </div>
                    <div className="mt-3 space-y-2">
                      {analysis.trace.nodes.map((node, index) => (
                        <details
                          key={`${node.node_id}-${index}`}
                          className="rounded-lg border border-slate-200 bg-white p-3"
                        >
                          <summary className="cursor-pointer text-sm font-medium text-slate-900">
                            {node.node_name || node.node_id || commonT('none')}
                            <span className="ml-2 text-xs font-normal text-slate-500">
                              {node.node_type || commonT('none')} · {node.status || commonT('none')}
                              {node.duration_ms > 0
                                ? ` · ${formatWorkflowElapsedMs(node.duration_ms)}`
                                : ''}
                            </span>
                          </summary>
                          {hasDisplayValue(node.input) || hasDisplayValue(node.output) ? (
                            <div className="mt-3 grid gap-3 lg:grid-cols-2">
                              {hasDisplayValue(node.input) ? (
                                <div className="rounded-lg bg-slate-50 p-3">
                                  <div className="mb-2 text-xs font-medium text-slate-500">
                                    输入
                                  </div>
                                  <pre className="max-h-64 overflow-auto whitespace-pre-wrap text-xs text-slate-700">
                                    {stringifyJson(node.input, commonT('none'))}
                                  </pre>
                                </div>
                              ) : null}
                              {hasDisplayValue(node.output) ? (
                                <div className="rounded-lg bg-slate-50 p-3">
                                  <div className="mb-2 text-xs font-medium text-slate-500">
                                    输出
                                  </div>
                                  <pre className="max-h-64 overflow-auto whitespace-pre-wrap text-xs text-slate-700">
                                    {stringifyJson(node.output, commonT('none'))}
                                  </pre>
                                </div>
                              ) : null}
                            </div>
                          ) : (
                            <div className="mt-3 rounded-lg bg-slate-50 px-3 py-2 text-sm text-slate-500">
                              运行日志未记录该节点的可展示输入/输出。
                            </div>
                          )}
                          {node.error ? (
                            <div className="mt-3 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">
                              {node.error}
                            </div>
                          ) : null}
                        </details>
                      ))}
                    </div>
                  </details>
                ) : null}
              </div>
            </section>
          </CardContent>
        </Card>

        {/*
        <div className="hidden" aria-hidden="true">
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

            {analysis ? (
              <section className="space-y-3">
                <h2 className="text-lg font-semibold text-slate-950">工作流诊断</h2>
                <div className="grid gap-4 lg:grid-cols-[220px_minmax(0,1fr)]">
                  <div className={cn('rounded-2xl border p-4', diagnosisStatusClass(analysis.summary.status))}>
                    <div className="text-sm opacity-80">诊断结论</div>
                    <div className="mt-2 text-2xl font-semibold">
                      {diagnosisStatusLabel(analysis.summary.status)}
                    </div>
                    <div className="mt-3 text-sm">
                      {analysis.summary.main_issue || '暂未发现明确问题'}
                    </div>
                    <div className="mt-4 grid grid-cols-3 gap-2 text-center text-xs">
                      <div className="rounded-xl bg-white/70 px-2 py-2">
                        <div className="font-semibold">{analysis.summary.passed}</div>
                        <div>通过</div>
                      </div>
                      <div className="rounded-xl bg-white/70 px-2 py-2">
                        <div className="font-semibold">{analysis.summary.failed}</div>
                        <div>失败</div>
                      </div>
                      <div className="rounded-xl bg-white/70 px-2 py-2">
                        <div className="font-semibold">{analysis.summary.review}</div>
                        <div>复核</div>
                      </div>
                    </div>
                  </div>

                  <div className="space-y-4">
                    {analysis.comparisons.overall ? (
                      <div className="rounded-2xl border border-slate-200 bg-white p-4 text-sm">
                        <div className="flex items-center justify-between gap-3">
                          <div className="font-medium text-slate-950">整体预期对比</div>
                          <Badge className={diagnosisStatusClass(analysis.comparisons.overall.status)}>
                            {diagnosisStatusLabel(analysis.comparisons.overall.status)}
                          </Badge>
                        </div>
                        <div className="mt-3 grid gap-3 lg:grid-cols-2">
                          <div className="rounded-xl bg-slate-50 p-3">
                            <div className="mb-1 text-xs text-slate-500">预期</div>
                            <div className="whitespace-pre-wrap text-slate-800">
                              {analysis.comparisons.overall.expected || commonT('none')}
                            </div>
                          </div>
                          <div className="rounded-xl bg-slate-50 p-3">
                            <div className="mb-1 text-xs text-slate-500">实际</div>
                            <div className="whitespace-pre-wrap text-slate-800">
                              {analysis.comparisons.overall.actual || commonT('none')}
                            </div>
                          </div>
                        </div>
                        <div className="mt-3 text-slate-600">{analysis.comparisons.overall.evidence}</div>
                      </div>
                    ) : null}

                    {analysis.comparisons.checks.length > 0 ? (
                      <div className="rounded-2xl border border-slate-200 bg-white p-4">
                        <div className="font-medium text-slate-950">检查点结果</div>
                        <div className="mt-3 space-y-2">
                          {analysis.comparisons.checks.map((check, index) => (
                            <div key={workflowTestCheckKey(check, index)} className="rounded-xl border border-slate-200 bg-slate-50 p-3 text-sm">
                              <div className="flex flex-wrap items-center justify-between gap-2">
                                <div className="font-medium text-slate-900">{check.label}</div>
                                <div className="flex items-center gap-2">
                                  {check.severity === 'critical' ? (
                                    <Badge className="bg-red-50 text-red-700">关键</Badge>
                                  ) : null}
                                  <Badge className={diagnosisStatusClass(check.status)}>
                                    {diagnosisStatusLabel(check.status)}
                                  </Badge>
                                </div>
                              </div>
                              <div className="mt-2 text-slate-600">{check.evidence}</div>
                              {check.suggestion ? (
                                <div className="mt-2 rounded-lg bg-amber-50 px-3 py-2 text-amber-800">
                                  {check.suggestion}
                                </div>
                              ) : null}
                            </div>
                          ))}
                        </div>
                      </div>
                    ) : null}
                  </div>
                </div>

                {analysis.trace.nodes.length > 0 ? (
                  <div className="rounded-2xl border border-slate-200 bg-white p-4">
                    <div className="font-medium text-slate-950">节点输入输出</div>
                    <div className="mt-3 space-y-2">
                      {analysis.trace.nodes.map((node, index) => (
                        <details key={`${node.node_id}-${index}`} className="rounded-xl border border-slate-200 bg-slate-50 p-3">
                          <summary className="cursor-pointer text-sm font-medium text-slate-900">
                            {node.node_name || node.node_id || commonT('none')}
                            <span className="ml-2 text-xs font-normal text-slate-500">
                              {node.node_type || commonT('none')} · {node.status || commonT('none')}
                              {node.duration_ms > 0 ? ` · ${formatWorkflowElapsedMs(node.duration_ms)}` : ''}
                            </span>
                          </summary>
                          <div className="mt-3 grid gap-3 lg:grid-cols-2">
                            <div className="rounded-lg bg-white p-3">
                              <div className="mb-2 text-xs font-medium text-slate-500">输入</div>
                              <pre className="max-h-64 overflow-auto whitespace-pre-wrap text-xs text-slate-700">
                                {stringifyJson(node.input, commonT('none'))}
                              </pre>
                            </div>
                            <div className="rounded-lg bg-white p-3">
                              <div className="mb-2 text-xs font-medium text-slate-500">输出</div>
                              <pre className="max-h-64 overflow-auto whitespace-pre-wrap text-xs text-slate-700">
                                {stringifyJson(node.output, commonT('none'))}
                              </pre>
                            </div>
                          </div>
                          {node.error ? (
                            <div className="mt-3 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">
                              {node.error}
                            </div>
                          ) : null}
                        </details>
                      ))}
                    </div>
                  </div>
                ) : null}

                {analysis.suggestions.length > 0 ? (
                  <div className="rounded-2xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
                    <div className="font-medium">优化建议</div>
                    <div className="mt-2 space-y-2">
                      {analysis.suggestions.map((suggestion, index) => (
                        <div key={`${suggestion.target}-${index}`}>
                          <span className="font-medium">{suggestion.target}：</span>
                          {suggestion.content}
                        </div>
                      ))}
                    </div>
                  </div>
                ) : null}
              </section>
            ) : null}

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
        */}
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
