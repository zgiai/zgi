'use client';

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import ChatWithController from '@/components/chat/chat-with-controller';
import { useChatStore } from '@/components/chat';
import { SingleChatController } from '@/components/chat/controllers/single-chat-controller';
import type { WebAppWorkflowConfig, WebAppVariable } from '@/services/types/webapp';
import type {
  WorkflowFeatures,
  FileUploadType,
  FileUploadMethod,
} from '@/components/workflow/store/type';
import type { InputVar } from '@/components/workflow/types/input-var';
import { Clock3, Loader2, Send, SlidersHorizontal } from 'lucide-react';
import { useT } from '@/i18n';
import { workflowService } from '@/services/workflow.service';
import { toast } from 'sonner';
import { useStore } from 'zustand';
import { useWebappConversationTransport } from '@/hooks/webapp/use-webapp-transport';
import { WorkflowPrecheckWarningBanner } from '@/components/workflow/common/workflow-precheck-warning';
import { stableStringify } from '@/utils/object';
import { getOpeningGuide } from '@/utils/webapp/opening-statement';
import { useAuthStore } from '@/store/auth-store';
import { ApprovalCompletedState } from '@/components/workflow/approval/approval-completed-state';
import ApprovalRuntimeForm from '@/components/workflow/approval/approval-runtime-form';
import { Button } from '@/components/ui/button';
import { isApprovalFormAlreadySubmittedError } from '@/services/approval.service';
import { WebAppOfflineState } from '@/components/webapp/offline-state';
import { useWebAppOfflineState } from '@/hooks/webapp/use-webapp-offline-state';
import { QuestionAnswerRuntimePrompt } from '@/components/workflow/question-answer/question-answer-runtime-prompt';
import { SUGGESTED_QUESTIONS_LIMIT } from '@/constants/suggested-questions';

interface WebappChatProps {
  versionUuid: string;
  config: WebAppWorkflowConfig;
  historyWindowSize?: number;
  /** Agent ID for stop functionality */
  agentId?: string;
  enablePrecheck?: boolean;
}

// Convert webapp variables to InputVar[] expected by Chat toolbar form
function toInputVars(vars: WebAppVariable[]): InputVar[] {
  return vars.map(v => ({
    type: v.type as InputVar['type'],
    variable: v.variable,
    label: v.label,
    required: v.required,
    max_length: v.max_length,
    default: v.default,
    options: v.options,
    allowed_file_upload_methods: v.allowed_file_upload_methods,
    allowed_file_types: v.allowed_file_types,
    allowed_file_extensions: v.allowed_file_extensions,
  }));
}

// Adapt webapp features to Chat prop type
function toChatFeatures(
  cfg: WebAppWorkflowConfig
): Pick<WorkflowFeatures, 'file_upload' | 'retriever_resource'> {
  return {
    file_upload: {
      enabled: cfg.features.file_upload.enabled,
      allowed_file_types: cfg.features.file_upload
        .allowed_file_types as unknown as FileUploadType[],
      allowed_file_extensions: cfg.features.file_upload.allowed_file_extensions,
      allowed_file_upload_methods: cfg.features.file_upload
        .allowed_file_upload_methods as unknown as FileUploadMethod[],
      number_limits: cfg.features.file_upload.number_limits,
    },
    retriever_resource: { enabled: cfg.features.retriever_resource.enabled },
  };
}

function sanitizeInputsForRestore(
  inputs: Record<string, unknown> | undefined,
  variables: InputVar[]
): Record<string, unknown> {
  if (!inputs) return {};

  const sanitizedInputs = { ...inputs };
  variables.forEach(variable => {
    if (variable.type === 'file') {
      const value = sanitizedInputs[variable.variable];
      if (
        value &&
        typeof value === 'object' &&
        'upload_file_id' in (value as Record<string, unknown>)
      ) {
        sanitizedInputs[variable.variable] = (value as { upload_file_id: string }).upload_file_id;
      }
    }

    if (variable.type === 'file-list') {
      const value = sanitizedInputs[variable.variable];
      if (Array.isArray(value)) {
        sanitizedInputs[variable.variable] = value.map(item => {
          if (item && typeof item === 'object' && 'upload_file_id' in item) {
            return (item as { upload_file_id: string }).upload_file_id;
          }
          return item;
        });
      }
    }
  });

  return sanitizedInputs;
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

const WebappChat: React.FC<WebappChatProps> = ({
  versionUuid,
  config,
  agentId,
  enablePrecheck = false,
}) => {
  const t = useT();
  const { isOffline } = useWebAppOfflineState();

  // Stop functionality state
  const [isRunning, setIsRunning] = useState(false);
  const [isStopping, setIsStopping] = useState(false);
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const taskIdRef = useRef<string | null>(null);
  // Remember last submitted inputs to repopulate form
  const [lastInputs, setLastInputs] = useState<Record<string, unknown>>({});
  const lastInputsSignatureRef = useRef(stableStringify({}));
  const startVariables = useMemo(() => toInputVars(config.variables), [config.variables]);

  // Create transport and controller instances (memoized to prevent re-creation)
  const {
    transport,
    precheckWarnings,
    latestTaskId,
    approvalForm,
    approvalToken,
    approvalLoading,
    approvalError,
    approvalSubmitting,
    approvalSubmittedAction,
    questionAnswerPrompt,
    questionAnswerSubmitting,
    syncQuestionAnswerRuntime,
    submitApproval,
    retryApprovalForm,
    resumeWorkflowRun,
    continueWorkflowRun,
  } = useWebappConversationTransport(versionUuid, { enablePrecheck });
  const controllerRef = useRef<SingleChatController | null>(null);
  const controller = useMemo(() => {
    if (controllerRef.current) {
      return controllerRef.current;
    }

    const nextController = new SingleChatController(transport);
    controllerRef.current = nextController;
    return nextController;
  }, [transport]);

  // Handle stop workflow
  const handleStop = useCallback(async () => {
    const stopRunId = taskIdRef.current;
    if (!stopRunId || !agentId) return;
    setIsStopping(true);
    try {
      await workflowService.stopWorkflowTask(agentId, stopRunId);
      const state = controller.store.getState();
      state.setIsSending(false);
      state.setIsPaused(false);
      setIsRunning(false);
      taskIdRef.current = null;

      const activeId = state.activeId;
      const conversation = activeId ? useChatStore.getState().conversations[activeId] : undefined;
      const latestMessage = conversation?.messages[conversation.messages.length - 1];
      const tempKey =
        typeof latestMessage?.messageData?.tempKey === 'string'
          ? latestMessage.messageData.tempKey
          : '';
      if (activeId && tempKey) {
        useChatStore.getState().finalizeAiMessage(activeId, tempKey, {
          status: 'stopped',
        });
      }
    } catch {
      toast.error(t('agents.workflow.stopFailed'));
    } finally {
      setIsStopping(false);
    }
  }, [agentId, controller, t]);

  useEffect(() => {
    controller.initTransport();
  }, [controller]);

  useEffect(() => {
    const syncLastInputs = (inputs: Record<string, unknown> | undefined) => {
      if (!inputs) return;

      const sanitizedInputs = sanitizeInputsForRestore(inputs, startVariables);
      const nextSignature = stableStringify(sanitizedInputs);

      if (lastInputsSignatureRef.current === nextSignature) {
        return;
      }

      lastInputsSignatureRef.current = nextSignature;
      setLastInputs(sanitizedInputs);
    };

    syncLastInputs(controller.store.getState().lastInputs);

    const unsubscribe = controller.subscribe((state, prevState) => {
      if (state.lastInputs === prevState.lastInputs) {
        return;
      }

      syncLastInputs(state.lastInputs);
    });

    return () => {
      unsubscribe();
    };
  }, [controller, startVariables]);

  useEffect(() => {
    controller.updateTransport(transport);
  }, [controller, transport]);

  useEffect(() => {
    taskIdRef.current = latestTaskId;
  }, [latestTaskId]);

  useEffect(() => {
    const html = document.documentElement;
    const body = document.body;
    const prevHtmlOverflow = html.style.overflow;
    const prevBodyOverflow = body.style.overflow;
    const prevHtmlOverscroll = html.style.overscrollBehavior;
    const prevBodyOverscroll = body.style.overscrollBehavior;

    // Webapp chat is a full-height application surface with its own internal
    // scroll areas, so the browser viewport itself should remain locked.
    html.style.overflow = 'hidden';
    body.style.overflow = 'hidden';
    html.style.overscrollBehavior = 'none';
    body.style.overscrollBehavior = 'none';

    return () => {
      html.style.overflow = prevHtmlOverflow;
      body.style.overflow = prevBodyOverflow;
      html.style.overscrollBehavior = prevHtmlOverscroll;
      body.style.overscrollBehavior = prevBodyOverscroll;
    };
  }, []);

  const features = useMemo(() => toChatFeatures(config), [config]);

  const historyWindowSize = useMemo(() => {
    const ch = config?.features?.conversation_history;
    if (!ch || !ch.enabled) return 0;
    const n = typeof ch.history_window_size === 'number' ? ch.history_window_size : 1;
    return Math.max(1, Math.min(50, Math.floor(n)));
  }, [config]);

  const webappWorkflowConfig = useMemo(
    () => ({
      allow_view_run_detail:
        config.features.webapp_workflow_config?.allow_view_run_detail ?? true,
      auto_expand_run_detail:
        config.features.webapp_workflow_config?.auto_expand_run_detail ?? false,
    }),
    [config.features.webapp_workflow_config]
  );
  const openingGuide = useMemo(() => getOpeningGuide(config.features), [config.features]);
  const suggestedQuestions = useMemo(
    () =>
      (config.features.suggested_questions ?? [])
        .map(item => item.trim())
        .filter(Boolean)
        .slice(0, SUGGESTED_QUESTIONS_LIMIT),
    [config.features.suggested_questions]
  );
  const inputPlaceholder =
    startVariables.length > 0
      ? t('webapp.chat.inputPlaceholderWithInputs')
      : t('webapp.chat.inputPlaceholder');
  const suggestionsTitle =
    suggestedQuestions.length > 0 ? t('webapp.chat.suggestedQuestionsTitle') : undefined;

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

  // Listen to controller isSending state to track running state
  const controllerIsSending = useStore(controller.store, s => s.isSending);
  const activeConversationId = useStore(controller.store, s => s.activeId);
  const activeConversation = useChatStore(state =>
    activeConversationId ? state.conversations[activeConversationId] : undefined
  );
  const latestActiveMessage = activeConversation?.messages[activeConversation.messages.length - 1];
  const latestRunStatus =
    latestActiveMessage?.WorkflowRunInfo?.status ?? latestActiveMessage?.clientState?.status;
  const isApprovalPending = latestRunStatus === 'pending_approval';
  const isQuestionPending = latestRunStatus === 'pending_question' && Boolean(questionAnswerPrompt);
  const allowQuestionTextInput =
    isQuestionPending && Boolean(questionAnswerPrompt) && questionAnswerPrompt?.choices.length === 0;

  useEffect(() => {
    syncQuestionAnswerRuntime(activeConversationId ?? undefined);
  }, [activeConversationId, latestActiveMessage, syncQuestionAnswerRuntime]);

  const handleQuestionAnswerChoice = useCallback(
    (choice: { id: string; label?: string; value?: string }) => {
      const query = String(choice.label || choice.value || choice.id || '').trim();
      if (!activeConversationId || !query) return;
      controller.send({
        query,
        inputs: { question_answer_option_id: choice.id },
        historyWindowSize,
      });
    },
    [activeConversationId, controller, historyWindowSize]
  );

  useEffect(() => {
    setIsRunning(controllerIsSending);
  }, [controllerIsSending]);

  useEffect(() => {
    if (!activeConversationId || !activeConversation?.messages.length) return;

    const latestMessage = activeConversation.messages[activeConversation.messages.length - 1];
    const workflowRunId =
      (typeof latestMessage.WorkflowRunInfo?.id === 'string'
        ? latestMessage.WorkflowRunInfo.id
        : '') ||
      (typeof latestMessage.messageData?.workflow_run_id === 'string'
        ? (latestMessage.messageData.workflow_run_id as string)
        : '');
    if (!workflowRunId) return;

    const tempKey =
      typeof latestMessage.messageData?.tempKey === 'string'
        ? (latestMessage.messageData.tempKey as string)
        : '';
    const isRestoredMessage = tempKey.startsWith('restore:');

    const runStatus = latestMessage.WorkflowRunInfo?.status;
    const shouldResume =
      runStatus === 'running' ||
      runStatus === 'pending_approval' ||
      runStatus === 'pending_question' ||
      latestMessage.clientState?.phase === 'streaming';

    if (isRestoredMessage && shouldResume) {
      resumeWorkflowRun(activeConversationId, latestMessage);
      return;
    }

    if (!isRestoredMessage && runStatus === 'pending_approval') {
      continueWorkflowRun(activeConversationId, latestMessage);
    }
  }, [activeConversation, activeConversationId, continueWorkflowRun, resumeWorkflowRun]);

  const approvalInputReplacement = useMemo(() => {
    if (isQuestionPending) return null;
    if (!isApprovalPending) return null;

    if (approvalSubmittedAction) {
      return <ApprovalWaitingState loading submitted />;
    }

    if (!approvalToken) {
      return <ApprovalWaitingState />;
    }

    if (approvalLoading) {
      return <ApprovalWaitingState loading />;
    }

    if (!approvalForm && isApprovalFormAlreadySubmittedError(approvalError)) {
      return <ApprovalCompletedState compact />;
    }

    if (!approvalForm && approvalError) {
      return (
        <div className="rounded-xl border bg-card p-4 text-center shadow-sm">
          <div className="text-sm font-medium">{t('nodes.approval.runtime.loadFailed')}</div>
          <p className="mt-2 text-xs text-muted-foreground">
            {approvalError instanceof Error
              ? approvalError.message
              : t('nodes.approval.runtime.loadFailedDescription')}
          </p>
          <Button type="button" size="sm" className="mt-3" onClick={retryApprovalForm}>
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
            onSubmit={payload => void submitApproval(payload)}
            isSubmitting={approvalSubmitting}
            submittedAction={approvalSubmittedAction}
          />
        </div>
      );
    }

    return <ApprovalWaitingState loading />;
  }, [
    approvalError,
    approvalForm,
    approvalLoading,
    approvalSubmittedAction,
    approvalSubmitting,
    approvalToken,
    isApprovalPending,
    retryApprovalForm,
    submitApproval,
    t,
    isQuestionPending,
  ]);

  const questionAnswerNotice = useMemo(() => {
    if (!isQuestionPending || !questionAnswerPrompt) return null;
    return (
      <QuestionAnswerRuntimePrompt
        question={questionAnswerPrompt.question}
        choices={questionAnswerPrompt.choices}
        round={questionAnswerPrompt.round}
        submitting={questionAnswerSubmitting}
        onSelectChoice={handleQuestionAnswerChoice}
      />
    );
  }, [
    handleQuestionAnswerChoice,
    isQuestionPending,
    questionAnswerPrompt,
    questionAnswerSubmitting,
  ]);

  if (isOffline) {
    return <WebAppOfflineState />;
  }

  return (
    <div className="box-border h-full min-h-0 overflow-hidden">
      <ChatWithController
        className="h-full"
        controller={controller}
        features={features}
        enableUpload={features?.file_upload?.enabled ?? true}
        openingGuide={openingGuide}
        suggestions={suggestedQuestions}
        suggestionsTitle={suggestionsTitle}
        toolbarForm={toolbarFormSpec}
        webappMeta={config.config}
        historyWindowSize={historyWindowSize}
        showWorkflowRunHeader={webappWorkflowConfig.allow_view_run_detail}
        showWorkflowDetail={webappWorkflowConfig.allow_view_run_detail}
        hideCompletedWorkflowDetail
        showWorkflowNodeDetail={false}
        allowWorkflowDetailExpand={webappWorkflowConfig.allow_view_run_detail}
        defaultWorkflowDetailOpen={
          webappWorkflowConfig.allow_view_run_detail && webappWorkflowConfig.auto_expand_run_detail
        }
        uploadAccessMode={isAuthenticated ? 'enabled' : 'login-required'}
        allowWorkspaceSwitch
        onStop={handleStop}
        isRunning={isRunning}
        isStopping={isStopping}
        placeholder={inputPlaceholder}
        surface="webapp"
        inputReplacement={approvalInputReplacement}
        inputTopNotice={
          questionAnswerNotice ||
          (enablePrecheck && precheckWarnings.length > 0 ? (
            <WorkflowPrecheckWarningBanner
              warnings={precheckWarnings}
              scope="webapp"
              storageKey={`webapp-chat:${versionUuid}`}
            />
          ) : null)
        }
        allowPendingQuestionInput={allowQuestionTextInput}
      />
    </div>
  );
};

export default WebappChat;
