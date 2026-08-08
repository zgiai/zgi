import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type {
  ConversationTransport,
  ConversationSummary,
  ConversationDetail,
  Pagination,
  SendMessagePayload,
  ChatRunCallbacks,
} from '@/components/chat/controllers/types';
import { useChatStore } from '@/components/chat/store';
import { WebAppService } from '@/services/webapp.service';
import { workflowService } from '@/services/workflow.service';
import type { WebAppRunRequest, WebAppRunSseCallbacks } from '@/services/types/webapp';
import type { WorkflowPrecheckWarning } from '@/services/types/workflow';
import type { QuestionAnswerChoice } from '@/services/types/workflow';
import { toast } from 'sonner';
import { useRunWebAppWorkflowStream } from './use-run-webapp-workflow-stream';
import { useWorkflowRunEventsStream } from '@/hooks/workflow/use-workflow-run-events-stream';
import {
  WorkflowConversationRuntimeController,
  type WorkflowConnectionState,
} from '@/hooks/workflow/workflow-runtime-controller';
import { queryClient } from '@/lib/query-client';
import { unwrap, mapNode } from '@/utils/webapp/run-mappers';
import { WEBAPP_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { useWebAppPrecheck } from './use-webapp-precheck';
import { useWorkflowBillingFeedback } from '@/hooks/workflow/use-workflow-billing-feedback';
import { useApprovalForm, useSubmitApprovalForm } from '@/hooks/workflow/use-approval-form';
import {
  parseApprovalRequestedEvent,
} from '@/components/workflow/approval/runtime-events';
import { useApprovalRuntimeEvents } from '@/components/workflow/approval/use-approval-runtime-events';
import {
  appendQuestionAnswerTranscriptQuestion,
  applyQuestionAnswerTranscriptLocalAnswer,
  applyQuestionAnswerTranscriptSubmission,
  isQuestionAnswerPromptMessage,
  parseQuestionAnswerRequestedEvent,
  parseQuestionAnswerSubmittedEvent,
  type QuestionAnswerRuntimePromptState,
  type QuestionAnswerTranscriptItem,
} from '@/components/workflow/question-answer/runtime-events';
import { parseWorkflowPausedEvent } from '@/components/workflow/runtime/pause-events';
import { normalizeQuestionAnswerTranscript } from '@/components/workflow/question-answer/question-answer-transcript';
import { getWorkflowPrecheckWarnings } from '@/utils/workflow/billing';
import { resolveWorkflowRunId } from '@/utils/workflow/run-identity.js';
import { emitWebAppOffline, isWebAppOfflineError } from '@/utils/webapp/errors';
import { createWorkflowRunNodeAccumulator } from '@/utils/webapp/workflow-run-node-accumulator';
import type {
  UseWebappConversationTransportOptions,
  UseWebappConversationTransportResult,
} from './use-webapp-transport/types';
import {
  getPendingQuestionAnswerPromptFromRuntimeMessage,
  hasPendingQuestionAnswerMessage,
  isWorkflowConversationBusyError,
  isWorkspaceNotFoundError,
  mapWebAppConversationDetailToDetail,
  mapWebAppConversationToSummary,
  mapWebAppSearchResult,
  normalizeFinalRunStatus,
  parseSseRunError,
  stripQuestionAnswerPromptText,
} from './use-webapp-transport/mappers';
import { useWebappWorkflowRunEvents } from './use-webapp-transport/events';

export function useWebappConversationTransport(
  versionUuid: string,
  options: UseWebappConversationTransportOptions = {}
): UseWebappConversationTransportResult {
  const t = useT();
  const stopAgentIdRef = useRef(options.agentId);
  stopAgentIdRef.current = options.agentId;
  const conversationRuntimeControllerRef = useRef<WorkflowConversationRuntimeController | null>(
    null
  );
  if (!conversationRuntimeControllerRef.current) {
    conversationRuntimeControllerRef.current = new WorkflowConversationRuntimeController({
      stopRun: async workflowRunId => {
        const agentId = stopAgentIdRef.current;
        if (!agentId) throw new Error('workflow stop is unavailable');
        await workflowService.stopWorkflowTask(agentId, workflowRunId);
      },
    });
  }
  const { start } = useRunWebAppWorkflowStream(versionUuid);
  const { start: startWorkflowRunEvents, cancel: cancelWorkflowRunEvents } =
    useWorkflowRunEventsStream({ transport: 'webapp' });
  const precheckMutation = useWebAppPrecheck(versionUuid);
  const { notifyBillingError, getWorkflowRunErrorText } = useWorkflowBillingFeedback('webapp');
  const [precheckWarnings, setPrecheckWarnings] = useState<WorkflowPrecheckWarning[]>([]);
  const [latestTaskId, setLatestTaskId] = useState<string | null>(null);
  const [connectionStateByConversation, setConnectionStateByConversation] = useState<
    Record<string, WorkflowConnectionState>
  >({});
  const [questionAnswerPrompt, setQuestionAnswerPrompt] =
    useState<QuestionAnswerRuntimePromptState | null>(null);
  const [questionAnswerSubmitting, setQuestionAnswerSubmitting] = useState(false);
  const {
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
  const runCallbacksByConversationRef = useRef<Map<string, ChatRunCallbacks>>(new Map());
  const abortSignalRef = useRef<AbortSignal | undefined>(undefined);
  const restoredRunRef = useRef<Set<string>>(new Set());
  const questionAnswerTranscriptRef = useRef<QuestionAnswerTranscriptItem[]>([]);
  const questionAnswerPendingRef = useRef(false);
  const approvalFormQuery = useApprovalForm(approvalToken, Boolean(approvalToken && !approvalForm));
  const approvalSubmitMutation = useSubmitApprovalForm(approvalToken);
  const clearPrecheckWarnings = useCallback(() => setPrecheckWarnings([]), []);
  const setConversationConnectionState = useCallback(
    (conversationId: string, state: WorkflowConnectionState) => {
      setConnectionStateByConversation(current =>
        current[conversationId] === state ? current : { ...current, [conversationId]: state }
      );
    },
    []
  );
  const attachRun = useCallback((conversationId: string, workflowRunId: string) => {
    conversationRuntimeControllerRef.current?.attachRun(conversationId, workflowRunId);
  }, []);
  const recoverRun = useCallback((conversationId: string, workflowRunId: string) => {
    conversationRuntimeControllerRef.current?.recoverRun(conversationId, workflowRunId);
  }, []);
  const stopRun = useCallback(async (conversationId: string) => {
    await conversationRuntimeControllerRef.current?.stopRun(conversationId);
  }, []);
  const detachForeground = useCallback((conversationId: string) => {
    conversationRuntimeControllerRef.current?.detachForeground(conversationId);
  }, []);
  const disposeConversation = useCallback((conversationId: string) => {
    conversationRuntimeControllerRef.current?.disposeConversation(conversationId);
  }, []);
  const refreshConversationRuntime = useCallback(
    (conversationId?: string) => {
      void queryClient.invalidateQueries({
        queryKey: WEBAPP_KEYS.conversations(versionUuid),
      });
      if (conversationId) {
        void queryClient.invalidateQueries({
          queryKey: WEBAPP_KEYS.conversation(versionUuid, conversationId),
        });
      }
    },
    [versionUuid]
  );
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
  const reconcileConversationRun = useCallback(
    async (conversationId: string, workflowRunId: string): Promise<WorkflowConnectionState> => {
      try {
        const response = await WebAppService.getConversation(versionUuid, conversationId);
        const detail = mapWebAppConversationDetailToDetail(response.data);
        useChatStore.getState().initSingle({
          id: detail.summary.id,
          conversationId: detail.summary.conversationId,
          title: detail.summary.title,
          messages: detail.messages,
        });
        const message = detail.messages.find(item => item.WorkflowRunInfo?.id === workflowRunId);
        const status = String(
          message?.WorkflowRunInfo?.status ?? detail.summary.metadata?.runtime_status ?? 'idle'
        ).toLowerCase();
        return ['running', 'resuming', 'stopping', 'pending_approval', 'pending_question'].includes(
          status
        )
          ? 'disconnected'
          : 'idle';
      } catch {
        return 'disconnected';
      }
    },
    [versionUuid]
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
        nodeId: parsed.nodeId,
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
        nodeId: parsed.nodeId,
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

  const handleQuestionAnswerSubmitted = useCallback(
    (payload?: unknown, callbacks?: ChatRunCallbacks) => {
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
    },
    []
  );

  const handleWorkflowPaused = useCallback(
    (payload: unknown, callbacks: ChatRunCallbacks) => {
      const parsed = parseWorkflowPausedEvent(payload);
      const data = unwrap(payload) as Record<string, unknown>;
      const workflowRunId = resolveWorkflowRunId(payload, { allowLegacyId: true });
      if (parsed.hasApproval) {
        dispatchApprovalRuntimeEvent('workflow_paused', payload);
        callbacks.onPaused?.({
          elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
          workflowRunId: workflowRunId || undefined,
          nodeIds: parsed.approval.nodeIds,
          status: 'pending_approval',
          nodeType: 'approval',
        });
      }

      if (parsed.hasQuestionAnswer) {
        if (parsed.questionAnswer.prompt) {
          handleQuestionAnswerRequested(parsed.questionAnswer.prompt, callbacks);
        }
        callbacks.onPaused?.({
          elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
          workflowRunId: workflowRunId || parsed.questionAnswer.workflowRunId,
          nodeIds: parsed.questionAnswer.nodeIds,
          status: 'pending_question',
          nodeType: 'question-answer',
        });
      }
    },
    [dispatchApprovalRuntimeEvent, handleQuestionAnswerRequested]
  );

  const handleApprovalRequested = useCallback(
    (payload: unknown) => {
      const parsed = parseApprovalRequestedEvent(payload);
      if (!parsed.form && !parsed.token && !parsed.formId && !parsed.nodeId) return;
      dispatchApprovalRuntimeEvent('approval_requested', payload);
    },
    [dispatchApprovalRuntimeEvent]
  );

  const handleApprovalResultFilled = useCallback(
    (payload: unknown) => {
      dispatchApprovalRuntimeEvent('approval_result_filled', payload);
    },
    [dispatchApprovalRuntimeEvent]
  );

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
      const raw = event.data && typeof event.data === 'object' ? event.data : event;
      const eventData = unwrap(raw) as Record<string, unknown>;
      const eventConversationId =
        typeof eventData.conversation_id === 'string'
          ? eventData.conversation_id
          : useChatStore.getState().currentId;
      const callbacks = eventConversationId
        ? runCallbacksByConversationRef.current.get(eventConversationId)
        : undefined;
      if (!callbacks) return;

      const payload = raw;
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
          const workflowRunId = resolveWorkflowRunId(payload, { allowLegacyId: true });
          setLatestTaskId(
            (typeof data.task_id === 'string' ? data.task_id : null) ?? (workflowRunId || null)
          );
          callbacks.onStarted({
            conversationId,
            messageId: typeof data.message_id === 'string' ? data.message_id : undefined,
            workflowRunId: workflowRunId || undefined,
          });
          break;
        }
        case 'workflow_resumed':
          resetApprovalRuntime();
          questionAnswerPendingRef.current = false;
          setQuestionAnswerPrompt(null);
          setQuestionAnswerSubmitting(false);
          callbacks.mergeMessageData?.({
            approval: null,
            questionAnswerPrompt: null,
          });
          break;
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
              resolveWorkflowRunId(payload, { allowLegacyId: true }) || undefined,
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
              resolveWorkflowRunId(payload, { allowLegacyId: true }) || undefined,
            model: null,
          });
          if (
            (finalStatus === 'failed' || finalStatus === 'error') &&
            !abortSignalRef.current?.aborted
          ) {
            notifyBillingError(data.error);
          }
          setLatestTaskId(null);
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
          if (isWorkflowConversationBusyError(payload)) {
            const busyData = unwrap(payload);
            const conversationId =
              typeof busyData['conversation_id'] === 'string'
                ? (busyData['conversation_id'] as string)
                : undefined;
            refreshConversationRuntime(conversationId);
            callbacks.onError(
              Object.assign(new Error(t('webapp.chat.conversationBusy')), {
                code: 'workflow_conversation_busy',
                runtimeStatus: busyData['runtime_status'],
                workflowRunId: busyData['workflow_run_id'],
              })
            );
            setLatestTaskId(null);
            setQuestionAnswerSubmitting(false);
            return;
          }
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
      notifyBillingError,
      refreshConversationRuntime,
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
      const callbacks = runCallbacksByConversationRef.current.get(conversationId);
      const activePrompt = questionAnswerPrompt;
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
      const runNodes = createWorkflowRunNodeAccumulator({
        onNodeStarted: callbacks.onNodeStarted,
        onNodeFinished: callbacks.onNodeFinished,
      });
      const runCallbacks: WebAppRunSseCallbacks = {
        onWorkflowStarted: payload =>
          dispatchApprovalEvent({ event: 'workflow_started', data: payload }),
        onWorkflowResumed: payload =>
          dispatchApprovalEvent({ event: 'workflow_resumed', data: payload }),
        onNodeStarted: runNodes.onNodeStarted,
        onNodeFinished: runNodes.onNodeFinished,
        onIterationStarted: runNodes.onIterationStarted,
        onIterationNext: runNodes.onIterationNext,
        onIterationCompleted: runNodes.onIterationCompleted,
        onLoopStarted: runNodes.onLoopStarted,
        onLoopNext: runNodes.onLoopNext,
        onLoopCompleted: runNodes.onLoopCompleted,
        onWorkflowPaused: payload => handleWorkflowPaused(payload, callbacks),
        onApprovalRequested: payload =>
          dispatchApprovalEvent({ event: 'approval_requested', data: payload }),
        onApprovalResultFilled: payload =>
          dispatchApprovalEvent({ event: 'approval_result_filled', data: payload }),
        onApprovalExpired: payload =>
          dispatchApprovalEvent({ event: 'approval_expired', data: payload }),
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
            ...(activePrompt?.nodeId
              ? { question_answer_node_id: activePrompt.nodeId }
              : {}),
            ...(typeof activePrompt?.round === 'number'
              ? { question_answer_round: activePrompt.round }
              : {}),
          },
        },
        runCallbacks,
        { abortSignal: abortSignalRef.current }
      );
    },
    [dispatchApprovalEvent, handleWorkflowPaused, questionAnswerPrompt, start]
  );

  const { resumeWorkflowRun, continueWorkflowRun } = useWebappWorkflowRunEvents({
    startWorkflowRunEvents,
    cancelWorkflowRunEvents,
    restoredRunRef,
    questionAnswerTranscriptRef,
    setLatestTaskId,
    getWorkflowRunErrorText,
    handleApprovalExpired,
    handleApprovalRequested,
    handleApprovalResultFilled,
    handleQuestionAnswerRequested,
    handleQuestionAnswerSubmitted,
    resetApprovalRuntime,
    clearQuestionAnswerRuntime: resetQuestionAnswerRuntime,
    workflowRunFailedText: t('webapp.chat.workflowRunFailed'),
    setConversationConnectionState,
    reconcileConversationRun,
  });
  useEffect(() => {
    if (!approvalToken || !approvalSubmittedAction) return;
    const timer = window.setTimeout(() => {
      const conversationId = useChatStore.getState().currentId;
      if (!conversationId) return;
      const messages = useChatStore.getState().conversations[conversationId]?.messages ?? [];
      const latestMessage = messages.at(-1);
      if (!latestMessage) return;
      resumeWorkflowRun(conversationId, latestMessage);
    }, 5000);
    return () => window.clearTimeout(timer);
  }, [approvalSubmittedAction, approvalToken, resumeWorkflowRun]);

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
                  nodeId?: string;
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

      async search(query: string, limit: number) {
        const normalizedQuery = query.trim();
        if (!normalizedQuery) return [];
        const response = await WebAppService.searchConversations(versionUuid, {
          query: normalizedQuery,
          limit,
        });
        return response.data.map(mapWebAppSearchResult);
      },

      async create(payload?: { title?: string }): Promise<ConversationSummary> {
        resetQuestionAnswerRuntime();
        const draft: ConversationSummary = {
          id: `draft-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
          conversationId: '',
          title: payload?.title ?? '',
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
        runCallbacksByConversationRef.current.set(payload.conversationId, callbacks);
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
        const latestMessage =
          useChatStore.getState().conversations[payload.conversationId]?.messages.slice(-1)[0];
        const activeQuestionAnswerPrompt =
          getPendingQuestionAnswerPromptFromRuntimeMessage(latestMessage) ?? questionAnswerPrompt;
        const isQuestionAnswerResume = Boolean(
          activeQuestionAnswerPrompt || hasPendingQuestionAnswerMessage(payload.conversationId)
        );
        const runPayload: WebAppRunRequest = {
          query: payload.query,
          conversation_id: payload.conversationId,
          history_window_size: payload.historyWindowSize,
          files: payload.files,
          inputs: {
            ...payload.inputs,
            ...(activeQuestionAnswerPrompt?.nodeId
              ? { question_answer_node_id: activeQuestionAnswerPrompt.nodeId }
              : {}),
            ...(typeof activeQuestionAnswerPrompt?.round === 'number'
              ? { question_answer_round: activeQuestionAnswerPrompt.round }
              : {}),
          },
        };
        let durableConversationId = payload.conversationId;

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
            resetApprovalRuntime();
            setQuestionAnswerPrompt(null);
            setQuestionAnswerSubmitting(false);
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

            const runNodes = createWorkflowRunNodeAccumulator({
              onNodeStarted: callbacks.onNodeStarted,
              onNodeFinished: callbacks.onNodeFinished,
            });

            await start(
              runPayload,
              {
                onWorkflowStarted: (ctx: unknown) => {
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
                  durableConversationId = conversationId || durableConversationId;
                  const workflowRunId = resolveWorkflowRunId(ctx, { allowLegacyId: true });
                  if (conversationId) {
                    runCallbacksByConversationRef.current.set(conversationId, callbacks);
                    if (workflowRunId) attachRun(conversationId, workflowRunId);
                  }
                  setLatestTaskId(data.task_id ?? (workflowRunId || null));

                  callbacks.onStarted({
                    conversationId,
                    messageId: data.message_id,
                    workflowRunId: workflowRunId || undefined,
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
                onNodeStarted: runNodes.onNodeStarted,
                onNodeFinished: runNodes.onNodeFinished,
                onIterationStarted: runNodes.onIterationStarted,
                onIterationNext: runNodes.onIterationNext,
                onIterationCompleted: runNodes.onIterationCompleted,
                onLoopStarted: runNodes.onLoopStarted,
                onLoopNext: runNodes.onLoopNext,
                onLoopCompleted: runNodes.onLoopCompleted,
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
                      resolveWorkflowRunId(payload, { allowLegacyId: true }) || undefined,
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
                    isQuestionAnswerPromptMessage(data) ? stripQuestionAnswerPromptText(data) : data
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
                      resolveWorkflowRunId(ctx, { allowLegacyId: true }) || undefined,
                    model: null,
                  });

                  if (status === 'error' && !abortSignal?.aborted) {
                    notifyBillingError(data.error);
                  }
                  setLatestTaskId(null);
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
                  if (isWorkflowConversationBusyError(err)) {
                    const busyData = unwrap(err);
                    refreshConversationRuntime(runPayload.conversation_id);
                    setLatestTaskId(null);
                    resetApprovalRuntime();
                    setQuestionAnswerSubmitting(false);
                    questionAnswerPendingRef.current = false;
                    callbacks.onError(
                      Object.assign(new Error(t('webapp.chat.conversationBusy')), {
                        code: 'workflow_conversation_busy',
                        runtimeStatus: busyData['runtime_status'],
                        workflowRunId: busyData['workflow_run_id'],
                      })
                    );
                    return;
                  }
                  const reason = isWorkspaceNotFoundError(parsedError)
                    ? t('webapp.chat.workspaceRequiredForConversation')
                    : (getWorkflowRunErrorText(err) ?? parsedError.message);
                  const normalizedError = new Error(reason || 'Unknown error');
                  setLatestTaskId(null);
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
              {
                abortSignal,
                onTransportInterrupted: workflowRunId => {
                  const store = useChatStore.getState();
                  const conversationId =
                    durableConversationId ||
                    Object.keys(store.conversations).find(id =>
                      store.conversations[id]?.messages.some(
                        message => message.WorkflowRunInfo?.id === workflowRunId
                      )
                    ) ||
                    '';
                  if (!conversationId) return;
                  setConversationConnectionState(conversationId, 'reconnecting');
                  const message = store.conversations[conversationId]?.messages.find(
                    item => item.WorkflowRunInfo?.id === workflowRunId
                  );
                  if (message) resumeWorkflowRun(conversationId, message);
                },
              }
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
      notifyBillingError,
      options.enablePrecheck,
      precheckMutation,
      questionAnswerPrompt,
      refreshConversationRuntime,
      resetApprovalRuntime,
      resetQuestionAnswerRuntime,
      resumeWorkflowRun,
      setConversationConnectionState,
      start,
      t,
      attachRun,
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
    attachRun,
    recoverRun,
    stopRun,
    detachForeground,
    disposeConversation,
    connectionStateByConversation,
  };
}
