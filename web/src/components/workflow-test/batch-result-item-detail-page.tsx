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
import {
  getWorkflowTestUserError,
  type WorkflowTestErrorTranslator,
} from '@/utils/workflow-test-error';
import type {
  WorkflowTestAnalysis,
  WorkflowTestBatchItem,
  WorkflowTestCheckResult,
  WorkflowTestEvaluationAssertion,
  WorkflowTestEvaluationSchema,
} from '@/services/types/workflow-test';
import { useWorkflowDraft } from '@/hooks/workflow/use-workflow';
import { useWorkflowRunNodeExecutions } from '@/hooks/workflow/use-workflow-run-node-executions';
import { NODE_THEMES } from '@/components/workflow/nodes/custom/config';
import { cn } from '@/lib/utils';
import MarkdownViewer from '@/components/common/markdown-viewer';
import type { WorkflowNodeExecution } from '@/services/types/workflow';
import { formatQuestionTypeLabel } from './question-type';
import { getAgentDetailBatchTestHref } from '@/utils/agent-detail-routes';

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

type ItemStatusKey = 'pending' | 'running' | 'passed' | 'failed' | 'review' | 'canceled';
type TaskGoalTypeKey =
  | 'extract'
  | 'classify'
  | 'transform'
  | 'analyze'
  | 'decision'
  | 'action'
  | 'general';
type TaskAssertionTypeKey =
  | 'must_include'
  | 'must_not_include'
  | 'missing_policy'
  | 'fact_present'
  | 'missing_field_marked'
  | 'state_present'
  | 'format'
  | 'source_grounding'
  | 'action_result'
  | 'semantic_match'
  | 'assertion';
type TaskAssertionSeverityKey = 'critical' | 'normal' | 'hint';

type ItemStatusLabels = Record<ItemStatusKey, string>;
type TaskGoalTypeLabels = Record<TaskGoalTypeKey, string>;
type TaskAssertionTypeLabels = Record<TaskAssertionTypeKey, string>;
type TaskAssertionSeverityLabels = Record<TaskAssertionSeverityKey, string>;

interface TextLocalizationLabels {
  labelSeparator: string;
  listSeparator: string;
  missingFieldDate: string;
  missingFieldRelatedParty: string;
}

interface WorkflowTestErrorLabels {
  judgeModelRequired: string;
  judgeNotConfigured: string;
  judgeEmptyResult: string;
  judgeFailed: string;
  judgeFailedSuggestion: string;
  judgeManualReviewSuggestion: string;
  judgeConfigureSuggestion: string;
  modelUnavailable: string;
  modelUnavailableMarker: string;
  text: TextLocalizationLabels;
}

interface RecommendationLabels {
  genericTarget: string;
  relatedNode: string;
  failedCheck: (label: string) => string;
  reviewCheck: (label: string) => string;
  excludedTerms: string[];
}

interface FallbackNodeLabels {
  start: string;
  answer: string;
  branch: string;
}

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

function itemStatusLabel(status: string, labels: ItemStatusLabels, none: string) {
  return labels[status as ItemStatusKey] || status || none;
}

function diagnosisStatusLabel(status: string, labels: ItemStatusLabels) {
  return labels[status as ItemStatusKey] || status || labels.review;
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
  const trace = workflowTestTraceNodes(outputs);
  if (trace.some(node => node.status === 'failed' || Boolean(node.error))) {
    return true;
  }
  return false;
}

function workflowTestTraceNodes(outputs: Record<string, unknown>): Array<Record<string, unknown>> {
  const trace = outputs.workflow_trace;
  if (!isRecord(trace)) {
    return [];
  }
  const nodes = trace.nodes;
  if (!Array.isArray(nodes)) {
    return [];
  }
  return nodes.filter(isRecord);
}

interface ExecutionFailureLabels {
  withReason: (reason: string) => string;
  localizeReason: (reason: string) => string;
  noReply: string;
  nodeFailed: string;
}

function executionFailureText(
  outputs: Record<string, unknown>,
  labels: ExecutionFailureLabels,
  itemError?: string
) {
  if (itemError?.trim()) {
    return labels.withReason(labels.localizeReason(itemError.trim()));
  }
  if (typeof outputs.error === 'string' && outputs.error.trim()) {
    return labels.withReason(labels.localizeReason(outputs.error.trim()));
  }
  const traceError = workflowTestTraceNodes(outputs)
    .map(node => {
      const error = typeof node.error === 'string' ? node.error.trim() : '';
      if (!error && node.status !== 'failed') return '';
      const nodeLabel =
        (typeof node.node_name === 'string' && node.node_name.trim()) ||
        (typeof node.node_id === 'string' && node.node_id.trim()) ||
        (typeof node.id === 'string' && node.id.trim()) ||
        '';
      if (error && nodeLabel) return `${nodeLabel}: ${labels.localizeReason(error)}`;
      if (error) return labels.localizeReason(error);
      if (nodeLabel) return `${nodeLabel}: ${labels.nodeFailed}`;
      return labels.nodeFailed;
    })
    .find(Boolean);
  if (traceError) {
    return labels.withReason(traceError);
  }
  if (outputs.status === 'failed') {
    return labels.noReply;
  }
  return '';
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
    return null;
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
  labels: WorkflowTestErrorLabels,
  errorT: WorkflowTestErrorTranslator
) {
  const normalized = message.trim().toLowerCase();
  if (!normalized) return message;
  const userError = getWorkflowTestUserError(message, errorT);
  if (userError) return userError;
  if (
    normalized.includes('all providers failed') ||
    normalized.includes('current user api does not support http call') ||
    normalized.includes('upstream service error') ||
    normalized.includes('no provider available') ||
    normalized.includes('model unavailable') ||
    normalized.includes(labels.modelUnavailableMarker.toLowerCase())
  ) {
    return labels.modelUnavailable;
  }
  if (
    normalized.includes('judge failed: model field is required') ||
    normalized.includes('model field is required')
  ) {
    return labels.judgeModelRequired;
  }
  if (normalized.includes('judge is not configured')) {
    return labels.judgeNotConfigured;
  }
  if (normalized.includes('judge returned empty result')) {
    return labels.judgeEmptyResult;
  }
  if (
    normalized === 'ai scoring failed; review manually or rerun the test' ||
    normalized === labels.judgeFailedSuggestion.toLowerCase()
  ) {
    return labels.judgeFailedSuggestion;
  }
  if (
    normalized === 'review this result manually' ||
    normalized === labels.judgeManualReviewSuggestion.toLowerCase()
  ) {
    return labels.judgeManualReviewSuggestion;
  }
  if (
    normalized === 'configure ai scoring and rerun, or review manually' ||
    normalized === labels.judgeConfigureSuggestion.toLowerCase()
  ) {
    return labels.judgeConfigureSuggestion;
  }
  if (normalized === 'judge failed') {
    return labels.judgeFailed;
  }
  if (normalized.startsWith('judge failed: ')) {
    const sourceDetail = message.trim().slice('judge failed: '.length);
    const detail = translateMissingFieldPhrases(sourceDetail, labels.text);
    return detail !== sourceDetail
      ? `${labels.judgeFailed}${labels.text.labelSeparator}${detail}`
      : labels.judgeFailed;
  }
  const translated = translateMissingFieldPhrases(message, labels.text);
  if (translated !== message || /\p{Script=Han}/u.test(translated)) return translated;
  return errorT('unknown');
}

function translateMissingFieldPhrases(value: string, labels: TextLocalizationLabels) {
  return value
    .replace(/missing field:\s*date/gi, labels.missingFieldDate)
    .replace(/missing field:\s*related party/gi, labels.missingFieldRelatedParty)
    .replace(/missing fields:\s*date/gi, labels.missingFieldDate)
    .replace(/missing fields:\s*related party/gi, labels.missingFieldRelatedParty);
}

function workflowTestCheckKey(check: WorkflowTestCheckResult, index: number) {
  const parts = [check.id, check.type, check.label, check.severity, String(index)]
    .map(part => String(part || '').trim())
    .filter(Boolean);
  return parts.length > 0 ? parts.join('::') : `check::${index}`;
}

function isGenericCheck(check: Pick<WorkflowTestCheckResult, 'id' | 'type'>) {
  return check.type === 'expected_result' && /^turn_\d+_expected$/.test(check.id);
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

function taskGoalTypeLabel(value: string | undefined, labels: TaskGoalTypeLabels) {
  return labels[String(value || '').trim() as TaskGoalTypeKey] || value || labels.general;
}

function taskAssertionTypeLabel(value: string | undefined, labels: TaskAssertionTypeLabels) {
  return labels[String(value || '').trim() as TaskAssertionTypeKey] || value || labels.assertion;
}

function taskAssertionSeverityLabel(
  value: string | undefined,
  labels: TaskAssertionSeverityLabels
) {
  return labels[String(value || '').trim() as TaskAssertionSeverityKey] || value || labels.normal;
}

function taskAssertionText(
  assertion: WorkflowTestEvaluationAssertion,
  typeLabels: TaskAssertionTypeLabels,
  textLabels: TextLocalizationLabels
) {
  const description = typeof assertion.description === 'string' ? assertion.description.trim() : '';
  if (description) return translateMissingFieldPhrases(description, textLabels);
  const values = Array.isArray(assertion.values)
    ? assertion.values.filter(Boolean).join(textLabels.listSeparator)
    : '';
  return translateMissingFieldPhrases(
    values || taskAssertionTypeLabel(assertion.type, typeLabels),
    textLabels
  );
}

function compactTaskSchemaAssertions(schema: WorkflowTestEvaluationSchema | undefined) {
  return Array.isArray(schema?.assertions)
    ? (schema.assertions.filter(isRecord) as unknown as WorkflowTestEvaluationAssertion[])
    : [];
}

function readableAnalysisIssue(
  analysis: WorkflowTestAnalysis | null,
  textLabels: TextLocalizationLabels
) {
  const firstProblem = relevantChecks(analysis)[0];
  if (firstProblem?.evidence?.trim()) {
    return translateMissingFieldPhrases(firstProblem.evidence.trim(), textLabels);
  }
  const issue = analysis?.summary?.main_issue;
  if (typeof issue !== 'string') return '';
  const trimmed = issue.trim();
  return trimmed && analysis?.summary?.failed_stage !== 'expected_result'
    ? translateMissingFieldPhrases(trimmed, textLabels)
    : '';
}

function workflowOptimizationText(
  check: WorkflowTestCheckResult,
  textLabels: TextLocalizationLabels,
  labels: RecommendationLabels
) {
  const label = translateMissingFieldPhrases(
    isGenericCheck(check) ? labels.genericTarget : check.label || labels.relatedNode,
    textLabels
  );
  if (check.status === 'failed') {
    return labels.failedCheck(label);
  }
  return labels.reviewCheck(label);
}

function hasDisplayValue(value: unknown) {
  if (value === undefined || value === null || value === '') return false;
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === 'object') return Object.keys(value as Record<string, unknown>).length > 0;
  return true;
}

interface DiagnosticTraceNode {
  node_id: string;
  node_name: string;
  node_type: string;
  status: string;
  duration_ms: number;
  input: unknown;
  output: unknown;
  error: string;
}

function diagnosticExecutionError(error: WorkflowNodeExecution['error']) {
  if (typeof error === 'string') return error.trim();
  if (error && typeof error === 'object') return stringifyJson(error, '');
  return '';
}

function buildDiagnosticTraceNodes(
  records: WorkflowNodeExecution[],
  fallback: WorkflowTestAnalysis['trace']['nodes']
): DiagnosticTraceNode[] {
  const nodes: DiagnosticTraceNode[] =
    records.length > 0
      ? records.map(record => ({
          node_id: record.node_id,
          node_name: record.title,
          node_type: record.node_type,
          status: record.status,
          duration_ms: record.elapsed_time,
          input: record.inputs,
          output: record.outputs,
          error: diagnosticExecutionError(record.error),
        }))
      : fallback.map(node => ({
          node_id: node.node_id,
          node_name: node.node_name,
          node_type: node.node_type,
          status: node.status,
          duration_ms: node.duration_ms,
          input: node.input,
          output: node.output,
          error: node.error ?? '',
        }));

  return nodes.filter(
    node =>
      hasDisplayValue(node.input) || hasDisplayValue(node.output) || Boolean(node.error.trim())
  );
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

function fallbackExecutionStepMeta(
  nodeId: string,
  labels: FallbackNodeLabels
): WorkflowDraftNodeMeta {
  if (!nodeId) return { id: nodeId, title: nodeId, type: '' };
  if (nodeId.includes('start')) return { id: nodeId, title: labels.start, type: 'start' };
  if (nodeId.includes('answer') || nodeId.includes('reply')) {
    return { id: nodeId, title: labels.answer, type: 'answer' };
  }
  if (nodeId.includes('llm')) return { id: nodeId, title: 'LLM', type: 'llm' };
  if (nodeId.includes('branch')) return { id: nodeId, title: labels.branch, type: 'if-else' };
  return { id: nodeId, title: nodeId, type: '' };
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

function outputAnswer(
  outputs: Record<string, unknown>,
  labels: ExecutionFailureLabels,
  itemError?: string
) {
  const failureText = executionFailureText(outputs, labels, itemError);
  if (failureText) {
    return failureText;
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
  item: WorkflowTestBatchItem,
  labels: ExecutionFailureLabels
): ConversationTurnSnapshot[] {
  const turnResults = extractTurnResultSnapshots(outputs);
  const plannedTurns = item.case_snapshot.turns?.length
    ? item.case_snapshot.turns
    : [
        {
          content: item.case_snapshot.content,
          expected_result: item.case_snapshot.expected_result,
        },
      ];
  if (turnResults.length > 0) {
    const resultsByIndex = new Map(turnResults.map(turn => [turn.turnIndex, turn]));
    const lastExecutedTurnIndex = Math.max(...turnResults.map(turn => turn.turnIndex));
    const turnCount = Math.max(plannedTurns.length, lastExecutedTurnIndex);
    return Array.from({ length: turnCount }, (_, index) => {
      const fallbackTurn = plannedTurns[index];
      const turn = resultsByIndex.get(index + 1);
      return {
        turnIndex: index + 1,
        content: turn?.content || fallbackTurn?.content || item.case_snapshot.content || '',
        workflowRunId: turn?.workflowRunId || '',
        outputs: turn?.outputs || {},
        answer:
          turn && turn.turnIndex === lastExecutedTurnIndex
            ? outputAnswer(turn.outputs, labels, item.error)
            : turn
              ? outputAnswer(turn.outputs, labels)
              : '',
        expected: fallbackTurn?.expected_result || item.case_snapshot.expected_result || '',
      };
    });
  }

  const answer = outputAnswer(outputs, labels, item.error);
  if (!answer.trim()) {
    return [];
  }
  return plannedTurns.map((turn, index) => ({
    turnIndex: index + 1,
    content: turn.content || (index === 0 ? item.case_snapshot.content : ''),
    workflowRunId: index === 0 ? item.workflow_run_id || '' : '',
    outputs: index === 0 ? outputs : {},
    answer: index === 0 ? answer : '',
    expected: turn.expected_result || item.case_snapshot.expected_result || '',
  }));
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
  none: string,
  textLabels: TextLocalizationLabels,
  labels: RecommendationLabels
) {
  if (analysis?.mode === 'task' && analysis.summary?.status === 'passed') {
    return [];
  }
  const values: string[] = [];
  const normalizedJudgeSuggestion = judgeSuggestionText.trim().toLowerCase();
  if (
    judgeSuggestionText &&
    judgeSuggestionText !== none &&
    !labels.excludedTerms.some(term => normalizedJudgeSuggestion.includes(term.toLowerCase()))
  ) {
    values.push(translateMissingFieldPhrases(judgeSuggestionText, textLabels));
  }
  analysis?.suggestions.forEach(suggestion => {
    const target =
      typeof suggestion.target === 'string'
        ? translateMissingFieldPhrases(suggestion.target.trim(), textLabels)
        : '';
    const content =
      typeof suggestion.content === 'string'
        ? translateMissingFieldPhrases(suggestion.content.trim(), textLabels)
        : '';
    if (!content) return;
    if (labels.excludedTerms.some(term => content.toLowerCase().includes(term.toLowerCase()))) {
      return;
    }
    values.push(
      target && suggestion.type !== 'expected_result' ? `${target}: ${content}` : content
    );
  });
  relevantChecks(analysis)
    .filter(check => check.status === 'failed')
    .slice(0, 3)
    .forEach(check => {
      values.push(workflowOptimizationText(check, textLabels, labels));
    });
  return uniqueTexts(values).slice(0, 5);
}

function analysisMainIssue(
  analysis: WorkflowTestAnalysis | null,
  textLabels: TextLocalizationLabels
) {
  return readableAnalysisIssue(analysis, textLabels);
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
  const errorT = useT('agents.workflowTest.userErrors');
  const itemStatusLabels: ItemStatusLabels = {
    pending: t('itemStatus.pending'),
    running: t('itemStatus.running'),
    passed: t('itemStatus.passed'),
    failed: t('itemStatus.failed'),
    review: t('itemStatus.review'),
    canceled: t('itemStatus.canceled'),
  };
  const textLabels: TextLocalizationLabels = {
    labelSeparator: t('labelSeparator'),
    listSeparator: t('listSeparator'),
    missingFieldDate: t('errors.missingFieldDate'),
    missingFieldRelatedParty: t('errors.missingFieldRelatedParty'),
  };
  const errorLabels: WorkflowTestErrorLabels = {
    judgeModelRequired: t('errors.judgeModelRequired'),
    judgeNotConfigured: t('errors.judgeNotConfigured'),
    judgeEmptyResult: t('errors.judgeEmptyResult'),
    judgeFailed: t('errors.judgeFailed'),
    judgeFailedSuggestion: t('errors.judgeFailedSuggestion'),
    judgeManualReviewSuggestion: t('errors.judgeManualReviewSuggestion'),
    judgeConfigureSuggestion: t('errors.judgeConfigureSuggestion'),
    modelUnavailable: t('errors.modelUnavailable'),
    modelUnavailableMarker: t('errors.modelUnavailableMarker'),
    text: textLabels,
  };
  const taskGoalTypeLabels: TaskGoalTypeLabels = {
    extract: t('taskGoalType.extract'),
    classify: t('taskGoalType.classify'),
    transform: t('taskGoalType.transform'),
    analyze: t('taskGoalType.analyze'),
    decision: t('taskGoalType.decision'),
    action: t('taskGoalType.action'),
    general: t('taskGoalType.general'),
  };
  const taskAssertionTypeLabels: TaskAssertionTypeLabels = {
    must_include: t('taskAssertionType.mustInclude'),
    must_not_include: t('taskAssertionType.mustNotInclude'),
    missing_policy: t('taskAssertionType.missingPolicy'),
    fact_present: t('taskAssertionType.factPresent'),
    missing_field_marked: t('taskAssertionType.missingFieldMarked'),
    state_present: t('taskAssertionType.statePresent'),
    format: t('taskAssertionType.format'),
    source_grounding: t('taskAssertionType.sourceGrounding'),
    action_result: t('taskAssertionType.actionResult'),
    semantic_match: t('taskAssertionType.semanticMatch'),
    assertion: t('taskAssertionType.assertion'),
  };
  const taskAssertionSeverityLabels: TaskAssertionSeverityLabels = {
    critical: t('taskAssertionSeverity.critical'),
    normal: t('taskAssertionSeverity.normal'),
    hint: t('taskAssertionSeverity.hint'),
  };
  const recommendationLabels: RecommendationLabels = {
    genericTarget: t('recommendations.genericTarget'),
    relatedNode: t('recommendations.relatedNode'),
    failedCheck: label => t('recommendations.failedCheck', { label }),
    reviewCheck: label => t('recommendations.reviewCheck', { label }),
    excludedTerms: [
      t('recommendations.filters.aiEvaluation'),
      t('recommendations.filters.manualReview'),
      t('recommendations.filters.checkEvaluation'),
      'ai scoring',
    ],
  };
  const fallbackNodeLabels: FallbackNodeLabels = {
    start: t('fallbackNode.start'),
    answer: t('fallbackNode.answer'),
    branch: t('fallbackNode.branch'),
  };
  const getItemStatusLabel = (status: string) =>
    itemStatusLabel(status, itemStatusLabels, commonT('none'));
  const getDiagnosisStatusLabel = (status: string) =>
    diagnosisStatusLabel(status, itemStatusLabels);
  const batchResultHref = `${getAgentDetailBatchTestHref(agentId, 'workflow')}/${batchId}`;
  const getBatchItemHref = (nextItemId: string) => `${batchResultHref}/items/${nextItemId}`;
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
  const workflowRunId = selectedItem?.workflow_run_id || null;
  const { records: workflowNodeExecutions } = useWorkflowRunNodeExecutions(
    { agentId, runId: workflowRunId },
    { enabled: Boolean(workflowRunId) }
  );
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
  const executionFailureLabels: ExecutionFailureLabels = {
    withReason: reason => t('executionFailedWithReason', { reason }),
    localizeReason: reason => localizeWorkflowTestError(reason, errorLabels, errorT),
    noReply: t('executionFailedNoReply'),
    nodeFailed: t('nodeExecutionFailed'),
  };
  const analysis = workflowTestAnalysis(outputs);
  const diagnosticTraceNodes = buildDiagnosticTraceNodes(
    workflowNodeExecutions,
    analysis?.trace.nodes ?? []
  );
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
  const conversationTurns = buildConversationTurnSnapshots(
    outputs,
    selectedItem,
    executionFailureLabels
  );
  const judgeScore = deriveJudgeScore(selectedItem, outputs);
  const judgeScoreText = judgeScore === null ? t('notScored') : formatJudgeScore(judgeScore);
  const questionSnapshot = selectedItem.case_snapshot.content || commonT('none');
  const expectedResult = selectedItem.case_snapshot.expected_result || commonT('none');
  const primaryReasonText =
    (selectedItem.error
      ? localizeWorkflowTestError(selectedItem.error, errorLabels, errorT)
      : '') ||
    (selectedItem.judge_reason
      ? localizeWorkflowTestError(selectedItem.judge_reason, errorLabels, errorT)
      : '') ||
    (selectedItem.status === 'passed' ? t('passedReasonFallback') : commonT('none'));
  const judgeReasonText = selectedItem.judge_reason
    ? localizeWorkflowTestError(selectedItem.judge_reason, errorLabels, errorT)
    : commonT('none');
  const judgeSuggestionText = selectedItem.judge_suggestion
    ? localizeWorkflowTestError(selectedItem.judge_suggestion, errorLabels, errorT)
    : '';
  const recommendations =
    selectedItem.status === 'passed'
      ? []
      : buildRecommendationTexts(
          judgeSuggestionText,
          analysis,
          commonT('none'),
          textLabels,
          recommendationLabels
        );
  const diagnosisIssue = analysisMainIssue(analysis, textLabels);
  const displayedDiagnosisStatus =
    selectedItem.status === 'passed' && analysis?.mode === 'task'
      ? 'passed'
      : analysis?.summary.status;
  const failedNodes = failedExecutionNodes(executionNodes);
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
                <Link href={batchResultHref}>
                  <ArrowLeft className="mr-2 size-4" />
                  {t('back')}
                </Link>
              </Button>
              <div className="flex items-center gap-3">
                {itemPosition ? <div className="text-sm text-slate-500">{itemPosition}</div> : null}
                <div className="flex items-center gap-2">
                  {previousItem ? (
                    <Button variant="outline" size="sm" asChild>
                      <Link href={getBatchItemHref(previousItem.id)}>
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
                      <Link href={getBatchItemHref(nextItem.id)}>
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
                  {getItemStatusLabel(selectedItem.status)}
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
                  {getItemStatusLabel(selectedItem.status)}
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
                          {t('turnTitle', { index: index + 1 })}
                        </div>
                        {analysis?.comparisons.turns?.[index] ? (
                          <Badge
                            className={diagnosisStatusClass(
                              analysis.comparisons.turns[index].status
                            )}
                          >
                            {getDiagnosisStatusLabel(analysis.comparisons.turns[index].status)}
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
                                {t('expectedFocus')}
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
                          onClick={() =>
                            openRawView(
                              t('turnOutputTitle', { index: turn.turnIndex }),
                              turn.outputs
                            )
                          }
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
                      <div className="mb-2 text-xs font-medium text-slate-500">
                        {t('expectedFocus')}
                      </div>
                      <div className="whitespace-pre-wrap text-sm text-slate-800">
                        {expectedResult}
                      </div>
                    </div>
                  </div>
                  <div className="rounded-2xl border border-slate-200 bg-white p-4">
                    <div className="mb-2 text-xs font-medium text-slate-500">{t('agentReply')}</div>
                    <div className="max-h-96 overflow-auto text-sm leading-6 text-slate-800">
                      <WorkflowOutput
                        content={outputAnswer(outputs, executionFailureLabels, selectedItem.error)}
                        none={commonT('none')}
                      />
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
                      {getItemStatusLabel(selectedItem.status)}
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
                <h2 className="text-lg font-semibold text-slate-950">
                  {t('workflowOptimizationTitle')}
                </h2>
                <div className="rounded-2xl border border-amber-200 bg-amber-50 p-5 text-sm text-amber-950">
                  <div className="mb-3 text-amber-800">{t('workflowOptimizationDescription')}</div>
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
                        ? t('nodesNeedAttention', { count: failedNodes.length })
                        : displayedDiagnosisStatus
                          ? t('diagnosisResult', {
                              status: getDiagnosisStatusLabel(displayedDiagnosisStatus),
                            })
                          : commonT('none')}
                    </div>
                  </div>
                  {displayedDiagnosisStatus ? (
                    <Badge className={diagnosisStatusClass(displayedDiagnosisStatus)}>
                      {getDiagnosisStatusLabel(displayedDiagnosisStatus)}
                    </Badge>
                  ) : null}
                </div>

                <div className="mt-4 flex flex-wrap items-center gap-2">
                  {executionNodes.length > 0 ? (
                    executionNodes.map((node, index) => {
                      const meta =
                        draftNodeMetaById.get(node.id) ||
                        fallbackExecutionStepMeta(node.id, fallbackNodeLabels);
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
                        <div className="text-sm font-medium text-slate-950">
                          {t('taskEvaluationTitle')}
                        </div>
                        <div className="mt-1 text-xs text-slate-500">
                          {t('taskEvaluationDescription')}
                        </div>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        <Badge variant="outline">
                          {taskGoalTypeLabel(taskEvaluationSchema.goal_type, taskGoalTypeLabels)}
                        </Badge>
                        {taskEvaluationSchema.source_grounding ? (
                          <Badge variant="outline">{t('sourceGrounding')}</Badge>
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
                                {taskAssertionTypeLabel(assertion.type, taskAssertionTypeLabels)}
                              </Badge>
                              {assertion.severity ? (
                                <span className="text-xs text-slate-500">
                                  {taskAssertionSeverityLabel(
                                    assertion.severity,
                                    taskAssertionSeverityLabels
                                  )}
                                </span>
                              ) : null}
                            </div>
                            <div className="mt-2 line-clamp-2 text-slate-800">
                              {taskAssertionText(assertion, taskAssertionTypeLabels, textLabels)}
                            </div>
                          </div>
                        ))}
                      </div>
                    ) : null}

                    <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-600">
                      {taskEvaluationSchema.allowed_extra_types?.length ? (
                        <span className="rounded-full bg-white px-2 py-1">
                          {t('allowedExtras', {
                            values: taskEvaluationSchema.allowed_extra_types.join(
                              textLabels.listSeparator
                            ),
                          })}
                        </span>
                      ) : null}
                      {taskEvaluationSchema.missing_policy?.mode ? (
                        <span className="rounded-full bg-white px-2 py-1">
                          {t('missingPolicy')}
                        </span>
                      ) : null}
                    </div>

                    {taskEvaluationAssertions.length > visibleTaskEvaluationAssertions.length ? (
                      <details className="mt-3">
                        <summary className="cursor-pointer text-sm font-medium text-slate-700">
                          {t('viewAllAssertions', { count: taskEvaluationAssertions.length })}
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
                                  {taskAssertionTypeLabel(assertion.type, taskAssertionTypeLabels)}
                                </span>
                                {taskAssertionText(assertion, taskAssertionTypeLabels, textLabels)}
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
                      <div className="text-sm font-medium text-slate-950">{t('checkSummary')}</div>
                      <div className="flex flex-wrap gap-2 text-xs">
                        <Badge className="border-emerald-200 bg-emerald-50 text-emerald-700">
                          {t('passed', { count: checkCounts.passed })}
                        </Badge>
                        <Badge className="border-red-200 bg-red-50 text-red-700">
                          {t('failed', { count: checkCounts.failed })}
                        </Badge>
                        <Badge className="border-amber-200 bg-amber-50 text-amber-700">
                          {t('review', { count: checkCounts.review })}
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
                                {isGenericCheck(check) ? t('genericCheckLabel') : check.label}
                              </div>
                              <Badge className={diagnosisStatusClass(check.status)}>
                                {getDiagnosisStatusLabel(check.status)}
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
                        {t('viewAllChecks', { count: checks.length })}
                      </summary>
                      <div className="mt-3 max-h-96 space-y-2 overflow-auto pr-1">
                        {checks.map((check, index) => (
                          <div
                            key={workflowTestCheckKey(check, index)}
                            className="rounded-lg border border-slate-200 bg-white p-3 text-sm"
                          >
                            <div className="flex flex-wrap items-center justify-between gap-2">
                              <div className="font-medium text-slate-900">
                                {isGenericCheck(check) ? t('genericCheckLabel') : check.label}
                              </div>
                              <Badge className={diagnosisStatusClass(check.status)}>
                                {getDiagnosisStatusLabel(check.status)}
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

                {diagnosticTraceNodes.length ? (
                  <details className="mt-4 rounded-xl border border-slate-200 bg-slate-50 p-3">
                    <summary className="cursor-pointer text-sm font-medium text-slate-900">
                      {t('troubleshootingTitle')}
                    </summary>
                    <div className="mt-2 text-xs text-slate-500">
                      {t('troubleshootingDescription')}
                    </div>
                    <div className="mt-3 space-y-2">
                      {diagnosticTraceNodes.map((node, index) => (
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
                          <div className="mt-3 grid gap-3 lg:grid-cols-2">
                            {hasDisplayValue(node.input) ? (
                              <div className="rounded-lg bg-slate-50 p-3">
                                <div className="mb-2 text-xs font-medium text-slate-500">
                                  {t('input')}
                                </div>
                                <pre className="max-h-64 overflow-auto whitespace-pre-wrap text-xs text-slate-700">
                                  {stringifyJson(node.input, commonT('none'))}
                                </pre>
                              </div>
                            ) : null}
                            {hasDisplayValue(node.output) ? (
                              <div className="rounded-lg bg-slate-50 p-3">
                                <div className="mb-2 text-xs font-medium text-slate-500">
                                  {t('output')}
                                </div>
                                <pre className="max-h-64 overflow-auto whitespace-pre-wrap text-xs text-slate-700">
                                  {stringifyJson(node.output, commonT('none'))}
                                </pre>
                              </div>
                            ) : null}
                          </div>
                          {node.error ? (
                            <div className="mt-3 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">
                              {localizeWorkflowTestError(node.error, errorLabels, errorT)}
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
      </div>

      <Dialog open={Boolean(rawView)} onOpenChange={open => !open && setRawView(null)}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>{rawView?.title || t('rawData')}</DialogTitle>
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
