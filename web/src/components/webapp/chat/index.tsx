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
import { SlidersHorizontal } from 'lucide-react';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { useStore } from 'zustand';
import { useWebappConversationTransport } from '@/hooks/webapp/use-webapp-transport';
import { WorkflowPrecheckWarningBanner } from '@/components/workflow/common/workflow-precheck-warning';
import { stableStringify } from '@/utils/object';
import { getOpeningGuide } from '@/utils/webapp/opening-statement';
import { useAuthStore } from '@/store/auth-store';
import WorkflowApprovalInteractionCard from '@/components/workflow/approval/workflow-approval-interaction-card';
import { isWorkflowApprovalInlineAllowed } from '@/components/workflow/approval/workflow-approval-surface';
import { isApprovalFormAlreadySubmittedError } from '@/services/approval.service';
import { WebAppOfflineState } from '@/components/webapp/offline-state';
import { useWebAppOfflineState } from '@/hooks/webapp/use-webapp-offline-state';
import { QuestionAnswerRuntimePrompt } from '@/components/workflow/question-answer/question-answer-runtime-prompt';
import { WorkflowRuntimeStopAction } from '@/components/workflow/runtime/workflow-runtime-stop-action';
import { SUGGESTED_QUESTIONS_LIMIT } from '@/constants/suggested-questions';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import {
  type ConversationRouteHandoff,
  resolveConversationRouteSync,
} from '@/components/chat/runtime/conversation-route-handoff';

interface WebappChatProps {
  versionUuid: string;
  config: WebAppWorkflowConfig;
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
    description: v.description,
    required: v.required,
    max_length: v.max_length,
    default: v.default,
    default_datetime_mode: v.default_datetime_mode,
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

const WebappChat: React.FC<WebappChatProps> = ({
  versionUuid,
  config,
  agentId,
  enablePrecheck = false,
}) => {
  const t = useT();
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const conversationIdParam = searchParams.get('convId');
  const { isOffline } = useWebAppOfflineState();

  // Stop functionality state
  const [isRunning, setIsRunning] = useState(false);
  const [stoppingByConversation, setStoppingByConversation] = useState<Record<string, boolean>>({});
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  // Remember last submitted inputs to repopulate form
  const [lastInputs, setLastInputs] = useState<Record<string, unknown>>({});
  const lastInputsSignatureRef = useRef(stableStringify({}));
  const startVariables = useMemo(() => toInputVars(config.variables), [config.variables]);

  // Create transport and controller instances (memoized to prevent re-creation)
  const {
    transport,
    precheckWarnings,
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
    recoverRun,
    stopRun,
    detachForeground,
    connectionStateByConversation,
  } = useWebappConversationTransport(versionUuid, { enablePrecheck, agentId });
  const controllerRef = useRef<SingleChatController | null>(null);
  const routeHandoffRef = useRef<ConversationRouteHandoff | undefined>(undefined);
  const lastInitializedConversationIdRef = useRef<string | null | undefined>(undefined);
  const controller = useMemo(() => {
    if (controllerRef.current) {
      return controllerRef.current;
    }

    const nextController = new SingleChatController(transport);
    controllerRef.current = nextController;
    return nextController;
  }, [transport]);

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

  const replaceConversationRoute = useCallback(
    (conversationId: string | null, nullMode: 'new-chat' | 'draft-persistence' = 'new-chat') => {
      const params = new URLSearchParams(searchParams.toString());
      routeHandoffRef.current = conversationId
        ? { conversationId, mode: 'selection' }
        : { conversationId: null, mode: nullMode };
      if (conversationId) params.set('convId', conversationId);
      else params.delete('convId');
      const query = params.toString();
      router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
    },
    [pathname, router, searchParams]
  );

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

  const webappWorkflowConfig = useMemo(
    () => ({
      allow_view_run_detail: config.features.webapp_workflow_config?.allow_view_run_detail ?? true,
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
  const activeRunId =
    (typeof latestActiveMessage?.WorkflowRunInfo?.id === 'string'
      ? latestActiveMessage.WorkflowRunInfo.id
      : '') ||
    (typeof latestActiveMessage?.messageData?.workflow_run_id === 'string'
      ? (latestActiveMessage.messageData.workflow_run_id as string)
      : '');
  const latestRunStatus =
    latestActiveMessage?.WorkflowRunInfo?.status ?? latestActiveMessage?.clientState?.status;
  const activeConversationIsDraft = Boolean(
    activeConversation &&
      (activeConversationId?.startsWith('draft-') || !activeConversation.conversationId)
  );
  const activeConnectionState = activeConversationId
    ? (connectionStateByConversation[activeConversationId] ?? 'idle')
    : 'idle';
  const isStopping = activeConversationId
    ? Boolean(stoppingByConversation[activeConversationId])
    : false;
  const isApprovalPending = latestRunStatus === 'pending_approval';
  const isQuestionPending = latestRunStatus === 'pending_question' && Boolean(questionAnswerPrompt);
  const allowQuestionTextInput =
    isQuestionPending &&
    Boolean(questionAnswerPrompt) &&
    questionAnswerPrompt?.choices.length === 0;

  useEffect(() => {
    syncQuestionAnswerRuntime(activeConversationId ?? undefined);
  }, [activeConversationId, latestActiveMessage, syncQuestionAnswerRuntime]);

  const handleQuestionAnswerChoice = useCallback(
    (choice: { id: string; label?: string; value?: string }) => {
      const query = String(choice.label || choice.value || choice.id || '').trim();
      if (!activeConversationId || !query) return;
      controller.send({
        query,
        inputs: {
          question_answer_option_id: choice.id,
          ...(questionAnswerPrompt?.nodeId
            ? { question_answer_node_id: questionAnswerPrompt.nodeId }
            : {}),
          ...(questionAnswerPrompt?.round !== undefined
            ? { question_answer_round: questionAnswerPrompt.round }
            : {}),
        },
      });
    },
    [activeConversationId, controller, questionAnswerPrompt]
  );

  useEffect(() => {
    setIsRunning(controllerIsSending);
  }, [controllerIsSending]);

  useEffect(() => {
    if (['completed', 'stopped', 'error', 'expired'].includes(String(latestRunStatus))) {
      if (!activeConversationId) return;
      setStoppingByConversation(current => {
        if (!current[activeConversationId]) return current;
        const next = { ...current };
        delete next[activeConversationId];
        return next;
      });
    }
  }, [activeConversationId, latestRunStatus]);

  const handleStop = useCallback(async () => {
    if (!activeConversationId || !activeRunId || !agentId) return;
    setStoppingByConversation(current => ({ ...current, [activeConversationId]: true }));
    try {
      await stopRun(activeConversationId);
    } catch {
      setStoppingByConversation(current => {
        const next = { ...current };
        delete next[activeConversationId];
        return next;
      });
      toast.error(t('agents.workflow.stopFailed'));
    }
  }, [activeConversationId, activeRunId, agentId, stopRun, t]);

  const interactionStopAction = useMemo(
    () =>
      activeRunId ? (
        <WorkflowRuntimeStopAction
          onStop={handleStop}
          isStopping={isStopping}
          disabled={approvalSubmitting || questionAnswerSubmitting}
        />
      ) : null,
    [activeRunId, approvalSubmitting, handleStop, isStopping, questionAnswerSubmitting]
  );

  useEffect(() => {
    if (lastInitializedConversationIdRef.current === conversationIdParam) return;
    lastInitializedConversationIdRef.current = conversationIdParam;
    if (conversationIdParam) {
      routeHandoffRef.current = { conversationId: conversationIdParam, mode: 'selection' };
      controller.init(conversationIdParam);
      return;
    }
    routeHandoffRef.current = { conversationId: null, mode: 'new-chat' };
    if (!activeConversationId || !activeConversationIsDraft) {
      const draft = controller.createDraft(t('agents.workflow.chat.newConversation'));
      controller.select(draft.id);
    }
  }, [activeConversationIsDraft, activeConversationId, controller, conversationIdParam, t]);

  useEffect(() => {
    const decision = resolveConversationRouteSync({
      activeConversationId,
      currentConversationId: conversationIdParam,
      routeHandoff: routeHandoffRef.current,
      activeConversationIsDraft,
    });
    routeHandoffRef.current = decision.routeHandoff;
    if (decision.action.type === 'replace') {
      replaceConversationRoute(decision.action.conversationId);
    } else if (decision.action.type === 'clear') {
      replaceConversationRoute(null);
    }
  }, [
    activeConversationId,
    activeConversationIsDraft,
    conversationIdParam,
    replaceConversationRoute,
  ]);

  const handleSelectConversation = useCallback(
    (conversationId: string) => {
      if (!conversationId) return;
      if (activeConversationId && activeConversationId !== conversationId) {
        detachForeground(activeConversationId);
      }
      routeHandoffRef.current = { conversationId, mode: 'selection' };
      replaceConversationRoute(conversationId);
      void controller.loadAndSelect(conversationId);
    },
    [activeConversationId, controller, detachForeground, replaceConversationRoute]
  );

  const handleStartNewConversation = useCallback(() => {
    if (activeConversationId) detachForeground(activeConversationId);
    const draft = controller.createDraft(t('agents.workflow.chat.newConversation'));
    controller.select(draft.id);
    replaceConversationRoute(null, 'draft-persistence');
  }, [activeConversationId, controller, detachForeground, replaceConversationRoute, t]);

  const handleReconnect = useCallback(() => {
    if (!activeConversationId || !latestActiveMessage) return;
    if (activeRunId) recoverRun(activeConversationId, activeRunId);
    resumeWorkflowRun(activeConversationId, latestActiveMessage);
  }, [activeConversationId, activeRunId, latestActiveMessage, recoverRun, resumeWorkflowRun]);

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
    recoverRun(activeConversationId, workflowRunId);

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
  }, [
    activeConversation,
    activeConversationId,
    continueWorkflowRun,
    recoverRun,
    resumeWorkflowRun,
  ]);

  const approvalInputReplacement = useMemo(() => {
    if (isQuestionPending) return null;
    if (!isApprovalPending) return null;

    if (approvalSubmittedAction) {
      return (
        <WorkflowApprovalInteractionCard mode="submitted" secondaryAction={interactionStopAction} />
      );
    }

    if (!approvalToken) {
      return (
        <WorkflowApprovalInteractionCard mode="external" secondaryAction={interactionStopAction} />
      );
    }

    if (approvalLoading) {
      return (
        <WorkflowApprovalInteractionCard mode="loading" secondaryAction={interactionStopAction} />
      );
    }

    if (!approvalForm && isApprovalFormAlreadySubmittedError(approvalError)) {
      return <WorkflowApprovalInteractionCard mode="completed" />;
    }

    if (!approvalForm && approvalError) {
      return (
        <WorkflowApprovalInteractionCard
          mode="error"
          error={approvalError}
          onRetry={retryApprovalForm}
          secondaryAction={interactionStopAction}
        />
      );
    }

    if (approvalForm) {
      const canSubmitInline = isWorkflowApprovalInlineAllowed({
        surface: 'workflow-webapp',
        form: approvalForm,
      });
      return (
        <WorkflowApprovalInteractionCard
          mode={canSubmitInline ? 'form' : 'external'}
          form={approvalForm}
          onSubmit={payload => void submitApproval(payload)}
          isSubmitting={approvalSubmitting}
          submittedAction={approvalSubmittedAction}
          secondaryAction={interactionStopAction}
        />
      );
    }

    return (
      <WorkflowApprovalInteractionCard mode="loading" secondaryAction={interactionStopAction} />
    );
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
    isQuestionPending,
    interactionStopAction,
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
        secondaryAction={interactionStopAction}
      />
    );
  }, [
    handleQuestionAnswerChoice,
    isQuestionPending,
    questionAnswerPrompt,
    questionAnswerSubmitting,
    interactionStopAction,
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
        conversationSearchKey={['webapp', 'conversations', versionUuid, 'search']}
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
        onSelectConversation={handleSelectConversation}
        onStartNewConversation={handleStartNewConversation}
        connectionState={activeConnectionState}
        onReconnect={handleReconnect}
        pendingInteractionControlsIntegrated={isApprovalPending || isQuestionPending}
      />
    </div>
  );
};

export default WebappChat;
