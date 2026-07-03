import React, { useCallback, useMemo, useState, useEffect, useRef } from 'react';
import { Panel, useReactFlow } from '@xyflow/react';
import { usePanelStackItem } from '../../hooks';
import { useWorkflowDraft, useSaveWorkflowDraft } from '@/hooks';
import type { InputVar } from '@/components/workflow/types/input-var';
import { useWorkflowStore } from '@/components/workflow/store';
import type { WorkflowNode, WorkflowEdge } from '@/components/workflow/store/type';
import type { WorkflowRunInputValues, WorkflowRunSseCallbacks } from '@/services/workflow.service';
import type { WorkflowRunNodeListItem } from '@/components/workflow/ui/workflow-run-nodes-list';
import type { FormInputs } from '@/components/workflow/common/workflow-input-form';
import { useRunWorkflowDraftStream, useWorkflowRunEventsStream } from '@/hooks';
import { useStopWorkflowTask } from '@/hooks/workflow/use-stop-workflow-task';
import { History, Loader, Pause } from 'lucide-react';
import { Button } from '@/components/ui/button';
import Header from './components/header';
import DraftContent from './components/draft-content';
import HistoryContent from './components/history-content';
import WorkflowRunsDropdown from '../workflow-runs-dropdown';
import { useHistoryView } from './hooks/use-history-view';
import { useSseCallbacks } from './hooks/use-sse-callbacks';
import type { WorkflowFinishedData, HistoryResult } from './types';
import useWorkflowValidation from '../../hooks/use-workflow-validation';
import { flushWorkflowPendingEdits } from '../../hooks/pending-edits';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { isBannerHidden, hideBanner, BannerKey } from '@/utils/ui-local';
import { useT } from '@/i18n';
import { WorkflowPrecheckWarningBanner } from '@/components/workflow/common/workflow-precheck-warning';
import type { QuestionAnswerChoice, WorkflowPrecheckWarning } from '@/services/types/workflow';
import { useWorkflowDraftPrecheck } from '@/hooks/workflow/use-workflow-precheck';
import { getWorkflowPrecheckWarnings } from '@/utils/workflow/billing';
import { useWorkflowBillingFeedback } from '@/hooks/workflow/use-workflow-billing-feedback';
import { toast } from 'sonner';
import { fetchApprovalEvents, useApprovalForm, useSubmitApprovalForm } from '@/hooks';
import {
  getApprovalEventSequence,
  parseApprovalRequestedEvent,
  parseApprovalPausedEvent,
} from '@/components/workflow/approval/runtime-events';
import { useApprovalRuntimeEvents } from '@/components/workflow/approval/use-approval-runtime-events';
import { getRightPanelMotionClassName, getRightPanelMotionStyle } from '../right-panel-motion';
import {
  appendQuestionAnswerTranscriptQuestion,
  applyQuestionAnswerTranscriptSubmission,
  parseQuestionAnswerPausedEvent,
  parseQuestionAnswerRequestedEvent,
  parseQuestionAnswerSubmittedEvent,
  type QuestionAnswerTranscriptItem,
} from '@/components/workflow/question-answer/runtime-events';
import { getQuestionAnswerChoiceQuery } from '@/components/workflow/question-answer/question-answer-runtime-prompt';
import { useResizableRightPanel } from '../use-resizable-right-panel';
import { useActivePanel } from '../../hooks/use-active-panel';

interface WorkflowRunPanelProps {
  // Whether the run panel is visible
  open: boolean;
  // Whether the panel should temporarily slide off-canvas during canvas operations
  temporarilyHidden?: boolean;
  // Close handler
  onClose: () => void;
  // Current agent id for API calls
  agentId: string;
}

interface WorkflowRunEventEnvelope {
  event?: string;
  data?: unknown;
  [key: string]: unknown;
}

/**
 * WorkflowRunPanel - Floating panel to run the current draft workflow only.
 * Uses @xyflow/react Panel to float over the canvas without affecting layout.
 */
const WorkflowRunPanel: React.FC<WorkflowRunPanelProps> = ({
  open,
  temporarilyHidden = false,
  onClose,
  agentId,
}) => {
  const { panelWidth, isResizing, panelWidthStyle, resizeHandleProps } = useResizableRightPanel({
    cssVar: '--workflow-run-panel-w',
    defaultWidth: 460,
    minWidth: 380,
    maxWidth: 620,
    maxViewportRatio: 0.52,
  });

  // Register with panel stack for horizontal arrangement at top-right
  const { panelStyle } = usePanelStackItem({
    id: 'run-details',
    position: 'top-right',
    order: 1,
    visible: open,
    width: panelWidth,
    gap: 8,
  });
  const t = useT();
  const setActivePanel = useActivePanel(state => state.setActive);
  const workflowDraftPrecheck = useWorkflowDraftPrecheck(agentId);
  const { notifyBillingError } = useWorkflowBillingFeedback('agents');

  // Local graph state from store (reflects unsaved edits immediately)
  const nodes = useWorkflowStore.use.nodes();
  const edges = useWorkflowStore.use.edges();
  const viewport = useWorkflowStore.use.viewport();
  const mode = useWorkflowStore.use.mode();
  const selectedRunId = useWorkflowStore.use.selectedRunId();
  const enterHistoryMode = useWorkflowStore.use.enterHistoryMode();
  const setHistorySnapshot = useWorkflowStore.use.setHistorySnapshot();
  const isHistory = mode === 'history';
  const debugRunsQuery = useMemo(() => ({ triggered_from: 'debugging' as const }), []);
  const handleSelectDebugRun = useCallback(
    (runId: string) => {
      enterHistoryMode(runId);
      setActivePanel('run');
    },
    [enterHistoryMode, setActivePanel]
  );
  // Run-status slice actions/state
  const resetRunStatus = useWorkflowStore.use.resetRunStatus();
  const setNodeRunStatus = useWorkflowStore.use.setNodeRunStatus();
  const setActiveOutputHandle = useWorkflowStore.use.setActiveOutputHandle();
  const resetActiveOutputHandles = useWorkflowStore.use.resetActiveOutputHandles();
  const beginRuntimeLogPopoverAutoOpen =
    useWorkflowStore.use.beginRuntimeLogPopoverAutoOpen();
  const finalizeRuntimeLogPopoversAfterRun =
    useWorkflowStore.use.finalizeRuntimeLogPopoversAfterRun();
  const setAutoFollow = useWorkflowStore.use.setAutoFollow();
  const setRuntimeLogItems = useWorkflowStore.use.setRuntimeLogItems();
  // const isAutoFollow = useWorkflowStore.use.isAutoFollow();
  const setCurrentRunningNodeId = useWorkflowStore.use.setCurrentRunningNodeId();
  const setLastDebugInputs = useWorkflowStore.use.setLastDebugInputs();

  // React Flow instance for centering view to running node
  const rf = useReactFlow();

  // Query: Load workflow draft to access start node variables (fallback only)
  const { data: draft, isLoading: isLoadingDraft } = useWorkflowDraft(agentId);

  // History View hook
  const {
    loading: isHistoryLoading,
    summary: historyRunSummary,
    items: historyExecutionItems,
    result: historyResult,
    graph: historyGraph,
  } = useHistoryView({ agentId, open, isHistory, selectedRunId });

  // Apply history graph snapshot if available
  useEffect(() => {
    if (!open || !isHistory || !selectedRunId) return;
    const graph = historyGraph;
    if (!graph) return;
    const gNodes = (Array.isArray(graph.nodes) ? graph.nodes : []) as unknown as WorkflowNode[];
    const gEdges = (Array.isArray(graph.edges) ? graph.edges : []) as unknown as WorkflowEdge[];
    const gViewport = graph.viewport ?? { x: 0, y: 0, zoom: 1 };
    setHistorySnapshot(selectedRunId as string, {
      nodes: gNodes,
      edges: gEdges,
      viewport: gViewport,
    });
  }, [open, isHistory, selectedRunId, historyGraph, setHistorySnapshot]);

  // Execution items derived from SSE events
  const [runItems, setRunItems] = useState<WorkflowRunNodeListItem[]>([]);
  // Streamed plain text output from text_chunk events
  const [streamedText, setStreamedText] = useState<string>('');
  const [finalResult, setFinalResult] = useState<HistoryResult | null>(null);
  // kept for backward compatibility, not used after Results extraction
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const resultsScrollRef = useRef<HTMLDivElement | null>(null);
  // Active tab for the panel (inputs/execution/details/results)
  const [activeTab, setActiveTab] = useState<'inputs' | 'execution' | 'details' | 'results'>(
    'inputs'
  );
  const draftActiveTab: 'inputs' | 'execution' | 'details' | 'results' = activeTab;

  // moved to Results component

  // Local SSE handle and starting state
  const [isStarting, setIsStarting] = useState(false);
  const [shake, setShake] = useState(false);
  const [precheckWarnings, setPrecheckWarnings] = useState<WorkflowPrecheckWarning[]>([]);

  // Remember last submitted inputs for details view
  const [lastInputs, setLastInputs] = useState<FormInputs | undefined>(undefined);
  // Workflow run summary captured from workflow_finished event
  const [runSummary, setRunSummary] = useState<WorkflowFinishedData | null>(null);
  const [workflowRunId, setWorkflowRunId] = useState<string | null>(null);
  const [workflowConversationId, setWorkflowConversationId] = useState<string | null>(null);
  const [questionAnswerPrompt, setQuestionAnswerPrompt] = useState<{
    question: string;
    choices: QuestionAnswerChoice[];
    round?: number;
  } | null>(null);
  const [questionAnswerSubmitting, setQuestionAnswerSubmitting] = useState(false);
  const [questionAnswerTranscript, setQuestionAnswerTranscript] = useState<
    QuestionAnswerTranscriptItem[]
  >([]);
  const {
    state: approvalRuntimeState,
    activeEntry: approvalEntry,
    activeForm: approvalForm,
    activeToken: approvalToken,
    submittedAction: approvalSubmittedAction,
    isSubmitting: approvalRuntimeSubmitting,
    isExpired: approvalExpired,
    dispatchApprovalEvent,
    setSubmitting: setApprovalSubmitting,
    setSubmitted: setApprovalSubmitted,
    setWaiting: setApprovalWaiting,
    setLoadedForm: setLoadedApprovalForm,
    resetApprovalRuntime,
  } = useApprovalRuntimeEvents();
  const approvalRuntimeStateRef = useRef(approvalRuntimeState);
  const approvalFormQuery = useApprovalForm(approvalToken, Boolean(approvalToken && !approvalForm));
  const approvalSubmitMutation = useSubmitApprovalForm(approvalToken);
  const { start: startWorkflowRunEvents, cancel: cancelWorkflowRunEvents } =
    useWorkflowRunEventsStream();
  const cancelStreamRef = useRef<() => void>(() => {});
  const approvalEventCursorRef = useRef(0);
  const approvalResumeStreamActiveRef = useRef(false);
  const workflowFinishedRef = useRef(false);
  const runtimeLogSignatureRef = useRef('');

  useEffect(() => {
    approvalRuntimeStateRef.current = approvalRuntimeState;
  }, [approvalRuntimeState]);

  const hasBlockingApprovalStop = useCallback(
    () =>
      Object.values(approvalRuntimeStateRef.current.byKey).some(entry =>
        ['waiting', 'submitting'].includes(entry.status)
      ),
    []
  );

  const isApprovalStopBlocked = useMemo(
    () =>
      Object.values(approvalRuntimeState.byKey).some(entry =>
        ['waiting', 'submitting'].includes(entry.status)
      ),
    [approvalRuntimeState.byKey]
  );

  useEffect(() => {
    const items = isHistory ? (open ? historyExecutionItems : []) : runItems;
    const signature = items
      .map(item => {
        let payloadSignature = '';
        try {
          payloadSignature = JSON.stringify({
            nodeInput: item.nodeInput,
            nodeOutput: item.nodeOutput,
            modelInput: item.modelInput,
            processData: item.processData,
            executionMetadata: item.executionMetadata,
            iterationRounds: item.iterationRounds,
            loopRounds: item.loopRounds,
          });
        } catch {
          payloadSignature = '[unserializable]';
        }
        return [
          item.nodeId,
          item.executionId ?? '',
          item.receivedOrder ?? '',
          item.createdAtMs ?? '',
          item.status,
          item.elapsedTime ?? '',
          item.error ?? '',
          payloadSignature,
        ].join(':');
      })
      .join('|');
    if (runtimeLogSignatureRef.current === signature) return;
    runtimeLogSignatureRef.current = signature;
    setRuntimeLogItems(items);
  }, [historyExecutionItems, isHistory, open, runItems, setRuntimeLogItems]);

  // TanStack Query client for cache invalidation on run finish
  // const queryClient = useQueryClient(); // no longer needed here

  // Extract start node variables (prefer local store; fallback to server draft), and filter hidden
  const startVariables = useMemo(() => {
    // Prefer local store nodes
    const localStart = nodes.find(
      n =>
        (n?.data as unknown as { type?: string })?.type === 'start' ||
        (n as unknown as { type?: string })?.type === 'start'
    );
    let vars: InputVar[] | undefined = (localStart?.data as unknown as { variables?: InputVar[] })
      ?.variables;

    // Fallback to server draft if local not available
    if (!Array.isArray(vars)) {
      const dNodes = draft?.graph?.nodes ?? [];
      const start = dNodes.find(
        n =>
          (n?.data as unknown as { type?: string })?.type === 'start' ||
          (n as unknown as { type?: string })?.type === 'start'
      );
      vars = (start?.data as unknown as { variables?: InputVar[] })?.variables;
    }

    return Array.isArray(vars) ? vars.filter(v => !v.hide) : [];
  }, [nodes, draft]);
  const debugSetupHints = useMemo(() => {
    const nodeTypes = new Set(
      nodes
        .map(node => (node?.data as { type?: string } | undefined)?.type)
        .filter((type): type is string => typeof type === 'string')
    );
    const hints: string[] = [];

    if (startVariables.some(v => v.type === 'file' || v.type === 'file-list')) {
      hints.push(t('agents.workflow.debugGuide.fileInputHint'));
    }
    if (nodeTypes.has('document-extractor')) {
      hints.push(t('agents.workflow.debugGuide.documentHint'));
    }
    if (nodeTypes.has('knowledge-retrieval')) {
      hints.push(t('agents.workflow.debugGuide.knowledgeHint'));
    }
    if (nodeTypes.has('sql-generator')) {
      hints.push(t('agents.workflow.debugGuide.databaseHint'));
    }
    if (nodeTypes.has('http-request')) {
      hints.push(t('agents.workflow.debugGuide.httpHint'));
    }
    if (nodeTypes.has('notification-sms')) {
      hints.push(t('agents.workflow.debugGuide.smsHint'));
    }
    if (nodeTypes.has('approval')) {
      hints.push(t('agents.workflow.debugGuide.approvalHint'));
    }
    if (nodeTypes.has('code')) {
      hints.push(t('agents.workflow.debugGuide.codeHint'));
    }
    if (nodeTypes.has('image-gen')) {
      hints.push(t('agents.workflow.debugGuide.imageHint'));
    }

    return Array.from(new Set(hints));
  }, [nodes, startVariables, t]);

  // Build SSE callbacks via extracted hook
  const sseCallbacks: WorkflowRunSseCallbacks = useSseCallbacks({
    rf,
    viewport,
    nodes,
    edges,
    setHistorySnapshot,
    resetRunStatus,
    setNodeRunStatus,
    setActiveOutputHandle,
    resetActiveOutputHandles,
    beginRuntimeLogPopoverAutoOpen,
    finalizeRuntimeLogPopoversAfterRun,
    setAutoFollow,
    setCurrentRunningNodeId,
    lastInputs: lastInputs as unknown as Record<string, unknown> | undefined,
    setRunItems,
    setStreamedText,
    setRunSummary,
    setActiveTab,
    setFinalResult,
  });

  const rememberApprovalEventSequence = useCallback((payload: unknown) => {
    const record =
      payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
    const sequence = getApprovalEventSequence(record);
    if (sequence === null) return;
    approvalEventCursorRef.current = Math.max(approvalEventCursorRef.current, sequence);
  }, []);

  const getEventData = useCallback((payload: unknown): Record<string, unknown> => {
    const record =
      payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
    const data = record.data;
    return data && typeof data === 'object' ? (data as Record<string, unknown>) : record;
  }, []);

  const isStaleApprovalResumeEvent = useCallback((payload: unknown) => {
    const record =
      payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
    const sequence = getApprovalEventSequence(record);
    return sequence !== null && sequence <= approvalEventCursorRef.current;
  }, []);

  const toWorkflowRunEvent = useCallback(
    (eventName: string, payload: unknown): WorkflowRunEventEnvelope => {
      const record =
        payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
      if (typeof record.event === 'string') {
        return record;
      }
      return { event: eventName, data: payload };
    },
    []
  );

  const markApprovalPausedNodes = useCallback(
    (nodeIds: string[], payload: unknown) => {
      if (nodeIds.length === 0) return;
      const data = getEventData(payload);
      const reasons = Array.isArray(data.reasons) ? data.reasons : [];
      const titleByNodeId = new Map<string, string>();
      reasons.forEach(reason => {
        if (!reason || typeof reason !== 'object') return;
        const record = reason as Record<string, unknown>;
        const nodeId = typeof record.node_id === 'string' ? record.node_id : '';
        const title = typeof record.node_title === 'string' ? record.node_title : '';
        if (nodeId && title) titleByNodeId.set(nodeId, title);
      });

      nodeIds.forEach(nodeId => setNodeRunStatus(nodeId, 'paused'));
      setCurrentRunningNodeId(null);
      setRunItems(items => {
        const existingIds = new Set(items.map(item => item.nodeId));
        const updated = items.map(item =>
          nodeIds.includes(item.nodeId)
            ? {
                ...item,
                status: 'paused' as const,
              }
            : item
        );
        const missing = nodeIds
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
    },
    [getEventData, setCurrentRunningNodeId, setNodeRunStatus]
  );

  const markQuestionAnswerPausedNodes = useCallback(
    (nodeIds: string[], payload: unknown) => {
      if (nodeIds.length === 0) return;
      const data = getEventData(payload);
      const reasons = Array.isArray(data.reasons) ? data.reasons : [];
      const titleByNodeId = new Map<string, string>();
      reasons.forEach(reason => {
        if (!reason || typeof reason !== 'object') return;
        const record = reason as Record<string, unknown>;
        const nodeId = typeof record.node_id === 'string' ? record.node_id : '';
        const title = typeof record.node_title === 'string' ? record.node_title : '';
        if (nodeId && title) titleByNodeId.set(nodeId, title);
      });

      nodeIds.forEach(nodeId => setNodeRunStatus(nodeId, 'paused'));
      setCurrentRunningNodeId(null);
      setRunItems(items => {
        const existingIds = new Set(items.map(item => item.nodeId));
        const updated = items.map(item =>
          nodeIds.includes(item.nodeId) ? { ...item, status: 'paused' as const } : item
        );
        const missing = nodeIds
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
    },
    [getEventData, setCurrentRunningNodeId, setNodeRunStatus]
  );

  const handleWorkflowFinished = useCallback(
    (payload: unknown) => {
      rememberApprovalEventSequence(payload);
      sseCallbacks.onWorkflowFinished?.(payload);
      const data =
        typeof payload === 'object' && payload && 'data' in (payload as Record<string, unknown>)
          ? ((payload as { data?: unknown }).data as Record<string, unknown> | undefined)
          : (payload as Record<string, unknown> | undefined);
      const status = typeof data?.['status'] === 'string' ? (data['status'] as string) : '';
      if (status === 'failed' || status === 'error') {
        notifyBillingError(data?.['error']);
      }
      cancelWorkflowRunEvents();
      approvalResumeStreamActiveRef.current = false;
      workflowFinishedRef.current = true;
      resetApprovalRuntime();
      setQuestionAnswerPrompt(null);
      setQuestionAnswerSubmitting(false);
      setWorkflowRunId(null);
      setWorkflowConversationId(null);
    },
    [
      cancelWorkflowRunEvents,
      notifyBillingError,
      rememberApprovalEventSequence,
      resetApprovalRuntime,
      sseCallbacks,
    ]
  );

  const handleWorkflowStarted = useCallback(
    (payload: unknown) => {
      rememberApprovalEventSequence(payload);
      sseCallbacks.onWorkflowStarted?.(payload);
      const data = getEventData(payload);
      const runId =
        typeof data.id === 'string'
          ? data.id
          : typeof data.workflow_run_id === 'string'
            ? data.workflow_run_id
            : '';
      const inputs =
        data.inputs && typeof data.inputs === 'object'
          ? (data.inputs as Record<string, unknown>)
          : {};
      const conversationId =
        typeof data.conversation_id === 'string'
          ? data.conversation_id
          : typeof inputs['sys.conversation_id'] === 'string'
            ? inputs['sys.conversation_id']
            : '';
      setWorkflowRunId(runId || null);
      setWorkflowConversationId(conversationId || null);
      workflowFinishedRef.current = false;
      setQuestionAnswerSubmitting(false);
    },
    [getEventData, rememberApprovalEventSequence, sseCallbacks]
  );

  const getWorkflowRunIdFromPayload = useCallback(
    (payload: unknown): string => {
      const data = getEventData(payload);
      return typeof data.id === 'string'
        ? data.id
        : typeof data.workflow_run_id === 'string'
          ? data.workflow_run_id
          : '';
    },
    [getEventData]
  );

  const handleApprovalRequested = useCallback(
    (payload: unknown) => {
      rememberApprovalEventSequence(payload);
      const parsed = parseApprovalRequestedEvent(payload);
      if (!parsed.form && !parsed.token && !parsed.formId && !parsed.nodeId) return;
      dispatchApprovalEvent('approval_requested', payload);
      if (parsed.form?.node_id) {
        const nodeId = parsed.form.node_id;
        setNodeRunStatus(nodeId, 'paused');
        setCurrentRunningNodeId(null);
        setRunItems(items => {
          const exists = items.some(item => item.nodeId === nodeId);
          const updated = items.map(item =>
            item.nodeId === nodeId ? { ...item, status: 'paused' as const } : item
          );
          if (exists) return updated;
          return [
            ...updated,
            {
              nodeId,
              title: parsed.form?.node_title || nodeId,
              nodeType: 'approval',
              status: 'paused' as const,
            },
          ];
        });
      }
      setActiveTab('results');
    },
    [
      dispatchApprovalEvent,
      rememberApprovalEventSequence,
      setCurrentRunningNodeId,
      setNodeRunStatus,
    ]
  );

  const handleApprovalResultFilled = useCallback(
    (payload: unknown) => {
      rememberApprovalEventSequence(payload);
      dispatchApprovalEvent('approval_result_filled', payload);
    },
    [dispatchApprovalEvent, rememberApprovalEventSequence]
  );

  const handleApprovalExpired = useCallback(
    (payload: unknown) => {
      rememberApprovalEventSequence(payload);
      dispatchApprovalEvent('approval_expired', payload);
      toast.info(t('nodes.approval.runtime.expired'));
    },
    [dispatchApprovalEvent, rememberApprovalEventSequence, t]
  );

  const handleQuestionAnswerRequested = useCallback(
    (payload: unknown) => {
      const parsed = parseQuestionAnswerRequestedEvent(payload);
      if (!parsed) return;
      sseCallbacks.onQuestionAnswerRequested?.(payload);
      setQuestionAnswerPrompt({
        question: parsed.question,
        choices: parsed.choices,
        round: parsed.round,
      });
      setQuestionAnswerTranscript(prev => appendQuestionAnswerTranscriptQuestion(prev, parsed));
      setQuestionAnswerSubmitting(false);
      if (parsed.nodeId) {
        markQuestionAnswerPausedNodes([parsed.nodeId], payload);
      }
      setActiveTab('inputs');
    },
    [markQuestionAnswerPausedNodes, sseCallbacks]
  );

  const handleQuestionAnswerSubmitted = useCallback(
    (payload: unknown) => {
      sseCallbacks.onQuestionAnswerSubmitted?.(payload);
      const parsed = parseQuestionAnswerSubmittedEvent(payload);
      if (parsed) {
        setQuestionAnswerTranscript(prev => applyQuestionAnswerTranscriptSubmission(prev, parsed));
      }
      setQuestionAnswerSubmitting(true);
    },
    [sseCallbacks]
  );

  const dispatchWorkflowRunEvent = useCallback(
    (event: WorkflowRunEventEnvelope) => {
      if (isStaleApprovalResumeEvent(event)) return;
      rememberApprovalEventSequence(event);

      switch (event.event) {
        case 'workflow_started':
          handleWorkflowStarted(event);
          break;
        case 'approval_requested':
          handleApprovalRequested(event);
          break;
        case 'approval_result_filled':
          handleApprovalResultFilled(event);
          break;
        case 'approval_expired':
          handleApprovalExpired(event);
          break;
        case 'question_answer_requested':
          handleQuestionAnswerRequested(event);
          break;
        case 'question_answer_submitted':
          handleQuestionAnswerSubmitted(event);
          break;
        case 'workflow_paused': {
          sseCallbacks.onWorkflowPaused?.(event);
          const parsed = parseApprovalPausedEvent(event);
          if (parsed.isApproval) {
            markApprovalPausedNodes(parsed.nodeIds, event);
            dispatchApprovalEvent('workflow_paused', event);
            setActiveTab('results');
            break;
          }
          const qaPaused = parseQuestionAnswerPausedEvent(event);
          if (qaPaused.isQuestionAnswer) {
            if (qaPaused.prompt) handleQuestionAnswerRequested(qaPaused.prompt);
            markQuestionAnswerPausedNodes(qaPaused.nodeIds, event);
            setActiveTab('inputs');
          }
          break;
        }
        case 'node_started':
          setQuestionAnswerSubmitting(false);
          sseCallbacks.onNodeStarted?.(event);
          break;
        case 'node_finished':
          sseCallbacks.onNodeFinished?.(event);
          break;
        case 'text_chunk':
          sseCallbacks.onTextChunk?.(event);
          break;
        case 'text_replace':
          sseCallbacks.onTextReplace?.(event);
          break;
        case 'message':
        case 'data':
          sseCallbacks.onMessage?.(event);
          break;
        case 'message_end':
          sseCallbacks.onMessageEnd?.(event);
          break;
        case 'workflow_finished':
        case 'workflow_stopped':
        case 'workflow_failed':
        case 'workflow_succeeded':
        case 'workflow_completed':
          handleWorkflowFinished(event);
          break;
        case 'error':
          sseCallbacks.onError?.(event);
          break;
        case 'iteration_started':
          sseCallbacks.onIterationStarted?.(event);
          break;
        case 'iteration_next':
          sseCallbacks.onIterationNext?.(event);
          break;
        case 'iteration_completed':
          sseCallbacks.onIterationCompleted?.(event);
          break;
        case 'loop_started':
          sseCallbacks.onLoopStarted?.(event);
          break;
        case 'loop_next':
          sseCallbacks.onLoopNext?.(event);
          break;
        case 'loop_completed':
          sseCallbacks.onLoopCompleted?.(event);
          break;
      }
    },
    [
      dispatchApprovalEvent,
      handleApprovalExpired,
      handleApprovalRequested,
      handleApprovalResultFilled,
      handleQuestionAnswerRequested,
      handleQuestionAnswerSubmitted,
      handleWorkflowFinished,
      handleWorkflowStarted,
      isStaleApprovalResumeEvent,
      markApprovalPausedNodes,
      markQuestionAnswerPausedNodes,
      rememberApprovalEventSequence,
      sseCallbacks,
    ]
  );

  const startApprovalResumeEventStream = useCallback(
    (payload?: unknown) => {
      const runId = workflowRunId || (payload ? getWorkflowRunIdFromPayload(payload) : '');
      if (!runId) return;
      setWorkflowRunId(runId);
      approvalResumeStreamActiveRef.current = true;
      workflowFinishedRef.current = false;
      const streamParams =
        approvalEventCursorRef.current > 0
          ? { after: approvalEventCursorRef.current, continue_on_pause: true }
          : { include_snapshot: true, continue_on_pause: true };
      void startWorkflowRunEvents(
        runId,
        {
          onWorkflowStarted: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('workflow_started', streamPayload)),
          onApprovalRequested: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('approval_requested', streamPayload)),
          onApprovalResultFilled: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('approval_result_filled', streamPayload)),
          onApprovalExpired: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('approval_expired', streamPayload)),
          onQuestionAnswerRequested: streamPayload =>
            dispatchWorkflowRunEvent(
              toWorkflowRunEvent('question_answer_requested', streamPayload)
            ),
          onQuestionAnswerSubmitted: streamPayload =>
            dispatchWorkflowRunEvent(
              toWorkflowRunEvent('question_answer_submitted', streamPayload)
            ),
          onWorkflowPaused: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('workflow_paused', streamPayload)),
          onNodeStarted: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('node_started', streamPayload)),
          onNodeFinished: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('node_finished', streamPayload)),
          onTextChunk: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('text_chunk', streamPayload)),
          onTextReplace: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('text_replace', streamPayload)),
          onMessage: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('message', streamPayload)),
          onMessageEnd: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('message_end', streamPayload)),
          onWorkflowFinished: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('workflow_finished', streamPayload)),
          onError: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('error', streamPayload)),
          onIterationStarted: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('iteration_started', streamPayload)),
          onIterationNext: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('iteration_next', streamPayload)),
          onIterationCompleted: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('iteration_completed', streamPayload)),
          onLoopStarted: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('loop_started', streamPayload)),
          onLoopNext: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('loop_next', streamPayload)),
          onLoopCompleted: streamPayload =>
            dispatchWorkflowRunEvent(toWorkflowRunEvent('loop_completed', streamPayload)),
        },
        streamParams,
        {
          onClose: () => {
            approvalResumeStreamActiveRef.current = false;
            if (!workflowFinishedRef.current) {
              window.setTimeout(() => {
                if (!workflowFinishedRef.current) startApprovalResumeEventStream();
              }, 1000);
            }
          },
        }
      );
    },
    [
      dispatchWorkflowRunEvent,
      getWorkflowRunIdFromPayload,
      startWorkflowRunEvents,
      toWorkflowRunEvent,
      workflowRunId,
    ]
  );

  useEffect(() => {
    if (!approvalToken || !approvalSubmittedAction) return;
    let cancelled = false;

    const pollApprovalResumeEvents = async () => {
      try {
        const events = await fetchApprovalEvents(approvalToken, {
          after: approvalEventCursorRef.current,
          limit: 100,
        });
        if (cancelled || events.length === 0) return;
        events.forEach(event => {
          dispatchWorkflowRunEvent(event);
        });
      } catch {
        // Keep waiting for the SSE stream; polling is only a resume safety net.
      }
    };

    void pollApprovalResumeEvents();
    const timer = window.setInterval(pollApprovalResumeEvents, 2000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [approvalSubmittedAction, approvalToken, dispatchWorkflowRunEvent]);

  const handleWorkflowPaused = useCallback(
    (payload: unknown) => {
      rememberApprovalEventSequence(payload);
      sseCallbacks.onWorkflowPaused?.(payload);
      const parsed = parseApprovalPausedEvent(payload);
      if (parsed.isApproval) {
        markApprovalPausedNodes(parsed.nodeIds, payload);
        dispatchApprovalEvent('workflow_paused', payload);
        setActiveTab('results');
      } else {
        const qaPaused = parseQuestionAnswerPausedEvent(payload);
        if (!qaPaused.isQuestionAnswer) return;
        if (qaPaused.prompt) handleQuestionAnswerRequested(qaPaused.prompt);
        markQuestionAnswerPausedNodes(qaPaused.nodeIds, payload);
        setActiveTab('inputs');
      }
      if (!approvalResumeStreamActiveRef.current) {
        startApprovalResumeEventStream(payload);
      }
    },
    [
      dispatchApprovalEvent,
      handleQuestionAnswerRequested,
      markApprovalPausedNodes,
      markQuestionAnswerPausedNodes,
      rememberApprovalEventSequence,
      sseCallbacks,
      startApprovalResumeEventStream,
    ]
  );

  useEffect(() => {
    if (approvalFormQuery.data) {
      setLoadedApprovalForm(approvalFormQuery.data);
    }
  }, [approvalFormQuery.data, setLoadedApprovalForm]);

  const handleApprovalSubmit = useCallback(
    async (payload: { inputs: Record<string, unknown>; action: string }) => {
      if (!approvalToken || !approvalEntry) return;
      setApprovalSubmitting(approvalEntry.key, payload.action);
      if (!approvalResumeStreamActiveRef.current) {
        startApprovalResumeEventStream();
      }
      try {
        await approvalSubmitMutation.mutateAsync(payload);
        setApprovalSubmitted(approvalEntry.key, payload.action);
      } catch (error) {
        setApprovalWaiting(approvalEntry.key);
        toast.error(
          error instanceof Error ? error.message : t('nodes.approval.runtime.submitFailed')
        );
      }
    },
    [
      approvalEntry,
      approvalSubmitMutation,
      approvalToken,
      setApprovalSubmitted,
      setApprovalSubmitting,
      setApprovalWaiting,
      startApprovalResumeEventStream,
      t,
    ]
  );

  const saveWorkflowDraft = useSaveWorkflowDraft();
  const { errors } = useWorkflowValidation();

  const persistDraftBeforeRun = useCallback(async () => {
    flushWorkflowPendingEdits();
    const { nodes, edges, viewport, workflowData } = useWorkflowStore.getState();
    const updatedWorkflowData = {
      ...workflowData,
      graph: { nodes, edges, viewport },
      hash: Date.now().toString(),
    } as typeof workflowData;
    const prevFlags = useWorkflowStore.getState();
    useWorkflowStore.setState({
      isDirty: false,
      hasLayoutChanges: false,
      lastSavedAt: Date.now(),
    });
    try {
      await saveWorkflowDraft.mutateAsync({
        agentId,
        workflowData: updatedWorkflowData,
        silent: true,
      });
    } catch (e) {
      useWorkflowStore.setState({
        isDirty: prevFlags.isDirty,
        hasLayoutChanges: prevFlags.hasLayoutChanges,
        lastSavedAt: prevFlags.lastSavedAt ?? null,
      });
      throw e;
    }
  }, [agentId, saveWorkflowDraft]);

  // Stop workflow mutation
  const stopWorkflowMutation = useStopWorkflowTask();
  const markRunningItemsStopped = useCallback(() => {
    runItems.forEach(item => {
      if (item.status === 'running') {
        setNodeRunStatus(item.nodeId, 'stopped');
      }
    });
    setCurrentRunningNodeId(null);
    setRunItems(items =>
      items.map(item => (item.status === 'running' ? { ...item, status: 'stopped' } : item))
    );
  }, [runItems, setCurrentRunningNodeId, setNodeRunStatus]);

  const handleStop = useCallback(() => {
    if (hasBlockingApprovalStop()) {
      toast.info(t('nodes.approval.runtime.stopDisabled'));
      return;
    }
    const activeRunId = workflowRunId || runSummary?.id;
    if (!activeRunId) return;
    stopWorkflowMutation.mutate(
      { agentId, workflowRunId: activeRunId },
      {
        onSuccess: () => {
          markRunningItemsStopped();
          handleWorkflowFinished({
            data: {
              id: activeRunId,
              workflow_run_id: activeRunId,
              status: 'stopped',
              elapsed_time: runSummary?.elapsed_time,
              total_steps: runSummary?.total_steps,
              outputs: runSummary?.outputs,
            },
          });
          cancelStreamRef.current();
          cancelWorkflowRunEvents();
          approvalResumeStreamActiveRef.current = false;
        },
      }
    );
  }, [
    agentId,
    cancelWorkflowRunEvents,
    hasBlockingApprovalStop,
    handleWorkflowFinished,
    markRunningItemsStopped,
    runSummary?.elapsed_time,
    runSummary?.id,
    runSummary?.outputs,
    runSummary?.total_steps,
    stopWorkflowMutation,
    t,
    workflowRunId,
  ]);
  const setOpenValidationIssues = useWorkflowStore.use.setOpenValidationIssues();
  const openIssues = useCallback(() => setOpenValidationIssues(true), [setOpenValidationIssues]);
  const [runWarnOpen, setRunWarnOpen] = useState(false);
  const [dontWarnAgain, setDontWarnAgain] = useState(false);
  // Store pending inputs when warning dialog is shown, to execute after user confirms
  const pendingInputsRef = useRef<FormInputs | null>(null);

  // stream hook encapsulates SSE, lifecycle and cache invalidation
  const {
    start,
    cancel,
    isStarting: isHookStarting,
    isRunning,
  } = useRunWorkflowDraftStream(agentId, {
    enabled: !isHistory,
    onStarted: handleWorkflowStarted,
    onPaused: handleWorkflowPaused,
    onApprovalRequested: handleApprovalRequested,
    onApprovalResultFilled: handleApprovalResultFilled,
    onApprovalExpired: handleApprovalExpired,
    onQuestionAnswerRequested: handleQuestionAnswerRequested,
    onQuestionAnswerSubmitted: handleQuestionAnswerSubmitted,
    onNodeStarted: sseCallbacks.onNodeStarted,
    onNodeFinished: sseCallbacks.onNodeFinished,
    onError: sseCallbacks.onError,
    onTextChunk: sseCallbacks.onTextChunk,
    onFinished: handleWorkflowFinished,
    onIterationStarted: sseCallbacks.onIterationStarted,
    onIterationNext: sseCallbacks.onIterationNext,
    onIterationCompleted: sseCallbacks.onIterationCompleted,
    onLoopStarted: sseCallbacks.onLoopStarted,
    onLoopNext: sseCallbacks.onLoopNext,
    onLoopCompleted: sseCallbacks.onLoopCompleted,
  });
  useEffect(() => {
    setIsStarting(isHookStarting);
  }, [isHookStarting]);
  useEffect(() => {
    cancelStreamRef.current = cancel;
  }, [cancel]);

  const runDraftWithPrecheck = useCallback(
    async (values: FormInputs) => {
      try {
        if (isHistory) return;

        const isQuestionAnswerResume = Boolean(questionAnswerPrompt);
        if (!isQuestionAnswerResume) {
          await persistDraftBeforeRun();
          resetApprovalRuntime();
          setQuestionAnswerPrompt(null);
          setQuestionAnswerSubmitting(false);
          setQuestionAnswerTranscript([]);
          approvalEventCursorRef.current = 0;
          setWorkflowRunId(null);
          setWorkflowConversationId(null);
          workflowFinishedRef.current = false;
          cancelWorkflowRunEvents();
          approvalResumeStreamActiveRef.current = false;
        } else {
          setQuestionAnswerSubmitting(true);
        }

        const payload = { inputs: values as unknown as WorkflowRunInputValues };
        if (!isQuestionAnswerResume) {
          const precheck = await workflowDraftPrecheck.mutateAsync(payload);
          const warnings = getWorkflowPrecheckWarnings(precheck);
          if (precheck.status === 'warning' && warnings.length > 0) {
            setPrecheckWarnings(warnings);
          } else {
            setPrecheckWarnings([]);
          }
        }

        setLastInputs(values);
        setLastDebugInputs({ ...(values as unknown as Record<string, unknown>) });
        start(payload);
      } catch (error) {
        if (!notifyBillingError(error)) {
          toast.error(
            error instanceof Error ? error.message : t('agents.workflow.errors.executionFailed')
          );
        }
        if (questionAnswerPrompt) {
          setQuestionAnswerSubmitting(false);
        }
      }
    },
    [
      cancelWorkflowRunEvents,
      isHistory,
      persistDraftBeforeRun,
      resetApprovalRuntime,
      questionAnswerPrompt,
      notifyBillingError,
      setLastDebugInputs,
      start,
      t,
      workflowDraftPrecheck,
    ]
  );

  // Ensure we close SSE when panel gets hidden, component unmounts, or entering history mode
  useEffect(() => {
    if (!open || isHistory) {
      cancel();
      cancelWorkflowRunEvents();
      approvalResumeStreamActiveRef.current = false;
    }
    return () => {
      cancel();
      cancelWorkflowRunEvents();
      approvalResumeStreamActiveRef.current = false;
    };
  }, [open, isHistory, cancel, cancelWorkflowRunEvents]);

  useEffect(() => {
    const win = window as Window & {
      __workflowRunPanelOpen?: boolean;
      __workflowRunPanelShake?: () => void;
    };
    win.__workflowRunPanelOpen = open;
    win.__workflowRunPanelShake = () => {
      setShake(true);
      window.setTimeout(() => setShake(false), 600);
    };
    return () => {
      win.__workflowRunPanelOpen = false;
      win.__workflowRunPanelShake = undefined as unknown as () => void;
    };
  }, [open]);

  // Force Details tab when in history mode
  useEffect(() => {
    if (isHistory) setActiveTab('details');
  }, [isHistory]);

  const onSubmit = useCallback(
    async (values: FormInputs) => {
      if (errors.length > 0 && !isBannerHidden(BannerKey.WorkflowRunErrorsWarning)) {
        pendingInputsRef.current = values;
        setRunWarnOpen(true);
        return;
      }
      await runDraftWithPrecheck(values);
    },
    [errors.length, runDraftWithPrecheck]
  );

  const handleRunNoInputs = useCallback(async () => {
    if (errors.length > 0 && !isBannerHidden(BannerKey.WorkflowRunErrorsWarning)) {
      pendingInputsRef.current = {};
      setRunWarnOpen(true);
      return;
    }
    await runDraftWithPrecheck({});
  }, [errors.length, runDraftWithPrecheck]);

  const handleQuestionAnswerSelect = useCallback(
    (choice: QuestionAnswerChoice) => {
      const query = getQuestionAnswerChoiceQuery(choice);
      if (!query || questionAnswerSubmitting) return;
      setQuestionAnswerSubmitting(true);
      void runDraftWithPrecheck({
        ...(lastInputs ?? {}),
        query,
        'sys.query': query,
        ...(workflowConversationId
          ? {
              conversation_id: workflowConversationId,
              'sys.conversation_id': workflowConversationId,
            }
          : {}),
        ...(workflowRunId ? { 'sys.workflow_run_id': workflowRunId } : {}),
        question_answer_option_id: choice.id,
      } as FormInputs);
    },
    [
      lastInputs,
      questionAnswerSubmitting,
      runDraftWithPrecheck,
      workflowConversationId,
      workflowRunId,
    ]
  );

  // history derivations moved to useHistoryView
  const runSummaryStatus = (runSummary?.status || '').toLowerCase();
  const isPaused = !isHistory && runSummaryStatus === 'paused';

  // After all hooks are called, conditionally render nothing when closed
  if (!open) {
    return null;
  }

  return (
    <Panel
      position="top-right"
      aria-hidden={temporarilyHidden}
      className={getRightPanelMotionClassName(
        `relative p-0 bg-primary-foreground border border-muted rounded-lg shadow-lg h-[calc(100%-120px)] overflow-hidden ${isResizing ? 'select-none' : ''} ${shake ? 'workflow-panel-attention' : ''}`,
        temporarilyHidden
      )}
      style={{
        ...getRightPanelMotionStyle(panelStyle, temporarilyHidden),
        ...panelWidthStyle,
      }}
    >
      {!isHistory && (
        <div
          aria-hidden="true"
          className="absolute left-0 top-0 z-20 h-full w-2 cursor-ew-resize transition-colors hover:bg-primary/10"
          {...resizeHandleProps}
        />
      )}
      <div className="flex flex-col h-full">
        {/* Header */}
        <Header
          title={isHistory ? t('agents.workflow.runDetails') : t('agents.workflow.debugTitle')}
          showClose={!isHistory}
          closeLabel={t('common.close')}
          onClose={onClose}
          actions={
            <WorkflowRunsDropdown
              agentId={agentId}
              query={debugRunsQuery}
              icon={<History size={14} />}
              tooltipLabel={t('agents.workflow.debugRuns')}
              dropdownLabel={t('agents.workflow.debugRuns')}
              triggerText={t('agents.workflow.debugRuns')}
              triggerVariant="outline"
              triggerSize="xs"
              triggerClassName="h-7"
              refreshOnOpen
              onSelect={handleSelectDebugRun}
            />
          }
        />

        {/* Run Status Bar */}
        {isRunning && !isHistory && (
          <div className="flex items-center px-4 py-2 bg-highlight/10 border-b border-highlight/30">
            <div className="flex items-center gap-2">
              <Loader className="h-4 w-4 animate-spin text-highlight" />
              <span className="text-sm font-medium text-highlight">
                {t('agents.workflow.running')}
              </span>
            </div>
          </div>
        )}
        {!isRunning && isPaused ? (
          <div className="flex items-center justify-between px-4 py-2 bg-warning/10 border-b border-warning/30">
            <div className="flex items-center gap-2">
              <div className="flex h-4 w-4 items-center justify-center rounded-full bg-warning text-white">
                <Pause className="h-3 w-3" />
              </div>
              <span className="text-sm font-medium text-warning">
                {t('agents.workflow.paused')}
              </span>
            </div>
          </div>
        ) : null}

        {/* Content */}
        <div className="h-0 grow overflow-hidden flex flex-col">
          {isHistory ? (
            <HistoryContent
              activeTab={activeTab}
              setActiveTab={v => setActiveTab(v)}
              loading={isHistoryLoading}
              summary={historyRunSummary}
              items={historyExecutionItems}
              result={historyResult as HistoryResult}
            />
          ) : (
            <DraftContent
              activeTab={draftActiveTab}
              setActiveTab={tab => setActiveTab(tab)}
              isLoadingDraft={isLoadingDraft}
              hasLocalNodes={(nodes?.length ?? 0) > 0}
              startVariables={startVariables}
              initialValues={lastInputs}
              isStarting={isStarting}
              isRunning={isRunning}
              isStopping={stopWorkflowMutation.isPending}
              stopDisabled={isApprovalStopBlocked}
              stopDisabledMessage={t('nodes.approval.runtime.stopDisabled')}
              onSubmit={onSubmit}
              onRunNoInputs={handleRunNoInputs}
              onInputChange={setLastInputs}
              onStop={handleStop}
              debugSetupHints={debugSetupHints}
              inputTopNotice={
                precheckWarnings.length > 0 ? (
                  <WorkflowPrecheckWarningBanner
                    warnings={precheckWarnings}
                    scope="agents"
                    storageKey={`workflow-run-panel:${agentId}`}
                    placement="inline"
                  />
                ) : null
              }
              runItems={runItems}
              runSummary={runSummary}
              streamedText={streamedText}
              finalResult={finalResult}
              approvalForm={approvalForm}
              approvalLoading={Boolean(
                approvalToken &&
                  !approvalForm &&
                  (approvalFormQuery.isLoading || approvalFormQuery.isFetching)
              )}
              approvalError={approvalFormQuery.error}
              approvalExpired={approvalExpired}
              onApprovalRetry={() => void approvalFormQuery.refetch()}
              approvalSubmitting={approvalSubmitMutation.isPending || approvalRuntimeSubmitting}
              approvalSubmittedAction={approvalSubmittedAction}
              onApprovalSubmit={handleApprovalSubmit}
              questionAnswerPrompt={questionAnswerPrompt}
              questionAnswerSubmitting={questionAnswerSubmitting}
              onQuestionAnswerSelect={handleQuestionAnswerSelect}
              questionAnswerTranscript={questionAnswerTranscript}
            />
          )}
          <Dialog open={runWarnOpen} onOpenChange={setRunWarnOpen}>
            <DialogContent className="max-w-[440px] p-0 overflow-hidden">
              <DialogHeader className="pb-2">
                <DialogTitle className="text-xl font-black tracking-tight flex items-center gap-3">
                  <div className="h-8 w-8 bg-amber-100 text-amber-500 flex items-center justify-center rounded-lg">
                    <span className="text-lg font-black">!</span>
                  </div>
                  {t('agents.workflow.runErrorsDialog.title')}
                </DialogTitle>
              </DialogHeader>

              <DialogBody className="py-6 space-y-6">
                <div className="bg-amber-50/50 p-4 rounded-2xl border border-amber-100 text-sm font-medium leading-relaxed text-neutral-600">
                  {t('agents.workflow.runErrorsDialog.description')}
                </div>

                <div
                  className="flex items-center gap-3 px-1 group cursor-pointer"
                  onClick={() => setDontWarnAgain(!dontWarnAgain)}
                >
                  <Checkbox
                    id="wf-run-warn-hide-panel"
                    checked={dontWarnAgain}
                    onCheckedChange={v => setDontWarnAgain(Boolean(v))}
                    className="w-5 h-5"
                  />
                  <Label
                    htmlFor="wf-run-warn-hide-panel"
                    className="text-sm font-bold text-neutral-500 group-hover:text-primary transition-colors cursor-pointer"
                  >
                    {t('agents.workflow.runErrorsDialog.dontShowAgain')}
                  </Label>
                </div>
              </DialogBody>

              <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t font-medium">
                <Button
                  variant="ghost"
                  className="font-semibold"
                  onClick={() => {
                    setRunWarnOpen(false);
                    openIssues();
                  }}
                >
                  {t('agents.workflow.runErrorsDialog.viewErrors')}
                </Button>
                <Button
                  size="lg"
                  className="px-10 font-bold shadow-sm"
                  onClick={() => {
                    if (dontWarnAgain) hideBanner(BannerKey.WorkflowRunErrorsWarning);
                    setRunWarnOpen(false);
                    // Execute with pending inputs after closing dialog
                    const pending = pendingInputsRef.current;
                    pendingInputsRef.current = null;
                    if (pending !== null) {
                      void runDraftWithPrecheck(pending);
                    }
                  }}
                >
                  {t('agents.workflow.runErrorsDialog.continueRun')}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      </div>
    </Panel>
  );
};

export default WorkflowRunPanel;
