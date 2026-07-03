'use client';

import {
  createElement,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
} from 'react';
import { createPortal } from 'react-dom';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';
import { useStore } from 'zustand';
import { ArrowDown, Settings2 } from 'lucide-react';
import type {
  ModelSelectorModelProps,
  ModelSelectorValue,
} from '@/components/common/model-selector';
import type { AIChatController } from '@/components/chat/controllers/aichat-controller';
import type {
  ConversationSearchFn,
  ConversationSummary,
} from '@/components/chat/controllers/types';
import {
  selectActiveConversation,
  selectActiveMessagePagination,
  selectActiveMessages,
  selectBranchNavigationByMessageId,
  selectDisplayMessageIds,
  selectDisplayMessages,
  selectIsRecoveringMessages,
  selectIsLoadingOlderMessages,
  selectIsStopping,
  mergeRuntimeTimelineWithMessageTimeline,
  timelineFromAIChatMessage,
} from '@/components/chat/controllers/aichat/selectors';
import { Sidebar } from '@/components/chat/variants/common/sidebar';
import { Sheet, SheetContent, SheetDescription, SheetTitle } from '@/components/ui/sheet';
import { Button } from '@/components/ui/button';
import {
  useAIChatSkillPreference,
  useAIChatSkills,
  useUpdateAIChatSkillPreference,
} from '@/hooks/aichat/use-aichat-skills';
import { useLocale } from '@/hooks/use-locale';
import { useIsMobile } from '@/hooks/use-mobile';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { useWorkspaceStore } from '@/store/workspace-store';
import type {
  AIChatConversation,
  AIChatMessage,
  AIChatMessageFile,
  AIChatRuntimeSurface,
  AIChatUserInputRequest,
  AIChatWorkflowRunMetadata,
} from '@/services/types/aichat';
import {
  buildChatMessageTopology,
  buildChatMessageTopologyKey,
  type ChatMessageTopology,
} from '@/components/chat/utils/message-tree';
import { AIChatHeader } from '@/components/chat/variants/aichat/chat-header';
import { AIChatHomeView } from '@/components/chat/variants/aichat/home-view';
import { AIChatAssetAuditButton } from '@/components/chat/variants/aichat/asset-audit-button';
import {
  AIChatEmbeddedConversationControls,
  embeddedControlButtonClassName,
} from '@/components/chat/variants/aichat/embedded-conversation-controls';
import {
  AIChatInputArea,
  type AIChatUploadScope,
} from '@/components/chat/variants/aichat/input-area';
import { AIChatMessageList } from '@/components/chat/variants/aichat/message-list';
import {
  resolvePendingToolGovernanceApprovalFromTimeline,
  type AIChatToolGovernanceDecisionSubmitPayload,
} from '@/components/chat/variants/aichat/agentic-timeline';
import {
  buildAIChatSkillDisplayMap,
  isHiddenSystemSkill,
} from '@/components/chat/variants/aichat/skill-display';
import { AIChatSkillPreferenceDialog } from '@/components/chat/variants/aichat/skill-preference-dialog';
import {
  isToolGovernancePendingApprovalDismissed,
  ToolGovernancePendingApprovalScopeProvider,
  type ToolGovernancePendingApproval,
} from '@/components/chat/variants/aichat/tool-governance-decision-card';
import { useAIChatScroll } from '@/components/chat/variants/aichat/use-aichat-scroll';
import {
  getAIChatMessageErrorInput,
  isAIChatContinuationLikelyStarted,
  resolveAIChatErrorMessage,
} from '@/components/chat/variants/aichat/error-utils';
import {
  WorkflowBillingToastAction,
  workflowBillingToastClassNames,
} from '@/components/workflow/common/workflow-billing-toast-action';
import { normalizeApprovalRuntimeForm } from '@/components/workflow/approval/runtime-events';
import { AICHAT_SIDEBAR_BG_IMAGE } from '@/lib/config';
import {
  MAX_AICHAT_BRANCHES,
  type AIChatModelValue,
  type AIChatSuggestion,
  type AIChatWorkflowApprovalRequest,
  type AIChatWorkflowApprovalSubmitPayload,
} from '@/components/chat/variants/aichat/types';
import type {
  AIChatOperationContext,
  AIChatToolGovernancePermissionTier,
} from '@/components/aichat/contextual/types';

export { AIChatMessageBubble } from '@/components/chat/variants/aichat/message-bubble';
export type { AIChatModelValue } from '@/components/chat/variants/aichat/types';

interface AIChatShellProps {
  controller: AIChatController;
  modelSelectorValue: AIChatModelValue;
  modelProps?: ModelSelectorModelProps | null;
  supportsVisionOverride?: boolean;
  isModelInitializing?: boolean;
  onModelChange: (value: ModelSelectorValue) => void;
  beforeSend?: () => boolean | Promise<boolean>;
  variant?: 'full' | 'embedded';
  showModelSelector?: boolean;
  requireModel?: boolean;
  showMemoryToggle?: boolean;
  forcedUseMemory?: boolean;
  enableUpload?: boolean;
  uploadScope?: AIChatUploadScope;
  showFileLibraryPicker?: boolean;
  allowWorkspaceSwitch?: boolean;
  homeBrand?: React.ReactNode;
  homeTitle?: string;
  homeDescription?: string;
  suggestions?: string[];
  inputPlaceholder?: string;
  embeddedConversationMode?: 'none' | 'drawer';
  embeddedConversationControlsMode?: 'internal' | 'external';
  embeddedConversationControlsClassName?: string;
  embeddedConversationControlsPortalId?: string;
  renderEmbeddedConversationControls?: (controls: {
    openConversations: () => void;
    startNewConversation: () => void;
    isHome: boolean;
  }) => React.ReactNode;
  onSelectConversation?: (id: string) => void;
  onStartNewConversation?: () => void;
  showAssistantModelMeta?: boolean;
  surface?: 'aichat' | 'agent-draft' | 'agent-webapp';
  runtimeSurface?: AIChatRuntimeSurface;
  themeColor?: string;
  enableToolGovernance?: boolean;
}

const CHAT_THEME_PRIMARY: Record<string, string> = {
  blue: '217 91% 60%',
  emerald: '160 84% 39%',
  violet: '262 83% 58%',
  rose: '346 77% 50%',
  amber: '38 92% 50%',
  slate: '215 20% 45%',
};
const AGENT_WORKFLOW_QUESTION_SOURCE = 'agent_workflow_question_answer';
const TOOL_GOVERNANCE_PERMISSION_TIER_STORAGE_KEY = 'zgi:aichat:tool-governance-permission-tier';

function normalizeToolGovernancePermissionTier(value: unknown): AIChatToolGovernancePermissionTier {
  return value === 'advanced' || value === 'full' ? value : 'basic';
}

function toolGovernanceDecisionKey(
  conversationId?: string | null,
  messageId?: string | null,
  correlationId?: string | null
) {
  return [conversationId, messageId, correlationId].map(value => value?.trim() ?? '').join(':');
}

function normalizeSkillIds(skillIds: string[]) {
  return Array.from(new Set(skillIds.filter(Boolean))).sort();
}

function areSkillIdsEqual(left: string[], right: string[]) {
  const normalizedLeft = normalizeSkillIds(left);
  const normalizedRight = normalizeSkillIds(right);
  return (
    normalizedLeft.length === normalizedRight.length &&
    normalizedLeft.every((skillId, index) => skillId === normalizedRight[index])
  );
}

function resolveWorkflowQuestionAnswerInputs(
  request: AIChatUserInputRequest,
  fallbackQuery: string,
  answers?: Record<string, string>
): { query: string; question_answer_option_id?: string } | null {
  const firstQuestion = request.questions[0];
  if (!firstQuestion) return null;
  const answerKey = firstQuestion.id || 'q1';
  const rawAnswer = (answers?.[answerKey] ?? '').trim();
  const query = rawAnswer || fallbackQuery.trim();
  if (!query) return null;
  const selectedOption = (firstQuestion.options ?? []).find(option => {
    const candidates = [option.label, option.value, option.option_id]
      .map(value => value?.trim())
      .filter(Boolean);
    return candidates.includes(rawAnswer) || candidates.includes(query);
  });
  return {
    query: selectedOption?.label?.trim() || query,
    question_answer_option_id: selectedOption?.option_id?.trim() || undefined,
  };
}

/**
 * @component AIChatShell
 * @category Feature
 * @status Stable
 * @description Standalone console AIChat interface backed by /console/api/aichat.
 * @usage Use only for /console/work/chat
 * @example
 * <AIChatShell controller={controller} modelSelectorValue={value} onModelChange={setValue} />
 */
export function AIChatShell({
  controller,
  modelSelectorValue,
  modelProps,
  supportsVisionOverride,
  isModelInitializing = false,
  onModelChange,
  beforeSend,
  variant = 'full',
  showModelSelector = true,
  requireModel = true,
  showMemoryToggle = true,
  forcedUseMemory,
  enableUpload = true,
  uploadScope,
  showFileLibraryPicker = true,
  allowWorkspaceSwitch = false,
  homeBrand,
  homeTitle,
  homeDescription,
  suggestions: configuredSuggestions,
  inputPlaceholder,
  embeddedConversationMode = 'none',
  embeddedConversationControlsMode = 'internal',
  embeddedConversationControlsClassName,
  embeddedConversationControlsPortalId,
  renderEmbeddedConversationControls,
  onSelectConversation,
  onStartNewConversation,
  showAssistantModelMeta = true,
  surface = 'aichat',
  runtimeSurface = 'work_chat',
  themeColor,
  enableToolGovernance = false,
}: AIChatShellProps) {
  const router = useRouter();
  const t = useT('webapp');
  const tGlobal = useT();
  const { locale } = useLocale();
  const isMobile = useIsMobile();
  const isEmbedded = variant === 'embedded';
  const toolGovernanceApprovalScopeId = useId();
  const showEmbeddedConversationDrawer = isEmbedded && embeddedConversationMode === 'drawer';
  const themeStyle = useMemo<CSSProperties | undefined>(() => {
    const primary = themeColor ? CHAT_THEME_PRIMARY[themeColor] : undefined;
    return primary ? ({ '--primary': primary } as CSSProperties) : undefined;
  }, [themeColor]);
  const [input, setInput] = useState('');
  const [editingMessageId, setEditingMessageId] = useState<string | null>(null);
  const [editingQuery, setEditingQuery] = useState('');
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const [externalControlsPortal, setExternalControlsPortal] = useState<HTMLElement | null>(null);
  const [skillPreferenceOpen, setSkillPreferenceOpen] = useState(false);
  const [draftSkillPreferenceIds, setDraftSkillPreferenceIds] = useState<string[]>([]);
  const [submittedToolGovernanceDecisionKeys, setSubmittedToolGovernanceDecisionKeys] = useState<
    Set<string>
  >(() => new Set());
  const [approvedToolGovernanceDecisionKeys, setApprovedToolGovernanceDecisionKeys] = useState<
    Set<string>
  >(() => new Set());
  const [inputAreaHeight, setInputAreaHeight] = useState(160);
  const [toolGovernancePermissionTier, setToolGovernancePermissionTier] =
    useState<AIChatToolGovernancePermissionTier>('basic');
  const [toolGovernancePermissionTierLoaded, setToolGovernancePermissionTierLoaded] =
    useState(false);
  const topologyRef = useRef<{ key: string; topology: ChatMessageTopology } | null>(null);
  const lastErrorToastRef = useRef<string | null>(null);

  const conversations = useStore(controller.store, state => state.conversations);
  const activeConversationId = useStore(controller.store, state => state.activeConversationId);
  const activeConversation = useStore(controller.store, selectActiveConversation);
  const activeMessages = useStore(controller.store, selectActiveMessages);
  const activeMessagePagination = useStore(controller.store, selectActiveMessagePagination);
  const isLoadingMessages = useStore(controller.store, state => state.isLoadingMessages);
  const isLoadingOlderMessages = useStore(controller.store, selectIsLoadingOlderMessages);
  const isRecoveringMessages = useStore(controller.store, selectIsRecoveringMessages);
  const isStopping = useStore(controller.store, selectIsStopping);
  const isSending = useStore(controller.store, state => state.isSending);
  const streamingByMessageId = useStore(controller.store, state => state.streamingByMessageId);
  const error = useStore(controller.store, state => state.error);
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const organizationRole = useWorkspaceStore.use.permissionState().organizationRole;
  const isBillingAdmin = organizationRole === 'owner' || organizationRole === 'admin';
  const enableAIChatSkillPreference =
    surface === 'aichat' && (!isEmbedded || runtimeSurface === 'contextual_sidebar');
  const effectiveRuntimeSurface: AIChatRuntimeSurface =
    surface === 'agent-draft' || surface === 'agent-webapp' ? 'external_page_chat' : runtimeSurface;
  const { data: availableSkills = [] } = useAIChatSkills({ enabled: enableAIChatSkillPreference });
  const { data: skillPreference, isLoading: isLoadingSkillPreference } = useAIChatSkillPreference({
    enabled: enableAIChatSkillPreference,
  });
  const updateSkillPreference = useUpdateAIChatSkillPreference();
  const skillDisplayById = useMemo(
    () => buildAIChatSkillDisplayMap(availableSkills, locale),
    [availableSkills, locale]
  );
  const aichatConfigurableSkills = useMemo(
    () =>
      availableSkills.filter(skill => {
        if (!skill.enabled || skill.status === 'invalid') return false;
        if (isHiddenSystemSkill(skill.skill_id)) return false;
        const callers = skill.supported_callers ?? [];
        return callers.length === 0 || callers.includes('aichat');
      }),
    [availableSkills]
  );

  useEffect(() => {
    if (!enableAIChatSkillPreference || !skillPreference) return;
    setDraftSkillPreferenceIds(skillPreference.enabled_skill_ids ?? []);
  }, [enableAIChatSkillPreference, skillPreference]);
  const enableToolGovernanceApprovals =
    surface === 'aichat' && Boolean(controller.continueToolGovernanceDecision);
  const showToolGovernancePermissionControl =
    surface === 'aichat' &&
    (effectiveRuntimeSurface === 'work_chat' ||
      (effectiveRuntimeSurface === 'contextual_sidebar' && enableToolGovernance));
  useEffect(() => {
    if (!showToolGovernancePermissionControl) {
      setToolGovernancePermissionTierLoaded(false);
      return;
    }
    if (typeof window === 'undefined') return;
    const savedTier = window.localStorage.getItem(TOOL_GOVERNANCE_PERMISSION_TIER_STORAGE_KEY);
    setToolGovernancePermissionTier(normalizeToolGovernancePermissionTier(savedTier));
    setToolGovernancePermissionTierLoaded(true);
  }, [showToolGovernancePermissionControl]);
  useEffect(() => {
    if (
      !showToolGovernancePermissionControl ||
      typeof window === 'undefined' ||
      !toolGovernancePermissionTierLoaded
    ) {
      return;
    }
    window.localStorage.setItem(
      TOOL_GOVERNANCE_PERMISSION_TIER_STORAGE_KEY,
      toolGovernancePermissionTier
    );
  }, [
    showToolGovernancePermissionControl,
    toolGovernancePermissionTier,
    toolGovernancePermissionTierLoaded,
  ]);
  const savedSkillPreferenceIds = useMemo(
    () => skillPreference?.enabled_skill_ids ?? [],
    [skillPreference?.enabled_skill_ids]
  );
  const hasSkillPreferenceChanges = useMemo(
    () => !areSkillIdsEqual(draftSkillPreferenceIds, savedSkillPreferenceIds),
    [draftSkillPreferenceIds, savedSkillPreferenceIds]
  );
  const effectiveToolGovernancePermissionTier = showToolGovernancePermissionControl
    ? toolGovernancePermissionTier
    : 'basic';
  const toolGovernanceOperationContext = useMemo<AIChatOperationContext | undefined>(
    () =>
      showToolGovernancePermissionControl
        ? {
            schema: 'zgi.aichat.operation_context.v1',
            version: 1,
            tool_governance: {
              permission_tier: effectiveToolGovernancePermissionTier,
            },
            resources: [],
            capabilities: [],
            risk_summary: {
              requires_confirmation: false,
            },
            summary: {
              resource_count: 0,
              capability_count: 0,
              omitted_resource_count: 0,
              omitted_capability_count: 0,
            },
          }
        : undefined,
    [effectiveToolGovernancePermissionTier, showToolGovernancePermissionControl]
  );
  const messageTopologyKey = useMemo(
    () => buildChatMessageTopologyKey(activeMessages),
    [activeMessages]
  );
  const messageTopology = useMemo(() => {
    if (topologyRef.current?.key === messageTopologyKey) {
      return topologyRef.current.topology;
    }

    const topology = buildChatMessageTopology(activeMessages);
    topologyRef.current = { key: messageTopologyKey, topology };
    return topology;
  }, [activeMessages, messageTopologyKey]);
  const displayMessageIds = useMemo(
    () => selectDisplayMessageIds(activeConversation, messageTopology),
    [activeConversation, messageTopology]
  );
  const messages = useMemo(
    () => selectDisplayMessages(displayMessageIds, activeMessages),
    [activeMessages, displayMessageIds]
  );
  const assetAuditRefreshKey = useMemo(
    () =>
      activeMessages
        .map(message => {
          const invocationCount = message.metadata?.skill_invocations?.length ?? 0;
          return `${message.id}:${message.updated_at}:${message.status}:${invocationCount}`;
        })
        .join('|'),
    [activeMessages]
  );
  const branchNavigationByMessageId = useMemo(
    () => selectBranchNavigationByMessageId(displayMessageIds, messageTopology),
    [displayMessageIds, messageTopology]
  );
  const isHome = !activeConversationId && messages.length === 0 && !isSending;
  const modelMissing = requireModel && !isModelInitializing && !modelSelectorValue.model;
  const {
    bottomRef,
    scrollViewportRef,
    handleMessagesScroll,
    isAutoFollowPaused,
    resumeAutoFollow,
  } = useAIChatScroll({
    messages,
    activeMessagePagination,
    isLoadingMessages,
    isLoadingOlderMessages,
    isSending,
    loadOlderMessages: controller.loadOlderMessages,
  });
  const hasActiveStreamingMessage = useMemo(
    () =>
      Object.values(streamingByMessageId).some(
        streaming =>
          streaming.status === 'streaming' && streaming.conversation_id === activeConversationId
      ),
    [activeConversationId, streamingByMessageId]
  );
  const activeUserInputMessage = useMemo(() => {
    if (
      !activeConversation ||
      activeConversation.runtime_status !== 'idle' ||
      isSending ||
      hasActiveStreamingMessage
    ) {
      return null;
    }
    const leafMessageId = activeConversation.current_leaf_message_id;
    if (!leafMessageId) return null;
    const leafMessage = activeMessages.find(message => message.id === leafMessageId) ?? null;
    const request = leafMessage?.metadata?.user_input_request;
    if (!request?.questions?.some(question => question.question?.trim())) {
      return null;
    }
    return leafMessage;
  }, [activeConversation, activeMessages, hasActiveStreamingMessage, isSending]);
  const activeUserInputRequest = activeUserInputMessage?.metadata?.user_input_request ?? null;
  const activeWorkflowApprovalRequest = useMemo(
    () =>
      surface === 'agent-draft' || surface === 'agent-webapp'
        ? resolveActiveWorkflowApprovalRequest(
            activeConversation,
            activeMessages,
            isSending,
            hasActiveStreamingMessage
          )
        : null,
    [activeConversation, activeMessages, hasActiveStreamingMessage, isSending, surface]
  );
  const canStopPendingWorkflowInteraction = Boolean(
    activeWorkflowApprovalRequest ||
      (activeUserInputMessage?.status === 'waiting_question' &&
        activeUserInputRequest?.source === AGENT_WORKFLOW_QUESTION_SOURCE)
  );
  const messageActionsLocked = Boolean(activeWorkflowApprovalRequest);
  const showResumeScrollButton = isAutoFollowPaused && (isSending || hasActiveStreamingMessage);
  const showAssetAuditControl =
    showToolGovernancePermissionControl && surface === 'aichat' && Boolean(activeConversationId);
  const assetAuditButton = useMemo(
    () =>
      showAssetAuditControl ? (
        <AIChatAssetAuditButton
          conversationId={activeConversationId}
          refreshKey={assetAuditRefreshKey}
          className={embeddedControlButtonClassName}
        />
      ) : null,
    [activeConversationId, assetAuditRefreshKey, showAssetAuditControl]
  );

  useEffect(() => {
    if (!error) {
      lastErrorToastRef.current = null;
      return;
    }

    const matchingErrorMessage = [...activeMessages]
      .reverse()
      .find(message => message.status === 'error' && message.error === error);
    const errorInput = matchingErrorMessage
      ? getAIChatMessageErrorInput(matchingErrorMessage)
      : { message: error };
    const resolvedError = resolveAIChatErrorMessage(
      (key, values) => tGlobal(key as never, values),
      errorInput,
      {
        isAdmin: isBillingAdmin,
        workspaceId: currentWorkspace?.id,
      }
    );
    const toastKey = `${resolvedError.code ?? 'unknown'}:${error}`;

    if (lastErrorToastRef.current === toastKey) {
      return;
    }

    lastErrorToastRef.current = toastKey;
    const toastFn = resolvedError.isBilling ? toast.warning : toast.error;
    toastFn(resolvedError.title || resolvedError.description, {
      id: resolvedError.code ? `aichat-billing-${resolvedError.code}` : undefined,
      description: resolvedError.title ? resolvedError.description : undefined,
      classNames: resolvedError.isBilling ? workflowBillingToastClassNames : undefined,
      action:
        isBillingAdmin && resolvedError.href && resolvedError.actionLabel
          ? createElement(WorkflowBillingToastAction, {
              label: resolvedError.actionLabel,
              onClick: () => router.push(resolvedError.href as string),
            })
          : undefined,
    });
  }, [activeMessages, currentWorkspace?.id, error, isBillingAdmin, router, tGlobal]);

  const conversationSummaries = useMemo<ConversationSummary[]>(
    () =>
      conversations.map(conversation => ({
        id: conversation.id,
        conversationId: conversation.id,
        title: conversation.title,
        dialogueCount: conversation.dialogue_count,
        updatedAt: conversation.updated_at * 1000,
        status: conversation.status,
        metadata: {
          source: conversation.source,
          current_leaf_message_id: conversation.current_leaf_message_id,
          runtime_status: conversation.runtime_status,
          active_message_id: conversation.active_message_id,
          isRecovering:
            conversation.id === activeConversationId
              ? isRecoveringMessages
              : conversation.runtime_status === 'streaming',
          isStopping: conversation.id === activeConversationId ? isStopping : false,
        },
      })),
    [activeConversationId, conversations, isRecoveringMessages, isStopping]
  );
  const conversationSearchKey = useMemo(
    () =>
      [
        'aichat-runtime',
        effectiveRuntimeSurface,
        uploadScope?.type === 'webapp' ? uploadScope.webAppId : 'console',
        'conversations',
        'search',
      ] as const,
    [effectiveRuntimeSurface, uploadScope]
  );
  const searchConversations = useCallback<ConversationSearchFn>(
    (query, limit) =>
      controller.search?.(query, limit, { surface: effectiveRuntimeSurface }) ??
      Promise.resolve([]),
    [controller, effectiveRuntimeSurface]
  );

  const suggestions = useMemo<AIChatSuggestion[]>(() => {
    if (configuredSuggestions) {
      return configuredSuggestions
        .map(text => text.trim())
        .filter(Boolean)
        .slice(0, 6)
        .map((text, index) => ({ text, key: `configured-${index}` }));
    }

    return [
      { text: t('chat.suggestions.email'), key: 'email' },
      { text: t('chat.suggestions.meeting'), key: 'meeting' },
      { text: t('chat.suggestions.report'), key: 'report' },
      { text: t('chat.suggestions.polish'), key: 'polish' },
    ];
  }, [configuredSuggestions, t]);

  const canReplaceRootMessage = useCallback(
    (message: AIChatMessage) => {
      const conversation = activeConversation;
      return Boolean(
        conversation &&
          conversation.runtime_status === 'idle' &&
          conversation.dialogue_count === 1 &&
          !message.parent_id &&
          conversation.current_leaf_message_id === message.id &&
          (message.status === 'completed' ||
            message.status === 'stopped' ||
            message.status === 'error') &&
          activeMessages.length === 1
      );
    },
    [activeConversation, activeMessages.length]
  );

  const handleToggleSidebar = useCallback(() => {
    if (isMobile) {
      setMobileSidebarOpen(true);
      return;
    }
    setSidebarOpen(value => !value);
  }, [isMobile]);

  const handleSend = useCallback(
    async (files: AIChatMessageFile[] = [], useMemory = false): Promise<boolean> => {
      if (activeWorkflowApprovalRequest) {
        toast.info(t('consoleChat.workflow.approvalInputLocked'));
        return false;
      }
      const query = input.trim();
      if (!query || isSending) return false;
      if (requireModel && !modelSelectorValue.model) {
        toast.error(t('consoleChat.modelRequired'));
        return false;
      }
      if (beforeSend) {
        const canSend = await beforeSend();
        if (!canSend) return false;
      }

      setInput('');
      void controller.send({
        query,
        files,
        model: {
          provider: modelSelectorValue.provider,
          model: modelSelectorValue.model,
          parameters: modelSelectorValue.params,
        },
        useMemory: forcedUseMemory ?? useMemory,
        runtimeSurface: effectiveRuntimeSurface,
        operationContext: toolGovernanceOperationContext,
      });
      return true;
    },
    [
      activeWorkflowApprovalRequest,
      beforeSend,
      controller,
      forcedUseMemory,
      input,
      isSending,
      modelSelectorValue,
      requireModel,
      effectiveRuntimeSurface,
      t,
      toolGovernanceOperationContext,
    ]
  );

  const handleUserInputRequestSubmit = useCallback(
    (query: string, useMemory: boolean, answers?: Record<string, string>) => {
      const trimmedQuery = query.trim();
      if (!trimmedQuery || isSending || !activeUserInputMessage) return;
      const activeRequest = activeUserInputMessage.metadata?.user_input_request;
      if (activeRequest?.source === AGENT_WORKFLOW_QUESTION_SOURCE) {
        if (!controller.continueWorkflowQuestion) return;
        const inputs = resolveWorkflowQuestionAnswerInputs(activeRequest, trimmedQuery, answers);
        if (!inputs) return;
        void controller.continueWorkflowQuestion(
          activeUserInputMessage.conversation_id,
          activeUserInputMessage.id,
          inputs
        );
        return;
      }
      if (requireModel && !modelSelectorValue.model) {
        toast.error(t('consoleChat.modelRequired'));
        return;
      }

      void controller.send({
        query: trimmedQuery,
        parentId: activeUserInputMessage.id,
        model: {
          provider: modelSelectorValue.provider,
          model: modelSelectorValue.model,
          parameters: modelSelectorValue.params,
        },
        useMemory: forcedUseMemory ?? useMemory,
        runtimeSurface: effectiveRuntimeSurface,
        operationContext: toolGovernanceOperationContext,
      });
    },
    [
      activeUserInputMessage,
      controller,
      forcedUseMemory,
      isSending,
      modelSelectorValue,
      requireModel,
      effectiveRuntimeSurface,
      t,
      toolGovernanceOperationContext,
    ]
  );

  const handleRegenerate = useCallback(
    (message: AIChatMessage) => {
      const branchCount = branchNavigationByMessageId.get(message.id)?.total ?? 1;
      const canReplaceRoot = canReplaceRootMessage(message);
      if (messageActionsLocked) return;
      if (!canReplaceRoot && (!message.parent_id || branchCount >= MAX_AICHAT_BRANCHES)) return;
      if (requireModel && !modelSelectorValue.model) {
        toast.error(t('consoleChat.modelRequired'));
        return;
      }

      void controller.regenerate(
        message.id,
        {
          provider: modelSelectorValue.provider,
          model: modelSelectorValue.model,
          parameters: modelSelectorValue.params,
        },
        {
          operationContext: toolGovernanceOperationContext,
          runtimeSurface: effectiveRuntimeSurface,
        }
      );
    },
    [
      branchNavigationByMessageId,
      canReplaceRootMessage,
      controller,
      messageActionsLocked,
      modelSelectorValue,
      requireModel,
      effectiveRuntimeSurface,
      t,
      toolGovernanceOperationContext,
    ]
  );

  const handleEditStart = useCallback(
    (message: AIChatMessage) => {
      const branchCount = branchNavigationByMessageId.get(message.id)?.total ?? 1;
      const canReplaceRoot = canReplaceRootMessage(message);
      if (messageActionsLocked) return;
      if (!canReplaceRoot && (!message.parent_id || branchCount >= MAX_AICHAT_BRANCHES)) return;
      setEditingMessageId(message.id);
      setEditingQuery(message.query);
    },
    [branchNavigationByMessageId, canReplaceRootMessage, messageActionsLocked]
  );

  const handleEditCancel = useCallback(() => {
    setEditingMessageId(null);
    setEditingQuery('');
  }, []);

  const handleEditSubmit = useCallback(
    (message: AIChatMessage) => {
      const query = editingQuery.trim();
      const branchCount = branchNavigationByMessageId.get(message.id)?.total ?? 1;
      const canReplaceRoot = canReplaceRootMessage(message);
      if (messageActionsLocked) return;
      if (
        !query ||
        isSending ||
        (!canReplaceRoot && (!message.parent_id || branchCount >= MAX_AICHAT_BRANCHES))
      ) {
        return;
      }
      if (requireModel && !modelSelectorValue.model) {
        toast.error(t('consoleChat.modelRequired'));
        return;
      }

      setEditingMessageId(null);
      setEditingQuery('');
      if (canReplaceRoot) {
        void controller.replaceRootMessage({
          messageId: message.id,
          query,
          model: {
            provider: modelSelectorValue.provider,
            model: modelSelectorValue.model,
            parameters: modelSelectorValue.params,
          },
          runtimeSurface: effectiveRuntimeSurface,
          operationContext: toolGovernanceOperationContext,
        });
        return;
      }

      void controller.send({
        query,
        parentId: message.parent_id,
        model: {
          provider: modelSelectorValue.provider,
          model: modelSelectorValue.model,
          parameters: modelSelectorValue.params,
        },
        useMemory: Boolean(message.metadata?.use_memory),
        forceAdvanceLeaf: true,
        runtimeSurface: effectiveRuntimeSurface,
        operationContext: toolGovernanceOperationContext,
      });
    },
    [
      branchNavigationByMessageId,
      canReplaceRootMessage,
      controller,
      editingQuery,
      isSending,
      messageActionsLocked,
      modelSelectorValue,
      requireModel,
      effectiveRuntimeSurface,
      t,
      toolGovernanceOperationContext,
    ]
  );

  const handleSwitchBranch = useCallback(
    (messageId: string) => {
      setEditingMessageId(null);
      setEditingQuery('');
      controller.switchBranch(messageId);
    },
    [controller]
  );

  const handleWorkflowApprovalSubmit = useCallback(
    (request: AIChatWorkflowApprovalRequest, payload: AIChatWorkflowApprovalSubmitPayload) => {
      if (!controller.continueWorkflowApproval) return;
      void controller.continueWorkflowApproval(request.conversationId, request.messageId, {
        approvalToken: request.approvalToken,
        inputs: payload.inputs,
        action: payload.action,
      });
    },
    [controller]
  );

  const handleToolGovernanceDecision = useCallback(
    (payload: AIChatToolGovernanceDecisionSubmitPayload) => {
      if (!controller.continueToolGovernanceDecision) return;
      const decisionKey = toolGovernanceDecisionKey(
        payload.conversationId,
        payload.messageId,
        payload.correlationId
      );
      setSubmittedToolGovernanceDecisionKeys(current => {
        const next = new Set(current);
        next.add(decisionKey);
        return next;
      });
      if (payload.action === 'approve') {
        setApprovedToolGovernanceDecisionKeys(current => {
          const next = new Set(current);
          next.add(decisionKey);
          return next;
        });
      }
      return controller
        .continueToolGovernanceDecision(
          payload.conversationId,
          payload.messageId,
          payload.correlationId,
          {
            action: payload.action,
            reason: payload.reason,
            remember_for_session: payload.rememberForSession,
          }
        )
        .catch(error => {
          if (isAIChatContinuationLikelyStarted(error)) {
            return;
          }
          setSubmittedToolGovernanceDecisionKeys(current => {
            const next = new Set(current);
            next.delete(decisionKey);
            return next;
          });
          setApprovedToolGovernanceDecisionKeys(current => {
            const next = new Set(current);
            next.delete(decisionKey);
            return next;
          });
          throw error;
        });
    },
    [controller]
  );
  useEffect(() => {
    setSubmittedToolGovernanceDecisionKeys(new Set());
    setApprovedToolGovernanceDecisionKeys(new Set());
  }, [activeConversation?.id]);

  const activeToolGovernanceApprovalFallback = useMemo<ToolGovernancePendingApproval | null>(() => {
    if (
      !enableToolGovernanceApprovals ||
      !activeConversation ||
      activeConversation.runtime_status !== 'idle' ||
      isSending ||
      hasActiveStreamingMessage
    ) {
      return null;
    }
    const leafMessageId = activeConversation.current_leaf_message_id;
    if (!leafMessageId) return null;
    const leafMessage = activeMessages.find(message => message.id === leafMessageId) ?? null;
    if (!leafMessage || leafMessage.status !== 'waiting_approval') return null;
    const persistedTimeline = timelineFromAIChatMessage(leafMessage);
    const streamingTimeline =
      streamingByMessageId[leafMessage.id]?.conversation_id === activeConversation.id
        ? streamingByMessageId[leafMessage.id]?.timeline
        : undefined;
    const timeline = mergeRuntimeTimelineWithMessageTimeline(persistedTimeline, streamingTimeline);
    const approval = resolvePendingToolGovernanceApprovalFromTimeline(
      timeline,
      skillDisplayById,
      locale,
      t,
      handleToolGovernanceDecision
    );
    if (
      approval &&
      (isToolGovernancePendingApprovalDismissed(approval.id, toolGovernanceApprovalScopeId) ||
        submittedToolGovernanceDecisionKeys.has(approval.id))
    ) {
      return null;
    }
    return approval;
  }, [
    activeConversation,
    activeMessages,
    enableToolGovernanceApprovals,
    handleToolGovernanceDecision,
    hasActiveStreamingMessage,
    isSending,
    locale,
    skillDisplayById,
    streamingByMessageId,
    submittedToolGovernanceDecisionKeys,
    t,
    toolGovernanceApprovalScopeId,
  ]);

  const handleNewChat = useCallback(() => {
    if (isHome) {
      toast.info(t('chat.alreadyInDraft'));
      setMobileSidebarOpen(false);
      return;
    }
    if (onStartNewConversation) {
      onStartNewConversation();
    } else {
      controller.startNew();
    }
    setMobileSidebarOpen(false);
  }, [controller, isHome, onStartNewConversation, t]);

  const handleSelectConversation = useCallback(
    (id: string) => {
      if (onSelectConversation) {
        onSelectConversation(id);
      } else {
        void controller.select(id);
      }
      setMobileSidebarOpen(false);
    },
    [controller, onSelectConversation]
  );

  const handleDeleteConversation = useCallback(
    (id: string) => {
      void controller.remove(id);
      setMobileSidebarOpen(false);
    },
    [controller]
  );

  const handleRenameConversation = useCallback(
    async (id: string, title: string) => {
      await controller.rename(id, title);
    },
    [controller]
  );

  const handleToggleSkillPreference = useCallback((skillId: string, checked: boolean) => {
    setDraftSkillPreferenceIds(current =>
      checked ? Array.from(new Set([...current, skillId])) : current.filter(id => id !== skillId)
    );
  }, []);

  const handleSkillPreferenceOpenChange = useCallback(
    (open: boolean) => {
      if (!open && updateSkillPreference.isPending) return;
      setDraftSkillPreferenceIds(savedSkillPreferenceIds);
      setSkillPreferenceOpen(open);
    },
    [savedSkillPreferenceIds, updateSkillPreference.isPending]
  );

  const handleSaveSkillPreference = useCallback(() => {
    const requestedSkillIds = normalizeSkillIds(draftSkillPreferenceIds);
    updateSkillPreference.mutate(
      { payload: { enabled_skill_ids: requestedSkillIds } },
      {
        onSuccess: response => {
          const savedSkillIds = normalizeSkillIds(
            response.data?.enabled_skill_ids ?? requestedSkillIds
          );
          setDraftSkillPreferenceIds(savedSkillIds);
          setSkillPreferenceOpen(false);
          if (areSkillIdsEqual(requestedSkillIds, savedSkillIds)) {
            toast.success(t('consoleChat.skillPreferences.saved'));
          } else {
            toast.warning(t('consoleChat.skillPreferences.savedWithChanges'));
          }
        },
        onError: error => {
          toast.error(
            error instanceof Error ? error.message : t('consoleChat.skillPreferences.saveFailed')
          );
        },
      }
    );
  }, [draftSkillPreferenceIds, t, updateSkillPreference]);

  const skillPreferenceAction = useMemo(() => {
    if (!enableAIChatSkillPreference) return null;
    return (
      <Button
        variant="ghost"
        isIcon
        className={cn(
          isEmbedded ? embeddedControlButtonClassName : 'size-8 text-muted-foreground'
        )}
        onClick={() => handleSkillPreferenceOpenChange(true)}
        title={t('consoleChat.skillPreferences.action')}
        aria-label={t('consoleChat.skillPreferences.action')}
      >
        <Settings2 className="size-4" />
      </Button>
    );
  }, [enableAIChatSkillPreference, handleSkillPreferenceOpenChange, isEmbedded, t]);

  const embeddedConversationControls = useMemo(() => {
    if (!showEmbeddedConversationDrawer) return null;
    const controls = {
      openConversations: () => setMobileSidebarOpen(true),
      startNewConversation: handleNewChat,
      isHome,
    };
    if (renderEmbeddedConversationControls) {
      return renderEmbeddedConversationControls(controls);
    }
    return (
      <AIChatEmbeddedConversationControls
        openConversations={controls.openConversations}
        startNewConversation={controls.startNewConversation}
        conversationsLabel={t('consoleChat.toggleSidebar')}
        newConversationLabel={t('chat.newConversation')}
        trailingAction={
          assetAuditButton || skillPreferenceAction ? (
            <>
              {assetAuditButton}
              {skillPreferenceAction}
            </>
          ) : null
        }
      />
    );
  }, [
    assetAuditButton,
    handleNewChat,
    isHome,
    renderEmbeddedConversationControls,
    showEmbeddedConversationDrawer,
    skillPreferenceAction,
    t,
  ]);

  useEffect(() => {
    if (
      embeddedConversationControlsMode !== 'external' ||
      !embeddedConversationControlsPortalId ||
      typeof document === 'undefined'
    ) {
      setExternalControlsPortal(null);
      return;
    }
    setExternalControlsPortal(document.getElementById(embeddedConversationControlsPortalId));
  }, [embeddedConversationControlsMode, embeddedConversationControlsPortalId]);

  return (
    <div className="flex h-full w-full overflow-hidden bg-background" style={themeStyle}>
      {!isEmbedded ? (
        <div className="hidden md:block">
          <Sidebar
            activeId={activeConversationId}
            conversations={conversationSummaries}
            isOpen={sidebarOpen}
            isHome={isHome}
            onNewChat={handleNewChat}
            onSelect={handleSelectConversation}
            onDelete={handleDeleteConversation}
            onRename={handleRenameConversation}
            backgroundImage={AICHAT_SIDEBAR_BG_IMAGE}
            search={searchConversations}
            searchKey={conversationSearchKey}
          />
        </div>
      ) : null}

      <ToolGovernancePendingApprovalScopeProvider scopeId={toolGovernanceApprovalScopeId}>
        <main className="relative flex min-w-0 flex-1 flex-col overflow-hidden bg-background">
          {!isEmbedded ? (
            <AIChatHeader
              isMobile={isMobile}
              isHome={isHome}
              title={activeConversation?.title || t('consoleChat.title')}
              onToggleSidebar={handleToggleSidebar}
              onStartNew={handleNewChat}
              rightAction={
                assetAuditButton || skillPreferenceAction ? (
                  <div className="flex items-center justify-end gap-1">
                    {assetAuditButton}
                    {skillPreferenceAction}
                  </div>
                ) : undefined
              }
            />
          ) : null}

          {showEmbeddedConversationDrawer && embeddedConversationControlsMode === 'internal' ? (
            <div
              className={cn(
                'absolute z-30',
                embeddedConversationControlsClassName ?? 'left-3 top-3'
              )}
            >
              {embeddedConversationControls}
            </div>
          ) : null}

          {externalControlsPortal && embeddedConversationControls
            ? createPortal(embeddedConversationControls, externalControlsPortal)
            : null}

          <AIChatMessageList
            messages={messages}
            activeConversation={activeConversation}
            activeMessageCount={activeMessages.length}
            branchNavigationByMessageId={branchNavigationByMessageId}
            isLoadingMessages={isLoadingMessages}
            isLoadingOlderMessages={isLoadingOlderMessages}
            isSending={isSending || messageActionsLocked}
            streamingByMessageId={streamingByMessageId}
            skillDisplayById={skillDisplayById}
            editingMessageId={editingMessageId}
            editingQuery={editingQuery}
            bottomRef={bottomRef}
            scrollViewportRef={scrollViewportRef}
            bottomSpacerHeight={Math.max(inputAreaHeight + 72, 180)}
            onScroll={handleMessagesScroll}
            onRegenerate={handleRegenerate}
            onToolGovernanceDecision={
              enableToolGovernanceApprovals ? handleToolGovernanceDecision : undefined
            }
            enableToolGovernanceApprovals={enableToolGovernanceApprovals}
            onSwitchBranch={handleSwitchBranch}
            onEditStart={handleEditStart}
            onEditChange={setEditingQuery}
            onEditCancel={handleEditCancel}
            onEditSubmit={handleEditSubmit}
            showAssistantModelMeta={showAssistantModelMeta}
            layout={isEmbedded ? 'embedded' : 'full'}
            showMemoryKey={surface !== 'agent-webapp'}
            showSkillEventDetails={surface !== 'agent-webapp'}
            approvedToolGovernanceDecisionKeys={approvedToolGovernanceDecisionKeys}
          />

          <AIChatHomeView
            isVisible={isHome && !isLoadingMessages}
            suggestions={suggestions}
            onSelectSuggestion={setInput}
            brand={homeBrand}
            title={homeTitle}
            description={homeDescription}
            composerHeight={inputAreaHeight}
            surface={surface}
          />

          {showResumeScrollButton ? (
            <Button
              type="button"
              size="sm"
              variant="secondary"
              className="absolute left-1/2 z-30 -translate-x-1/2 rounded-full border bg-background/95 px-3 shadow-lg backdrop-blur"
              style={{ bottom: Math.max(inputAreaHeight + 18, 96) }}
              onClick={resumeAutoFollow}
            >
              <ArrowDown className="mr-1.5 size-4" />
              {t('consoleChat.resumeAutoScroll')}
            </Button>
          ) : null}

          <AIChatInputArea
            isHome={isHome}
            isLoadingMessages={isLoadingMessages}
            input={input}
            modelSelectorValue={modelSelectorValue}
            modelProps={modelProps}
            supportsVisionOverride={supportsVisionOverride}
            isModelInitializing={isModelInitializing}
            modelMissing={modelMissing}
            isSending={isSending}
            canStop={canStopPendingWorkflowInteraction || isSending}
            isStopping={isStopping}
            onInputChange={setInput}
            onSend={handleSend}
            activeUserInputRequest={activeUserInputRequest}
            onUserInputRequestSubmit={handleUserInputRequestSubmit}
            activeWorkflowApprovalRequest={activeWorkflowApprovalRequest}
            onWorkflowApprovalSubmit={handleWorkflowApprovalSubmit}
            onStop={controller.stop}
            onModelChange={onModelChange}
            onHeightChange={setInputAreaHeight}
            showModelSelector={showModelSelector}
            showMemoryToggle={showMemoryToggle}
            enableUpload={enableUpload}
            uploadScope={uploadScope}
            showFileLibraryPicker={showFileLibraryPicker}
            allowWorkspaceSwitch={allowWorkspaceSwitch}
            inputPlaceholder={inputPlaceholder}
            surface={surface}
            showToolGovernancePermissionControl={showToolGovernancePermissionControl}
            toolGovernancePermissionTier={toolGovernancePermissionTier}
            onToolGovernancePermissionTierChange={setToolGovernancePermissionTier}
            enableToolGovernanceApprovals={enableToolGovernanceApprovals}
            activeConversationId={activeConversation?.id ?? null}
            activeToolGovernanceMessageId={
              activeConversation?.active_message_id ??
              activeConversation?.current_leaf_message_id ??
              null
            }
            activeToolGovernanceApprovalFallback={activeToolGovernanceApprovalFallback}
          />
        </main>
      </ToolGovernancePendingApprovalScopeProvider>

      {!isEmbedded || showEmbeddedConversationDrawer ? (
        <Sheet open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
          <SheetContent side="left" className="max-w-none p-0 sm:max-w-sm" showClose={false}>
            <SheetTitle className="sr-only">{t('chat.conversations')}</SheetTitle>
            <SheetDescription className="sr-only">
              {t('consoleChat.conversationListDescription')}
            </SheetDescription>
            <Sidebar
              activeId={activeConversationId}
              conversations={conversationSummaries}
              isOpen
              isHome={isHome}
              className="w-full border-r-0"
              onNewChat={handleNewChat}
              onSelect={handleSelectConversation}
              onDelete={handleDeleteConversation}
              onRename={handleRenameConversation}
              backgroundImage={AICHAT_SIDEBAR_BG_IMAGE}
              onClose={() => setMobileSidebarOpen(false)}
              search={searchConversations}
              searchKey={conversationSearchKey}
            />
          </SheetContent>
        </Sheet>
      ) : null}

      {enableAIChatSkillPreference ? (
        <AIChatSkillPreferenceDialog
          open={skillPreferenceOpen}
          locale={locale}
          skills={aichatConfigurableSkills}
          selectedSkillIds={draftSkillPreferenceIds}
          isLoading={isLoadingSkillPreference}
          isSaving={updateSkillPreference.isPending}
          hasChanges={hasSkillPreferenceChanges}
          onOpenChange={handleSkillPreferenceOpenChange}
          onToggleSkill={handleToggleSkillPreference}
          onSave={handleSaveSkillPreference}
        />
      ) : null}
    </div>
  );
}

function resolveActiveWorkflowApprovalRequest(
  conversation: AIChatConversation | null,
  messages: AIChatMessage[],
  isSending: boolean,
  hasActiveStreamingMessage: boolean
): AIChatWorkflowApprovalRequest | null {
  if (
    !conversation ||
    conversation.runtime_status !== 'idle' ||
    isSending ||
    hasActiveStreamingMessage
  ) {
    return null;
  }
  const leafMessageId = conversation.current_leaf_message_id;
  if (!leafMessageId) return null;
  const leafMessage = messages.find(message => message.id === leafMessageId) ?? null;
  if (!leafMessage || leafMessage.status !== 'waiting_approval') return null;
  const continuation = leafMessage.metadata?.agent_workflow_continuation;
  const continuationWorkflowRunId = stringFromRecord(continuation, 'workflow_run_id');
  const continuationStatus = stringFromRecord(continuation, 'status');
  const workflowRuns = leafMessage.metadata?.workflow_runs ?? [];
  const runWithApproval = [...workflowRuns]
    .reverse()
    .find(
      run =>
        isActiveWorkflowApprovalRun(run, continuationWorkflowRunId, continuationStatus) &&
        Boolean(resolveApprovalToken(run.approval))
    );
  const runForContinuation = continuationWorkflowRunId
    ? workflowRuns.find(run => stringFromUnknown(run.workflow_run_id) === continuationWorkflowRunId)
    : null;
  if (
    !runWithApproval &&
    runForContinuation &&
    !isActiveWorkflowApprovalRun(runForContinuation, continuationWorkflowRunId, continuationStatus)
  ) {
    return null;
  }
  const approval = runWithApproval?.approval;
  const inlineApprovalForm = normalizeApprovalRuntimeForm(approval?.approval_form);
  const approvalToken =
    resolveApprovalToken(approval) ||
    stringFromRecord(continuation, 'approval_token') ||
    stringFromUnknown(inlineApprovalForm?.token);
  if (!approvalToken) return null;
  return {
    conversationId: conversation.id,
    messageId: leafMessage.id,
    workflowRunId: stringFromUnknown(runWithApproval?.workflow_run_id) || continuationWorkflowRunId,
    approvalToken,
    approvalUrl: stringFromUnknown(approval?.approval_url),
    approvalFormId:
      stringFromUnknown(approval?.approval_form_id) || stringFromUnknown(inlineApprovalForm?.id),
    approvalForm: inlineApprovalForm,
  };
}

function isActiveWorkflowApprovalRun(
  run: AIChatWorkflowRunMetadata,
  continuationWorkflowRunId: string,
  continuationStatus: string
): boolean {
  const status = String(run.status ?? '').toLowerCase();
  const runId = stringFromUnknown(run.workflow_run_id);
  const isContinuationWaitingRun =
    Boolean(continuationWorkflowRunId && runId === continuationWorkflowRunId) &&
    continuationStatus === 'waiting_approval';
  if (
    !isContinuationWaitingRun &&
    status !== 'pending_approval' &&
    status !== 'waiting_approval' &&
    status !== 'paused'
  ) {
    return false;
  }
  if (run.approval_result || run.approval_expired) return false;
  const approvalStatus = String(run.approval?.status ?? '').toLowerCase();
  return !['submitted', 'approved', 'rejected', 'expired'].includes(approvalStatus);
}

function stringFromUnknown(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function resolveApprovalToken(approval: unknown): string {
  if (!approval || typeof approval !== 'object') return '';
  const record = approval as Record<string, unknown>;
  const inlineForm = normalizeApprovalRuntimeForm(record.approval_form);
  return (
    stringFromRecord(record, 'approval_token') ||
    stringFromRecord(record, 'token') ||
    stringFromUnknown(inlineForm?.token)
  );
}

function stringFromRecord(value: unknown, key: string): string {
  if (!value || typeof value !== 'object') return '';
  return stringFromUnknown((value as Record<string, unknown>)[key]);
}
