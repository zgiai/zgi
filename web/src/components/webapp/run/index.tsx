'use client';

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { useThrottledTextStream } from '@/hooks/use-throttled-text-stream';
import { STREAM_RENDER_THROTTLE_MS } from '@/lib/config';
import { cn } from '@/lib/utils';
import WorkflowInputForm, {
  type WorkflowInputFormHandle,
} from '@/components/workflow/common/workflow-input-form';
import type { FormInputs } from '@/components/workflow/common/workflow-input-form';
import type { InputVar } from '@/components/workflow/types/input-var';
import type { WebAppWorkflowConfig, WebAppVariable } from '@/services/types/webapp';
import { useRunWebAppWorkflowStream } from '@/hooks/webapp/use-run-webapp-workflow-stream';
import { useWorkflowRunEventsStream } from '@/hooks/workflow/use-workflow-run-events-stream';
import { Button } from '@/components/ui/button';
import { ChevronDown, Clock3, HelpCircle, Play, FileOutput } from 'lucide-react';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import ExecutionTab from '@/components/workflow/ui/workflow-run-panel/components/workflow-run-panel-execution';
import Results from '@/components/workflow/ui/workflow-run-panel/components/results';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Settings2, Activity, Terminal } from 'lucide-react';
import type { HistoryResult } from '@/components/workflow/ui/workflow-run-panel/types';
import type { WorkflowRunNodeListItem } from '@/components/workflow/ui/workflow-run-nodes-list';
import { unwrap } from '@/utils/webapp/run-mappers';
import { useWebAppPrecheck } from '@/hooks/webapp/use-webapp-precheck';
import { WorkflowPrecheckWarningBanner } from '@/components/workflow/common/workflow-precheck-warning';
import type {
  WorkflowNodeExecution,
  WorkflowPrecheckWarning,
  WorkflowRunDetail,
} from '@/services/types/workflow';
import { getWorkflowPrecheckWarnings } from '@/utils/workflow/billing';
import {
  extractLlmGatewayRequest,
  extractWorkflowRunContainerContext,
  getWorkflowRunCreatedAtMs,
  getWorkflowRunExecutionId,
  getWorkflowRunItemKey,
  getWorkflowRunRoundDurationMap,
  getWorkflowRunRoundElapsedTime,
  sortWorkflowRunItems,
  sortWorkflowRunRounds,
} from '@/utils/workflow/run-events';
import { useWorkflowBillingFeedback } from '@/hooks/workflow/use-workflow-billing-feedback';
import { getEffectiveAllowedFileExtensions } from '@/utils/file-helpers';
import { useAuthStore } from '@/store/auth-store';
import {
  getSensitiveOutputTextFromPayload,
  SENSITIVE_OUTPUT_BLOCKED_TOKEN,
} from '@/utils/model-output-filter';
import WorkflowApprovalInteractionCard from '@/components/workflow/approval/workflow-approval-interaction-card';
import { isWorkflowApprovalInlineAllowed } from '@/components/workflow/approval/workflow-approval-surface';
import { isApprovalFormAlreadySubmittedError } from '@/services/approval.service';
import { useApprovalForm, useSubmitApprovalForm } from '@/hooks/workflow/use-approval-form';
import {
  createWorkflowSnapshotPauseEvent,
  parseApprovalRequestedEvent,
} from '@/components/workflow/approval/runtime-events';
import { useApprovalRuntimeEvents } from '@/components/workflow/approval/use-approval-runtime-events';
import { WebAppOfflineState } from '@/components/webapp/offline-state';
import { useWebAppOfflineState } from '@/hooks/webapp/use-webapp-offline-state';
import { isWebAppOfflineError } from '@/utils/webapp/errors';
import type { QuestionAnswerChoice } from '@/services/types/workflow';
import {
  parseQuestionAnswerRequestedEvent,
  type QuestionAnswerRuntimePromptState,
} from '@/components/workflow/question-answer/runtime-events';
import { parseWorkflowPausedEvent } from '@/components/workflow/runtime/pause-events';
import {
  getQuestionAnswerChoiceQuery,
  QuestionAnswerRuntimePrompt,
} from '@/components/workflow/question-answer/question-answer-runtime-prompt';
import {
  buildWorkflowRunExecutionItems,
  buildWorkflowRunHistoryResult,
} from '@/components/workflow/ui/workflow-run-panel/utils/history-view-data';
import { WorkflowRuntimeStopAction } from '@/components/workflow/runtime/workflow-runtime-stop-action';
import type { NodeInfo } from '@/components/chat/types';
import { createWorkflowRunNodeAccumulator } from '@/utils/webapp/workflow-run-node-accumulator';

interface WebappRunProps {
  versionUuid: string;
  config: WebAppWorkflowConfig;
  enablePrecheck?: boolean;
}

const COMPACT_RUN_LAYOUT_WIDTH = 960;

function workflowRunItemFromNodeInfo(node: NodeInfo): WorkflowRunNodeListItem {
  const nodeId = node.nodeId || [node.nodeType, node.title].filter(Boolean).join('|') || 'unknown';
  const status: WorkflowRunNodeListItem['status'] =
    node.status === 'success' || node.status === 'partial-succeeded' ? 'succeeded' : node.status;
  return {
    nodeId,
    executionId: node.executionId,
    createdAtMs: node.createdAtMs,
    receivedOrder: node.receivedOrder,
    title: node.title || node.nodeType || nodeId,
    nodeType: node.nodeType || 'unknown',
    status,
    nodeInput: node.data?.input,
    nodeOutput: node.data?.output,
    modelInput: node.data?.modelInput,
    elapsedTime: node.elapsedTime,
    error: node.error ?? null,
    iterationInputs: node.iterationInputs,
    iterationOutputs: node.iterationOutputs,
    iterationRounds: node.iterationRounds?.map(round => ({
      index: round.index,
      nodes: round.nodes.map(workflowRunItemFromNodeInfo),
      elapsedTime: round.elapsedTime,
    })),
    loopInputs: node.loopInputs,
    loopOutputs: node.loopOutputs,
    loopRounds: node.loopRounds?.map(round => ({
      index: round.index,
      nodes: round.nodes.map(workflowRunItemFromNodeInfo),
      elapsedTime: round.elapsedTime,
      variables: round.variables,
    })),
    steps: node.steps,
  };
}

function nodeInfoFromWorkflowRunItem(item: WorkflowRunNodeListItem): NodeInfo {
  return {
    nodeId: item.nodeId,
    executionId: item.executionId,
    createdAtMs: item.createdAtMs,
    receivedOrder: item.receivedOrder,
    title: item.title,
    nodeType: item.nodeType,
    status: item.status,
    data: {
      input: item.nodeInput,
      output: item.nodeOutput,
      modelInput: item.modelInput,
    },
    elapsedTime: item.elapsedTime,
    error: item.error ?? undefined,
    iterationInputs: item.iterationInputs,
    iterationOutputs: item.iterationOutputs,
    iterationRounds: item.iterationRounds?.map(round => ({
      index: round.index,
      nodes: round.nodes.map(nodeInfoFromWorkflowRunItem),
      elapsedTime: round.elapsedTime,
    })),
    loopInputs: item.loopInputs,
    loopOutputs: item.loopOutputs,
    loopRounds: item.loopRounds?.map(round => ({
      index: round.index,
      nodes: round.nodes.map(nodeInfoFromWorkflowRunItem),
      elapsedTime: round.elapsedTime,
      variables: round.variables,
    })),
    steps: item.steps,
  };
}

function useMeasuredRunWidth() {
  const ref = useRef<HTMLDivElement | null>(null);
  const [width, setWidth] = useState(0);

  useEffect(() => {
    const node = ref.current;
    if (!node) return;

    const measure = () => setWidth(Math.floor(node.getBoundingClientRect().width));
    measure();

    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', measure);
      return () => window.removeEventListener('resize', measure);
    }

    const observer = new ResizeObserver(entries => {
      const entry = entries[0];
      setWidth(Math.floor(entry?.contentRect.width ?? node.getBoundingClientRect().width));
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  return { ref, width };
}

// Convert webapp variables to InputVar[] for WorkflowInputForm
function toInputVars(vars: WebAppVariable[], fileUploadLimit?: number): InputVar[] {
  return vars.map(v => ({
    type: v.type as InputVar['type'],
    variable: v.variable,
    label: v.label,
    description: v.description,
    required: v.required,
    max_length:
      v.type === 'file-list' && (typeof v.max_length !== 'number' || v.max_length <= 0)
        ? fileUploadLimit
        : v.max_length,
    default: v.default,
    default_datetime_mode: v.default_datetime_mode,
    options: v.options,
    allowed_file_upload_methods: v.allowed_file_upload_methods,
    allowed_file_types: v.allowed_file_types,
    allowed_file_extensions: v.allowed_file_extensions,
  }));
}

export const WebappRun: React.FC<WebappRunProps> = ({
  versionUuid,
  config,
  enablePrecheck = false,
}) => {
  const t = useT('webapp');
  const globalT = useT();
  const { isOffline, markOffline } = useWebAppOfflineState();
  const { getWorkflowRunErrorText } = useWorkflowBillingFeedback('webapp');
  const precheckMutation = useWebAppPrecheck(versionUuid);
  const notifyPublishedWorkflowFailure = useCallback(() => {
    toast.error(t('run.failed'));
  }, [t]);

  const [runItems, setRunItems] = useState<WorkflowRunNodeListItem[]>([]);
  const [streamedText, setStreamedText] = useState<string>('');
  const [isRunning, setIsRunning] = useState(false);
  const [isStopping, setIsStopping] = useState(false);
  const [finalResult, setFinalResult] = useState<HistoryResult | null>(null);
  const [activeTab, setActiveTab] = useState('input');
  const [executionOpen, setExecutionOpen] = useState(false);
  const [precheckWarnings, setPrecheckWarnings] = useState<WorkflowPrecheckWarning[]>([]);
  const [approvalPaused, setApprovalPaused] = useState(false);
  const [questionAnswerPrompt, setQuestionAnswerPrompt] =
    useState<QuestionAnswerRuntimePromptState | null>(null);
  const [questionAnswerSubmitting, setQuestionAnswerSubmitting] = useState(false);
  const [workflowRunId, setWorkflowRunId] = useState<string | null>(null);
  const {
    state: approvalRuntimeState,
    activeEntry: approvalEntry,
    activeForm: approvalForm,
    activeToken: approvalToken,
    submittedAction: approvalSubmittedAction,
    isSubmitting: approvalRuntimeSubmitting,
    dispatchApprovalEvent: dispatchApprovalRuntimeEvent,
    setSubmitting: setApprovalSubmitting,
    setSubmitted: setApprovalSubmitted,
    setWaiting: setApprovalWaiting,
    setLoadedForm: setLoadedApprovalForm,
    resetApprovalRuntime,
  } = useApprovalRuntimeEvents();
  const { start: startWorkflowRunEvents, cancel: cancelWorkflowRunEvents } =
    useWorkflowRunEventsStream({ transport: 'webapp' });
  const workflowRunIdRef = useRef<string | null>(null);
  const workflowRunEventsActiveRef = useRef(false);
  const workflowFinishedRef = useRef(false);
  const startWorkflowRunEventStreamRef = useRef<(payload?: unknown) => void>(() => {});
  const questionAnswerResumeRef = useRef(false);
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const { ref: runContainerRef, width: runContainerWidth } = useMeasuredRunWidth();
  const isCompactLayout = runContainerWidth === 0 || runContainerWidth < COMPACT_RUN_LAYOUT_WIDTH;
  const searchParams = useSearchParams();
  const conversationIdParam = searchParams.get('convId')?.trim() || '';
  const formRef = useRef<WorkflowInputFormHandle | null>(null);
  const approvalFormQuery = useApprovalForm(approvalToken, Boolean(approvalToken && !approvalForm));
  const approvalSubmitMutation = useSubmitApprovalForm(approvalToken);

  const throttler = useThrottledTextStream(STREAM_RENDER_THROTTLE_MS, (text: string) => {
    setStreamedText(prev => prev + text);
  });
  const runItemsRef = useRef<WorkflowRunNodeListItem[]>([]);

  useEffect(() => {
    runItemsRef.current = runItems;
  }, [runItems]);

  const applyProjectedRunNode = useCallback((node: NodeInfo) => {
    const next = workflowRunItemFromNodeInfo(node);
    setRunItems(items => {
      const index = items.findIndex(item => item.nodeId === next.nodeId);
      if (index < 0) return [...items, next];
      return items.map((item, itemIndex) =>
        itemIndex === index
          ? {
              ...item,
              ...next,
              nodeInput: next.nodeInput ?? item.nodeInput,
              nodeOutput: next.nodeOutput ?? item.nodeOutput,
              modelInput: next.modelInput ?? item.modelInput,
              elapsedTime: next.elapsedTime ?? item.elapsedTime,
              iterationInputs: next.iterationInputs ?? item.iterationInputs,
              iterationOutputs: next.iterationOutputs ?? item.iterationOutputs,
              iterationRounds: next.iterationRounds ?? item.iterationRounds,
              loopInputs: next.loopInputs ?? item.loopInputs,
              loopOutputs: next.loopOutputs ?? item.loopOutputs,
              loopRounds: next.loopRounds ?? item.loopRounds,
              steps: next.steps ?? item.steps,
            }
          : item
      );
    });
  }, []);

  const durableRunNodeAccumulator = useMemo(
    () =>
      createWorkflowRunNodeAccumulator({
        onNodeStarted: applyProjectedRunNode,
        onNodeFinished: applyProjectedRunNode,
      }),
    [applyProjectedRunNode]
  );

  useEffect(() => {
    if (approvalFormQuery.data) {
      setLoadedApprovalForm(approvalFormQuery.data);
      setApprovalPaused(true);
    }
  }, [approvalFormQuery.data, setLoadedApprovalForm]);

  const startVariables = useMemo(
    () => toInputVars(config.variables, config.features.file_upload?.number_limits),
    [config.features.file_upload?.number_limits, config.variables]
  );
  const hasAnonymousFileInputs = useMemo(
    () =>
      !isAuthenticated &&
      startVariables.some(variable => variable.type === 'file' || variable.type === 'file-list'),
    [isAuthenticated, startVariables]
  );
  const hasRequiredAnonymousFileInputs = useMemo(
    () =>
      !isAuthenticated &&
      startVariables.some(
        variable =>
          (variable.type === 'file' || variable.type === 'file-list') && Boolean(variable.required)
      ),
    [isAuthenticated, startVariables]
  );
  const varsSig = useMemo(
    () =>
      JSON.stringify(
        startVariables.map(v => ({
          variable: v.variable,
          description: v.description ?? undefined,
          type: v.type,
          required: Boolean(v.required),
          options: v.options ?? [],
          allowed_file_types: v.allowed_file_types ?? [],
          effective_allowed_file_extensions: getEffectiveAllowedFileExtensions(
            v.allowed_file_types ?? [],
            v.allowed_file_extensions ?? []
          ),
          max_length: v.max_length ?? undefined,
        }))
      ),
    [startVariables]
  );

  const iterationSessions = useRef(
    new Map<
      string,
      {
        nodeId?: string;
        title?: string;
        nodeType?: string;
        inputs?: unknown;
        outputs?: unknown;
        elapsedTime?: number;
        error?: string;
        rounds: Array<{
          index: number;
          nodes: WorkflowRunNodeListItem[];
          elapsedTime?: number;
        }>;
        activeIndex?: number | null;
      }
    >()
  );
  const activeIteration = useRef<{ nodeId: string | null; index: number | null }>({
    nodeId: null,
    index: null,
  });
  const loopSessions = useRef(
    new Map<
      string,
      {
        nodeId?: string;
        title?: string;
        nodeType?: string;
        inputs?: unknown;
        outputs?: unknown;
        elapsedTime?: number;
        error?: string;
        rounds: Array<{
          index: number;
          nodes: WorkflowRunNodeListItem[];
          elapsedTime?: number;
          variables?: unknown;
        }>;
        activeIndex?: number | null;
      }
    >()
  );
  const activeLoop = useRef<{ nodeId: string | null; index: number | null }>({
    nodeId: null,
    index: null,
  });
  const receiveOrder = useRef(0);

  const nextReceiveOrder = useCallback(() => {
    receiveOrder.current += 1;
    return receiveOrder.current;
  }, []);

  const normalizeOutputs = useCallback((outputs: unknown): HistoryResult => {
    if (outputs === null || outputs === undefined) return { kind: 'empty' };
    if (typeof outputs === 'string') return { kind: 'text', content: outputs };
    if (typeof outputs === 'number' || typeof outputs === 'boolean') {
      return { kind: 'text', content: String(outputs) };
    }
    if (typeof outputs === 'object') {
      const rec = outputs as Record<string, unknown>;
      const keys = Object.keys(rec);
      if (keys.length === 1) {
        const v = rec[keys[0]];
        if (typeof v === 'string' || typeof v === 'number') {
          return { kind: 'text', content: String(v) };
        }
        if (Array.isArray(v) && (v as unknown[]).every(x => typeof x === 'string')) {
          return { kind: 'text', content: (v as string[]).join('\n\n') };
        }
        return { kind: 'json', value: outputs };
      }
      return { kind: 'json', value: outputs };
    }
    return { kind: 'empty' };
  }, []);

  const getWorkflowRunIdFromPayload = useCallback((payload: unknown): string => {
    const data = unwrap(payload);
    return (
      (typeof data.id === 'string' ? data.id : '') ||
      (typeof data.workflow_run_id === 'string' ? data.workflow_run_id : '') ||
      (typeof data.task_id === 'string' ? data.task_id : '')
    );
  }, []);

  const rememberWorkflowRunId = useCallback((runId: string | null) => {
    workflowRunIdRef.current = runId || null;
    setWorkflowRunId(runId || null);
  }, []);

  const markApprovalPausedNodes = useCallback((nodeIds: string[], payload: unknown) => {
    const data = unwrap(payload);
    const targetNodeIds =
      nodeIds.length > 0 ? nodeIds : typeof data.node_id === 'string' ? [data.node_id] : [];
    if (targetNodeIds.length === 0) return;
    const reasons = Array.isArray(data.reasons) ? data.reasons : [];
    const titleByNodeId = new Map<string, string>();
    reasons.forEach(reason => {
      if (!reason || typeof reason !== 'object') return;
      const record = reason as Record<string, unknown>;
      const nodeId = typeof record.node_id === 'string' ? record.node_id : '';
      const title = typeof record.node_title === 'string' ? record.node_title : '';
      if (nodeId && title) titleByNodeId.set(nodeId, title);
    });
    setRunItems(items => {
      const existingIds = new Set(items.map(item => item.nodeId));
      const updated = items.map(item =>
        targetNodeIds.includes(item.nodeId) ? { ...item, status: 'paused' as const } : item
      );
      const missing = targetNodeIds
        .filter(nodeId => !existingIds.has(nodeId))
        .map(nodeId => ({
          nodeId,
          title:
            titleByNodeId.get(nodeId) || (typeof data.title === 'string' ? data.title : nodeId),
          nodeType: 'approval',
          status: 'paused' as const,
        }));
      return missing.length > 0 ? [...updated, ...missing] : updated;
    });
  }, []);

  const markQuestionAnswerPausedNodes = useCallback((nodeIds: string[], payload: unknown) => {
    const data = unwrap(payload);
    const targetNodeIds =
      nodeIds.length > 0 ? nodeIds : typeof data.node_id === 'string' ? [data.node_id] : [];
    if (targetNodeIds.length === 0) return;
    const reasons = Array.isArray(data.reasons) ? data.reasons : [];
    const titleByNodeId = new Map<string, string>();
    reasons.forEach(reason => {
      if (!reason || typeof reason !== 'object') return;
      const record = reason as Record<string, unknown>;
      const nodeId = typeof record.node_id === 'string' ? record.node_id : '';
      const title = typeof record.node_title === 'string' ? record.node_title : '';
      if (nodeId && title) titleByNodeId.set(nodeId, title);
    });
    setRunItems(items => {
      const existingIds = new Set(items.map(item => item.nodeId));
      const updated = items.map(item =>
        targetNodeIds.includes(item.nodeId) ? { ...item, status: 'paused' as const } : item
      );
      const missing = targetNodeIds
        .filter(nodeId => !existingIds.has(nodeId))
        .map(nodeId => ({
          nodeId,
          title:
            titleByNodeId.get(nodeId) || (typeof data.title === 'string' ? data.title : nodeId),
          nodeType: 'question-answer',
          status: 'paused' as const,
        }));
      return missing.length > 0 ? [...updated, ...missing] : updated;
    });
  }, []);

  const { start, stop, isStarting } = useRunWebAppWorkflowStream(versionUuid, {
    enabled: true,
    agentId: config.config.agent_id,
    onStarted: payload => {
      const runId = getWorkflowRunIdFromPayload(payload);
      const isQuestionAnswerResume = questionAnswerResumeRef.current;
      questionAnswerResumeRef.current = false;
      rememberWorkflowRunId(runId || null);
      setIsRunning(true);
      if (isQuestionAnswerResume) {
        setQuestionAnswerPrompt(null);
        setQuestionAnswerSubmitting(false);
        workflowFinishedRef.current = false;
        setActiveTab('execution');
        return;
      }
      setRunItems([]);
      durableRunNodeAccumulator.reset();
      setStreamedText('');
      setFinalResult(null);
      resetApprovalRuntime();
      setApprovalPaused(false);
      setQuestionAnswerPrompt(null);
      setQuestionAnswerSubmitting(false);
      cancelWorkflowRunEvents();
      workflowRunEventsActiveRef.current = false;
      workflowFinishedRef.current = false;
      iterationSessions.current.clear();
      activeIteration.current = { nodeId: null, index: null };
      loopSessions.current.clear();
      activeLoop.current = { nodeId: null, index: null };
      receiveOrder.current = 0;
      throttler.cancel();
      setActiveTab('execution');
    },
    onIterationStarted: payload => {
      const d = unwrap(payload);
      const nodeId = typeof d['node_id'] === 'string' ? (d['node_id'] as string) : undefined;
      const title =
        typeof d['title'] === 'string'
          ? (d['title'] as string)
          : typeof d['node_type'] === 'string'
            ? (d['node_type'] as string)
            : (nodeId ?? 'iteration');
      const nodeType =
        typeof d['node_type'] === 'string' ? (d['node_type'] as string) : 'iteration';
      const inputs = d['inputs'];
      const key = nodeId ?? title;
      iterationSessions.current.set(key, {
        nodeId,
        title,
        inputs,
        nodeType,
        rounds: [],
        activeIndex: null,
      });
      activeIteration.current = { nodeId: key, index: null };
      setRunItems(items => {
        const exists = items.find(it => it.nodeId === key);
        const base = {
          nodeId: key,
          title,
          nodeType: 'iteration',
          status: 'running' as const,
          iterationInputs: inputs,
          iterationRounds: [],
        } as WorkflowRunNodeListItem;
        return exists
          ? items.map(it => (it.nodeId === key ? { ...it, ...base } : it))
          : [...items, base];
      });
    },
    onIterationNext: payload => {
      const d = unwrap(payload);
      const nodeId = typeof d['node_id'] === 'string' ? (d['node_id'] as string) : undefined;
      const title =
        typeof d['title'] === 'string'
          ? (d['title'] as string)
          : typeof d['node_type'] === 'string'
            ? (d['node_type'] as string)
            : (nodeId ?? 'iteration');
      const index = typeof d['index'] === 'number' ? (d['index'] as number) : 0;
      const key = nodeId ?? title;
      const sess = iterationSessions.current.get(key) ?? {
        nodeId,
        title,
        nodeType: 'iteration',
        rounds: [],
      };
      const hasRound = sess.rounds.some(r => r.index === index);
      if (!hasRound) sess.rounds.push({ index, nodes: [] as WorkflowRunNodeListItem[] });
      sess.activeIndex = index;
      iterationSessions.current.set(key, sess);
      activeIteration.current = { nodeId: key, index };
      setRunItems(items =>
        items.map(it =>
          it.nodeId === key
            ? {
                ...it,
                status: 'running',
                nodeType: 'iteration',
                iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                  index: r.index,
                  nodes: sortWorkflowRunItems(r.nodes),
                })),
              }
            : it
        )
      );
    },
    onNodeStarted: payload => {
      const d = unwrap(payload);
      const id = typeof d['node_id'] === 'string' ? (d['node_id'] as string) : undefined;
      if (!id) return;
      const title =
        typeof d['title'] === 'string'
          ? (d['title'] as string)
          : typeof d['node_type'] === 'string'
            ? (d['node_type'] as string)
            : id;
      const nodeType = typeof d['node_type'] === 'string' ? (d['node_type'] as string) : 'unknown';
      const { loopId, loopIndex, iterationId, iterationIndex } =
        extractWorkflowRunContainerContext(d);
      const executionId = getWorkflowRunExecutionId(d);
      const createdAtMs = getWorkflowRunCreatedAtMs(d);
      const receivedOrder = nextReceiveOrder();
      if (loopId) {
        const sess = loopSessions.current.get(loopId);
        const targetIndex =
          typeof loopIndex === 'number'
            ? loopIndex
            : (sess?.activeIndex ?? activeLoop.current.index);
        if (sess && typeof targetIndex === 'number') {
          const rIdx = sess.rounds.findIndex(r => r.index === targetIndex);
          if (rIdx < 0) {
            sess.rounds.push({ index: targetIndex, nodes: [] });
          }
          const round = sess.rounds.find(r => r.index === targetIndex);
          if (!round) return;
          const childKey = getWorkflowRunItemKey({ executionId, nodeId: id, nodeType, title });
          const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
          const child: WorkflowRunNodeListItem = {
            nodeId: id ?? `${nodeType}|${title}`,
            executionId,
            createdAtMs,
            receivedOrder,
            title,
            nodeType,
            status: 'running' as const,
            nodeInput: d['inputs'],
          };
          if (cIdx >= 0) {
            const existing = round.nodes[cIdx];
            round.nodes[cIdx] = {
              ...existing,
              ...child,
              createdAtMs: existing.createdAtMs ?? child.createdAtMs,
              receivedOrder: existing.receivedOrder ?? child.receivedOrder,
            };
          } else round.nodes.push(child);
          sess.activeIndex = targetIndex;
          loopSessions.current.set(loopId, { ...sess });
          activeLoop.current = { nodeId: loopId, index: targetIndex };
          setRunItems(items =>
            items.map(it =>
              it.nodeId === loopId
                ? {
                    ...it,
                    loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                      index: r.index,
                      nodes: sortWorkflowRunItems(r.nodes),
                    })),
                  }
                : it
            )
          );
          return;
        }
      }
      const targetIterationId = iterationId ?? activeIteration.current.nodeId;
      const targetIterationIndex =
        typeof iterationIndex === 'number' ? iterationIndex : activeIteration.current.index;
      if (targetIterationId && targetIterationIndex !== null) {
        const sess = iterationSessions.current.get(targetIterationId);
        if (sess) {
          const rIdx = sess.rounds.findIndex(r => r.index === targetIterationIndex);
          if (rIdx < 0) {
            sess.rounds.push({ index: targetIterationIndex, nodes: [] });
          }
          const round = sess.rounds.find(r => r.index === targetIterationIndex);
          if (!round) return;
          const childKey = getWorkflowRunItemKey({ executionId, nodeId: id, nodeType, title });
          const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
          const child: WorkflowRunNodeListItem = {
            nodeId: id ?? `${nodeType}|${title}`,
            executionId,
            createdAtMs,
            receivedOrder,
            title,
            nodeType,
            status: 'running' as const,
            nodeInput: d['inputs'],
          };
          if (cIdx >= 0) {
            const existing = round.nodes[cIdx];
            round.nodes[cIdx] = {
              ...existing,
              ...child,
              createdAtMs: existing.createdAtMs ?? child.createdAtMs,
              receivedOrder: existing.receivedOrder ?? child.receivedOrder,
            };
          } else round.nodes.push(child);
          sess.activeIndex = targetIterationIndex;
          iterationSessions.current.set(targetIterationId, { ...sess });
          setRunItems(items =>
            items.map(it =>
              it.nodeId === targetIterationId
                ? {
                    ...it,
                    iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                      index: r.index,
                      nodes: sortWorkflowRunItems(r.nodes),
                    })),
                  }
                : it
            )
          );
          return;
        }
      }
      setRunItems(prev => {
        const exists = prev.find(p => p.nodeId === id);
        if (exists) {
          return prev.map(p =>
            p.nodeId === id ? { ...p, status: 'running', nodeInput: d['inputs'] } : p
          );
        }
        const item: WorkflowRunNodeListItem = {
          nodeId: id,
          executionId,
          createdAtMs,
          receivedOrder,
          title,
          nodeType,
          status: 'running',
          nodeInput: d['inputs'],
        };
        return [...prev, item];
      });
    },
    onNodeFinished: payload => {
      const d = unwrap(payload);
      const id = typeof d['node_id'] === 'string' ? (d['node_id'] as string) : undefined;
      if (!id) return;
      const title =
        typeof d['title'] === 'string'
          ? (d['title'] as string)
          : typeof d['node_type'] === 'string'
            ? (d['node_type'] as string)
            : id;
      const nodeType = typeof d['node_type'] === 'string' ? (d['node_type'] as string) : 'unknown';
      const { loopId, loopIndex, iterationId, iterationIndex } =
        extractWorkflowRunContainerContext(d);
      const executionId = getWorkflowRunExecutionId(d);
      const createdAtMs = getWorkflowRunCreatedAtMs(d);
      const receivedOrder = nextReceiveOrder();
      const rawStatus = typeof d['status'] === 'string' ? (d['status'] as string) : undefined;
      const status: WorkflowRunNodeListItem['status'] =
        rawStatus === 'succeeded' || rawStatus === 'success' || rawStatus === 'completed'
          ? 'succeeded'
          : 'failed';
      const elapsedTime =
        typeof d['elapsed_time'] === 'number' ? Math.max(0, d['elapsed_time'] as number) : 0;
      const nodeInput = d['inputs'];
      const nodeOutput = d['outputs'];
      const error = getWorkflowRunErrorText(d['error']) ?? null;
      if (loopId) {
        const sess = loopSessions.current.get(loopId);
        const targetIndex =
          typeof loopIndex === 'number'
            ? loopIndex
            : (sess?.activeIndex ?? activeLoop.current.index);
        if (sess && typeof targetIndex === 'number') {
          const rIdx = sess.rounds.findIndex(r => r.index === targetIndex);
          if (rIdx < 0) {
            sess.rounds.push({ index: targetIndex, nodes: [] });
          }
          const round = sess.rounds.find(r => r.index === targetIndex);
          if (!round) return;
          const childKey = getWorkflowRunItemKey({ executionId, nodeId: id, nodeType, title });
          const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
          const child: WorkflowRunNodeListItem = {
            nodeId: id ?? `${nodeType}|${title}`,
            executionId,
            createdAtMs,
            receivedOrder,
            title,
            nodeType,
            status,
            elapsedTime,
            error,
            nodeInput,
            nodeOutput,
            modelInput: extractLlmGatewayRequest(d),
          };
          if (cIdx >= 0) {
            const existing = round.nodes[cIdx];
            round.nodes[cIdx] = {
              ...existing,
              ...child,
              createdAtMs: existing.createdAtMs ?? child.createdAtMs,
              receivedOrder: existing.receivedOrder ?? child.receivedOrder,
            };
          } else round.nodes.push(child);
          sess.activeIndex = targetIndex;
          loopSessions.current.set(loopId, { ...sess });
          activeLoop.current = { nodeId: loopId, index: targetIndex };
          setRunItems(items =>
            items.map(it =>
              it.nodeId === loopId
                ? {
                    ...it,
                    loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                      index: r.index,
                      nodes: sortWorkflowRunItems(r.nodes),
                    })),
                  }
                : it
            )
          );
          return;
        }
      }
      const targetIterationId = iterationId ?? activeIteration.current.nodeId;
      const targetIterationIndex =
        typeof iterationIndex === 'number' ? iterationIndex : activeIteration.current.index;
      if (targetIterationId && targetIterationIndex !== null) {
        const sess = iterationSessions.current.get(targetIterationId);
        if (sess) {
          const rIdx = sess.rounds.findIndex(r => r.index === targetIterationIndex);
          if (rIdx < 0) {
            sess.rounds.push({ index: targetIterationIndex, nodes: [] });
          }
          const round = sess.rounds.find(r => r.index === targetIterationIndex);
          if (!round) return;
          const childKey = getWorkflowRunItemKey({ executionId, nodeId: id, nodeType, title });
          const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
          const child: WorkflowRunNodeListItem = {
            nodeId: id ?? `${nodeType}|${title}`,
            executionId,
            createdAtMs,
            receivedOrder,
            title,
            nodeType,
            status,
            elapsedTime,
            error,
            nodeInput,
            nodeOutput,
            modelInput: extractLlmGatewayRequest(d),
          };
          if (cIdx >= 0) {
            const existing = round.nodes[cIdx];
            round.nodes[cIdx] = {
              ...existing,
              ...child,
              createdAtMs: existing.createdAtMs ?? child.createdAtMs,
              receivedOrder: existing.receivedOrder ?? child.receivedOrder,
            };
          } else round.nodes.push(child);
          sess.activeIndex = targetIterationIndex;
          iterationSessions.current.set(targetIterationId, { ...sess });
          setRunItems(items =>
            items.map(it =>
              it.nodeId === targetIterationId
                ? {
                    ...it,
                    iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                      index: r.index,
                      nodes: sortWorkflowRunItems(r.nodes),
                    })),
                  }
                : it
            )
          );
          return;
        }
      }
      setRunItems(prev => {
        const exists = prev.find(p => p.nodeId === id);
        if (exists) {
          return prev.map(p =>
            p.nodeId === id
              ? {
                  ...p,
                  title,
                  nodeType,
                  status,
                  elapsedTime,
                  nodeInput,
                  nodeOutput,
                  modelInput: extractLlmGatewayRequest(d),
                  error,
                }
              : p
          );
        }
        const item: WorkflowRunNodeListItem = {
          nodeId: id,
          executionId,
          createdAtMs,
          receivedOrder,
          title,
          nodeType,
          status,
          elapsedTime,
          nodeInput,
          nodeOutput,
          modelInput: extractLlmGatewayRequest(d),
          error,
        };
        return [...prev, item];
      });
    },
    onPaused: payload => {
      const parsed = parseWorkflowPausedEvent(payload);
      setIsRunning(false);
      setIsStopping(false);
      throttler.flush();
      if (parsed.hasApproval) {
        setApprovalPaused(true);
        dispatchApprovalRuntimeEvent('workflow_paused', payload);
        markApprovalPausedNodes(parsed.approval.nodeIds, payload);
      }
      if (parsed.hasQuestionAnswer) {
        if (parsed.questionAnswer.prompt) {
          setQuestionAnswerPrompt({
            nodeId: parsed.questionAnswer.prompt.nodeId,
            question: parsed.questionAnswer.prompt.question,
            choices: parsed.questionAnswer.prompt.choices,
            round: parsed.questionAnswer.prompt.round,
          });
          setQuestionAnswerSubmitting(false);
        }
        markQuestionAnswerPausedNodes(parsed.questionAnswer.nodeIds, payload);
      }
      if (!parsed.hasApproval && !parsed.hasQuestionAnswer) return;
      setActiveTab('input');
      startWorkflowRunEventStreamRef.current(payload);
    },
    onApprovalRequested: payload => {
      const parsed = parseApprovalRequestedEvent(payload);
      if (!parsed.form && !parsed.token && !parsed.formId && !parsed.nodeId) return;
      dispatchApprovalRuntimeEvent('approval_requested', payload);
      setApprovalPaused(true);
      setActiveTab('input');
    },
    onQuestionAnswerRequested: payload => {
      const parsed = parseQuestionAnswerRequestedEvent(payload);
      if (!parsed) return;
      setQuestionAnswerPrompt({
        nodeId: parsed.nodeId,
        question: parsed.question,
        choices: parsed.choices,
        round: parsed.round,
      });
      setQuestionAnswerSubmitting(false);
      if (parsed.nodeId) markQuestionAnswerPausedNodes([parsed.nodeId], payload);
      setActiveTab('input');
    },
    onQuestionAnswerSubmitted: () => {
      setQuestionAnswerSubmitting(true);
    },
    onApprovalResultFilled: payload => {
      dispatchApprovalRuntimeEvent('approval_result_filled', payload);
      setApprovalPaused(false);
    },
    onApprovalExpired: payload => {
      dispatchApprovalRuntimeEvent('approval_expired', payload);
      toast.info(globalT('nodes.approval.runtime.expired'));
    },
    onTextReplace: payload => {
      throttler.cancel();
      const text = getSensitiveOutputTextFromPayload(payload) ?? SENSITIVE_OUTPUT_BLOCKED_TOKEN;
      setStreamedText(text);
      setFinalResult({ kind: 'text', content: text });
      setActiveTab('result');
    },
    onTextChunk: payload => {
      const d = unwrap(payload);
      const text = typeof d['text'] === 'string' ? (d['text'] as string) : undefined;
      if (typeof text === 'string' && text.length > 0) {
        throttler.append(text);
      }
    },
    onMessage: payload => {
      try {
        unwrap(payload) as Record<string, unknown>;
      } catch (_e) {
        void _e;
      }
    },
    onMessageEnd: payload => {
      try {
        const d = unwrap(payload) as Record<string, unknown>;
        const ev = typeof d['event'] === 'string' ? (d['event'] as string) : '';
        const body =
          typeof d['data'] === 'object' && d['data'] ? (d['data'] as Record<string, unknown>) : d;
        const maybeOutputs = (body as Record<string, unknown>)['outputs'] as unknown;
        if (ev === 'workflow_finished' || typeof maybeOutputs !== 'undefined') {
          const result = normalizeOutputs(maybeOutputs);
          setFinalResult(result);
          if (result.kind === 'text') setStreamedText(result.content);
        }
      } catch (_e) {
        void _e;
      }
    },
    onIterationCompleted: payload => {
      const d = unwrap(payload);
      const nodeId = typeof d['node_id'] === 'string' ? (d['node_id'] as string) : undefined;
      const title =
        typeof d['title'] === 'string'
          ? (d['title'] as string)
          : typeof d['node_type'] === 'string'
            ? (d['node_type'] as string)
            : (nodeId ?? 'iteration');
      const key = nodeId ?? title;
      const elapsedTime =
        typeof d['elapsed_time'] === 'number' ? Math.max(0, d['elapsed_time'] as number) : 0;
      const error = typeof d['error'] === 'string' ? (d['error'] as string) : null;
      const outputs = d['outputs'];
      const roundDurations = getWorkflowRunRoundDurationMap(d, 'iteration');
      const sess = iterationSessions.current.get(key) ?? {
        nodeId,
        title,
        nodeType: 'iteration',
        rounds: [],
      };
      sess.elapsedTime = elapsedTime;
      sess.error = error ?? undefined;
      sess.outputs = outputs;
      sess.rounds = sess.rounds.map(r => ({
        ...r,
        elapsedTime: roundDurations.get(r.index) ?? getWorkflowRunRoundElapsedTime(r),
      }));
      iterationSessions.current.set(key, sess);
      activeIteration.current = { nodeId: null, index: null };
      setRunItems(items =>
        items.map(it =>
          it.nodeId === key
            ? {
                ...it,
                status: error ? 'failed' : 'succeeded',
                elapsedTime,
                iterationOutputs: outputs,
                iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                  index: r.index,
                  nodes: sortWorkflowRunItems(r.nodes),
                  elapsedTime: r.elapsedTime,
                })),
              }
            : it
        )
      );
    },
    onLoopStarted: payload => {
      const d = unwrap(payload);
      const nodeId = typeof d['node_id'] === 'string' ? (d['node_id'] as string) : undefined;
      const title =
        typeof d['title'] === 'string'
          ? (d['title'] as string)
          : typeof d['node_type'] === 'string'
            ? (d['node_type'] as string)
            : (nodeId ?? 'loop');
      const nodeType = typeof d['node_type'] === 'string' ? (d['node_type'] as string) : 'loop';
      const inputs = d['inputs'];
      const key = nodeId ?? title;
      loopSessions.current.set(key, {
        nodeId,
        title,
        inputs,
        nodeType,
        rounds: [],
        activeIndex: null,
      });
      activeLoop.current = { nodeId: key, index: null };
      setRunItems(items => {
        const exists = items.find(it => it.nodeId === key);
        const base = {
          nodeId: key,
          title,
          nodeType: 'loop',
          status: 'running' as const,
          loopInputs: inputs,
          loopRounds: [],
        } as WorkflowRunNodeListItem;
        return exists
          ? items.map(it => (it.nodeId === key ? { ...it, ...base } : it))
          : [...items, base];
      });
    },
    onLoopNext: payload => {
      const d = unwrap(payload);
      const nodeId = typeof d['node_id'] === 'string' ? (d['node_id'] as string) : undefined;
      const title =
        typeof d['title'] === 'string'
          ? (d['title'] as string)
          : typeof d['node_type'] === 'string'
            ? (d['node_type'] as string)
            : (nodeId ?? 'loop');
      const index = typeof d['index'] === 'number' ? (d['index'] as number) : 0;
      const key = nodeId ?? title;
      const sess = loopSessions.current.get(key) ?? {
        nodeId,
        title,
        nodeType: 'loop',
        rounds: [],
      };
      const hasRound = sess.rounds.some(r => r.index === index);
      if (!hasRound) sess.rounds.push({ index, nodes: [] as WorkflowRunNodeListItem[] });
      sess.activeIndex = index;
      loopSessions.current.set(key, sess);
      activeLoop.current = { nodeId: key, index };
      setRunItems(items =>
        items.map(it =>
          it.nodeId === key
            ? {
                ...it,
                status: 'running',
                nodeType: 'loop',
                loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                  index: r.index,
                  nodes: sortWorkflowRunItems(r.nodes),
                })),
              }
            : it
        )
      );
    },
    onLoopCompleted: payload => {
      const d = unwrap(payload);
      const nodeId = typeof d['node_id'] === 'string' ? (d['node_id'] as string) : undefined;
      const title =
        typeof d['title'] === 'string'
          ? (d['title'] as string)
          : typeof d['node_type'] === 'string'
            ? (d['node_type'] as string)
            : (nodeId ?? 'loop');
      const key = nodeId ?? title;
      const elapsedTime =
        typeof d['elapsed_time'] === 'number' ? Math.max(0, d['elapsed_time'] as number) : 0;
      const statusRaw = typeof d['status'] === 'string' ? (d['status'] as string) : '';
      const isSuccess =
        statusRaw === 'succeeded' || statusRaw === 'success' || statusRaw === 'completed';
      const error = typeof d['error'] === 'string' ? (d['error'] as string) : null;
      const outputs = d['outputs'];
      const execMeta = d['execution_metadata'] as unknown;
      const variableMap: Record<string, unknown> | undefined =
        execMeta && typeof execMeta === 'object'
          ? ((execMeta as Record<string, unknown>)['loop_variable_map'] as
              Record<string, unknown> | undefined)
          : undefined;
      const roundDurations = getWorkflowRunRoundDurationMap(d, 'loop');
      const sess = loopSessions.current.get(key) ?? {
        nodeId,
        title,
        nodeType: 'loop',
        rounds: [],
      };
      sess.elapsedTime = elapsedTime;
      sess.error = error ?? undefined;
      sess.outputs = outputs;
      sess.rounds = sess.rounds.map(r => {
        const variables = variableMap?.[String(r.index)];
        return {
          ...r,
          elapsedTime: roundDurations.get(r.index) ?? getWorkflowRunRoundElapsedTime(r),
          variables: variables ?? r.variables,
        };
      });
      loopSessions.current.set(key, sess);
      activeLoop.current = { nodeId: null, index: null };
      setRunItems(items =>
        items.map(it =>
          it.nodeId === key
            ? {
                ...it,
                status: isSuccess ? 'succeeded' : 'failed',
                elapsedTime,
                loopOutputs: outputs,
                loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                  index: r.index,
                  nodes: sortWorkflowRunItems(r.nodes),
                  elapsedTime: r.elapsedTime,
                  variables: r.variables,
                })),
              }
            : it
        )
      );
    },
    onFinished: payload => {
      const data = unwrap(payload) as Record<string, unknown>;
      const status = typeof data['status'] === 'string' ? data['status'] : '';
      setIsRunning(false);
      setIsStopping(false);
      cancelWorkflowRunEvents();
      workflowRunEventsActiveRef.current = false;
      workflowFinishedRef.current = true;
      rememberWorkflowRunId(null);
      throttler.flush();
      const outputs = data['outputs'] as unknown;
      let result: HistoryResult = { kind: 'empty' };
      if (outputs === null || outputs === undefined) {
        result = { kind: 'empty' };
      } else if (typeof outputs === 'string') {
        result = { kind: 'text', content: outputs };
      } else if (typeof outputs === 'number' || typeof outputs === 'boolean') {
        result = { kind: 'text', content: String(outputs) };
      } else if (typeof outputs === 'object') {
        const rec = outputs as Record<string, unknown>;
        const keys = Object.keys(rec);
        if (keys.length === 1) {
          const v = rec[keys[0]];
          if (typeof v === 'string') {
            result = { kind: 'text', content: v };
          } else if (Array.isArray(v) && v.every(x => typeof x === 'string')) {
            result = { kind: 'text', content: (v as string[]).join('\n\n') };
          } else {
            result = { kind: 'json', value: outputs };
          }
        } else {
          result = { kind: 'json', value: outputs };
        }
      }
      setFinalResult(result);
      if (result.kind === 'text') setStreamedText(result.content);
      setApprovalPaused(false);
      resetApprovalRuntime();
      setQuestionAnswerPrompt(null);
      setQuestionAnswerSubmitting(false);
      setActiveTab('result');
      if (status === 'success' || status === 'succeeded' || status === 'completed') {
        toast.success(t('run.completed'));
      } else if (status === 'failed' || status === 'error') {
        notifyPublishedWorkflowFailure();
      }
    },
    onError: payload => {
      if (isWebAppOfflineError(payload)) {
        markOffline();
        return;
      }
      setIsRunning(false);
      setIsStopping(false);
      setApprovalPaused(false);
      questionAnswerResumeRef.current = false;
      cancelWorkflowRunEvents();
      workflowRunEventsActiveRef.current = false;
      workflowFinishedRef.current = true;
      rememberWorkflowRunId(null);
      resetApprovalRuntime();
      setQuestionAnswerSubmitting(false);
      throttler.flush();
      notifyPublishedWorkflowFailure();
    },
  });

  const dispatchApprovalEvent = useCallback(
    (event: { event?: string; data?: unknown; [key: string]: unknown }) => {
      const payload = event.data && typeof event.data === 'object' ? event.data : event;
      const data = unwrap(payload) as Record<string, unknown>;
      switch (event.event) {
        case 'workflow_started': {
          const runId = getWorkflowRunIdFromPayload(payload);
          if (runId) rememberWorkflowRunId(runId);
          workflowFinishedRef.current = false;
          setIsRunning(true);
          break;
        }
        case 'workflow_resumed': {
          setIsRunning(true);
          setApprovalPaused(false);
          resetApprovalRuntime();
          setQuestionAnswerPrompt(null);
          setQuestionAnswerSubmitting(false);
          break;
        }
        case 'approval_requested': {
          const parsed = parseApprovalRequestedEvent(event);
          if (!parsed.form && !parsed.token && !parsed.formId && !parsed.nodeId) return;
          dispatchApprovalRuntimeEvent('approval_requested', event);
          setApprovalPaused(true);
          setActiveTab('input');
          break;
        }
        case 'approval_result_filled': {
          dispatchApprovalRuntimeEvent('approval_result_filled', event);
          setApprovalPaused(false);
          break;
        }
        case 'approval_expired': {
          dispatchApprovalRuntimeEvent('approval_expired', event);
          toast.info(globalT('nodes.approval.runtime.expired'));
          break;
        }
        case 'question_answer_requested': {
          const parsed = parseQuestionAnswerRequestedEvent(event);
          if (!parsed) return;
          setQuestionAnswerPrompt({
            nodeId: parsed.nodeId,
            question: parsed.question,
            choices: parsed.choices,
            round: parsed.round,
          });
          setQuestionAnswerSubmitting(false);
          if (parsed.nodeId) markQuestionAnswerPausedNodes([parsed.nodeId], event);
          setActiveTab('input');
          break;
        }
        case 'question_answer_submitted': {
          setQuestionAnswerSubmitting(true);
          break;
        }
        case 'node_started': {
          setIsRunning(true);
          setQuestionAnswerSubmitting(false);
          const nodeId = typeof data.node_id === 'string' ? data.node_id : '';
          if (!nodeId) return;
          const title =
            typeof data.title === 'string'
              ? data.title
              : typeof data.node_type === 'string'
                ? data.node_type
                : nodeId;
          const nodeType = typeof data.node_type === 'string' ? data.node_type : 'unknown';
          setRunItems(items => {
            const exists = items.find(item => item.nodeId === nodeId);
            const next: WorkflowRunNodeListItem = {
              nodeId,
              title,
              nodeType,
              status: 'running',
              nodeInput: data.inputs,
            };
            return exists
              ? items.map(item => (item.nodeId === nodeId ? { ...item, ...next } : item))
              : [...items, next];
          });
          break;
        }
        case 'node_finished': {
          const nodeId = typeof data.node_id === 'string' ? data.node_id : '';
          if (!nodeId) return;
          const rawStatus = typeof data.status === 'string' ? data.status : '';
          const status: WorkflowRunNodeListItem['status'] =
            rawStatus === 'success' || rawStatus === 'succeeded' || rawStatus === 'completed'
              ? 'succeeded'
              : rawStatus === 'paused'
                ? 'paused'
                : rawStatus === 'stopped'
                  ? 'stopped'
                  : 'failed';
          const title =
            typeof data.title === 'string'
              ? data.title
              : typeof data.node_type === 'string'
                ? data.node_type
                : nodeId;
          const nodeType = typeof data.node_type === 'string' ? data.node_type : 'unknown';
          setRunItems(items => {
            const exists = items.find(item => item.nodeId === nodeId);
            const next: WorkflowRunNodeListItem = {
              nodeId,
              title,
              nodeType,
              status,
              elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : 0,
              nodeInput: data.inputs,
              nodeOutput: data.outputs,
              modelInput: extractLlmGatewayRequest(data),
              error: getWorkflowRunErrorText(data.error) ?? null,
            };
            return exists
              ? items.map(item => (item.nodeId === nodeId ? { ...item, ...next } : item))
              : [...items, next];
          });
          break;
        }
        case 'message':
        case 'text_chunk': {
          if (typeof data.answer_delta === 'string') {
            if (data.replace === true) {
              throttler.cancel();
              setStreamedText(data.answer_delta);
              setFinalResult({ kind: 'text', content: data.answer_delta });
            } else if (data.answer_delta.length > 0) {
              throttler.append(data.answer_delta);
            }
            break;
          }
          const text =
            typeof data.text === 'string'
              ? data.text
              : typeof data.answer === 'string'
                ? data.answer
                : typeof data.delta === 'string'
                  ? data.delta
                  : '';
          if (text) throttler.append(text);
          break;
        }
        case 'text_replace': {
          throttler.cancel();
          const text = getSensitiveOutputTextFromPayload(payload) ?? SENSITIVE_OUTPUT_BLOCKED_TOKEN;
          setStreamedText(text);
          setFinalResult({ kind: 'text', content: text });
          break;
        }
        case 'workflow_paused': {
          const parsed = parseWorkflowPausedEvent(payload);
          if (parsed.hasApproval) {
            setApprovalPaused(true);
            dispatchApprovalRuntimeEvent('workflow_paused', event);
            markApprovalPausedNodes(parsed.approval.nodeIds, payload);
          }
          if (parsed.hasQuestionAnswer) {
            if (parsed.questionAnswer.prompt) {
              setQuestionAnswerPrompt({
                nodeId: parsed.questionAnswer.prompt.nodeId,
                question: parsed.questionAnswer.prompt.question,
                choices: parsed.questionAnswer.prompt.choices,
                round: parsed.questionAnswer.prompt.round,
              });
              setQuestionAnswerSubmitting(false);
            }
            markQuestionAnswerPausedNodes(parsed.questionAnswer.nodeIds, payload);
          }
          if (!parsed.hasApproval && !parsed.hasQuestionAnswer) return;
          setActiveTab('input');
          break;
        }
        case 'workflow_finished':
        case 'workflow_stopped':
        case 'workflow_failed':
        case 'workflow_succeeded':
        case 'workflow_completed': {
          setIsRunning(false);
          setIsStopping(false);
          throttler.flush();
          const result = normalizeOutputs(data.outputs);
          setFinalResult(result);
          if (result.kind === 'text') setStreamedText(result.content);
          setApprovalPaused(false);
          resetApprovalRuntime();
          setQuestionAnswerPrompt(null);
          setQuestionAnswerSubmitting(false);
          rememberWorkflowRunId(null);
          cancelWorkflowRunEvents();
          workflowRunEventsActiveRef.current = false;
          workflowFinishedRef.current = true;
          setActiveTab('result');
          const eventStatus =
            event.event === 'workflow_stopped'
              ? 'stopped'
              : event.event === 'workflow_failed'
                ? 'failed'
                : event.event === 'workflow_succeeded' || event.event === 'workflow_completed'
                  ? 'succeeded'
                  : '';
          const status = (
            typeof data.status === 'string' ? data.status : eventStatus
          ).toLowerCase();
          if (status === 'failed' || status === 'error') {
            notifyPublishedWorkflowFailure();
          }
          break;
        }
        case 'error': {
          setIsRunning(false);
          setIsStopping(false);
          setApprovalPaused(false);
          resetApprovalRuntime();
          setQuestionAnswerSubmitting(false);
          rememberWorkflowRunId(null);
          cancelWorkflowRunEvents();
          workflowRunEventsActiveRef.current = false;
          workflowFinishedRef.current = true;
          throttler.flush();
          notifyPublishedWorkflowFailure();
          break;
        }
      }
    },
    [
      cancelWorkflowRunEvents,
      dispatchApprovalRuntimeEvent,
      getWorkflowRunIdFromPayload,
      getWorkflowRunErrorText,
      globalT,
      markApprovalPausedNodes,
      markQuestionAnswerPausedNodes,
      normalizeOutputs,
      notifyPublishedWorkflowFailure,
      rememberWorkflowRunId,
      resetApprovalRuntime,
      throttler,
    ]
  );

  const startWorkflowRunEventStream = useCallback(
    (payload?: unknown) => {
      const runId =
        workflowRunIdRef.current ||
        workflowRunId ||
        (payload ? getWorkflowRunIdFromPayload(payload) : '');
      if (!runId || workflowRunEventsActiveRef.current) return;

      rememberWorkflowRunId(runId);
      workflowRunEventsActiveRef.current = true;
      workflowFinishedRef.current = false;
      durableRunNodeAccumulator.replaceSnapshot(
        runItemsRef.current.map(nodeInfoFromWorkflowRunItem)
      );
      const streamParams =
        approvalRuntimeState.cursor > 0
          ? { after: approvalRuntimeState.cursor, continue_on_pause: true }
          : { include_snapshot: true, continue_on_pause: true };
      const dispatchWorkflowRunEvent = (eventName: string, eventPayload: unknown) => {
        const record =
          eventPayload && typeof eventPayload === 'object'
            ? (eventPayload as Record<string, unknown>)
            : {};
        if (typeof record.event === 'string') {
          dispatchApprovalEvent(record);
          return;
        }
        dispatchApprovalEvent({ event: eventName, data: eventPayload });
      };

      void startWorkflowRunEvents(
        runId,
        {
          onWorkflowSnapshot: streamPayload => {
            const snapshot = unwrap(streamPayload);
            const rawNodes = Array.isArray(snapshot.nodes) ? snapshot.nodes : [];
            const executionItems = buildWorkflowRunExecutionItems(
              rawNodes as WorkflowNodeExecution[]
            );
            setRunItems(executionItems);
            durableRunNodeAccumulator.replaceSnapshot(
              executionItems.map(nodeInfoFromWorkflowRunItem)
            );

            const message =
              snapshot.message && typeof snapshot.message === 'object'
                ? (snapshot.message as Record<string, unknown>)
                : null;
            const run =
              snapshot.workflow_run && typeof snapshot.workflow_run === 'object'
                ? (snapshot.workflow_run as Record<string, unknown>)
                : null;
            throttler.cancel();
            if (message && typeof message.answer === 'string') {
              setStreamedText(message.answer);
              if (message.answer.length > 0) {
                setFinalResult({ kind: 'text', content: message.answer });
              }
            }

            const status = typeof run?.status === 'string' ? run.status.toLowerCase() : '';
            if (
              ['succeeded', 'success', 'completed', 'failed', 'error', 'stopped'].includes(status)
            ) {
              const detail = run as unknown as WorkflowRunDetail;
              const result = buildWorkflowRunHistoryResult(detail);
              if (result.kind !== 'empty' || !message?.answer) {
                setFinalResult(result);
                if (result.kind === 'text') setStreamedText(result.content);
              }
              dispatchApprovalEvent({ event: 'workflow_finished', data: run });
              return;
            }

            const pauseEvent = createWorkflowSnapshotPauseEvent(streamPayload);
            if (pauseEvent) {
              dispatchApprovalEvent(pauseEvent);
            } else {
              setApprovalPaused(false);
              resetApprovalRuntime();
              setQuestionAnswerPrompt(null);
              setQuestionAnswerSubmitting(false);
              setIsRunning(true);
            }
          },
          onWorkflowStarted: streamPayload =>
            dispatchWorkflowRunEvent('workflow_started', streamPayload),
          onWorkflowResumed: streamPayload =>
            dispatchWorkflowRunEvent('workflow_resumed', streamPayload),
          onApprovalRequested: streamPayload =>
            dispatchWorkflowRunEvent('approval_requested', streamPayload),
          onApprovalResultFilled: streamPayload =>
            dispatchWorkflowRunEvent('approval_result_filled', streamPayload),
          onApprovalExpired: streamPayload =>
            dispatchWorkflowRunEvent('approval_expired', streamPayload),
          onQuestionAnswerRequested: streamPayload =>
            dispatchWorkflowRunEvent('question_answer_requested', streamPayload),
          onQuestionAnswerSubmitted: streamPayload =>
            dispatchWorkflowRunEvent('question_answer_submitted', streamPayload),
          onWorkflowPaused: streamPayload =>
            dispatchWorkflowRunEvent('workflow_paused', streamPayload),
          onNodeStarted: streamPayload => {
            setIsRunning(true);
            setQuestionAnswerSubmitting(false);
            durableRunNodeAccumulator.onNodeStarted(streamPayload);
          },
          onNodeFinished: streamPayload => durableRunNodeAccumulator.onNodeFinished(streamPayload),
          onTextChunk: streamPayload => dispatchWorkflowRunEvent('text_chunk', streamPayload),
          onTextReplace: streamPayload => dispatchWorkflowRunEvent('text_replace', streamPayload),
          onWorkflowFinished: streamPayload =>
            dispatchWorkflowRunEvent('workflow_finished', streamPayload),
          onError: streamPayload => dispatchWorkflowRunEvent('error', streamPayload),
          onMessage: streamPayload => dispatchWorkflowRunEvent('message', streamPayload),
          onMessageEnd: streamPayload => dispatchWorkflowRunEvent('message_end', streamPayload),
          onIterationStarted: streamPayload =>
            durableRunNodeAccumulator.onIterationStarted(streamPayload),
          onIterationNext: streamPayload =>
            durableRunNodeAccumulator.onIterationNext(streamPayload),
          onIterationCompleted: streamPayload =>
            durableRunNodeAccumulator.onIterationCompleted(streamPayload),
          onLoopStarted: streamPayload => durableRunNodeAccumulator.onLoopStarted(streamPayload),
          onLoopNext: streamPayload => durableRunNodeAccumulator.onLoopNext(streamPayload),
          onLoopCompleted: streamPayload =>
            durableRunNodeAccumulator.onLoopCompleted(streamPayload),
        },
        streamParams,
        {
          onClose: () => {
            workflowRunEventsActiveRef.current = false;
          },
        }
      );
    },
    [
      approvalRuntimeState.cursor,
      dispatchApprovalEvent,
      durableRunNodeAccumulator,
      getWorkflowRunIdFromPayload,
      rememberWorkflowRunId,
      resetApprovalRuntime,
      startWorkflowRunEvents,
      throttler,
      workflowRunId,
    ]
  );

  useEffect(() => {
    startWorkflowRunEventStreamRef.current = startWorkflowRunEventStream;
  }, [startWorkflowRunEventStream]);

  const recoverInterruptedWorkflowRun = useCallback(
    (runId: string) => {
      rememberWorkflowRunId(runId);
      startWorkflowRunEventStreamRef.current({ workflow_run_id: runId });
    },
    [rememberWorkflowRunId]
  );

  useEffect(
    () => () => {
      workflowFinishedRef.current = true;
      cancelWorkflowRunEvents();
      workflowRunEventsActiveRef.current = false;
    },
    [cancelWorkflowRunEvents]
  );

  const handleApprovalSubmit = useCallback(
    async (payload: { inputs: Record<string, unknown>; action: string }) => {
      if (!approvalToken || !approvalEntry) return;
      setApprovalSubmitting(approvalEntry.key, payload.action);
      if (!workflowRunEventsActiveRef.current) {
        startWorkflowRunEventStreamRef.current();
      }
      try {
        await approvalSubmitMutation.mutateAsync(payload);
        setApprovalSubmitted(approvalEntry.key, payload.action);
      } catch {
        setApprovalWaiting(approvalEntry.key);
        toast.error(globalT('nodes.approval.runtime.submitFailed'));
      }
    },
    [
      approvalEntry,
      approvalSubmitMutation,
      approvalToken,
      globalT,
      setApprovalSubmitted,
      setApprovalSubmitting,
      setApprovalWaiting,
    ]
  );

  const isApprovalPending = Boolean(
    approvalPaused || approvalEntry || approvalToken || approvalForm || approvalSubmittedAction
  );
  const isQuestionAnswerPending = Boolean(questionAnswerPrompt);
  const questionAnswerHasChoices = Boolean((questionAnswerPrompt?.choices.length ?? 0) > 0);
  const hasExecutionActivity = isRunning || runItems.length > 0;
  const hasRunState = runItems.length > 0 || Boolean(finalResult) || streamedText.trim().length > 0;

  useEffect(() => {
    if (hasExecutionActivity) {
      setExecutionOpen(true);
    }
  }, [hasExecutionActivity]);

  const handleSubmit = useCallback(
    async (values: FormInputs) => {
      try {
        if (isApprovalPending) {
          toast.info(globalT('nodes.approval.runtime.paused'));
          return;
        }

        const isQuestionAnswerResume = Boolean(questionAnswerPrompt);
        if (!isQuestionAnswerResume && hasRequiredAnonymousFileInputs) {
          toast.error(t('run.loginRequiredToRunWithFiles'));
          return;
        }

        const queryValue = typeof values['query'] === 'string' ? values['query'] : '';
        const runPayload = {
          query: queryValue,
          conversation_id: conversationIdParam || undefined,
          inputs: {
            ...(values as unknown as Record<string, unknown>),
            ...(questionAnswerPrompt?.nodeId
              ? { question_answer_node_id: questionAnswerPrompt.nodeId }
              : {}),
            ...(typeof questionAnswerPrompt?.round === 'number'
              ? { question_answer_round: questionAnswerPrompt.round }
              : {}),
          },
        };

        if (enablePrecheck && !isQuestionAnswerResume) {
          const precheck = await precheckMutation.mutateAsync(runPayload);
          const warnings = getWorkflowPrecheckWarnings(precheck.data);
          if (precheck.data.status === 'warning' && warnings.length > 0) {
            setPrecheckWarnings(warnings);
          } else {
            setPrecheckWarnings([]);
          }
        } else {
          setPrecheckWarnings([]);
        }

        if (isQuestionAnswerResume) {
          questionAnswerResumeRef.current = true;
          setQuestionAnswerSubmitting(true);
        } else {
          resetApprovalRuntime();
          setApprovalPaused(false);
          setQuestionAnswerPrompt(null);
          setQuestionAnswerSubmitting(false);
          rememberWorkflowRunId(null);
          workflowFinishedRef.current = false;
          cancelWorkflowRunEvents();
          workflowRunEventsActiveRef.current = false;
        }
        await start(runPayload, undefined, {
          onTransportInterrupted: recoverInterruptedWorkflowRun,
        });
      } catch (err) {
        questionAnswerResumeRef.current = false;
        setQuestionAnswerSubmitting(false);
        if (isWebAppOfflineError(err)) {
          markOffline();
          return;
        }
        toast.error(t('run.startFailed'));
      }
    },
    [
      cancelWorkflowRunEvents,
      enablePrecheck,
      conversationIdParam,
      globalT,
      hasRequiredAnonymousFileInputs,
      isApprovalPending,
      questionAnswerPrompt,
      markOffline,
      precheckMutation,
      rememberWorkflowRunId,
      recoverInterruptedWorkflowRun,
      resetApprovalRuntime,
      start,
      t,
    ]
  );

  const handleStop = useCallback(async () => {
    setIsStopping(true);
    try {
      await stop();
    } catch {
      setIsStopping(false);
    }
  }, [stop]);

  const interactionStopAction = (
    <WorkflowRuntimeStopAction
      onStop={handleStop}
      isStopping={isStopping}
      disabled={
        approvalSubmitMutation.isPending || approvalRuntimeSubmitting || questionAnswerSubmitting
      }
    />
  );

  const handleQuestionAnswerSelect = useCallback(
    async (choice: QuestionAnswerChoice) => {
      const query = getQuestionAnswerChoiceQuery(choice);
      if (!query || questionAnswerSubmitting) return;
      try {
        questionAnswerResumeRef.current = true;
        setQuestionAnswerSubmitting(true);
        await start(
          {
            query,
            inputs: {
              query,
              'sys.query': query,
              question_answer_option_id: choice.id,
              ...(questionAnswerPrompt?.nodeId
                ? { question_answer_node_id: questionAnswerPrompt.nodeId }
                : {}),
              ...(typeof questionAnswerPrompt?.round === 'number'
                ? { question_answer_round: questionAnswerPrompt.round }
                : {}),
            },
          },
          undefined,
          { onTransportInterrupted: recoverInterruptedWorkflowRun }
        );
      } catch (err) {
        questionAnswerResumeRef.current = false;
        setQuestionAnswerSubmitting(false);
        if (isWebAppOfflineError(err)) {
          markOffline();
          return;
        }
        toast.error(t('run.startFailed'));
      }
    },
    [
      markOffline,
      questionAnswerPrompt,
      questionAnswerSubmitting,
      recoverInterruptedWorkflowRun,
      start,
      t,
    ]
  );

  const questionAnswerNotice = questionAnswerPrompt ? (
    <div className="mb-3">
      <QuestionAnswerRuntimePrompt
        question={questionAnswerPrompt.question}
        choices={questionAnswerPrompt.choices}
        round={questionAnswerPrompt.round}
        submitting={questionAnswerSubmitting}
        onSelectChoice={handleQuestionAnswerSelect}
        secondaryAction={interactionStopAction}
      />
    </div>
  ) : null;

  // no clear output action needed; Results renders streamed text directly
  if (isOffline) {
    return <WebAppOfflineState />;
  }

  const inputFormContent = (
    <>
      {startVariables.length === 0 && !isQuestionAnswerPending ? (
        <div className="mb-3 rounded-lg border border-dashed bg-muted/20 px-4 py-3 text-sm text-muted-foreground">
          {t('run.noInputsRequired')}
        </div>
      ) : null}
      <WorkflowInputForm
        key={varsSig}
        ref={formRef}
        startVariables={startVariables}
        isStarting={
          isStarting ||
          isRunning ||
          isApprovalPending ||
          questionAnswerHasChoices ||
          questionAnswerSubmitting
        }
        onSubmit={handleSubmit}
        hideSubmitButton
        fileUploadAccessMode={isAuthenticated ? 'enabled' : 'login-required'}
        allowWorkspaceSwitch
        topNotice={
          <>
            {precheckWarnings.length > 0 ? (
              <WorkflowPrecheckWarningBanner
                warnings={precheckWarnings}
                scope="webapp"
                storageKey={`webapp-run:${versionUuid}`}
                placement="inline"
              />
            ) : null}
            {questionAnswerNotice}
            {hasAnonymousFileInputs ? (
              <div className="mb-3 rounded-md border border-border bg-muted/40 px-3 py-2 text-sm text-muted-foreground">
                {hasRequiredAnonymousFileInputs
                  ? t('run.loginRequiredToRunWithFiles')
                  : t('run.loginRequiredForFileInputs')}
              </div>
            ) : null}
          </>
        }
      />
    </>
  );

  const approvalInputContent = approvalSubmittedAction ? (
    <WorkflowApprovalInteractionCard mode="submitted" secondaryAction={interactionStopAction} />
  ) : !approvalToken ? (
    <WorkflowApprovalInteractionCard mode="external" secondaryAction={interactionStopAction} />
  ) : !approvalForm &&
    !approvalFormQuery.error &&
    (approvalFormQuery.isLoading || approvalFormQuery.isFetching) ? (
    <WorkflowApprovalInteractionCard mode="loading" secondaryAction={interactionStopAction} />
  ) : !approvalForm && isApprovalFormAlreadySubmittedError(approvalFormQuery.error) ? (
    <WorkflowApprovalInteractionCard mode="completed" />
  ) : !approvalForm && approvalFormQuery.error ? (
    <WorkflowApprovalInteractionCard
      mode="error"
      error={approvalFormQuery.error}
      onRetry={() => void approvalFormQuery.refetch()}
      secondaryAction={interactionStopAction}
    />
  ) : approvalForm ? (
    <WorkflowApprovalInteractionCard
      mode={
        isWorkflowApprovalInlineAllowed({ surface: 'workflow-webapp', form: approvalForm })
          ? 'form'
          : 'external'
      }
      form={approvalForm}
      onSubmit={payload => void handleApprovalSubmit(payload)}
      isSubmitting={approvalSubmitMutation.isPending || approvalRuntimeSubmitting}
      submittedAction={approvalSubmittedAction}
      secondaryAction={interactionStopAction}
    />
  ) : (
    <WorkflowApprovalInteractionCard mode="loading" secondaryAction={interactionStopAction} />
  );

  const failedExecution = runItems.some(item => item.status === 'failed');
  const stoppedExecution = runItems.some(item => item.status === 'stopped');
  const waitingForInput = isApprovalPending || isQuestionAnswerPending;
  const executionStatus = isRunning
    ? t('run.statusRunning')
    : waitingForInput
      ? t('run.statusWaiting')
      : failedExecution
        ? t('run.statusFailed')
        : stoppedExecution
          ? t('run.stopped')
          : runItems.length > 0
            ? t('run.completed')
            : t('run.notRunYet');
  const executionStatusClassName = isRunning
    ? 'bg-primary'
    : waitingForInput
      ? 'bg-amber-500'
      : failedExecution
        ? 'bg-destructive'
        : stoppedExecution
          ? 'bg-muted-foreground'
          : runItems.length > 0
            ? 'bg-emerald-500'
            : 'bg-muted-foreground/50';

  const runActionFooter = waitingForInput ? null : (
    <div
      className={cn(
        'shrink-0 bg-background/95 backdrop-blur',
        isCompactLayout ? 'rounded-xl border p-2 shadow-sm' : 'border-t border-border/70 px-4 py-3'
      )}
    >
      {isRunning ? (
        <Button
          onClick={handleStop}
          variant="destructive"
          className="h-10 w-full font-medium"
          disabled={isStopping}
        >
          {isStopping ? t('run.stopping') : t('run.stop')}
        </Button>
      ) : questionAnswerHasChoices ? (
        <div className="flex h-10 w-full items-center justify-center gap-2 rounded-lg border bg-muted/40 text-sm font-medium text-muted-foreground">
          <Clock3 className="size-4" />
          {globalT('nodes.questionAnswer.runtime.chooseOne')}
        </div>
      ) : (
        <Button
          onClick={() => {
            formRef.current?.submit();
          }}
          disabled={
            isStarting ||
            questionAnswerSubmitting ||
            isApprovalPending ||
            (!isQuestionAnswerPending && hasRequiredAnonymousFileInputs)
          }
          className="h-10 w-full font-medium shadow-sm"
        >
          <Play className="mr-2 size-4" />
          {isStarting || questionAnswerSubmitting
            ? t('run.starting')
            : isQuestionAnswerPending
              ? t('consoleChat.send')
              : hasRunState
                ? t('run.rerun')
                : t('run.runNow')}
        </Button>
      )}
      {!isRunning && !isQuestionAnswerPending && hasRequiredAnonymousFileInputs ? (
        <p className="pt-2 text-xs text-muted-foreground">{t('run.loginRequiredToRunWithFiles')}</p>
      ) : null}
    </div>
  );

  const inputSection = (
    <div className="flex h-full w-full flex-col overflow-hidden rounded-xl border bg-card shadow-sm">
      <div className="flex h-11 shrink-0 items-center gap-2 border-b bg-muted/30 px-4">
        {isApprovalPending ? (
          <Clock3 className="size-5 text-amber-600" />
        ) : isQuestionAnswerPending ? (
          <HelpCircle className="size-5 text-highlight" />
        ) : (
          <FileOutput className="size-5 text-highlight" />
        )}
        <h2 className="text-sm font-semibold text-foreground">
          {isApprovalPending
            ? globalT('nodes.approval.runtime.paused')
            : isQuestionAnswerPending
              ? globalT('nodes.questionAnswer.runtime.waitingAnswer')
              : t('run.inputsTitle')}
        </h2>
      </div>
      <div className="min-h-0 flex-1 overflow-auto px-5 py-3">
        {isApprovalPending ? approvalInputContent : inputFormContent}
      </div>
      {!isCompactLayout ? runActionFooter : null}
    </div>
  );

  const executionSection = (
    <Collapsible
      open={executionOpen}
      onOpenChange={setExecutionOpen}
      className={cn(
        'flex min-h-0 flex-col overflow-hidden rounded-xl border bg-card shadow-sm',
        isCompactLayout && executionOpen ? 'h-full' : 'shrink-0'
      )}
    >
      <CollapsibleTrigger asChild>
        <button
          type="button"
          aria-label={executionOpen ? t('run.collapseExecution') : t('run.expandExecution')}
          className="focus-ring flex h-11 w-full shrink-0 items-center gap-2 bg-muted/30 px-4 text-left transition-colors hover:bg-muted/50"
        >
          <Activity className="size-5 shrink-0 text-emerald-600" />
          <span className="text-sm font-semibold text-foreground">{t('run.execution')}</span>
          <span className="ml-auto flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
            <span className={cn('size-2 shrink-0 rounded-full', executionStatusClassName)} />
            <span aria-live="polite" className="truncate">
              {executionStatus}
            </span>
            {runItems.length > 0 ? (
              <span className="shrink-0 rounded-full bg-background px-2 py-0.5">
                {t('run.stepCount', { count: runItems.length })}
              </span>
            ) : null}
          </span>
          <ChevronDown
            className={cn(
              'size-4 shrink-0 text-muted-foreground transition-transform',
              executionOpen && 'rotate-180'
            )}
          />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent
        className={cn(
          'min-h-0 overflow-hidden border-t',
          isCompactLayout ? 'flex-1' : 'h-[clamp(180px,32vh,320px)]'
        )}
      >
        <ExecutionTab
          items={runItems}
          showDetail={false}
          showFailureDetails={false}
          showHeader={false}
          emptyTitle={t('run.notRunYet')}
          emptyDescription={t('run.executionEmptyDescription')}
          className="h-full"
        />
      </CollapsibleContent>
    </Collapsible>
  );

  const outputSection = (
    <div className="flex h-full min-h-0 flex-col overflow-hidden rounded-xl border bg-card shadow-sm">
      <Results
        mode="draft"
        title={t('run.output')}
        streamedText={streamedText}
        historyResult={finalResult}
        emptyText={t('run.outputEmptyDescription')}
        headerClassName="h-11 px-4 py-0"
        emptyStateClassName="gap-3 text-muted-foreground [&_svg]:size-10"
      />
    </div>
  );

  return (
    <div ref={runContainerRef} className="h-full w-full overflow-hidden p-2 md:p-4">
      <Tabs
        value={activeTab}
        onValueChange={setActiveTab}
        className={cn(
          'grid h-full min-h-0 overflow-hidden',
          isCompactLayout
            ? 'grid-cols-1 grid-rows-[auto_minmax(0,1fr)_auto] gap-2'
            : 'grid-cols-[clamp(360px,34%,420px)_minmax(0,1fr)] grid-rows-[auto_minmax(0,1fr)] gap-x-4 gap-y-3'
        )}
      >
        <TabsList className={cn('grid shrink-0 grid-cols-3', !isCompactLayout && 'hidden')}>
          <TabsTrigger value="input" className="flex min-w-0 items-center gap-1.5">
            <Settings2 className="w-3.5 h-3.5" />
            <span className="truncate">{t('run.inputsTitle')}</span>
          </TabsTrigger>
          <TabsTrigger value="execution" className="flex min-w-0 items-center gap-1.5">
            <Activity className="w-3.5 h-3.5" />
            <span className="truncate">{t('run.execution')}</span>
            {isRunning || waitingForInput ? (
              <span
                className={cn(
                  'size-1.5 shrink-0 rounded-full',
                  isRunning ? 'animate-pulse bg-primary' : 'bg-amber-500'
                )}
              />
            ) : null}
          </TabsTrigger>
          <TabsTrigger value="result" className="flex min-w-0 items-center gap-1.5">
            <Terminal className="w-3.5 h-3.5" />
            <span className="truncate">{t('run.output')}</span>
          </TabsTrigger>
        </TabsList>

        <TabsContent
          value="input"
          forceMount
          className={cn(
            'm-0 min-h-0 overflow-hidden focus-visible:ring-0',
            isCompactLayout
              ? 'col-start-1 row-start-2 h-full p-1 data-[state=inactive]:hidden'
              : 'col-start-1 row-span-2 row-start-1 h-full'
          )}
        >
          {inputSection}
        </TabsContent>
        <TabsContent
          value="execution"
          forceMount
          className={cn(
            'm-0 min-h-0 overflow-hidden focus-visible:ring-0',
            isCompactLayout
              ? 'col-start-1 row-start-2 h-full p-1 data-[state=inactive]:hidden'
              : 'col-start-2 row-start-1'
          )}
        >
          {executionSection}
        </TabsContent>
        <TabsContent
          value="result"
          forceMount
          className={cn(
            'm-0 min-h-0 overflow-hidden focus-visible:ring-0',
            isCompactLayout
              ? 'col-start-1 row-start-2 h-full p-1 data-[state=inactive]:hidden'
              : 'col-start-2 row-start-2 h-full'
          )}
        >
          {outputSection}
        </TabsContent>
        {isCompactLayout ? runActionFooter : null}
      </Tabs>
    </div>
  );
};
