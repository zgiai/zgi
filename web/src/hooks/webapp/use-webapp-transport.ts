import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type {
  ConversationTransport,
  ConversationSummary,
  ConversationDetail,
  Pagination,
  SendMessagePayload,
  ChatRunCallbacks,
} from '@/components/chat/controllers/types';
import type { Message, TerminalRunStatus } from '@/components/chat/types';
import { normalizeMessageRunStatus } from '@/components/chat/types';
import { resolveAnswerMergeMode } from '@/components/chat/utils/answer-merge';
import { useChatStore } from '@/components/chat/store';
import { WebAppService } from '@/services/webapp.service';
import type {
  WebAppConversation,
  WebAppConversationDetail,
  WebAppConversationMessageItem,
  WebAppRunRequest,
  WebAppRunSseCallbacks,
} from '@/services/types/webapp';
import type { WorkflowPrecheckWarning } from '@/services/types/workflow';
import type { QuestionAnswerChoice } from '@/services/types/workflow';
import { toast } from 'sonner';
import { useRunWebAppWorkflowStream } from './use-run-webapp-workflow-stream';
import {
  useWorkflowRunEventsStream,
  type WorkflowRunEventsStreamParams,
} from '@/hooks/workflow/use-workflow-run-events-stream';
import { queryClient } from '@/lib/query-client';
import { unwrap, mapNode } from '@/utils/webapp/run-mappers';
import { WEBAPP_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { useWebAppPrecheck } from './use-webapp-precheck';
import { useWorkflowBillingFeedback } from '@/hooks/workflow/use-workflow-billing-feedback';
import {
  fetchApprovalEvents,
  useApprovalForm,
  useSubmitApprovalForm,
} from '@/hooks/workflow/use-approval-form';
import type { ApprovalRuntimeForm as ApprovalRuntimeFormData } from '@/services/approval.service';
import {
  parseApprovalRequestedEvent,
  parseApprovalPausedEvent,
} from '@/components/workflow/approval/runtime-events';
import {
  hasUnresolvedApprovalEntries,
  useApprovalRuntimeEvents,
} from '@/components/workflow/approval/use-approval-runtime-events';
import {
  appendQuestionAnswerTranscriptQuestion,
  applyQuestionAnswerTranscriptLocalAnswer,
  applyQuestionAnswerTranscriptSubmission,
  isQuestionAnswerPromptMessage,
  parseQuestionAnswerPausedEvent,
  parseQuestionAnswerRequestedEvent,
  parseQuestionAnswerSubmittedEvent,
  type QuestionAnswerTranscriptItem,
} from '@/components/workflow/question-answer/runtime-events';
import { normalizeQuestionAnswerTranscript } from '@/components/workflow/question-answer/question-answer-transcript';
import {
  getWorkflowPrecheckWarnings,
  normalizeWorkflowBillingCode,
} from '@/utils/workflow/billing';
import { emitWebAppOffline, isWebAppOfflineError } from '@/utils/webapp/errors';

interface ParsedSseRunError {
  code?: string | number;
  message?: string;
}

interface UseWebappConversationTransportOptions {
  enablePrecheck?: boolean;
}

interface UseWebappConversationTransportResult {
  transport: ConversationTransport;
  precheckWarnings: WorkflowPrecheckWarning[];
  clearPrecheckWarnings: () => void;
  latestTaskId: string | null;
  approvalForm: ApprovalRuntimeFormData | null;
  approvalToken: string | null;
  approvalLoading: boolean;
  approvalError: unknown;
  approvalSubmitting: boolean;
  approvalSubmittedAction: string | null;
  questionAnswerPrompt: {
    question: string;
    choices: QuestionAnswerChoice[];
    round?: number;
  } | null;
  questionAnswerSubmitting: boolean;
  syncQuestionAnswerRuntime: (conversationId?: string) => void;
  submitApproval: (payload: { inputs: Record<string, unknown>; action: string }) => Promise<void>;
  submitQuestionAnswerChoice: (
    conversationId: string,
    choice: QuestionAnswerChoice
  ) => Promise<void>;
  retryApprovalForm: () => void;
  resumeWorkflowRun: (conversationId: string, message: Message) => void;
  continueWorkflowRun: (conversationId: string, message: Message) => void;
}

function getQuestionAnswerTranscriptFromMetadata(
  metadata: unknown
): QuestionAnswerTranscriptItem[] {
  if (!metadata || typeof metadata !== 'object') return [];
  const record = metadata as Record<string, unknown>;
  return normalizeQuestionAnswerTranscript(record.questionAnswerTranscript);
}

function getPendingQuestionAnswerPromptFromMessage(item: WebAppConversationMessageItem):
  | {
      question: string;
      choices: QuestionAnswerChoice[];
      round?: number;
    }
  | null {
  if (normalizeMessageRunStatus(item.status) !== 'pending_question') return null;
  const metadataPrompt = parseQuestionAnswerRequestedEvent(item.message_metadata?.questionAnswerPrompt);
  if (metadataPrompt) {
    return {
      question: metadataPrompt.question,
      choices: metadataPrompt.choices,
      round: metadataPrompt.round,
    };
  }
  const transcript = getQuestionAnswerTranscriptFromMetadata(item.message_metadata);
  for (let i = transcript.length - 1; i >= 0; i -= 1) {
    const entry = transcript[i];
    if (!entry.question || entry.answer) continue;
    return {
      question: entry.question,
      choices: [],
      round: entry.round,
    };
  }
  return null;
}

function parseSseRunError(error: unknown): ParsedSseRunError {
  const parsed =
    error && typeof error === 'object'
      ? (error as Record<string, unknown>)
      : typeof error === 'string'
        ? { message: error }
        : error instanceof Error
          ? { message: error.message }
          : {};

  return {
    code: parsed['code'] as string | number | undefined,
    message: parsed['message'] as string | undefined,
  };
}

function isWorkspaceNotFoundError(error: ParsedSseRunError): boolean {
  if (normalizeWorkflowBillingCode(error.code) === '205004') return true;
  return error.message?.toLowerCase() === 'workspace not found';
}

function stripQuestionAnswerPromptText(data: Record<string, unknown>): Record<string, unknown> {
  const next = { ...data };
  delete next.answer;
  delete next.text;
  delete next.content;
  delete next.delta;
  if (next.outputs && typeof next.outputs === 'object') {
    const outputs = { ...(next.outputs as Record<string, unknown>) };
    delete outputs.answer;
    delete outputs.text;
    next.outputs = outputs;
  }
  return next;
}

function hasPendingQuestionAnswerMessage(conversationId?: string): boolean {
  if (!conversationId) return false;
  const messages = useChatStore.getState().conversations[conversationId]?.messages ?? [];
  const latestMessage = messages[messages.length - 1];
  return (
    latestMessage?.WorkflowRunInfo?.status === 'pending_question' ||
    latestMessage?.clientState?.status === 'pending_question'
  );
}

function getPendingQuestionAnswerPromptFromRuntimeMessage(message?: Message):
  | {
      question: string;
      choices: QuestionAnswerChoice[];
      round?: number;
    }
  | null {
  const runStatus = message?.WorkflowRunInfo?.status ?? message?.clientState?.status;
  if (runStatus !== 'pending_question') return null;

  const metadata =
    message?.messageData?.metadata && typeof message.messageData.metadata === 'object'
      ? (message.messageData.metadata as Record<string, unknown>)
      : undefined;
  const metadataPrompt = parseQuestionAnswerRequestedEvent(
    message?.messageData?.questionAnswerPrompt ?? metadata?.questionAnswerPrompt
  );
  if (metadataPrompt) {
    return {
      question: metadataPrompt.question,
      choices: metadataPrompt.choices,
      round: metadataPrompt.round,
    };
  }

  const transcript = normalizeQuestionAnswerTranscript(
    message?.messageData?.questionAnswerTranscript ?? metadata?.questionAnswerTranscript
  );
  for (let i = transcript.length - 1; i >= 0; i -= 1) {
    const entry = transcript[i];
    if (!entry.question || entry.answer) continue;
    return {
      question: entry.question,
      choices: [],
      round: entry.round,
    };
  }
  return null;
}

// Map WebAppConversation to ConversationSummary
function mapWebAppConversationToSummary(item: WebAppConversation): ConversationSummary {
  return {
    id: item.id,
    conversationId: item.id,
    title: item.name,
    dialogueCount: item.dialogue_count,
    updatedAt: item.updated_at * 1000,
    status: item.status,
    metadata: {
      workflow_version_uuid: item.workflow_version_uuid,
      invoke_from: item.invoke_from,
      created_at: item.created_at,
    },
  };
}

// Map WebAppConversationMessageItem to Message
function mapWebAppMessageToMessage(item: WebAppConversationMessageItem): Message {
  const runStatus = normalizeMessageRunStatus(item.status);
  const questionAnswerTranscript = getQuestionAnswerTranscriptFromMetadata(item.message_metadata);

  return {
    messageId: item.id,
    query: item.query,
    answer: item.answer,
    parentId: '',
    model: null,
    clientState: {
      phase: runStatus === 'running' ? 'streaming' : 'completed',
      status: runStatus && runStatus !== 'running' ? runStatus : undefined,
      finishedAt: item.created_at * 1000,
    },
    WorkflowRunInfo:
      item.workflow_run_id && runStatus
        ? {
            id: item.workflow_run_id,
            status: runStatus,
            runNodeInfo: [],
          }
        : undefined,
    messageData: {
      ...(item.workflow_run_id ? { tempKey: `restore:${item.workflow_run_id}` } : {}),
      workflow_run_id: item.workflow_run_id,
      message_id: item.id,
      created_at: item.created_at,
      status: item.status,
      inputs: item.inputs,
      ...(item.message_metadata ? { metadata: item.message_metadata } : {}),
      ...(questionAnswerTranscript.length > 0 ? { questionAnswerTranscript } : {}),
    },
  };
}

function normalizeFinalRunStatus(status: unknown): TerminalRunStatus {
  const normalized = normalizeMessageRunStatus(status);
  if (normalized === 'completed' || normalized === 'stopped' || normalized === 'expired') {
    return normalized;
  }
  return 'error';
}

// Map WebAppConversationDetail to ConversationDetail
function mapWebAppConversationDetailToDetail(data: WebAppConversationDetail): ConversationDetail {
  const latestPendingQuestion = [...data.messages]
    .reverse()
    .map(getPendingQuestionAnswerPromptFromMessage)
    .find(Boolean);

  return {
    summary: {
      id: data.id,
      conversationId: data.id,
      title: data.name,
      dialogueCount: data.dialogue_count,
      updatedAt: data.updated_at * 1000,
      status: data.status,
      metadata: {
        agent_id: data.agent_id,
        mode: data.mode,
        workflow_version_uuid: data.workflow_version_uuid,
        invoke_from: data.invoke_from,
        created_at: data.created_at,
        inputs: data.inputs,
        ...(latestPendingQuestion ? { questionAnswerPrompt: latestPendingQuestion } : {}),
      },
    },
    messages: data.messages.map(mapWebAppMessageToMessage),
    loaded: true,
    loading: false,
  };
}

export function useWebappConversationTransport(
  versionUuid: string,
  options: UseWebappConversationTransportOptions = {}
): UseWebappConversationTransportResult {
  const t = useT();
  const { start } = useRunWebAppWorkflowStream(versionUuid);
  const { start: startWorkflowRunEvents, cancel: cancelWorkflowRunEvents } =
    useWorkflowRunEventsStream();
  const precheckMutation = useWebAppPrecheck(versionUuid);
  const { notifyBillingError, getWorkflowRunErrorText } = useWorkflowBillingFeedback('webapp');
  const [precheckWarnings, setPrecheckWarnings] = useState<WorkflowPrecheckWarning[]>([]);
  const [latestTaskId, setLatestTaskId] = useState<string | null>(null);
  const [questionAnswerPrompt, setQuestionAnswerPrompt] = useState<{
    question: string;
    choices: QuestionAnswerChoice[];
    round?: number;
  } | null>(null);
  const [questionAnswerSubmitting, setQuestionAnswerSubmitting] = useState(false);
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
  const activeRunCallbacksRef = useRef<ChatRunCallbacks | null>(null);
  const abortSignalRef = useRef<AbortSignal | undefined>(undefined);
  const restoredRunRef = useRef<string | null>(null);
  const workflowFinishedRef = useRef(false);
  const approvalRuntimeStateRef = useRef(approvalRuntimeState);
  const questionAnswerTranscriptRef = useRef<QuestionAnswerTranscriptItem[]>([]);
  const questionAnswerPendingRef = useRef(false);
  const approvalFormQuery = useApprovalForm(approvalToken, Boolean(approvalToken && !approvalForm));
  const approvalSubmitMutation = useSubmitApprovalForm(approvalToken);
  const clearPrecheckWarnings = useCallback(() => setPrecheckWarnings([]), []);
  const resetQuestionAnswerRuntime = useCallback(() => {
    questionAnswerTranscriptRef.current = [];
    questionAnswerPendingRef.current = false;
    setQuestionAnswerPrompt(null);
    setQuestionAnswerSubmitting(false);
  }, []);
  const syncQuestionAnswerRuntime = useCallback((conversationId?: string) => {
    const messages = conversationId
      ? (useChatStore.getState().conversations[conversationId]?.messages ?? [])
      : [];
    const latestMessage = messages[messages.length - 1];
    const prompt = getPendingQuestionAnswerPromptFromRuntimeMessage(latestMessage);

    questionAnswerTranscriptRef.current = normalizeQuestionAnswerTranscript(
      latestMessage?.messageData?.questionAnswerTranscript ??
        (latestMessage?.messageData?.metadata &&
        typeof latestMessage.messageData.metadata === 'object'
          ? (latestMessage.messageData.metadata as Record<string, unknown>).questionAnswerTranscript
          : undefined)
    );
    questionAnswerPendingRef.current = Boolean(prompt);
    setQuestionAnswerPrompt(prompt);
    if (!prompt) {
      setQuestionAnswerSubmitting(false);
    }
  }, []);

  useEffect(() => {
    approvalRuntimeStateRef.current = approvalRuntimeState;
  }, [approvalRuntimeState]);

  const hasUnresolvedApprovals = useCallback(
    () => hasUnresolvedApprovalEntries(approvalRuntimeStateRef.current),
    []
  );

  const handleQuestionAnswerRequested = useCallback(
    (payload: unknown, callbacks?: ChatRunCallbacks) => {
      const parsed = parseQuestionAnswerRequestedEvent(payload);
      if (!parsed) return;
      const runCallbacks = callbacks;
      const transcript = appendQuestionAnswerTranscriptQuestion(
        questionAnswerTranscriptRef.current,
        parsed
      );
      const prompt = {
        question: parsed.question,
        choices: parsed.choices,
        round: parsed.round,
      };
      questionAnswerTranscriptRef.current = transcript;
      questionAnswerPendingRef.current = true;
      runCallbacks?.mergeMessageData?.({
        questionAnswerTranscript: transcript,
        questionAnswerPrompt: prompt,
      });
      setQuestionAnswerPrompt({
        question: parsed.question,
        choices: parsed.choices,
        round: parsed.round,
      });
      setQuestionAnswerSubmitting(false);
      runCallbacks?.onPaused?.({
        workflowRunId: parsed.workflowRunId,
        nodeIds: parsed.nodeId ? [parsed.nodeId] : [],
        status: 'pending_question',
        nodeType: 'question-answer',
      });
      if (parsed.nodeId) {
        runCallbacks?.onNodeFinished?.({
          status: 'paused',
          nodeId: parsed.nodeId,
          nodeType: 'question-answer',
          title: parsed.nodeTitle || parsed.nodeId,
        });
      }
    },
    []
  );

  const handleQuestionAnswerSubmitted = useCallback((payload?: unknown, callbacks?: ChatRunCallbacks) => {
    const parsed = parseQuestionAnswerSubmittedEvent(payload);
    if (parsed) {
      const transcript = applyQuestionAnswerTranscriptSubmission(
        questionAnswerTranscriptRef.current,
        parsed
      );
      questionAnswerTranscriptRef.current = transcript;
      callbacks?.mergeMessageData?.({
        questionAnswerTranscript: transcript,
        questionAnswerPrompt: null,
      });
    }
    questionAnswerPendingRef.current = false;
    setQuestionAnswerPrompt(null);
    setQuestionAnswerSubmitting(true);
  }, []);

  const handleWorkflowPaused = useCallback(
    (payload: unknown, callbacks: ChatRunCallbacks) => {
      const parsed = parseApprovalPausedEvent(payload);
      const data = unwrap(payload) as Record<string, unknown>;
      const workflowRunId =
        (typeof data.id === 'string' ? data.id : '') ||
        (typeof data.workflow_run_id === 'string' ? data.workflow_run_id : '');
      if (parsed.isApproval) {
        dispatchApprovalRuntimeEvent('workflow_paused', payload);
        callbacks.onPaused?.({
          elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
          workflowRunId: workflowRunId || undefined,
          nodeIds: parsed.nodeIds,
          status: 'pending_approval',
          nodeType: 'approval',
        });
        return;
      }

      const qaPaused = parseQuestionAnswerPausedEvent(payload);
      if (!qaPaused.isQuestionAnswer) return;
      if (qaPaused.prompt) {
        handleQuestionAnswerRequested(qaPaused.prompt, callbacks);
      }
      callbacks.onPaused?.({
        elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
        workflowRunId: workflowRunId || qaPaused.workflowRunId,
        nodeIds: qaPaused.nodeIds,
        status: 'pending_question',
        nodeType: 'question-answer',
      });
    },
    [dispatchApprovalRuntimeEvent, handleQuestionAnswerRequested]
  );

  const handleApprovalRequested = useCallback((payload: unknown) => {
    const parsed = parseApprovalRequestedEvent(payload);
    if (!parsed.form && !parsed.token && !parsed.formId && !parsed.nodeId) return;
    dispatchApprovalRuntimeEvent('approval_requested', payload);
  }, [dispatchApprovalRuntimeEvent]);

  const handleApprovalResultFilled = useCallback((payload: unknown) => {
    dispatchApprovalRuntimeEvent('approval_result_filled', payload);
  }, [dispatchApprovalRuntimeEvent]);

  const handleApprovalExpired = useCallback(
    (payload: unknown) => {
      dispatchApprovalRuntimeEvent('approval_expired', payload);
      toast.info(t('nodes.approval.runtime.expired'));
    },
    [dispatchApprovalRuntimeEvent, t]
  );

  useEffect(() => {
    if (approvalFormQuery.data) {
      setLoadedApprovalForm(approvalFormQuery.data);
    }
  }, [approvalFormQuery.data, setLoadedApprovalForm]);

  const dispatchApprovalEvent = useCallback(
    (event: { event?: string; data?: unknown; [key: string]: unknown }) => {
      const callbacks = activeRunCallbacksRef.current;
      if (!callbacks) return;

      const payload = event.data && typeof event.data === 'object' ? event.data : event;
      const data = unwrap(payload) as Record<string, unknown>;

      switch (event.event) {
        case 'workflow_started': {
          const inputs =
            data.inputs && typeof data.inputs === 'object'
              ? (data.inputs as Record<string, unknown>)
              : undefined;
          const conversationId =
            (typeof data.conversation_id === 'string' ? data.conversation_id : '') ||
            (typeof inputs?.['sys.conversation_id'] === 'string'
              ? (inputs['sys.conversation_id'] as string)
              : '');
          setLatestTaskId(
            (typeof data.task_id === 'string' ? data.task_id : null) ??
              (typeof data.id === 'string' ? data.id : null) ??
              (typeof data.workflow_run_id === 'string' ? data.workflow_run_id : null)
          );
          callbacks.onStarted({
            conversationId,
            messageId: typeof data.message_id === 'string' ? data.message_id : undefined,
            workflowRunId:
              (typeof data.id === 'string' ? data.id : '') ||
              (typeof data.workflow_run_id === 'string' ? data.workflow_run_id : '') ||
              undefined,
          });
          break;
        }
        case 'approval_requested':
          handleApprovalRequested(event);
          if (callbacks.onNodeFinished) {
            const parsed = parseApprovalRequestedEvent(event);
            if (parsed.form?.node_id) {
              callbacks.onNodeFinished({
                status: 'paused',
                nodeId: parsed.form.node_id,
                nodeType: 'approval',
                title: parsed.form.node_title || parsed.form.node_id,
              });
            }
          }
          break;
        case 'approval_result_filled':
          handleApprovalResultFilled(event);
          break;
        case 'approval_expired':
          handleApprovalExpired(event);
          callbacks.onFinished({
            status: 'expired',
            elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
            messageId: typeof data.message_id === 'string' ? data.message_id : undefined,
            workflowRunId:
              (typeof data.id === 'string' ? data.id : '') ||
              (typeof data.workflow_run_id === 'string' ? data.workflow_run_id : '') ||
              undefined,
            model: null,
          });
          break;
        case 'question_answer_requested':
          handleQuestionAnswerRequested(event, callbacks);
          break;
        case 'question_answer_submitted':
          handleQuestionAnswerSubmitted(event, callbacks);
          break;
        case 'node_started':
          setQuestionAnswerSubmitting(false);
          callbacks.onNodeStarted?.(mapNode(payload, false));
          break;
        case 'node_finished':
          callbacks.onNodeFinished?.(mapNode(payload, true));
          break;
        case 'message':
        case 'text_chunk':
          if (isQuestionAnswerPromptMessage(data)) {
            callbacks.onMessage(stripQuestionAnswerPromptText(data));
            break;
          }
          callbacks.onMessage(data);
          break;
        case 'message_end':
          callbacks.onMessageEnd?.(data);
          break;
        case 'workflow_paused':
          handleWorkflowPaused(event, callbacks);
          break;
        case 'workflow_finished':
        case 'workflow_stopped':
        case 'workflow_failed':
        case 'workflow_succeeded':
        case 'workflow_completed': {
          const isSuccessfulTerminalEvent =
            event.event === 'workflow_finished' ||
            event.event === 'workflow_succeeded' ||
            event.event === 'workflow_completed';
          if (isSuccessfulTerminalEvent && hasUnresolvedApprovals()) {
            callbacks.onPaused?.({
              workflowRunId:
                (typeof data.id === 'string' ? data.id : '') ||
                (typeof data.workflow_run_id === 'string' ? data.workflow_run_id : '') ||
                undefined,
            });
            break;
          }
          const status = typeof data.status === 'string' ? data.status.toLowerCase() : '';
          const eventStatus =
            event.event === 'workflow_stopped'
              ? 'stopped'
              : event.event === 'workflow_failed'
                ? 'error'
                : event.event === 'workflow_succeeded' || event.event === 'workflow_completed'
                  ? 'completed'
                  : '';
          const finalStatus = status || eventStatus;
          const terminalStatus = normalizeFinalRunStatus(finalStatus);
          const error = getWorkflowRunErrorText(data.error);
          callbacks.onFinished({
            status: terminalStatus,
            error,
            elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
            messageId: typeof data.message_id === 'string' ? data.message_id : undefined,
            workflowRunId:
              (typeof data.id === 'string' ? data.id : '') ||
              (typeof data.workflow_run_id === 'string' ? data.workflow_run_id : '') ||
              undefined,
            model: null,
          });
          if (
            (finalStatus === 'failed' || finalStatus === 'error') &&
            !abortSignalRef.current?.aborted
          ) {
            notifyBillingError(data.error);
          }
          setLatestTaskId(null);
            workflowFinishedRef.current = true;
            resetApprovalRuntime();
            setQuestionAnswerPrompt(null);
            setQuestionAnswerSubmitting(false);
            questionAnswerPendingRef.current = false;
            break;
        }
        case 'error': {
          if (isWebAppOfflineError(payload)) {
            emitWebAppOffline();
            callbacks.onError(new Error(t('webapp.offline.title')));
            setLatestTaskId(null);
            return;
          }
          const parsedError = parseSseRunError(payload);
          const reason = isWorkspaceNotFoundError(parsedError)
            ? t('webapp.chat.workspaceRequiredForConversation')
            : (getWorkflowRunErrorText(payload) ?? parsedError.message);
          callbacks.onError(new Error(reason || 'Unknown error'));
          setLatestTaskId(null);
          setQuestionAnswerSubmitting(false);
          if (!abortSignalRef.current?.aborted) {
            if (isWorkspaceNotFoundError(parsedError)) {
              toast.error(reason);
            } else {
              notifyBillingError(payload);
            }
          }
          break;
        }
      }
    },
    [
      getWorkflowRunErrorText,
      handleApprovalExpired,
      handleApprovalRequested,
      handleApprovalResultFilled,
      handleQuestionAnswerRequested,
      handleQuestionAnswerSubmitted,
      handleWorkflowPaused,
      hasUnresolvedApprovals,
      notifyBillingError,
      resetApprovalRuntime,
      t,
    ]
  );

  const submitApproval = useCallback(
    async (payload: { inputs: Record<string, unknown>; action: string }) => {
      if (!approvalToken || !approvalEntry) return;
      setApprovalSubmitting(approvalEntry.key, payload.action);
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
      t,
    ]
  );

  const submitQuestionAnswerChoice = useCallback(
    async (conversationId: string, choice: QuestionAnswerChoice) => {
      const callbacks = activeRunCallbacksRef.current;
      const query = String(choice.label || choice.value || choice.id || '').trim();
      if (!callbacks || !conversationId || !query) return;
      const message =
        useChatStore.getState().conversations[conversationId]?.messages.slice(-1)[0] ?? null;
      const inputs =
        message?.inputs && typeof message.inputs === 'object'
          ? (message.inputs as Record<string, unknown>)
          : {};
      const transcript = applyQuestionAnswerTranscriptLocalAnswer(
        questionAnswerTranscriptRef.current,
        query
      );
      questionAnswerTranscriptRef.current = transcript;
      callbacks.mergeMessageData?.({
        questionAnswerTranscript: transcript,
        questionAnswerPrompt: null,
      });
      setQuestionAnswerPrompt(null);
      setQuestionAnswerSubmitting(true);
      const runCallbacks: WebAppRunSseCallbacks = {
        onWorkflowStarted: payload =>
          dispatchApprovalEvent({ event: 'workflow_started', data: payload }),
        onNodeStarted: payload => dispatchApprovalEvent({ event: 'node_started', data: payload }),
        onNodeFinished: payload => dispatchApprovalEvent({ event: 'node_finished', data: payload }),
        onWorkflowPaused: payload => handleWorkflowPaused(payload, callbacks),
        onApprovalRequested: payload =>
          dispatchApprovalEvent({ event: 'approval_requested', data: payload }),
        onApprovalResultFilled: payload =>
          dispatchApprovalEvent({ event: 'approval_result_filled', data: payload }),
        onApprovalExpired: payload => dispatchApprovalEvent({ event: 'approval_expired', data: payload }),
        onQuestionAnswerRequested: payload =>
          dispatchApprovalEvent({ event: 'question_answer_requested', data: payload }),
        onQuestionAnswerSubmitted: payload =>
          dispatchApprovalEvent({ event: 'question_answer_submitted', data: payload }),
        onTextChunk: payload => dispatchApprovalEvent({ event: 'text_chunk', data: payload }),
        onTextReplace: payload => dispatchApprovalEvent({ event: 'text_replace', data: payload }),
        onWorkflowFinished: payload =>
          dispatchApprovalEvent({ event: 'workflow_finished', data: payload }),
        onError: payload => dispatchApprovalEvent({ event: 'error', data: payload }),
        onMessage: payload => dispatchApprovalEvent({ event: 'message', data: payload }),
        onMessageEnd: payload => dispatchApprovalEvent({ event: 'message_end', data: payload }),
      };
      await start(
        {
          query,
          conversation_id: conversationId,
          inputs: {
            ...inputs,
            question_answer_option_id: choice.id,
          },
        },
        runCallbacks,
        { abortSignal: abortSignalRef.current }
      );
    },
    [dispatchApprovalEvent, handleWorkflowPaused, start]
  );

  const startWorkflowRunEventStream = useCallback(
    (conversationId: string, message: Message, params?: WorkflowRunEventsStreamParams) => {
      const workflowRunId =
        (typeof message.WorkflowRunInfo?.id === 'string' ? message.WorkflowRunInfo.id : '') ||
        (typeof message.messageData?.workflow_run_id === 'string'
          ? (message.messageData.workflow_run_id as string)
          : '');
      if (!conversationId || !workflowRunId) return;
      if (restoredRunRef.current === workflowRunId) return;

      const tempKey =
        typeof message.messageData?.tempKey === 'string'
          ? (message.messageData.tempKey as string)
          : `restore:${workflowRunId}`;
      const messageId =
        message.messageId ||
        (typeof message.messageData?.message_id === 'string'
          ? (message.messageData.message_id as string)
          : undefined);

      restoredRunRef.current = workflowRunId;
      workflowFinishedRef.current = false;
      setLatestTaskId(workflowRunId);
      useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
        messageId,
        workflowRunId,
        conversationId,
      });

      const effectiveParams =
        params ??
        (approvalRuntimeState.cursor > 0
          ? { after: approvalRuntimeState.cursor, continue_on_pause: true }
          : { include_snapshot: true, continue_on_pause: true });

      void startWorkflowRunEvents(
        workflowRunId,
        {
          onWorkflowStarted: payload => {
            const data = unwrap(payload);
            const serverConversationId =
              typeof data.conversation_id === 'string'
                ? (data.conversation_id as string)
                : conversationId;
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              messageId:
                typeof data.message_id === 'string' ? (data.message_id as string) : messageId,
              workflowRunId,
              conversationId: serverConversationId,
            });
            useChatStore.getState().resumeAiMessage(conversationId, tempKey, { workflowRunId });
          },
          onApprovalRequested: payload => {
            handleApprovalRequested(payload);
            const parsed = parseApprovalRequestedEvent(payload);
            if (parsed.form?.node_id) {
              useChatStore.getState().updateRunNode(conversationId, tempKey, {
                status: 'paused',
                nodeId: parsed.form.node_id,
                nodeType: 'approval',
                title: parsed.form.node_title || parsed.form.node_id,
              });
            }
          },
          onApprovalResultFilled: handleApprovalResultFilled,
          onApprovalExpired: payload => {
            handleApprovalExpired(payload);
            const data = unwrap(payload);
            useChatStore.getState().finalizeAiMessage(conversationId, tempKey, {
              status: 'expired',
              elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
              messageId:
                typeof data.message_id === 'string' ? (data.message_id as string) : messageId,
              workflowRunId,
              model: null,
            });
            setLatestTaskId(null);
            restoredRunRef.current = null;
            cancelWorkflowRunEvents();
          },
          onQuestionAnswerRequested: payload => {
            handleQuestionAnswerRequested(payload);
            const parsed = parseQuestionAnswerRequestedEvent(payload);
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              messageData: {
                questionAnswerTranscript: questionAnswerTranscriptRef.current,
                ...(parsed
                  ? {
                      questionAnswerPrompt: {
                        question: parsed.question,
                        choices: parsed.choices,
                        round: parsed.round,
                      },
                    }
                  : {}),
              },
            });
            useChatStore.getState().pauseAiMessage(conversationId, tempKey, {
              workflowRunId: parsed?.workflowRunId || workflowRunId,
              status: 'pending_question',
            });
            if (parsed?.nodeId) {
              useChatStore.getState().updateRunNode(conversationId, tempKey, {
                status: 'paused',
                nodeId: parsed.nodeId,
                nodeType: 'question-answer',
                title: parsed.nodeTitle || parsed.nodeId,
              });
            }
          },
          onQuestionAnswerSubmitted: payload => {
            handleQuestionAnswerSubmitted(payload);
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              messageData: {
                questionAnswerTranscript: questionAnswerTranscriptRef.current,
                questionAnswerPrompt: null,
              },
            });
          },
          onWorkflowPaused: payload => {
            const parsed = parseApprovalPausedEvent(payload);
            const data = unwrap(payload);
            if (parsed.isApproval) {
              handleApprovalRequested(payload);
              useChatStore.getState().pauseAiMessage(conversationId, tempKey, {
                elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
                workflowRunId,
                status: 'pending_approval',
              });
              parsed.nodeIds.forEach(nodeId => {
                useChatStore.getState().updateRunNode(conversationId, tempKey, {
                  status: 'paused',
                  nodeId,
                  nodeType: 'approval',
                });
              });
              return;
            }
            const qaPaused = parseQuestionAnswerPausedEvent(payload);
            if (!qaPaused.isQuestionAnswer) return;
            if (qaPaused.prompt) {
              handleQuestionAnswerRequested(qaPaused.prompt);
              useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
                messageData: {
                  questionAnswerTranscript: questionAnswerTranscriptRef.current,
                  questionAnswerPrompt: {
                    question: qaPaused.prompt.question,
                    choices: qaPaused.prompt.choices,
                    round: qaPaused.prompt.round,
                  },
                },
              });
            }
            useChatStore.getState().pauseAiMessage(conversationId, tempKey, {
              elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
              workflowRunId: qaPaused.workflowRunId || workflowRunId,
              status: 'pending_question',
            });
            qaPaused.nodeIds.forEach(nodeId => {
              useChatStore.getState().updateRunNode(conversationId, tempKey, {
                status: 'paused',
                nodeId,
                nodeType: 'question-answer',
                title:
                  qaPaused.prompt?.nodeId === nodeId
                    ? qaPaused.prompt.nodeTitle || nodeId
                    : nodeId,
              });
            });
          },
          onNodeStarted: payload => {
            useChatStore.getState().resumeAiMessage(conversationId, tempKey, { workflowRunId });
            useChatStore.getState().updateRunNode(conversationId, tempKey, mapNode(payload, false));
          },
          onNodeFinished: payload => {
            useChatStore.getState().updateRunNode(conversationId, tempKey, mapNode(payload, true));
          },
          onTextChunk: payload => {
            const data = unwrap(payload);
            const chunk =
              typeof payload === 'string'
                ? payload
                : typeof data.text === 'string'
                  ? (data.text as string)
                  : typeof data.answer === 'string'
                    ? (data.answer as string)
                    : typeof data.delta === 'string'
                      ? (data.delta as string)
                      : '';
            if (!chunk) return;
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              answer: chunk,
              answerMode: 'append',
              messageId,
              workflowRunId,
              conversationId,
            });
          },
          onMessage: payload => {
            const data = unwrap(payload);
            const messageData = isQuestionAnswerPromptMessage(data)
              ? stripQuestionAnswerPromptText(data)
              : data;
            const answer =
              typeof messageData.answer === 'string'
                ? (messageData.answer as string)
                : typeof messageData.text === 'string'
                  ? (messageData.text as string)
                  : typeof messageData.content === 'string'
                    ? (messageData.content as string)
                    : typeof messageData.delta === 'string'
                      ? (messageData.delta as string)
                      : '';
            const currentMessage = useChatStore
              .getState()
              .conversations[
                conversationId
              ]?.messages.find(item => item.messageData?.tempKey === tempKey);
            const answerMode = resolveAnswerMergeMode(currentMessage?.answer ?? '', answer);
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              ...(answerMode !== 'skip' ? { answer, answerMode } : {}),
              messageId:
                typeof messageData.message_id === 'string'
                  ? (messageData.message_id as string)
                  : messageId,
              workflowRunId:
                (typeof messageData.workflow_run_id === 'string'
                  ? (messageData.workflow_run_id as string)
                  : '') ||
                (typeof messageData.id === 'string' ? (messageData.id as string) : '') ||
                workflowRunId,
              conversationId:
                typeof messageData.conversation_id === 'string'
                  ? (messageData.conversation_id as string)
                  : conversationId,
            });
          },
          onMessageEnd: payload => {
            const data = unwrap(payload);
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              messageId:
                typeof data.message_id === 'string' ? (data.message_id as string) : messageId,
              workflowRunId:
                (typeof data.workflow_run_id === 'string'
                  ? (data.workflow_run_id as string)
                  : '') ||
                (typeof data.id === 'string' ? (data.id as string) : '') ||
                workflowRunId,
              conversationId:
                typeof data.conversation_id === 'string'
                  ? (data.conversation_id as string)
                  : conversationId,
              metadata:
                data.metadata && typeof data.metadata === 'object'
                  ? (data.metadata as Record<string, unknown>)
                  : undefined,
            });
          },
          onWorkflowFinished: payload => {
            const data = unwrap(payload);
            const rawStatus = typeof data.status === 'string' ? data.status.toLowerCase() : '';
            const isSuccessfulTerminalStatus = !['failed', 'error', 'stopped', 'expired'].includes(
              rawStatus
            );
            if (isSuccessfulTerminalStatus && hasUnresolvedApprovals()) {
              useChatStore.getState().pauseAiMessage(conversationId, tempKey, {
                workflowRunId,
              });
              return;
            }
            const status = normalizeFinalRunStatus(data.status);
            useChatStore.getState().finalizeAiMessage(conversationId, tempKey, {
              status,
              error: getWorkflowRunErrorText(data.error),
              elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
              messageId:
                typeof data.message_id === 'string' ? (data.message_id as string) : messageId,
              workflowRunId,
              model: null,
            });
            setLatestTaskId(null);
            workflowFinishedRef.current = true;
            resetApprovalRuntime();
            restoredRunRef.current = null;
            cancelWorkflowRunEvents();
          },
          onError: payload => {
            const errorText =
              getWorkflowRunErrorText(payload) ?? t('webapp.chat.workflowRunFailed');
            useChatStore.getState().finalizeAiMessage(conversationId, tempKey, {
              status: 'error',
              error: errorText,
              messageId,
              workflowRunId,
            });
            setLatestTaskId(null);
            restoredRunRef.current = null;
          },
        },
        effectiveParams,
        {
          onClose: () => {
            restoredRunRef.current = null;
            if (!workflowFinishedRef.current) {
              window.setTimeout(() => {
                if (!workflowFinishedRef.current) {
                  startWorkflowRunEventStream(conversationId, message);
                }
              }, 1000);
            }
          },
        }
      );
    },
    [
      approvalRuntimeState.cursor,
      cancelWorkflowRunEvents,
      getWorkflowRunErrorText,
      handleApprovalExpired,
      handleApprovalRequested,
      handleApprovalResultFilled,
      handleQuestionAnswerRequested,
      handleQuestionAnswerSubmitted,
      hasUnresolvedApprovals,
      resetApprovalRuntime,
      startWorkflowRunEvents,
      t,
    ]
  );

  const resumeWorkflowRun = useCallback(
    (conversationId: string, message: Message) => {
      startWorkflowRunEventStream(conversationId, message, {
        include_snapshot: true,
        continue_on_pause: true,
      });
    },
    [startWorkflowRunEventStream]
  );

  const continueWorkflowRun = useCallback(
    (conversationId: string, message: Message) => {
      startWorkflowRunEventStream(conversationId, message);
    },
    [startWorkflowRunEventStream]
  );

  useEffect(() => {
    if (!approvalToken || !approvalSubmittedAction) return;
    let cancelled = false;
    const timer = window.setInterval(async () => {
      try {
        const events = await fetchApprovalEvents(approvalToken, {
          after: approvalRuntimeState.cursor,
          limit: 100,
        });
        if (cancelled || events.length === 0) return;
        events.forEach(event => {
          dispatchApprovalEvent(event);
        });
      } catch {
        // Keep polling; transient failures should not discard the paused approval UI.
      }
    }, 2000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [
    approvalRuntimeState.cursor,
    approvalSubmittedAction,
    approvalToken,
    dispatchApprovalEvent,
  ]);

  const transport = useMemo<ConversationTransport>(
    () => ({
      async list(params: {
        page: number;
        limit: number;
      }): Promise<{ items: ConversationSummary[]; pagination: Pagination }> {
        try {
          const resp = await queryClient.fetchQuery({
            queryKey: WEBAPP_KEYS.conversationList(versionUuid, params),
            queryFn: () => WebAppService.getConversations(versionUuid, params),
            staleTime: 30 * 1000,
            gcTime: 5 * 60 * 1000,
            retry: false,
          });
          const { data, has_more, limit, page, total } = resp.data;
          return {
            items: data.map(mapWebAppConversationToSummary),
            pagination: { page, limit, total, hasMore: has_more },
          };
        } catch (err) {
          if (isWebAppOfflineError(err)) {
            emitWebAppOffline();
          }
          console.error('[WebappTransportHook] Failed to list conversations:', err);
          return {
            items: [],
            pagination: { page: params.page, limit: params.limit, total: 0, hasMore: false },
          };
        }
      },

      async get(conversationId: string): Promise<ConversationDetail> {
        try {
          const response = await WebAppService.getConversation(versionUuid, conversationId);
          const detail = mapWebAppConversationDetailToDetail(response.data);
          const restoredPrompt =
            detail.summary.metadata?.questionAnswerPrompt &&
            typeof detail.summary.metadata.questionAnswerPrompt === 'object'
              ? (detail.summary.metadata.questionAnswerPrompt as {
                  question: string;
                  choices: QuestionAnswerChoice[];
                  round?: number;
                })
              : null;
          setQuestionAnswerPrompt(restoredPrompt);
          questionAnswerTranscriptRef.current = normalizeQuestionAnswerTranscript(
            detail.messages.at(-1)?.messageData?.questionAnswerTranscript
          );
          questionAnswerPendingRef.current = Boolean(restoredPrompt);
          return detail;
        } catch (err) {
          if (isWebAppOfflineError(err)) {
            emitWebAppOffline();
          }
          console.error('[WebappTransportHook] Failed to get conversation:', err);
          throw err as Error;
        }
      },

      async create(payload?: { title?: string }): Promise<ConversationSummary> {
        resetQuestionAnswerRuntime();
        const draft: ConversationSummary = {
          id: `draft-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
          conversationId: '',
          title: payload?.title ?? `New conversation ${new Date().toLocaleString()}`,
          dialogueCount: 0,
          updatedAt: Date.now(),
          status: 'draft',
        };
        return draft;
      },

      async remove(conversationId: string): Promise<void> {
        // Skip delete for draft conversations (no server-side record)
        if (!conversationId || conversationId.startsWith('draft-')) {
          return Promise.resolve();
        }
        try {
          await WebAppService.deleteConversation(versionUuid, conversationId);
          // Invalidate conversation list cache
          queryClient.invalidateQueries({
            queryKey: WEBAPP_KEYS.conversationList(versionUuid, null),
          });
        } catch (err) {
          if (isWebAppOfflineError(err)) {
            emitWebAppOffline();
          }
          console.error('[WebappTransportHook] Failed to delete conversation:', err);
          throw err;
        }
      },

      send(
        payload: SendMessagePayload,
        callbacks: ChatRunCallbacks,
        abortSignal?: AbortSignal
      ): void {
        activeRunCallbacksRef.current = callbacks;
        abortSignalRef.current = abortSignal;
        let hasErrorToast = false;
        const showWorkflowErrorToast = (reason?: string) => {
          if (hasErrorToast) return;
          hasErrorToast = true;
          toast.error(
            reason?.trim()
              ? t('webapp.chat.workflowRunFailedWithReason', { reason: reason.trim() })
              : t('webapp.chat.workflowRunFailed')
          );
        };
        const runPayload: WebAppRunRequest = {
          query: payload.query,
          conversation_id: payload.conversationId,
          history_window_size: payload.historyWindowSize,
          files: payload.files,
          inputs: payload.inputs,
        };

        void (async () => {
          try {
            if (options.enablePrecheck) {
              const precheck = await precheckMutation.mutateAsync(runPayload);
              const warnings = getWorkflowPrecheckWarnings(precheck.data);
              if (precheck.data.status === 'warning' && warnings.length > 0) {
                setPrecheckWarnings(warnings);
              } else {
                clearPrecheckWarnings();
              }
            } else {
              clearPrecheckWarnings();
            }
            workflowFinishedRef.current = false;
            resetApprovalRuntime();
            setQuestionAnswerPrompt(null);
            setQuestionAnswerSubmitting(false);
            const isQuestionAnswerResume = hasPendingQuestionAnswerMessage(payload.conversationId);
            if (isQuestionAnswerResume) {
              const transcript = applyQuestionAnswerTranscriptLocalAnswer(
                questionAnswerTranscriptRef.current,
                payload.query
              );
              questionAnswerTranscriptRef.current = transcript;
              questionAnswerPendingRef.current = false;
              callbacks.mergeMessageData?.({
                questionAnswerTranscript: transcript,
                questionAnswerPrompt: null,
              });
              setQuestionAnswerSubmitting(true);
            } else {
              questionAnswerTranscriptRef.current = [];
              questionAnswerPendingRef.current = false;
            }

            await start(
              runPayload,
              {
                onWorkflowStarted: (ctx: unknown) => {
                  workflowFinishedRef.current = false;
                  const data = unwrap(ctx) as {
                    conversation_id?: string;
                    message_id?: string;
                    tempKey?: string;
                    task_id?: string;
                    id?: string;
                    workflow_run_id?: string;
                    inputs?: {
                      'sys.conversation_id'?: string;
                    };
                  };
                  const conversationId =
                    data.conversation_id || data.inputs?.['sys.conversation_id'] || '';
                  setLatestTaskId(data.task_id ?? data.id ?? data.workflow_run_id ?? null);

                  callbacks.onStarted({
                    conversationId,
                    messageId: data.message_id,
                    workflowRunId: data.id ?? data.workflow_run_id ?? data.task_id,
                    tempKey: data.tempKey,
                  });
                },
                onTextChunk: (token: unknown) => {
                  if (typeof token === 'string') {
                    callbacks.onToken(token);
                  } else if (token && typeof token === 'object') {
                    const tokenObject = token as Record<string, unknown>;
                    const s =
                      typeof tokenObject['text'] === 'string'
                        ? (tokenObject['text'] as string)
                        : typeof tokenObject['answer'] === 'string'
                          ? (tokenObject['answer'] as string)
                          : typeof tokenObject['delta'] === 'string'
                            ? (tokenObject['delta'] as string)
                            : '';
                    callbacks.onToken(s);
                  } else {
                    callbacks.onToken(String(token ?? ''));
                  }
                },
                onTextReplace: () => {
                  callbacks.onTextReplace?.();
                },
                onNodeStarted: (node: unknown) => {
                  if (callbacks.onNodeStarted) callbacks.onNodeStarted(mapNode(node, false));
                },
                onNodeFinished: (node: unknown) => {
                  if (callbacks.onNodeFinished) callbacks.onNodeFinished(mapNode(node, true));
                },
                onWorkflowPaused: (ctx: unknown) => {
                  handleWorkflowPaused(ctx, callbacks);
                },
                onApprovalRequested: (payload: unknown) => {
                  handleApprovalRequested(payload);
                  const parsed = parseApprovalRequestedEvent(payload);
                  if (parsed.form?.node_id) {
                    callbacks.onNodeFinished?.({
                      status: 'paused',
                      nodeId: parsed.form.node_id,
                      nodeType: 'approval',
                      title: parsed.form.node_title || parsed.form.node_id,
                    });
                  }
                },
                onApprovalResultFilled: handleApprovalResultFilled,
                onApprovalExpired: (payload: unknown) => {
                  handleApprovalExpired(payload);
                  const data = unwrap(payload) as Record<string, unknown>;
                  callbacks.onFinished({
                    status: 'expired',
                    elapsedTime:
                      typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
                    messageId:
                      typeof data.message_id === 'string' ? (data.message_id as string) : undefined,
                    workflowRunId:
                      (typeof data.id === 'string' ? data.id : '') ||
                      (typeof data.workflow_run_id === 'string'
                        ? (data.workflow_run_id as string)
                        : '') ||
                      undefined,
                    model: null,
                  });
                  setLatestTaskId(null);
                },
                onQuestionAnswerRequested: (payload: unknown) => {
                  handleQuestionAnswerRequested(payload, callbacks);
                },
                onQuestionAnswerSubmitted: (payload: unknown) => {
                  handleQuestionAnswerSubmitted(payload, callbacks);
                },
                onMessage: (meta: unknown) => {
                  const data = unwrap(meta);
                  callbacks.onMessage(
                    isQuestionAnswerPromptMessage(data)
                      ? stripQuestionAnswerPromptText(data)
                      : data
                  );
                },
                onMessageEnd: (meta: unknown) => {
                  if (callbacks.onMessageEnd) callbacks.onMessageEnd(unwrap(meta));
                },
                onWorkflowFinished: (ctx: unknown) => {
                  const terminalData = unwrap(ctx) as {
                    id?: string;
                    workflow_run_id?: string;
                    status?: string;
                  };
                  const rawStatus =
                    typeof terminalData.status === 'string'
                      ? terminalData.status.toLowerCase()
                      : '';
                  const isSuccessfulTerminalStatus = ![
                    'failed',
                    'error',
                    'stopped',
                    'expired',
                  ].includes(rawStatus);
                  if (isSuccessfulTerminalStatus && hasUnresolvedApprovals()) {
                    callbacks.onPaused?.({
                      workflowRunId:
                        (typeof terminalData.id === 'string' ? terminalData.id : '') ||
                        (typeof terminalData.workflow_run_id === 'string'
                          ? terminalData.workflow_run_id
                          : '') ||
                        undefined,
                    });
                    return;
                  }
                  const data = terminalData as {
                    id?: string;
                    workflow_run_id?: string;
                    status?: string;
                    error?: unknown;
                    elapsed_time?: number;
                    message_id?: string;
                  };
                  const s = typeof data.status === 'string' ? data.status.toLowerCase() : '';
                  const status = normalizeFinalRunStatus(s);
                  const err = getWorkflowRunErrorText(data.error);

                  callbacks.onFinished({
                    status,
                    error: err,
                    elapsedTime: data.elapsed_time,
                    messageId: data.message_id,
                    workflowRunId:
                      (typeof data.id === 'string' ? data.id : '') ||
                      (typeof data.workflow_run_id === 'string' ? data.workflow_run_id : '') ||
                      undefined,
                    model: null,
                  });

                  if (status === 'error' && !abortSignal?.aborted) {
                    notifyBillingError(data.error);
                  }
                  setLatestTaskId(null);
                  workflowFinishedRef.current = true;
                  resetApprovalRuntime();
                  setQuestionAnswerPrompt(null);
                  setQuestionAnswerSubmitting(false);
                  questionAnswerPendingRef.current = false;
                },
                onError: (err: unknown) => {
                  if (isWebAppOfflineError(err)) {
                    emitWebAppOffline();
                    setLatestTaskId(null);
                    callbacks.onError(new Error(t('webapp.offline.title')));
                    return;
                  }
                  const parsedError = parseSseRunError(err);
                  const reason = isWorkspaceNotFoundError(parsedError)
                    ? t('webapp.chat.workspaceRequiredForConversation')
                    : (getWorkflowRunErrorText(err) ?? parsedError.message);
                  const normalizedError = new Error(reason || 'Unknown error');
                  setLatestTaskId(null);
                  workflowFinishedRef.current = true;
                  resetApprovalRuntime();
                  setQuestionAnswerSubmitting(false);
                  questionAnswerPendingRef.current = false;
                  callbacks.onError(normalizedError);
                  if (!abortSignal?.aborted) {
                    if (isWorkspaceNotFoundError(parsedError)) {
                      showWorkflowErrorToast(reason);
                    } else {
                      notifyBillingError(err);
                    }
                  }
                },
              },
              { abortSignal }
            );
          } catch (error) {
            if (isWebAppOfflineError(error)) {
              emitWebAppOffline();
              callbacks.onError(new Error(t('webapp.offline.title')));
              return;
            }
            const reason =
              error instanceof Error ? error.message : t('webapp.chat.workflowRunFailed');
            callbacks.onError(error instanceof Error ? error : new Error(reason));
            if (!abortSignal?.aborted) {
              showWorkflowErrorToast(reason);
            }
          }
        })();
      },
    }),
    [
      clearPrecheckWarnings,
      getWorkflowRunErrorText,
            handleApprovalExpired,
            handleApprovalRequested,
            handleApprovalResultFilled,
            handleQuestionAnswerRequested,
            handleQuestionAnswerSubmitted,
            handleWorkflowPaused,
      hasUnresolvedApprovals,
      notifyBillingError,
      options.enablePrecheck,
      precheckMutation,
      resetApprovalRuntime,
      resetQuestionAnswerRuntime,
      start,
      t,
      versionUuid,
    ]
  );

  return {
    transport,
    precheckWarnings,
    clearPrecheckWarnings,
    latestTaskId,
    approvalForm,
    approvalToken,
    approvalLoading: Boolean(
      approvalToken &&
        !approvalForm &&
        (approvalFormQuery.isLoading || approvalFormQuery.isFetching)
    ),
    approvalError: approvalFormQuery.error,
    approvalSubmitting: approvalSubmitMutation.isPending || approvalRuntimeSubmitting,
    approvalSubmittedAction,
    questionAnswerPrompt,
    questionAnswerSubmitting,
    syncQuestionAnswerRuntime,
    submitApproval,
    submitQuestionAnswerChoice,
    retryApprovalForm: () => void approvalFormQuery.refetch(),
    resumeWorkflowRun,
    continueWorkflowRun,
  };
}
