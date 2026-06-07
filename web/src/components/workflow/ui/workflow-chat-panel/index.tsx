import React, { useCallback, useMemo, useRef, useState, useEffect } from 'react';
import { Panel, useReactFlow } from '@xyflow/react';
import { usePanelStackItem } from '../../hooks';
import Chat, { useChatApi, useChatStore } from '@/components/chat';
import {
  useApprovalForm,
  useSaveWorkflowDraft,
  useSubmitApprovalForm,
  useWorkflowRunEventsStream,
} from '@/hooks';
import { useRunWorkflowChatDraftStream } from '@/hooks/workflow/use-run-workflow-chat-draft-stream';
import { Clock3, History, Loader2, RotateCcw, Send, SlidersHorizontal, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useWorkflowStore } from '@/components/workflow/store';
import { initialWorkflowData } from '@/components/workflow/store/initial-data';
import { useSseCallbacks } from '../workflow-run-panel/hooks/use-sse-callbacks';
import type { ChatAttachment } from '@/components/chat/types';
import type { WorkflowFeatures } from '@/components/workflow/store/type';
import { useT } from '@/i18n';
import useWorkflowValidation from '../../hooks/use-workflow-validation';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Checkbox } from '@/components/ui/checkbox';
import { isBannerHidden, hideBanner, BannerKey } from '@/utils/ui-local';
// Form is now managed internally by UserInput; no direct import/render here
import type { InputVar } from '@/components/workflow/types/input-var';
import { WorkflowPrecheckWarningBanner } from '@/components/workflow/common/workflow-precheck-warning';
import type { QuestionAnswerChoice, WorkflowPrecheckWarning } from '@/services/types/workflow';
import { useWorkflowChatDraftPrecheck } from '@/hooks/workflow/use-workflow-precheck';
import { getWorkflowPrecheckWarnings } from '@/utils/workflow/billing';
import { useWorkflowBillingFeedback } from '@/hooks/workflow/use-workflow-billing-feedback';
import {
  extractLlmGatewayRequest,
  extractWorkflowRunContainerContext,
  getWorkflowRunCreatedAtMs,
  getWorkflowRunExecutionId,
  getWorkflowRunItemKey,
  getWorkflowRunRoundElapsedTime,
  sortWorkflowRunItems,
  sortWorkflowRunRounds,
} from '@/utils/workflow/run-events';
import { toast } from 'sonner';
import { getEffectiveAllowedFileExtensions } from '@/utils/file-helpers';
import { getOpeningGuide } from '@/utils/webapp/opening-statement';
import { ApprovalCompletedState } from '@/components/workflow/approval/approval-completed-state';
import ApprovalRuntimeForm from '@/components/workflow/approval/approval-runtime-form';
import {
  getApprovalEventSequence,
  parseApprovalRequestedEvent,
  parseApprovalPausedEvent,
} from '@/components/workflow/approval/runtime-events';
import { isApprovalFormAlreadySubmittedError } from '@/services/approval.service';
import { flushWorkflowPendingEdits } from '@/components/workflow/hooks/pending-edits';
import {
  hasUnresolvedApprovalEntries,
  useApprovalRuntimeEvents,
} from '@/components/workflow/approval/use-approval-runtime-events';
import { getRightPanelMotionClassName, getRightPanelMotionStyle } from '../right-panel-motion';
import {
  appendQuestionAnswerTranscriptQuestion,
  applyQuestionAnswerTranscriptSubmission,
  isQuestionAnswerPromptMessage,
  parseQuestionAnswerPausedEvent,
  parseQuestionAnswerRequestedEvent,
  parseQuestionAnswerSubmittedEvent,
  type QuestionAnswerTranscriptItem,
} from '@/components/workflow/question-answer/runtime-events';
import {
  getQuestionAnswerChoiceQuery,
  QuestionAnswerRuntimePrompt,
} from '@/components/workflow/question-answer/question-answer-runtime-prompt';
import { useResizableRightPanel } from '../use-resizable-right-panel';
import { SUGGESTED_QUESTIONS_LIMIT } from '@/constants/suggested-questions';
import WorkflowRunsDropdown from '../workflow-runs-dropdown';
import { useActivePanel } from '../../hooks/use-active-panel';
import type { WorkflowRunNodeListItem } from '../workflow-run-nodes-list';
import { generateClientId } from '@/utils/client-id';
import { buildOpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';

interface WorkflowChatPanelProps {
  open: boolean;
  temporarilyHidden?: boolean;
  onClose: () => void;
  agentId: string;
  agentName?: string;
  agentIconType?: string;
  agentIcon?: string;
  agentIconUrl?: string;
}

function ApprovalWaitingState({
  loading = false,
  submitted = false,
}: {
  loading?: boolean;
  submitted?: boolean;
}) {
  const t = useT();
  const Icon = loading ? Loader2 : Send;

  return (
    <div className="relative overflow-hidden rounded-xl border bg-card px-5 py-5 text-center shadow-sm">
      <div className="mx-auto flex size-11 items-center justify-center rounded-full bg-amber-500/10 text-amber-600 ring-1 ring-amber-500/20">
        <Icon className={loading ? 'size-5 animate-spin' : 'size-5'} />
      </div>
      <div className="mt-3 text-sm font-semibold text-foreground">
        {submitted
          ? t('nodes.approval.runtime.submitted')
          : loading
            ? t('nodes.approval.runtime.paused')
            : t('nodes.approval.runtime.requestSubmitted')}
      </div>
      <p className="mx-auto mt-1.5 max-w-md text-xs leading-5 text-muted-foreground">
        {submitted
          ? t('nodes.approval.runtime.waitingResume')
          : t('nodes.approval.runtime.waitingForReviewer')}
      </p>
      <div className="mt-3 inline-flex items-center gap-1.5 rounded-full border bg-muted/40 px-3 py-1 text-xs text-muted-foreground">
        <Clock3 className="size-3.5" />
        <span>{t('nodes.approval.runtime.waitingForReviewerStatus')}</span>
      </div>
    </div>
  );
}

const WorkflowChatPanel: React.FC<WorkflowChatPanelProps> = ({
  open,
  temporarilyHidden = false,
  onClose,
  agentId,
  agentName,
  agentIconType,
  agentIcon,
  agentIconUrl,
}) => {
  const t = useT();
  const workflowChatPrecheck = useWorkflowChatDraftPrecheck(agentId);
  const setActivePanel = useActivePanel(state => state.setActive);
  const saveWorkflowDraft = useSaveWorkflowDraft();
  const { getWorkflowRunErrorText, notifyBillingError } = useWorkflowBillingFeedback('agents');
  const [convId, setConvId] = useState(() => generateClientId('conversation'));
  const [shake, setShake] = useState(false);
  const { panelWidth, isResizing, panelWidthStyle, resizeHandleProps } = useResizableRightPanel({
    cssVar: '--workflow-chat-panel-w',
    defaultWidth: 460,
    minWidth: 380,
    maxWidth: 620,
    maxViewportRatio: 0.52,
  });
  const openingGuideBrand = useMemo(
    () =>
      buildOpeningGuideBrand({
        title: agentName,
        iconType: agentIconType,
        icon: agentIcon,
        iconUrl: agentIconUrl,
      }),
    [agentIcon, agentIconType, agentIconUrl, agentName]
  );
  const [precheckWarnings, setPrecheckWarnings] = useState<WorkflowPrecheckWarning[]>([]);
  const chatConv = useChatStore.use.getConversation()(convId);
  const initSingle = useChatStore.use.initSingle();
  const updateConversation = useChatStore.use.updateConversation();
  const { onAgentRun } = useChatApi();
  const runnerRef = useRef<ReturnType<typeof onAgentRun> | null>(null);
  const lastMessageIdRef = useRef<string | undefined>(undefined);
  // Remember last submitted inputs to repopulate form on re-render
  const [lastInputs, setLastInputs] = useState<Record<string, unknown>>({});
  const [workflowRunId, setWorkflowRunId] = useState<string | null>(null);
  const [isConversationPaused, setIsConversationPaused] = useState(false);
  const [questionAnswerPrompt, setQuestionAnswerPrompt] = useState<{
    question: string;
    choices: QuestionAnswerChoice[];
    round?: number;
  } | null>(null);
  const [questionAnswerSubmitting, setQuestionAnswerSubmitting] = useState(false);
  const [, setQuestionAnswerTranscript] = useState<QuestionAnswerTranscriptItem[]>([]);
  const updateQuestionAnswerTranscript = useCallback(
    (updater: (items: QuestionAnswerTranscriptItem[]) => QuestionAnswerTranscriptItem[]) => {
      setQuestionAnswerTranscript(prev => {
        const next = updater(prev);
        runnerRef.current?.mergeMessageData?.({ questionAnswerTranscript: next });
        return next;
      });
    },
    []
  );

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
  const approvalEventCursorRef = useRef(0);
  const approvalRuntimeStateRef = useRef(approvalRuntimeState);
  const { start: startWorkflowRunEvents, cancel: cancelWorkflowRunEvents } =
    useWorkflowRunEventsStream();
  const approvalResumeStreamActiveRef = useRef(false);
  const workflowFinishedRef = useRef(false);
  const startApprovalResumeEventStreamRef = useRef<(payload?: unknown) => void>(() => {});
  const approvalFormQuery = useApprovalForm(approvalToken, Boolean(approvalToken && !approvalForm));
  const approvalSubmitMutation = useSubmitApprovalForm(approvalToken);

  useEffect(() => {
    approvalRuntimeStateRef.current = approvalRuntimeState;
  }, [approvalRuntimeState]);

  // Wire up workflow store callbacks to mirror normal run panel behavior
  const rf = useReactFlow();
  const nodes = useWorkflowStore.use.nodes();
  const edges = useWorkflowStore.use.edges();
  const viewport = useWorkflowStore.use.viewport();
  const draft = useWorkflowStore.use.workflowData();
  const setHistorySnapshot = useWorkflowStore.use.setHistorySnapshot();
  const enterHistoryMode = useWorkflowStore.use.enterHistoryMode();
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
  const setCurrentRunningNodeId = useWorkflowStore.use.setCurrentRunningNodeId();
  const setLastDebugInputs = useWorkflowStore.use.setLastDebugInputs();
  const chatRunItemsRef = useRef<WorkflowRunNodeListItem[]>([]);
  const debugRunsQuery = useMemo(() => ({ triggered_from: 'debugging' as const }), []);
  const handleSelectDebugRun = useCallback(
    (runId: string) => {
      enterHistoryMode(runId);
      setActivePanel('conversation-history');
    },
    [enterHistoryMode, setActivePanel]
  );

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

    // Fallback to server draft only when local vars are truly unavailable (null/undefined)
    if (!Array.isArray(vars)) {
      const dNodes = draft?.graph?.nodes ?? [];
      const start = dNodes.find(
        n =>
          (n?.data as unknown as { type?: string })?.type === 'start' ||
          (n as unknown as { type?: string })?.type === 'start'
      );
      vars = (start?.data as unknown as { variables?: InputVar[] })?.variables;
    }

    return Array.isArray(vars) ? vars : [];
  }, [nodes, draft]);

  // Gating is handled inside UserInput

  const sseCallbacks = useSseCallbacks({
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
    lastInputs: undefined,
    setRunItems: updater => {
      const next =
        typeof updater === 'function'
          ? updater(chatRunItemsRef.current)
          : (updater as WorkflowRunNodeListItem[]);
      chatRunItemsRef.current = next;
      setRuntimeLogItems(next);
    },
    setStreamedText: () => {},
    setRunSummary: () => {},
    setActiveTab: () => {},
    setFinalResult: () => {},
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

  const rememberBackendConversation = useCallback(
    (payload: unknown) => {
      const data = getEventData(payload);
      const conversationId =
        typeof data.conversation_id === 'string' ? data.conversation_id.trim() : '';
      if (conversationId && chatConv?.conversationId !== conversationId) {
        if (!chatConv) {
          initSingle({
            id: convId,
            conversationId: '',
            title: t('agents.workflow.chat.newConversation'),
          });
        }
        updateConversation(convId, { conversationId });
      }
    },
    [chatConv, convId, getEventData, initSingle, t, updateConversation]
  );

  const isStaleApprovalResumeEvent = useCallback((payload: unknown) => {
    const record =
      payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
    const sequence = getApprovalEventSequence(record);
    return sequence !== null && sequence <= approvalEventCursorRef.current;
  }, []);

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

      nodeIds.forEach(nodeId => {
        setNodeRunStatus(nodeId, 'paused');
        runnerRef.current?.onNodeFinished?.({
          status: 'paused',
          nodeId,
          nodeType: 'approval',
          title:
            titleByNodeId.get(nodeId) || (typeof data.title === 'string' ? data.title : nodeId),
          data: { output: data.outputs },
        });
      });
      setCurrentRunningNodeId(null);
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

      nodeIds.forEach(nodeId => {
        setNodeRunStatus(nodeId, 'paused');
        runnerRef.current?.onNodeFinished?.({
          status: 'paused',
          nodeId,
          nodeType: 'question-answer',
          title:
            titleByNodeId.get(nodeId) || (typeof data.title === 'string' ? data.title : nodeId),
          data: { output: data.outputs },
        });
      });
      setCurrentRunningNodeId(null);
    },
    [getEventData, setCurrentRunningNodeId, setNodeRunStatus]
  );

  const handleApprovalRequested = useCallback(
    (payload: unknown) => {
      rememberApprovalEventSequence(payload);
      const parsed = parseApprovalRequestedEvent(payload);
      if (!parsed.form && !parsed.token && !parsed.formId && !parsed.nodeId) return;
      dispatchApprovalRuntimeEvent('approval_requested', payload);
      setIsConversationPaused(true);
      if (parsed.form?.node_id) {
        setNodeRunStatus(parsed.form.node_id, 'paused');
        runnerRef.current?.onNodeFinished?.({
          status: 'paused',
          nodeId: parsed.form.node_id,
          nodeType: 'approval',
          title: parsed.form.node_title || parsed.form.node_id,
        });
        setCurrentRunningNodeId(null);
      }
    },
    [
      dispatchApprovalRuntimeEvent,
      rememberApprovalEventSequence,
      setCurrentRunningNodeId,
      setNodeRunStatus,
    ]
  );

  const handleApprovalResultFilled = useCallback(
    (payload: unknown) => {
      rememberApprovalEventSequence(payload);
      dispatchApprovalRuntimeEvent('approval_result_filled', payload);
      setIsConversationPaused(true);
    },
    [dispatchApprovalRuntimeEvent, rememberApprovalEventSequence]
  );

  const handleApprovalExpired = useCallback(
    (payload: unknown) => {
      rememberApprovalEventSequence(payload);
      dispatchApprovalRuntimeEvent('approval_expired', payload);
      toast.info(t('nodes.approval.runtime.expired'));
    },
    [dispatchApprovalRuntimeEvent, rememberApprovalEventSequence, t]
  );

  const handleQuestionAnswerRequested = useCallback(
    (payload: unknown) => {
      const parsed = parseQuestionAnswerRequestedEvent(payload);
      if (!parsed) return;
      setQuestionAnswerPrompt({
        question: parsed.question,
        choices: parsed.choices,
        round: parsed.round,
      });
      updateQuestionAnswerTranscript(prev => appendQuestionAnswerTranscriptQuestion(prev, parsed));
      setQuestionAnswerSubmitting(false);
      setIsConversationPaused(true);
      sseCallbacks.onQuestionAnswerRequested?.(payload);
      if (parsed.nodeId) {
        markQuestionAnswerPausedNodes([parsed.nodeId], payload);
      }
    },
    [markQuestionAnswerPausedNodes, sseCallbacks, updateQuestionAnswerTranscript]
  );

  const handleQuestionAnswerSubmitted = useCallback(
    (payload: unknown) => {
      const parsed = parseQuestionAnswerSubmittedEvent(payload);
      if (parsed) {
        updateQuestionAnswerTranscript(prev =>
          applyQuestionAnswerTranscriptSubmission(prev, parsed)
        );
      }
      setQuestionAnswerSubmitting(true);
      sseCallbacks.onQuestionAnswerSubmitted?.(payload);
    },
    [sseCallbacks, updateQuestionAnswerTranscript]
  );

  const handleWorkflowPaused = useCallback(
    (payload: unknown) => {
      rememberApprovalEventSequence(payload);
      sseCallbacks.onWorkflowPaused?.(payload);
      const parsed = parseApprovalPausedEvent(payload);
      const data =
        typeof payload === 'object' && payload && 'data' in (payload as Record<string, unknown>)
          ? ((payload as { data?: unknown }).data as Record<string, unknown> | undefined)
          : (payload as Record<string, unknown> | undefined);

      if (parsed.isApproval) {
        dispatchApprovalRuntimeEvent('workflow_paused', payload);
        setIsConversationPaused(true);
        markApprovalPausedNodes(parsed.nodeIds, payload);
        runnerRef.current?.onWorkflowPaused?.({
          elapsedTime: typeof data?.elapsed_time === 'number' ? data.elapsed_time : undefined,
          workflowRunId:
            (typeof data?.id === 'string' ? data.id : '') ||
            (typeof data?.workflow_run_id === 'string' ? data.workflow_run_id : '') ||
            undefined,
          status: 'pending_approval',
          nodeType: 'approval',
        });
      } else {
        const qaPaused = parseQuestionAnswerPausedEvent(payload);
        if (!qaPaused.isQuestionAnswer) return;
        if (qaPaused.prompt) handleQuestionAnswerRequested(qaPaused.prompt);
        setIsConversationPaused(true);
        markQuestionAnswerPausedNodes(qaPaused.nodeIds, payload);
        runnerRef.current?.onWorkflowPaused?.({
          elapsedTime: typeof data?.elapsed_time === 'number' ? data.elapsed_time : undefined,
          workflowRunId: qaPaused.workflowRunId,
          nodeIds: qaPaused.nodeIds,
          status: 'pending_question',
          nodeType: 'question-answer',
        });
      }

      if (!approvalResumeStreamActiveRef.current) {
        startApprovalResumeEventStreamRef.current(payload);
      }
    },
    [
      dispatchApprovalRuntimeEvent,
      handleQuestionAnswerRequested,
      markApprovalPausedNodes,
      markQuestionAnswerPausedNodes,
      rememberApprovalEventSequence,
      sseCallbacks,
    ]
  );

  useEffect(() => {
    if (approvalFormQuery.data) {
      setLoadedApprovalForm(approvalFormQuery.data);
    }
  }, [approvalFormQuery.data, setLoadedApprovalForm]);

  const { start, cancel, stop, isStarting, isRunning, isStopping } = useRunWorkflowChatDraftStream(
    agentId,
    {
      onWorkflowStarted: payload => {
        rememberApprovalEventSequence(payload);
        rememberBackendConversation(payload);
        workflowFinishedRef.current = false;
        iterationSessionsRef.current.clear();
        activeIterationRef.current = { nodeId: null, index: null };
        loopSessionsRef.current.clear();
        activeLoopRef.current = { nodeId: null, index: null };
        receiveOrderRef.current = 0;
        chatRunItemsRef.current = [];
        setRuntimeLogItems([]);
        sseCallbacks.onWorkflowStarted?.(payload);
        const data = getEventData(payload);
        const runId =
          typeof data.id === 'string'
            ? data.id
            : typeof data.workflow_run_id === 'string'
              ? data.workflow_run_id
              : '';
        const isQuestionAnswerResume = Boolean(questionAnswerPrompt);
        setWorkflowRunId(runId || null);
        setIsConversationPaused(false);
        setQuestionAnswerPrompt(null);
        setQuestionAnswerSubmitting(false);
        if (!isQuestionAnswerResume) {
          setQuestionAnswerTranscript([]);
        }
        runnerRef.current?.onWorkflowStarted(payload);
      },
      onApprovalRequested: handleApprovalRequested,
      onApprovalResultFilled: handleApprovalResultFilled,
      onApprovalExpired: handleApprovalExpired,
      onQuestionAnswerRequested: handleQuestionAnswerRequested,
      onQuestionAnswerSubmitted: handleQuestionAnswerSubmitted,
      onNodeStarted: payload => {
        setIsConversationPaused(false);
        try {
          const d = (payload as { data?: unknown })?.['data'] ?? payload;
          const p = typeof d === 'object' && d ? (d as Record<string, unknown>) : {};
          const nodeInfo = {
            status: 'running' as const,
            nodeId: typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined,
            nodeType: typeof p['node_type'] === 'string' ? (p['node_type'] as string) : undefined,
            title: typeof p['title'] === 'string' ? (p['title'] as string) : undefined,
            data: {
              input: p['inputs'],
              output: undefined,
            },
          };
          sseCallbacks.onNodeStarted?.(payload);
          const { loopId, loopIndex, iterationId, iterationIndex } =
            extractWorkflowRunContainerContext(p);
          const executionId = getWorkflowRunExecutionId(p);
          const createdAtMs = getWorkflowRunCreatedAtMs(p);
          const receivedOrder = nextReceiveOrder();
          if (loopId) {
            const sess = loopSessionsRef.current.get(loopId);
            const targetIndex =
              typeof loopIndex === 'number'
                ? loopIndex
                : (sess?.activeIndex ?? activeLoopRef.current.index);
            if (sess && typeof targetIndex === 'number') {
              const rIdx = sess.rounds.findIndex(r => r.index === targetIndex);
              if (rIdx < 0) sess.rounds.push({ index: targetIndex, nodes: [] });
              const round = sess.rounds.find(r => r.index === targetIndex);
              const childKey = getWorkflowRunItemKey({
                executionId,
                nodeId: nodeInfo.nodeId,
                nodeType: nodeInfo.nodeType,
                title: nodeInfo.title,
              });
              if (!round) return;
              const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
              const child = {
                status: 'running' as const,
                nodeId: nodeInfo.nodeId,
                executionId,
                createdAtMs,
                receivedOrder,
                nodeType: nodeInfo.nodeType,
                title: nodeInfo.title,
                data: { input: nodeInfo.data?.input },
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
              loopSessionsRef.current.set(loopId, { ...sess });
              activeLoopRef.current = { nodeId: loopId, index: targetIndex };
              runnerRef.current?.onNodeStarted?.({
                status: 'running',
                nodeId: sess.nodeId,
                nodeType: 'loop',
                title: sess.title,
                loopInputs: sess.inputs,
                loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                  index: r.index,
                  nodes: sortWorkflowRunItems(r.nodes),
                })),
              } as never);
              return;
            }
          }
          const active = activeIterationRef.current;
          const targetIterationId = iterationId ?? active.nodeId;
          const targetIterationIndex =
            typeof iterationIndex === 'number' ? iterationIndex : active.index;
          if (targetIterationId && targetIterationIndex !== null) {
            const sess = iterationSessionsRef.current.get(targetIterationId) ?? {
              nodeId: targetIterationId,
              nodeType: 'iteration',
              title: 'iteration',
              rounds: [],
            };
            const rIdx = sess.rounds.findIndex(r => r.index === targetIterationIndex);
            if (rIdx < 0) sess.rounds.push({ index: targetIterationIndex, nodes: [] });
            const round = sess.rounds.find(r => r.index === targetIterationIndex);
            const childKey = getWorkflowRunItemKey({
              executionId,
              nodeId: nodeInfo.nodeId,
              nodeType: nodeInfo.nodeType,
              title: nodeInfo.title,
            });
            if (!round) return;
            const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
            const child = {
              status: 'running' as const,
              nodeId: nodeInfo.nodeId,
              executionId,
              createdAtMs,
              receivedOrder,
              nodeType: nodeInfo.nodeType,
              title: nodeInfo.title,
              data: { input: nodeInfo.data?.input },
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
            iterationSessionsRef.current.set(targetIterationId, { ...sess });
            runnerRef.current?.onNodeStarted?.({
              status: 'running',
              nodeId: sess.nodeId,
              nodeType: 'iteration',
              title: sess.title,
              iterationInputs: sess.inputs,
              iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                index: r.index,
                nodes: sortWorkflowRunItems(r.nodes),
              })),
            } as never);
          } else {
            runnerRef.current?.onNodeStarted?.(nodeInfo as never);
          }
        } catch {
          /* no-op */
        }
      },
      onNodeFinished: payload => {
        try {
          const d = (payload as { data?: unknown })?.['data'] ?? payload;
          const p = typeof d === 'object' && d ? (d as Record<string, unknown>) : {};
          const statusRaw = typeof p['status'] === 'string' ? (p['status'] as string) : 'running';
          const status = (
            statusRaw === 'failed'
              ? 'failed'
              : statusRaw === 'paused'
                ? 'paused'
                : statusRaw === 'stopped'
                  ? 'stopped'
                  : statusRaw === 'success' || statusRaw === 'succeeded'
                    ? 'succeeded'
                    : 'running'
          ) as 'failed' | 'stopped' | 'succeeded' | 'running' | 'paused';
          const err = getWorkflowRunErrorText(p['error']);
          const nodeInfo = {
            status,
            nodeId: typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined,
            nodeType: typeof p['node_type'] === 'string' ? (p['node_type'] as string) : undefined,
            title: typeof p['title'] === 'string' ? (p['title'] as string) : undefined,
            elapsedTime: typeof p['elapsed_time'] === 'number' ? (p['elapsed_time'] as number) : 0,
            error: err,
            data: {
              input: p['inputs'],
              output: p['outputs'],
              modelInput: extractLlmGatewayRequest(p),
            },
          };
          sseCallbacks.onNodeFinished?.(payload);
          const { loopId, loopIndex, iterationId, iterationIndex } =
            extractWorkflowRunContainerContext(p);
          const executionId = getWorkflowRunExecutionId(p);
          const createdAtMs = getWorkflowRunCreatedAtMs(p);
          const receivedOrder = nextReceiveOrder();
          if (loopId) {
            const sess = loopSessionsRef.current.get(loopId);
            const targetIndex =
              typeof loopIndex === 'number'
                ? loopIndex
                : (sess?.activeIndex ?? activeLoopRef.current.index);
            if (sess && typeof targetIndex === 'number') {
              const rIdx = sess.rounds.findIndex(r => r.index === targetIndex);
              if (rIdx < 0) sess.rounds.push({ index: targetIndex, nodes: [] });
              const round = sess.rounds.find(r => r.index === targetIndex);
              if (!round) return;
              const childKey = getWorkflowRunItemKey({
                executionId,
                nodeId: nodeInfo.nodeId,
                nodeType: nodeInfo.nodeType,
                title: nodeInfo.title,
              });
              const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
              const child = {
                status: nodeInfo.status,
                nodeId: nodeInfo.nodeId,
                executionId,
                createdAtMs,
                receivedOrder,
                nodeType: nodeInfo.nodeType,
                title: nodeInfo.title,
                elapsedTime: nodeInfo.elapsedTime,
                error: nodeInfo.error,
                data: {
                  input: nodeInfo.data?.input,
                  output: nodeInfo.data?.output,
                  modelInput: nodeInfo.data?.modelInput,
                },
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
              loopSessionsRef.current.set(loopId, { ...sess });
              activeLoopRef.current = { nodeId: loopId, index: targetIndex };
              runnerRef.current?.onNodeFinished?.({
                status: 'running',
                nodeId: sess.nodeId,
                nodeType: 'loop',
                title: sess.title,
                loopInputs: sess.inputs,
                loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                  index: r.index,
                  nodes: sortWorkflowRunItems(r.nodes),
                })),
              } as never);
              return;
            }
          }
          const active = activeIterationRef.current;
          const targetIterationId = iterationId ?? active.nodeId;
          const targetIterationIndex =
            typeof iterationIndex === 'number' ? iterationIndex : active.index;
          if (targetIterationId && targetIterationIndex !== null) {
            const sess = iterationSessionsRef.current.get(targetIterationId);
            if (sess) {
              const rIdx = sess.rounds.findIndex(r => r.index === targetIterationIndex);
              if (rIdx < 0) sess.rounds.push({ index: targetIterationIndex, nodes: [] });
              const round = sess.rounds.find(r => r.index === targetIterationIndex);
              if (!round) return;
              const childKey = getWorkflowRunItemKey({
                executionId,
                nodeId: nodeInfo.nodeId,
                nodeType: nodeInfo.nodeType,
                title: nodeInfo.title,
              });
              const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
              const child = {
                status: nodeInfo.status,
                nodeId: nodeInfo.nodeId,
                executionId,
                createdAtMs,
                receivedOrder,
                nodeType: nodeInfo.nodeType,
                title: nodeInfo.title,
                elapsedTime: nodeInfo.elapsedTime,
                error: nodeInfo.error,
                data: {
                  input: nodeInfo.data?.input,
                  output: nodeInfo.data?.output,
                  modelInput: nodeInfo.data?.modelInput,
                },
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
              iterationSessionsRef.current.set(targetIterationId, { ...sess });
              runnerRef.current?.onNodeFinished?.({
                status: 'running',
                nodeId: sess.nodeId,
                nodeType: 'iteration',
                title: sess.title,
                iterationInputs: sess.inputs,
                iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                  index: r.index,
                  nodes: sortWorkflowRunItems(r.nodes),
                })),
              } as never);
            }
          } else {
            runnerRef.current?.onNodeFinished?.(nodeInfo as never);
          }
        } catch {
          /* no-op */
        }
      },
      onMessage: payload => {
        try {
          const data = (payload as { data?: unknown })?.['data'] ?? payload;
          const p = typeof data === 'object' && data ? (data as Record<string, unknown>) : {};
          if (isQuestionAnswerPromptMessage(p)) {
            return;
          }
          const conversation_id =
            typeof p['conversation_id'] === 'string' ? (p['conversation_id'] as string) : '';
          const message_id =
            typeof p['message_id'] === 'string' ? (p['message_id'] as string) : undefined;
          if (conversation_id && chatConv?.conversationId !== conversation_id) {
            updateConversation(convId, { conversationId: conversation_id });
          }
          lastMessageIdRef.current = message_id;

          // Pass the entire metadata object to runner's onMessage
          // The runner will handle throttling and text extraction
          runnerRef.current?.onMessage?.(p);
        } catch {
          // swallow parse errors
        }
      },
      onTextReplace: () => {
        runnerRef.current?.onTextReplace?.();
      },
      onMessageEnd: payload => {
        // finalize the current ai message only if still pending
        runnerRef.current?.onMessageEnd(payload);
      },
      onWorkflowPaused: payload => {
        handleWorkflowPaused(payload);
      },
      onWorkflowFinished: payload => {
        setIsConversationPaused(false);
        workflowFinishedRef.current = true;
        resetApprovalRuntime();
        sseCallbacks.onWorkflowFinished?.(payload);
        try {
          const status =
            payload.status === 'failed'
              ? 'error'
              : payload.status === 'stopped'
                ? 'stopped'
                : 'completed';
          const rawError = (payload as unknown as { error?: unknown }).error;
          runnerRef.current?.onWorkflowFinished({
            status,
            messageId: lastMessageIdRef.current,
            workflowRunId:
              (typeof payload.id === 'string' ? payload.id : '') ||
              (typeof payload.workflow_run_id === 'string' ? payload.workflow_run_id : '') ||
              undefined,
            elapsedTime: payload.elapsed_time,
            error: getWorkflowRunErrorText(rawError),
          });
          if (status === 'error') {
            notifyBillingError(rawError);
          }
        } catch {
          runnerRef.current?.onWorkflowFinished({
            status: 'error',
            messageId: lastMessageIdRef.current,
            workflowRunId:
              (typeof payload.id === 'string' ? payload.id : '') ||
              (typeof payload.workflow_run_id === 'string' ? payload.workflow_run_id : '') ||
              undefined,
            elapsedTime: payload.elapsed_time,
            error: undefined,
          });
        }
      },
      onError: err => {
        setIsConversationPaused(false);
        workflowFinishedRef.current = true;
        resetApprovalRuntime();
        sseCallbacks.onError?.(err);
        runnerRef.current?.onWorkflowFinished({
          status: 'error',
          error: getWorkflowRunErrorText(err) ?? String(err ?? 'Error'),
        });
        notifyBillingError(err);
      },
      onIterationStarted: data => {
        try {
          const p = typeof data === 'object' && data ? (data as Record<string, unknown>) : {};
          const nodeId = typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined;
          const nodeType =
            typeof p['node_type'] === 'string' ? (p['node_type'] as string) : 'iteration';
          const title = typeof p['title'] === 'string' ? (p['title'] as string) : nodeType;
          const inputs = p['inputs'];
          iterationSessionsRef.current.set(nodeId ?? title, {
            nodeId,
            nodeType,
            title,
            inputs,
            rounds: [],
            activeIndex: null,
          });
          activeIterationRef.current = { nodeId: nodeId ?? title, index: null };
          runnerRef.current?.onNodeStarted?.({
            status: 'running',
            nodeId,
            nodeType,
            title,
            iterationInputs: inputs,
            iterationRounds: [],
            data: { input: undefined, output: undefined },
          } as never);
          sseCallbacks.onIterationStarted?.({ data });
        } catch {
          /* no-op */
        }
      },
      onIterationNext: data => {
        try {
          const p = typeof data === 'object' && data ? (data as Record<string, unknown>) : {};
          const nodeId = typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined;
          const nodeType =
            typeof p['node_type'] === 'string' ? (p['node_type'] as string) : 'iteration';
          const title = typeof p['title'] === 'string' ? (p['title'] as string) : nodeType;
          const index = typeof p['index'] === 'number' ? (p['index'] as number) : 0;
          const key = nodeId ?? title;
          const sess = iterationSessionsRef.current.get(key) ?? {
            nodeId,
            nodeType,
            title,
            rounds: [],
          };
          const hasRound = sess.rounds.some(r => r.index === index);
          if (!hasRound) sess.rounds.push({ index, nodes: [] });
          sess.activeIndex = index;
          iterationSessionsRef.current.set(key, sess);
          activeIterationRef.current = { nodeId: key, index };
          runnerRef.current?.onNodeStarted?.({
            status: 'running',
            nodeId,
            nodeType,
            title,
            iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
              index: r.index,
              nodes: sortWorkflowRunItems(r.nodes),
            })),
            data: { input: undefined, output: undefined },
          } as never);
          sseCallbacks.onIterationNext?.({ data });
        } catch {
          /* no-op */
        }
      },
      onIterationCompleted: data => {
        try {
          const p = typeof data === 'object' && data ? (data as Record<string, unknown>) : {};
          const nodeId = typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined;
          const nodeType =
            typeof p['node_type'] === 'string' ? (p['node_type'] as string) : 'iteration';
          const title = typeof p['title'] === 'string' ? (p['title'] as string) : nodeType;
          const elapsed = typeof p['elapsed_time'] === 'number' ? (p['elapsed_time'] as number) : 0;
          const error = typeof p['error'] === 'string' ? (p['error'] as string) : undefined;
          const outputs = p['outputs'];
          const status = error ? 'failed' : 'success';
          const key = nodeId ?? title;
          const sess = iterationSessionsRef.current.get(key) ?? {
            nodeId,
            nodeType,
            title,
            rounds: [],
          };
          sess.elapsedTime = elapsed;
          sess.error = error;
          sess.outputs = outputs;
          sess.rounds = sess.rounds.map(r => ({
            ...r,
            elapsedTime: getWorkflowRunRoundElapsedTime(r),
          }));
          iterationSessionsRef.current.set(key, sess);
          activeIterationRef.current = { nodeId: null, index: null };
          runnerRef.current?.onNodeFinished?.({
            status,
            nodeId,
            nodeType,
            title,
            elapsedTime: elapsed,
            error,
            iterationOutputs: outputs,
            iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
              index: r.index,
              nodes: sortWorkflowRunItems(r.nodes),
              elapsedTime: r.elapsedTime,
            })),
            data: { input: undefined, output: undefined },
          } as never);
          sseCallbacks.onIterationCompleted?.({ data });
        } catch {
          /* no-op */
        }
      },
      onLoopStarted: data => {
        try {
          const p = typeof data === 'object' && data ? (data as Record<string, unknown>) : {};
          const nodeId = typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined;
          const nodeType = typeof p['node_type'] === 'string' ? (p['node_type'] as string) : 'loop';
          const title = typeof p['title'] === 'string' ? (p['title'] as string) : nodeType;
          const inputs = p['inputs'];
          const key = nodeId ?? title;
          loopSessionsRef.current.set(key, {
            nodeId,
            nodeType,
            title,
            inputs,
            rounds: [],
            activeIndex: null,
          });
          activeLoopRef.current = { nodeId: key, index: null };
          runnerRef.current?.onNodeStarted?.({
            status: 'running',
            nodeId,
            nodeType: 'loop',
            title,
            loopInputs: inputs,
            loopRounds: [],
            data: { input: undefined, output: undefined },
          } as never);
          sseCallbacks.onLoopStarted?.({ data });
        } catch {
          /* no-op */
        }
      },
      onLoopNext: data => {
        try {
          const p = typeof data === 'object' && data ? (data as Record<string, unknown>) : {};
          const nodeId = typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined;
          const nodeType = typeof p['node_type'] === 'string' ? (p['node_type'] as string) : 'loop';
          const title = typeof p['title'] === 'string' ? (p['title'] as string) : nodeType;
          const index = typeof p['index'] === 'number' ? (p['index'] as number) : 0;
          const key = nodeId ?? title;
          const sess = loopSessionsRef.current.get(key) ?? {
            nodeId,
            nodeType,
            title,
            rounds: [],
          };
          const hasRound = sess.rounds.some(r => r.index === index);
          if (!hasRound) sess.rounds.push({ index, nodes: [] });
          sess.activeIndex = index;
          loopSessionsRef.current.set(key, sess);
          activeLoopRef.current = { nodeId: key, index };
          runnerRef.current?.onNodeStarted?.({
            status: 'running',
            nodeId,
            nodeType: 'loop',
            title,
            loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
              index: r.index,
              nodes: sortWorkflowRunItems(r.nodes),
            })),
            data: { input: undefined, output: undefined },
          } as never);
          sseCallbacks.onLoopNext?.({ data });
        } catch {
          /* no-op */
        }
      },
      onLoopCompleted: data => {
        try {
          const p = typeof data === 'object' && data ? (data as Record<string, unknown>) : {};
          const nodeId = typeof p['node_id'] === 'string' ? (p['node_id'] as string) : undefined;
          const nodeType = typeof p['node_type'] === 'string' ? (p['node_type'] as string) : 'loop';
          const title = typeof p['title'] === 'string' ? (p['title'] as string) : nodeType;
          const elapsed = typeof p['elapsed_time'] === 'number' ? (p['elapsed_time'] as number) : 0;
          const statusRaw = typeof p['status'] === 'string' ? (p['status'] as string) : '';
          const isSuccess =
            statusRaw === 'success' || statusRaw === 'succeeded' || statusRaw === 'completed';
          const error = typeof p['error'] === 'string' ? (p['error'] as string) : undefined;
          const outputs = p['outputs'];
          const execMeta =
            p['execution_metadata'] && typeof p['execution_metadata'] === 'object'
              ? (p['execution_metadata'] as Record<string, unknown>)
              : undefined;
          const variableMap =
            execMeta && typeof execMeta['loop_variable_map'] === 'object'
              ? (execMeta['loop_variable_map'] as Record<string, unknown>)
              : undefined;
          const key = nodeId ?? title;
          const sess = loopSessionsRef.current.get(key) ?? {
            nodeId,
            nodeType,
            title,
            rounds: [],
          };
          sess.elapsedTime = elapsed;
          sess.error = error;
          sess.outputs = outputs;
          sess.rounds = sess.rounds.map(r => {
            const variables = variableMap?.[String(r.index)];
            return {
              ...r,
              elapsedTime: getWorkflowRunRoundElapsedTime(r),
              variables: variables ?? r.variables,
            };
          });
          loopSessionsRef.current.set(key, sess);
          activeLoopRef.current = { nodeId: null, index: null };
          runnerRef.current?.onNodeFinished?.({
            status: isSuccess ? 'success' : 'failed',
            nodeId,
            nodeType: 'loop',
            title,
            elapsedTime: elapsed,
            error,
            loopOutputs: outputs,
            loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
              index: r.index,
              nodes: sortWorkflowRunItems(r.nodes),
              elapsedTime: r.elapsedTime,
              variables: r.variables,
            })),
            data: { input: undefined, output: undefined },
          } as never);
          sseCallbacks.onLoopCompleted?.({ data });
        } catch {
          /* no-op */
        }
      },
    }
  );

  const dispatchApprovalEvent = useCallback(
    (event: { event?: string; data?: unknown; [key: string]: unknown }) => {
      if (isStaleApprovalResumeEvent(event)) return;
      rememberApprovalEventSequence(event);
      const payload = event.data && typeof event.data === 'object' ? event.data : event;
      const record =
        typeof payload === 'object' && payload ? (payload as Record<string, unknown>) : {};

      switch (event.event) {
        case 'workflow_started':
          sseCallbacks.onWorkflowStarted?.(payload);
          rememberBackendConversation(payload);
          setIsConversationPaused(false);
          runnerRef.current?.onWorkflowStarted(payload as never);
          break;
        case 'approval_requested':
          handleApprovalRequested(event);
          break;
        case 'approval_result_filled':
          handleApprovalResultFilled(event);
          break;
        case 'approval_expired':
          handleApprovalExpired(event);
          runnerRef.current?.onWorkflowFinished({
            status: 'expired',
            messageId: lastMessageIdRef.current,
            workflowRunId:
              (typeof record.id === 'string' ? record.id : '') ||
              (typeof record.workflow_run_id === 'string' ? record.workflow_run_id : '') ||
              undefined,
            elapsedTime: typeof record.elapsed_time === 'number' ? record.elapsed_time : undefined,
          });
          break;
        case 'question_answer_requested':
          handleQuestionAnswerRequested(event);
          break;
        case 'question_answer_submitted':
          handleQuestionAnswerSubmitted(event);
          break;
        case 'node_started':
          setIsConversationPaused(false);
          setQuestionAnswerSubmitting(false);
          sseCallbacks.onNodeStarted?.(payload);
          {
            const nodeInfo = {
              status: 'running' as const,
              nodeId: typeof record.node_id === 'string' ? record.node_id : undefined,
              nodeType: typeof record.node_type === 'string' ? record.node_type : undefined,
              title: typeof record.title === 'string' ? record.title : undefined,
              data: { input: record.inputs },
            };
            const { loopId, loopIndex, iterationId, iterationIndex } =
              extractWorkflowRunContainerContext(record);
            const executionId = getWorkflowRunExecutionId(record);
            if (loopId) {
              const sess = loopSessionsRef.current.get(loopId);
              const targetIndex =
                typeof loopIndex === 'number'
                  ? loopIndex
                  : (sess?.activeIndex ?? activeLoopRef.current.index);
              if (sess && typeof targetIndex === 'number') {
                let round = sess.rounds.find(r => r.index === targetIndex);
                if (!round) {
                  round = { index: targetIndex, nodes: [] };
                  sess.rounds.push(round);
                }
                const childKey = getWorkflowRunItemKey({
                  executionId,
                  nodeId: nodeInfo.nodeId,
                  nodeType: nodeInfo.nodeType,
                  title: nodeInfo.title,
                });
                const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
                const child = { ...nodeInfo, executionId };
                if (cIdx >= 0) {
                  round.nodes[cIdx] = { ...round.nodes[cIdx], ...child };
                } else {
                  round.nodes.push(child);
                }
                sess.activeIndex = targetIndex;
                loopSessionsRef.current.set(loopId, { ...sess });
                activeLoopRef.current = { nodeId: loopId, index: targetIndex };
                runnerRef.current?.onNodeStarted?.({
                  status: 'running',
                  nodeId: sess.nodeId,
                  nodeType: 'loop',
                  title: sess.title,
                  loopInputs: sess.inputs,
                  loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                    index: r.index,
                    nodes: sortWorkflowRunItems(r.nodes),
                  })),
                } as never);
                break;
              }
            }
            const active = activeIterationRef.current;
            const targetIterationId = iterationId ?? active.nodeId;
            const targetIterationIndex =
              typeof iterationIndex === 'number' ? iterationIndex : active.index;
            if (targetIterationId && targetIterationIndex !== null) {
              const sess = iterationSessionsRef.current.get(targetIterationId);
              if (sess) {
                let round = sess.rounds.find(r => r.index === targetIterationIndex);
                if (!round) {
                  round = { index: targetIterationIndex, nodes: [] };
                  sess.rounds.push(round);
                }
                const childKey = getWorkflowRunItemKey({
                  executionId,
                  nodeId: nodeInfo.nodeId,
                  nodeType: nodeInfo.nodeType,
                  title: nodeInfo.title,
                });
                const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
                const child = { ...nodeInfo, executionId };
                if (cIdx >= 0) {
                  round.nodes[cIdx] = { ...round.nodes[cIdx], ...child };
                } else {
                  round.nodes.push(child);
                }
                sess.activeIndex = targetIterationIndex;
                iterationSessionsRef.current.set(targetIterationId, { ...sess });
                runnerRef.current?.onNodeStarted?.({
                  status: 'running',
                  nodeId: sess.nodeId,
                  nodeType: 'iteration',
                  title: sess.title,
                  iterationInputs: sess.inputs,
                  iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                    index: r.index,
                    nodes: sortWorkflowRunItems(r.nodes),
                  })),
                } as never);
                break;
              }
            }
            runnerRef.current?.onNodeStarted?.(nodeInfo);
          }
          break;
        case 'node_finished': {
          sseCallbacks.onNodeFinished?.(payload);
          const rawStatus = typeof record.status === 'string' ? record.status : '';
          const status: 'failed' | 'paused' | 'stopped' | 'succeeded' | 'running' =
            rawStatus === 'failed'
              ? 'failed'
              : rawStatus === 'paused'
                ? 'paused'
                : rawStatus === 'stopped'
                  ? 'stopped'
                  : rawStatus === 'success' ||
                      rawStatus === 'succeeded' ||
                      rawStatus === 'completed'
                    ? 'succeeded'
                    : 'running';
          const nodeInfo = {
            status,
            nodeId: typeof record.node_id === 'string' ? record.node_id : undefined,
            nodeType: typeof record.node_type === 'string' ? record.node_type : undefined,
            title: typeof record.title === 'string' ? record.title : undefined,
            elapsedTime: typeof record.elapsed_time === 'number' ? record.elapsed_time : 0,
            error: getWorkflowRunErrorText(record.error),
            data: {
              input: record.inputs,
              output: record.outputs,
              modelInput: extractLlmGatewayRequest(record),
            },
          };
          const { loopId, loopIndex, iterationId, iterationIndex } =
            extractWorkflowRunContainerContext(record);
          const executionId = getWorkflowRunExecutionId(record);
          if (loopId) {
            const sess = loopSessionsRef.current.get(loopId);
            const targetIndex =
              typeof loopIndex === 'number'
                ? loopIndex
                : (sess?.activeIndex ?? activeLoopRef.current.index);
            if (sess && typeof targetIndex === 'number') {
              let round = sess.rounds.find(r => r.index === targetIndex);
              if (!round) {
                round = { index: targetIndex, nodes: [] };
                sess.rounds.push(round);
              }
              const childKey = getWorkflowRunItemKey({
                executionId,
                nodeId: nodeInfo.nodeId,
                nodeType: nodeInfo.nodeType,
                title: nodeInfo.title,
              });
              const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
              const child = { ...nodeInfo, executionId };
              if (cIdx >= 0) {
                round.nodes[cIdx] = { ...round.nodes[cIdx], ...child };
              } else {
                round.nodes.push(child);
              }
              sess.activeIndex = targetIndex;
              loopSessionsRef.current.set(loopId, { ...sess });
              activeLoopRef.current = { nodeId: loopId, index: targetIndex };
              runnerRef.current?.onNodeFinished?.({
                status: 'running',
                nodeId: sess.nodeId,
                nodeType: 'loop',
                title: sess.title,
                loopInputs: sess.inputs,
                loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                  index: r.index,
                  nodes: sortWorkflowRunItems(r.nodes),
                })),
              } as never);
              break;
            }
          }
          const active = activeIterationRef.current;
          const targetIterationId = iterationId ?? active.nodeId;
          const targetIterationIndex =
            typeof iterationIndex === 'number' ? iterationIndex : active.index;
          if (targetIterationId && targetIterationIndex !== null) {
            const sess = iterationSessionsRef.current.get(targetIterationId);
            if (sess) {
              let round = sess.rounds.find(r => r.index === targetIterationIndex);
              if (!round) {
                round = { index: targetIterationIndex, nodes: [] };
                sess.rounds.push(round);
              }
              const childKey = getWorkflowRunItemKey({
                executionId,
                nodeId: nodeInfo.nodeId,
                nodeType: nodeInfo.nodeType,
                title: nodeInfo.title,
              });
              const cIdx = round.nodes.findIndex(c => getWorkflowRunItemKey(c) === childKey);
              const child = { ...nodeInfo, executionId };
              if (cIdx >= 0) {
                round.nodes[cIdx] = { ...round.nodes[cIdx], ...child };
              } else {
                round.nodes.push(child);
              }
              sess.activeIndex = targetIterationIndex;
              iterationSessionsRef.current.set(targetIterationId, { ...sess });
              runnerRef.current?.onNodeFinished?.({
                status: 'running',
                nodeId: sess.nodeId,
                nodeType: 'iteration',
                title: sess.title,
                iterationInputs: sess.inputs,
                iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                  index: r.index,
                  nodes: sortWorkflowRunItems(r.nodes),
                })),
              } as never);
              break;
            }
          }
          runnerRef.current?.onNodeFinished?.(nodeInfo);
          break;
        }
        case 'iteration_started':
          sseCallbacks.onIterationStarted?.(payload);
          {
            const nodeId =
              typeof record.node_id === 'string' ? (record.node_id as string) : undefined;
            const nodeType =
              typeof record.node_type === 'string' ? (record.node_type as string) : 'iteration';
            const title = typeof record.title === 'string' ? (record.title as string) : nodeType;
            const key = nodeId ?? title;
            iterationSessionsRef.current.set(key, {
              nodeId,
              nodeType,
              title,
              inputs: record.inputs,
              rounds: [],
              activeIndex: null,
            });
            activeIterationRef.current = { nodeId: key, index: null };
            runnerRef.current?.onNodeStarted?.({
              status: 'running',
              nodeId,
              nodeType,
              title,
              iterationInputs: record.inputs,
              iterationRounds: [],
              data: { input: undefined, output: undefined },
            } as never);
          }
          break;
        case 'iteration_next':
          sseCallbacks.onIterationNext?.(payload);
          {
            const nodeId =
              typeof record.node_id === 'string' ? (record.node_id as string) : undefined;
            const nodeType =
              typeof record.node_type === 'string' ? (record.node_type as string) : 'iteration';
            const title = typeof record.title === 'string' ? (record.title as string) : nodeType;
            const index = typeof record.index === 'number' ? record.index : 0;
            const key = nodeId ?? title;
            const sess = iterationSessionsRef.current.get(key) ?? {
              nodeId,
              nodeType,
              title,
              rounds: [],
            };
            if (!sess.rounds.some(r => r.index === index)) {
              sess.rounds.push({ index, nodes: [] });
            }
            sess.activeIndex = index;
            iterationSessionsRef.current.set(key, sess);
            activeIterationRef.current = { nodeId: key, index };
            runnerRef.current?.onNodeStarted?.({
              status: 'running',
              nodeId,
              nodeType,
              title,
              iterationInputs: sess.inputs,
              iterationRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                index: r.index,
                nodes: sortWorkflowRunItems(r.nodes),
              })),
            } as never);
          }
          break;
        case 'iteration_completed':
          sseCallbacks.onIterationCompleted?.(payload);
          {
            const nodeId =
              typeof record.node_id === 'string' ? (record.node_id as string) : undefined;
            const nodeType =
              typeof record.node_type === 'string' ? (record.node_type as string) : 'iteration';
            const title = typeof record.title === 'string' ? (record.title as string) : nodeType;
            const key = nodeId ?? title;
            const sess = iterationSessionsRef.current.get(key);
            if (sess) {
              sess.elapsedTime =
                typeof record.elapsed_time === 'number' ? record.elapsed_time : undefined;
              sess.outputs = record.outputs;
              sess.error = typeof record.error === 'string' ? record.error : undefined;
              sess.rounds = sess.rounds.map(r => ({
                ...r,
                elapsedTime: getWorkflowRunRoundElapsedTime(r),
              }));
              iterationSessionsRef.current.set(key, sess);
            }
            activeIterationRef.current = { nodeId: null, index: null };
            runnerRef.current?.onNodeFinished?.({
              status: typeof record.error === 'string' && record.error ? 'failed' : 'succeeded',
              nodeId,
              nodeType,
              title,
              elapsedTime: typeof record.elapsed_time === 'number' ? record.elapsed_time : 0,
              error: getWorkflowRunErrorText(record.error),
              iterationOutputs: record.outputs,
              iterationRounds: sortWorkflowRunRounds(sess?.rounds ?? []).map(r => ({
                index: r.index,
                nodes: sortWorkflowRunItems(r.nodes),
                elapsedTime: r.elapsedTime,
              })),
              data: { input: undefined, output: undefined },
            } as never);
          }
          break;
        case 'loop_started':
          sseCallbacks.onLoopStarted?.(payload);
          {
            const nodeId =
              typeof record.node_id === 'string' ? (record.node_id as string) : undefined;
            const nodeType = typeof record.node_type === 'string' ? (record.node_type as string) : 'loop';
            const title = typeof record.title === 'string' ? (record.title as string) : nodeType;
            const key = nodeId ?? title;
            loopSessionsRef.current.set(key, {
              nodeId,
              nodeType,
              title,
              inputs: record.inputs,
              rounds: [],
              activeIndex: null,
            });
            activeLoopRef.current = { nodeId: key, index: null };
            runnerRef.current?.onNodeStarted?.({
              status: 'running',
              nodeId,
              nodeType,
              title,
              loopInputs: record.inputs,
              loopRounds: [],
              data: { input: undefined, output: undefined },
            } as never);
          }
          break;
        case 'loop_next':
          sseCallbacks.onLoopNext?.(payload);
          {
            const nodeId =
              typeof record.node_id === 'string' ? (record.node_id as string) : undefined;
            const nodeType = typeof record.node_type === 'string' ? (record.node_type as string) : 'loop';
            const title = typeof record.title === 'string' ? (record.title as string) : nodeType;
            const index = typeof record.index === 'number' ? record.index : 0;
            const key = nodeId ?? title;
            const sess = loopSessionsRef.current.get(key) ?? {
              nodeId,
              nodeType,
              title,
              rounds: [],
            };
            if (!sess.rounds.some(r => r.index === index)) {
              sess.rounds.push({ index, nodes: [] });
            }
            sess.activeIndex = index;
            loopSessionsRef.current.set(key, sess);
            activeLoopRef.current = { nodeId: key, index };
            runnerRef.current?.onNodeStarted?.({
              status: 'running',
              nodeId,
              nodeType,
              title,
              loopInputs: sess.inputs,
              loopRounds: sortWorkflowRunRounds(sess.rounds).map(r => ({
                index: r.index,
                nodes: sortWorkflowRunItems(r.nodes),
              })),
            } as never);
          }
          break;
        case 'loop_completed':
          sseCallbacks.onLoopCompleted?.(payload);
          {
            const nodeId =
              typeof record.node_id === 'string' ? (record.node_id as string) : undefined;
            const nodeType = typeof record.node_type === 'string' ? (record.node_type as string) : 'loop';
            const title = typeof record.title === 'string' ? (record.title as string) : nodeType;
            const key = nodeId ?? title;
            const sess = loopSessionsRef.current.get(key);
            if (sess) {
              sess.elapsedTime =
                typeof record.elapsed_time === 'number' ? record.elapsed_time : undefined;
              sess.outputs = record.outputs;
              sess.error = typeof record.error === 'string' ? record.error : undefined;
              sess.rounds = sess.rounds.map(r => ({
                ...r,
                elapsedTime: getWorkflowRunRoundElapsedTime(r),
              }));
              loopSessionsRef.current.set(key, sess);
            }
            activeLoopRef.current = { nodeId: null, index: null };
            runnerRef.current?.onNodeFinished?.({
              status: typeof record.error === 'string' && record.error ? 'failed' : 'succeeded',
              nodeId,
              nodeType,
              title,
              elapsedTime: typeof record.elapsed_time === 'number' ? record.elapsed_time : 0,
              error: getWorkflowRunErrorText(record.error),
              loopOutputs: record.outputs,
              loopRounds: sortWorkflowRunRounds(sess?.rounds ?? []).map(r => ({
                index: r.index,
                nodes: sortWorkflowRunItems(r.nodes),
                elapsedTime: r.elapsedTime,
              })),
              data: { input: undefined, output: undefined },
            } as never);
          }
          break;
        case 'message':
        case 'text_chunk':
          runnerRef.current?.onMessage?.(record);
          break;
        case 'message_end':
          runnerRef.current?.onMessageEnd?.(payload as never);
          break;
        case 'workflow_paused':
          handleWorkflowPaused(event);
          break;
        case 'workflow_finished':
        case 'workflow_stopped':
        case 'workflow_failed':
        case 'workflow_succeeded':
        case 'workflow_completed': {
          if (hasUnresolvedApprovalEntries(approvalRuntimeStateRef.current)) {
            setIsConversationPaused(true);
            break;
          }
          sseCallbacks.onWorkflowFinished?.(payload);
          const rawStatus = typeof record.status === 'string' ? record.status : '';
          const eventStatus =
            event.event === 'workflow_stopped'
              ? 'stopped'
              : event.event === 'workflow_failed'
                ? 'error'
                : event.event === 'workflow_succeeded' || event.event === 'workflow_completed'
                  ? 'completed'
                  : '';
          const finalStatus = rawStatus || eventStatus;
          const status =
            finalStatus === 'failed' || finalStatus === 'error'
              ? 'error'
              : finalStatus === 'stopped'
                ? 'stopped'
                : finalStatus === 'expired'
                  ? 'expired'
                  : 'completed';
          const rawError = record.error;
          runnerRef.current?.onWorkflowFinished({
            status,
            messageId: lastMessageIdRef.current,
            workflowRunId:
              (typeof record.id === 'string' ? record.id : '') ||
              (typeof record.workflow_run_id === 'string' ? record.workflow_run_id : '') ||
              undefined,
            elapsedTime: typeof record.elapsed_time === 'number' ? record.elapsed_time : undefined,
            error: getWorkflowRunErrorText(rawError),
          });
          if (status === 'error') {
            notifyBillingError(rawError);
          }
          resetApprovalRuntime();
          setQuestionAnswerPrompt(null);
          setQuestionAnswerSubmitting(false);
          setQuestionAnswerTranscript([]);
          setWorkflowRunId(null);
          workflowFinishedRef.current = true;
          setIsConversationPaused(false);
          cancelWorkflowRunEvents();
          approvalResumeStreamActiveRef.current = false;
          break;
        }
        case 'error':
          setIsConversationPaused(false);
          setQuestionAnswerSubmitting(false);
          sseCallbacks.onError?.(payload);
          runnerRef.current?.onWorkflowFinished({
            status: 'error',
            error: getWorkflowRunErrorText(payload) ?? String(payload ?? 'Error'),
          });
          notifyBillingError(payload);
          break;
      }
    },
    [
      cancelWorkflowRunEvents,
      getWorkflowRunErrorText,
      handleApprovalExpired,
      handleApprovalRequested,
      handleApprovalResultFilled,
      handleQuestionAnswerRequested,
      handleQuestionAnswerSubmitted,
      handleWorkflowPaused,
      isStaleApprovalResumeEvent,
      notifyBillingError,
      rememberBackendConversation,
      rememberApprovalEventSequence,
      resetApprovalRuntime,
      sseCallbacks,
    ]
  );

  const startApprovalResumeEventStream = useCallback(
    (payload?: unknown) => {
      const data = payload ? getEventData(payload) : {};
      const runId =
        workflowRunId ||
        (typeof data.id === 'string'
          ? data.id
          : typeof data.workflow_run_id === 'string'
            ? data.workflow_run_id
            : '');
      if (!runId) return;

      setWorkflowRunId(runId);
      approvalResumeStreamActiveRef.current = true;
      workflowFinishedRef.current = false;
      const streamParams =
        approvalEventCursorRef.current > 0
          ? { after: approvalEventCursorRef.current, continue_on_pause: true }
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
          onWorkflowStarted: streamPayload =>
            dispatchWorkflowRunEvent('workflow_started', streamPayload),
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
          onNodeStarted: streamPayload => dispatchWorkflowRunEvent('node_started', streamPayload),
          onNodeFinished: streamPayload => dispatchWorkflowRunEvent('node_finished', streamPayload),
          onIterationStarted: streamPayload =>
            dispatchWorkflowRunEvent('iteration_started', streamPayload),
          onIterationNext: streamPayload =>
            dispatchWorkflowRunEvent('iteration_next', streamPayload),
          onIterationCompleted: streamPayload =>
            dispatchWorkflowRunEvent('iteration_completed', streamPayload),
          onLoopStarted: streamPayload => dispatchWorkflowRunEvent('loop_started', streamPayload),
          onLoopNext: streamPayload => dispatchWorkflowRunEvent('loop_next', streamPayload),
          onLoopCompleted: streamPayload =>
            dispatchWorkflowRunEvent('loop_completed', streamPayload),
          onWorkflowFinished: streamPayload =>
            dispatchWorkflowRunEvent('workflow_finished', streamPayload),
          onError: streamPayload => dispatchWorkflowRunEvent('error', streamPayload),
          onMessage: streamPayload => dispatchWorkflowRunEvent('message', streamPayload),
          onMessageEnd: streamPayload => dispatchWorkflowRunEvent('message_end', streamPayload),
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
    [dispatchApprovalEvent, getEventData, startWorkflowRunEvents, workflowRunId]
  );

  useEffect(() => {
    startApprovalResumeEventStreamRef.current = startApprovalResumeEventStream;
  }, [startApprovalResumeEventStream]);

  const handleApprovalSubmit = useCallback(
    async (payload: { inputs: Record<string, unknown>; action: string }) => {
      if (!approvalToken) return;
      if (!approvalEntry) return;
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

  const approvalInputReplacement = useMemo(() => {
    if (questionAnswerPrompt) return null;
    if (!isConversationPaused) return null;

    if (approvalSubmittedAction) {
      return <ApprovalWaitingState loading submitted />;
    }

    if (!approvalToken) {
      return <ApprovalWaitingState />;
    }

    if (!approvalForm && (approvalFormQuery.isLoading || approvalFormQuery.isFetching)) {
      return <ApprovalWaitingState loading />;
    }

    if (!approvalForm && isApprovalFormAlreadySubmittedError(approvalFormQuery.error)) {
      return <ApprovalCompletedState compact />;
    }

    if (!approvalForm && approvalFormQuery.error) {
      return (
        <div className="rounded-xl border bg-card p-4 text-center shadow-sm">
          <div className="text-sm font-medium">{t('nodes.approval.runtime.loadFailed')}</div>
          <p className="mt-2 text-xs text-muted-foreground">
            {approvalFormQuery.error instanceof Error
              ? approvalFormQuery.error.message
              : t('nodes.approval.runtime.loadFailedDescription')}
          </p>
          <Button
            type="button"
            size="sm"
            className="mt-3"
            onClick={() => void approvalFormQuery.refetch()}
          >
            {t('nodes.approval.runtime.retry')}
          </Button>
        </div>
      );
    }

    if (approvalForm) {
      return (
        <div className="max-h-[45vh] overflow-y-auto rounded-xl border bg-card p-3 shadow-sm">
          <ApprovalRuntimeForm
            form={approvalForm}
            onSubmit={payload => void handleApprovalSubmit(payload)}
            isSubmitting={approvalSubmitMutation.isPending || approvalRuntimeSubmitting}
            submittedAction={approvalSubmittedAction}
          />
        </div>
      );
    }

    return <ApprovalWaitingState loading />;
  }, [
    approvalForm,
    approvalFormQuery,
    approvalRuntimeSubmitting,
    approvalSubmitMutation.isPending,
    approvalSubmittedAction,
    approvalToken,
    handleApprovalSubmit,
    isConversationPaused,
    questionAnswerPrompt,
    t,
  ]);

  interface IterationRound {
    index: number;
    nodes: Array<{
      status: 'running' | 'failed' | 'success' | 'succeeded' | 'stopped' | 'paused';
      nodeId?: string;
      executionId?: string;
      createdAtMs?: number;
      receivedOrder?: number;
      nodeType?: string;
      title?: string;
      elapsedTime?: number;
      error?: string;
      data?: { input?: unknown; output?: unknown };
    }>;
    elapsedTime?: number;
  }
  interface IterationSession {
    nodeId?: string;
    nodeType?: string;
    title?: string;
    inputs?: unknown;
    outputs?: unknown;
    elapsedTime?: number;
    error?: string;
    rounds: IterationRound[];
    activeIndex?: number | null;
  }
  const iterationSessionsRef = useRef<Map<string, IterationSession>>(new Map());
  const activeIterationRef = useRef<{ nodeId: string | null; index: number | null }>({
    nodeId: null,
    index: null,
  });
  interface LoopRound {
    index: number;
    nodes: Array<{
      status: 'running' | 'failed' | 'success' | 'succeeded' | 'stopped' | 'paused';
      nodeId?: string;
      executionId?: string;
      createdAtMs?: number;
      receivedOrder?: number;
      nodeType?: string;
      title?: string;
      elapsedTime?: number;
      error?: string;
      data?: { input?: unknown; output?: unknown };
    }>;
    elapsedTime?: number;
    variables?: unknown;
  }
  interface LoopSession {
    nodeId?: string;
    nodeType?: string;
    title?: string;
    inputs?: unknown;
    outputs?: unknown;
    elapsedTime?: number;
    error?: string;
    rounds: LoopRound[];
    activeIndex?: number | null;
  }
  const loopSessionsRef = useRef<Map<string, LoopSession>>(new Map());
  const activeLoopRef = useRef<{ nodeId: string | null; index: number | null }>({
    nodeId: null,
    index: null,
  });
  const receiveOrderRef = useRef(0);

  const nextReceiveOrder = useCallback(() => {
    receiveOrderRef.current += 1;
    return receiveOrderRef.current;
  }, []);

  // No external form persistence; inputs will be provided by UserInput on send

  // Pull chat-related features from workflow store to avoid prop drilling
  const storeWorkflowData = useWorkflowStore.use.workflowData();
  const wfFeatures = storeWorkflowData?.features ?? initialWorkflowData.features;
  const features: Pick<WorkflowFeatures, 'file_upload' | 'retriever_resource'> = {
    file_upload: wfFeatures.file_upload,
    retriever_resource: wfFeatures.retriever_resource,
  };

  const openingGuide = useMemo(() => getOpeningGuide(wfFeatures), [wfFeatures]);
  const suggestedQuestions = useMemo(
    () =>
      (wfFeatures.suggested_questions ?? [])
        .map(question => question.trim())
        .filter(Boolean)
        .slice(0, SUGGESTED_QUESTIONS_LIMIT),
    [wfFeatures.suggested_questions]
  );

  const { errors } = useWorkflowValidation();
  const setOpenValidationIssues = useWorkflowStore.use.setOpenValidationIssues();
  const openIssues = useCallback(() => setOpenValidationIssues(true), [setOpenValidationIssues]);
  const [runWarnOpen, setRunWarnOpen] = useState(false);
  const [dontWarnAgain, setDontWarnAgain] = useState(false);
  // Store pending send data when warning dialog is shown, to execute after user confirms
  const pendingSendRef = useRef<{
    items: Array<{ id: string; conversationId: string | null }>;
    userInput: {
      query: string;
      files?: ChatAttachment[];
      inputs: Record<string, unknown>;
    };
  } | null>(null);

  const sanitizeInputsForRestore = useCallback(
    (rawInputs: Record<string, unknown>) => {
      const nextInputs = { ...rawInputs };
      startVariables.forEach(v => {
        if (v.type === 'file') {
          const val = nextInputs[v.variable];
          if (
            val &&
            typeof val === 'object' &&
            'upload_file_id' in (val as Record<string, unknown>)
          ) {
            nextInputs[v.variable] = (val as { upload_file_id: string }).upload_file_id;
          }
        }
        if (v.type === 'file-list') {
          const val = nextInputs[v.variable];
          if (Array.isArray(val)) {
            nextInputs[v.variable] = val.map(item => {
              if (item && typeof item === 'object' && 'upload_file_id' in item) {
                return (item as { upload_file_id: string }).upload_file_id;
              }
              return item;
            });
          }
        }
      });

      return nextInputs;
    },
    [startVariables]
  );

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

  const startChatWithPrecheck = useCallback(
    async (
      items: Array<{ id: string; conversationId: string | null }>,
      userInput: {
        query: string;
        files?: ChatAttachment[];
        inputs: Record<string, unknown>;
      }
    ) => {
      if (!chatConv) {
        initSingle({
          id: convId,
          conversationId: '',
          title: t('agents.workflow.chat.newConversation'),
        });
      }
      try {
        const latestConversationId =
          useChatStore.getState().conversations[convId]?.conversationId ||
          items[0]?.conversationId ||
          '';
        const payload: {
          query: string;
          conversation_id?: string;
          files?: ChatAttachment[];
          inputs: Record<string, unknown>;
        } = {
          query: userInput.query,
          conversation_id: latestConversationId || undefined,
          files: userInput.files,
          inputs: userInput.inputs ?? {},
        };

        const activeQuestionAnswerPrompt = questionAnswerPrompt;
        const isQuestionAnswerResume = Boolean(activeQuestionAnswerPrompt);
        if (!isQuestionAnswerResume) {
          await persistDraftBeforeRun();

          const precheck = await workflowChatPrecheck.mutateAsync(payload);
          const warnings = getWorkflowPrecheckWarnings(precheck);
          if (precheck.status === 'warning' && warnings.length > 0) {
            setPrecheckWarnings(warnings);
          } else {
            setPrecheckWarnings([]);
          }

          resetApprovalRuntime();
          setQuestionAnswerPrompt(null);
          setQuestionAnswerTranscript([]);
        } else {
          setPrecheckWarnings([]);
          setQuestionAnswerSubmitting(true);
          setIsConversationPaused((activeQuestionAnswerPrompt?.choices.length ?? 0) > 0);
        }
        if (!isQuestionAnswerResume) {
          approvalEventCursorRef.current = 0;
          setWorkflowRunId(null);
          workflowFinishedRef.current = false;
          setIsConversationPaused(false);
          cancelWorkflowRunEvents();
          approvalResumeStreamActiveRef.current = false;
          runnerRef.current = onAgentRun(convId, {
            onWorkflowStarted: () => {},
            onTextChunk: () => {},
            onNodeStarted: () => {},
            onNodeFinished: () => {},
            onError: () => {},
            onWorkflowFinished: () => {},
          });
          runnerRef.current.onWorkflowStarted({ query: userInput.query, inputs: userInput.inputs });
        }

        await start(payload);
        setLastDebugInputs({
          ...userInput.inputs,
          query: userInput.query,
          'sys.query': userInput.query,
          ...(userInput.files ? { files: userInput.files, 'sys.files': userInput.files } : {}),
          ...(latestConversationId ? { 'sys.conversation_id': latestConversationId } : {}),
        });
        setLastInputs(sanitizeInputsForRestore(userInput.inputs));
      } catch (error) {
        if (questionAnswerPrompt) {
          setQuestionAnswerSubmitting(false);
        }
        if (!notifyBillingError(error)) {
          toast.error(
            error instanceof Error ? error.message : t('agents.workflow.errors.executionFailed')
          );
        }
      }
    },
    [
      cancelWorkflowRunEvents,
      convId,
      chatConv,
      initSingle,
      onAgentRun,
      notifyBillingError,
      persistDraftBeforeRun,
      questionAnswerPrompt,
      resetApprovalRuntime,
      sanitizeInputsForRestore,
      setLastDebugInputs,
      start,
      t,
      workflowChatPrecheck,
    ]
  );

  const handleQuestionAnswerChoice = useCallback(
    (choice: QuestionAnswerChoice) => {
      const query = getQuestionAnswerChoiceQuery(choice);
      if (!query || questionAnswerSubmitting) return;
      setQuestionAnswerSubmitting(true);
      const conversationId =
        useChatStore.getState().conversations[convId]?.conversationId || chatConv?.conversationId;
      void startChatWithPrecheck([{ id: convId, conversationId: conversationId || null }], {
        query,
        inputs: {
          ...lastInputs,
          question_answer_option_id: choice.id,
        },
      });
    },
    [chatConv?.conversationId, convId, lastInputs, questionAnswerSubmitting, startChatWithPrecheck]
  );

  const questionAnswerInputNotice = useMemo(() => {
    if (!questionAnswerPrompt) return null;
    return (
      <QuestionAnswerRuntimePrompt
        question={questionAnswerPrompt.question}
        choices={questionAnswerPrompt.choices}
        round={questionAnswerPrompt.round}
        submitting={questionAnswerSubmitting}
        onSelectChoice={handleQuestionAnswerChoice}
      />
    );
  }, [handleQuestionAnswerChoice, questionAnswerPrompt, questionAnswerSubmitting]);

  const handleSend = useCallback(
    (
      items: Array<{ id: string; conversationId: string | null }>,
      userInput: {
        query: string;
        files?: ChatAttachment[];
        inputs: Record<string, unknown>;
      }
    ) => {
      if (isConversationPaused && !questionAnswerPrompt) return;
      if (errors.length > 0 && !isBannerHidden(BannerKey.WorkflowRunErrorsWarning)) {
        pendingSendRef.current = { items, userInput };
        setRunWarnOpen(true);
        return;
      }
      void startChatWithPrecheck(items, userInput);
    },
    [errors.length, isConversationPaused, questionAnswerPrompt, startChatWithPrecheck]
  );

  const handleReset = useCallback(() => {
    try {
      cancel();
    } catch {
      /* no-op */
    }
    try {
      runnerRef.current?.dispose?.();
    } catch {
      /* no-op */
    }
    runnerRef.current = null;
    lastMessageIdRef.current = undefined;
    resetApprovalRuntime();
    setQuestionAnswerPrompt(null);
    setQuestionAnswerSubmitting(false);
    setQuestionAnswerTranscript([]);
    approvalEventCursorRef.current = 0;
    setWorkflowRunId(null);
    workflowFinishedRef.current = true;
    setIsConversationPaused(false);
    cancelWorkflowRunEvents();
    approvalResumeStreamActiveRef.current = false;
    resetRunStatus();
    chatRunItemsRef.current = [];
    setRuntimeLogItems([]);
    setLastDebugInputs(null);
    setLastInputs({});
    const newId = generateClientId('conversation');
    setConvId(newId);
    // ensure new conversation starts fresh
    updateConversation(newId, { conversationId: '' });
  }, [
    cancel,
    cancelWorkflowRunEvents,
    resetApprovalRuntime,
    resetRunStatus,
    setLastDebugInputs,
    setRuntimeLogItems,
    setConvId,
    updateConversation,
  ]);

  // features already derived above

  // Register with panel stack like run panel (hooks must be unconditional)
  const { panelStyle } = usePanelStackItem({
    id: 'chat-draft',
    position: 'top-right',
    order: 1,
    visible: open,
    width: panelWidth,
    gap: 8,
  });

  useEffect(() => {
    const win = window as Window & {
      __workflowChatPanelOpen?: boolean;
      __workflowChatPanelShake?: () => void;
    };
    win.__workflowChatPanelOpen = open;
    win.__workflowChatPanelShake = () => {
      setShake(true);
      window.setTimeout(() => setShake(false), 600);
    };
    return () => {
      win.__workflowChatPanelOpen = false;
      win.__workflowChatPanelShake = undefined as unknown as () => void;
    };
  }, [open]);

  useEffect(() => {
    if (open) return;
    workflowFinishedRef.current = true;
    cancelWorkflowRunEvents();
    approvalResumeStreamActiveRef.current = false;
  }, [cancelWorkflowRunEvents, open]);

  const toolbarFormSpec = useMemo(
    () =>
      startVariables.length > 0
        ? {
            title: t('agents.workflow.startForm.chatTitle'),
            icon: <SlidersHorizontal size={16} />,
            variables: startVariables,
            initialValues: {
              ...startVariables.reduce(
                (acc, v) => {
                  if (v.default !== undefined) acc[v.variable] = v.default;
                  return acc;
                },
                {} as Record<string, unknown>
              ),
              ...lastInputs,
            },
          }
        : undefined,
    [startVariables, t, lastInputs]
  );

  // Stable signature of startVariables to force-refresh the Chat component when schema changes
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

  if (!open) return null;

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
      <div
        aria-hidden="true"
        className="absolute left-0 top-0 z-20 h-full w-2 cursor-ew-resize transition-colors hover:bg-primary/10"
        {...resizeHandleProps}
      />
      <div
        className="flex flex-col h-full"
        onContextMenu={e => {
          e.stopPropagation();
        }}
      >
        <div className="flex items-center justify-between border-b border-border/50 px-3 py-2">
          <div className="font-medium">{t('agents.workflow.debugTitle')}</div>
          <div className="flex items-center gap-2">
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
            <Button variant="ghost" isIcon onClick={handleReset} aria-label={t('common.reset')}>
              <RotateCcw size={16} className="text-primary" />
            </Button>
            <Button variant="ghost" isIcon onClick={onClose} aria-label={t('common.close')}>
              <X size={16} className="text-primary" />
            </Button>
          </div>
        </div>
        <div className="flex-1 min-h-0 flex flex-col overflow-hidden">
          <div className="flex-1 min-h-0">
            <Chat
              key={`${varsSig}-${convId}`}
              className="h-full"
              mode="singleTest"
              conversation={{ id: convId, conversationId: chatConv?.conversationId ?? '' }}
              onSend={handleSend}
              onStop={stop}
              features={features}
              enableUpload={features?.file_upload?.enabled ?? true}
              openingGuide={openingGuide}
              openingGuideBrand={openingGuideBrand}
              suggestions={suggestedQuestions}
              inputDisabled={undefined}
              showWorkflowRunHeader
              showWorkflowDetail
              showWorkflowNodeDetail
              placeholder={
                startVariables.length > 0
                  ? t('agents.workflow.startForm.fillFormToStart')
                  : undefined
              }
              toolbarForm={toolbarFormSpec}
              inputTopNotice={
                questionAnswerInputNotice ||
                (precheckWarnings.length > 0 ? (
                  <WorkflowPrecheckWarningBanner
                    warnings={precheckWarnings}
                    scope="agents"
                    storageKey={`workflow-chat-panel:${agentId}`}
                  />
                ) : null)
              }
              inputReplacement={approvalInputReplacement}
              sendDisabled={Boolean(
                isStarting ||
                  (isConversationPaused && !questionAnswerPrompt) ||
                  questionAnswerSubmitting
              )}
              isRunning={isRunning}
              isStopping={isStopping}
            />
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
                      id="wf-chat-warn-hide"
                      checked={dontWarnAgain}
                      onCheckedChange={v => setDontWarnAgain(Boolean(v))}
                      className="w-5 h-5"
                    />
                    <Label
                      htmlFor="wf-chat-warn-hide"
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
                      // Execute with pending send data after closing dialog
                      const pending = pendingSendRef.current;
                      pendingSendRef.current = null;
                      if (pending) {
                        void startChatWithPrecheck(pending.items, pending.userInput);
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
      </div>
    </Panel>
  );
};

export default WorkflowChatPanel;
