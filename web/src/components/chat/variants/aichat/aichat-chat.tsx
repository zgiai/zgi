'use client';

import type { FileUploadSelection } from '@/services/upload.service';
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
import { ArrowDown, RefreshCw } from 'lucide-react';
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
  shouldTreatConversationAsRunning,
  mergeRuntimeTimelineWithMessageTimeline,
  timelineFromAIChatMessage,
} from '@/components/chat/controllers/aichat/selectors';
import { applyConnectorRuntimeResolutionState } from '@/components/chat/controllers/aichat/state-reducers';
import { Sidebar } from '@/components/chat/variants/common/sidebar';
import { Sheet, SheetContent, SheetDescription, SheetTitle } from '@/components/ui/sheet';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  useAIChatSkillPreference,
  useAIChatSkills,
  useUpdateAIChatSkillPreference,
} from '@/hooks/aichat/use-aichat-skills';
import { useSystemFeatures } from '@/hooks/auth/use-system-features';
import { useLocale } from '@/hooks/use-locale';
import { useIsMobile } from '@/hooks/use-mobile';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { useWorkspaceStore } from '@/store/workspace-store';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { useChatConnectorDraft } from './use-chat-connector-draft';
import { useChatConnectorPreferences } from '@/hooks/connectors/use-chat-connector-preferences';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import type {
  AIChatConversation,
  AIChatMessage,
  AIChatMessageFile,
  AIChatRuntimeInput,
  AIChatRuntimeSurface,
  AIChatUserInputRequest,
  AIChatWorkflowQuestionAnswerInputs,
  AIChatWorkflowRunMetadata,
} from '@/services/types/aichat';
import {
  buildChatMessageTopology,
  buildChatMessageTopologyKey,
  type ChatMessageTopology,
} from '@/components/chat/utils/message-tree';
import { AIChatHeader } from '@/components/chat/variants/aichat/chat-header';
import { AIChatHomeView } from '@/components/chat/variants/aichat/home-view';
import { AIChatRuntimeErrorNotice } from '@/components/chat/variants/aichat/runtime-error-notice';
import {
  AIChatAssetAuditButton,
  AIChatAssetAuditPanel,
} from '@/components/chat/variants/aichat/asset-audit-button';
import type { OpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';
import {
  AIChatEmbeddedConversationControls,
  embeddedControlButtonClassName,
} from '@/components/chat/variants/aichat/embedded-conversation-controls';
import {
  AIChatInputArea,
  type AIChatUploadScope,
} from '@/components/chat/variants/aichat/input-area';
import {
  AIChatMessageList,
  type AIChatPendingUserMessage,
} from '@/components/chat/variants/aichat/message-list';
import {
  resolvePendingToolGovernanceApprovalFromTimeline,
  type AIChatToolGovernanceDecisionSubmitPayload,
  type AITaskProposalActionPayload,
} from '@/components/chat/variants/aichat/agentic-timeline';
import {
  buildAIChatSkillDisplayMap,
  isSkillSelectableForCaller,
} from '@/components/chat/variants/aichat/skill-display';
import { normalizeAIChatSkillIds } from '@/components/chat/variants/aichat/skill-identity';
import { AIChatSkillPreferenceDialog } from '@/components/chat/variants/aichat/skill-preference-dialog';
import {
  AIChatConnectedAppsDialog,
  type AIChatConnectedAppsSummary,
} from '@/components/chat/variants/aichat/connected-apps-dialog';
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
import {
  submitConnectorRuntimeDecision,
  type ConnectorRuntimeConfirmationCommand,
  type ConnectorRuntimeDecisionResult,
} from '@/components/connectors/runtime';
import { isWorkflowApprovalInlineAllowed } from '@/components/workflow/approval/workflow-approval-surface';
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
import type { AvailableModelUseCase, ModelUseCase } from '@/services/types/model';
import { messageFilesForReplay } from '@/components/chat/runtime/controller/chat-runtime-controller-utils';
import {
  isImageExtension,
  isVisionModel,
} from '@/components/chat/variants/aichat/input-area-utils';
import type { AIChatVoiceTranscriber } from '@/components/chat/variants/aichat/voice/pcm-audio';
import type { AIChatSpeechSynthesizer } from '@/components/chat/variants/aichat/voice/speech-playback';
import { useAgentSpeechPlayback } from '@/components/chat/variants/aichat/voice/use-agent-speech-playback';
import { AIChatRuntimeInputQueue } from '@/components/chat/variants/aichat/runtime-input-queue';
import { isDraftAIChatConversationId } from '@/components/chat/utils/aichat-message';
import { selectRuntimeInputQueue } from '@/components/chat/utils/runtime-input-reconcile';
import {
  enqueueQueuedChatTurn,
  MAX_QUEUED_CHAT_TURNS,
  removeQueuedChatTurn,
  type AIChatQueuedTurn,
} from '@/components/chat/utils/queued-turns';
import { type AIChatShellSurface, resolveChatSurfaceProfile } from './chat-surface-profile';

export { AIChatMessageBubble } from '@/components/chat/variants/aichat/message-bubble';
export type { AIChatModelValue } from '@/components/chat/variants/aichat/types';

interface AIChatShellProps {
  controller: AIChatController;
  modelSelectorValue: AIChatModelValue;
  modelSelectorWarning?: boolean;
  modelProps?: ModelSelectorModelProps | null;
  supportsVisionOverride?: boolean;
  isModelInitializing?: boolean;
  onModelChange: (value: ModelSelectorValue) => void;
  beforeSend?: () => boolean | Promise<boolean>;
  inputTopNotice?: React.ReactNode;
  variant?: 'full' | 'embedded';
  showModelSelector?: boolean;
  modelUseCase?: ModelUseCase;
  modelAvailabilityUseCase?: AvailableModelUseCase;
  preferredModelUseCase?: ModelUseCase;
  requireModel?: boolean;
  showMemoryEntry?: boolean;
  forcedUseMemory?: boolean;
  enableUpload?: boolean;
  fileUploadPolicy?: FileUploadSelection | null;
  uploadRequiresLogin?: boolean;
  uploadScope?: AIChatUploadScope;
  showFileLibraryPicker?: boolean;
  allowWorkspaceSwitch?: boolean;
  homeBrand?: React.ReactNode;
  openingGuideBrand?: OpeningGuideBrand;
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
  routeConversationId?: string | null;
  isConversationTransitioning?: boolean;
  conversationListTitle?: string;
  conversationListDescription?: string;
  defaultSidebarOpen?: boolean;
  showAssistantModelMeta?: boolean;
  showSkillEventDetails?: boolean;
  enableMessageReplay?: boolean;
  messageDensity?: 'default' | 'compact';
  surface?: AIChatShellSurface;
  runtimeSurface?: AIChatRuntimeSurface;
  themeColor?: string;
  enableToolGovernance?: boolean;
  onTaskProposalAction?: (payload: AITaskProposalActionPayload) => void | Promise<void>;
  onTaskMutation?: () => void;
  voiceTranscriber?: AIChatVoiceTranscriber;
  speechSynthesizer?: AIChatSpeechSynthesizer;
  headerRightAction?: React.ReactNode;
  // Text the composer starts with, e.g. a follow-up written for the reader;
  // a new value replaces what is typed.
  initialInput?: string;
}

const CHAT_THEME_PRIMARY: Record<string, string> = {
  blue: 'hsl(217 91% 60%)',
  emerald: 'hsl(160 84% 39%)',
  violet: 'hsl(262 83% 58%)',
  rose: 'hsl(346 77% 50%)',
  amber: 'hsl(38 92% 50%)',
  slate: 'hsl(215 20% 45%)',
};
const AGENT_WORKFLOW_QUESTION_SOURCE = 'agent_workflow_question_answer';
const AICHAT_WAITING_FOR_PERSON_STATUSES: ReadonlySet<AIChatMessage['status']> = new Set([
  'waiting_approval',
  'waiting_question',
  'waiting_client_action',
  'waiting_budget',
]);
const TOOL_GOVERNANCE_PERMISSION_TIER_STORAGE_KEY = 'zgi:aichat:tool-governance-permission-tier';

type PendingVisionReplayAction =
  | { type: 'regenerate'; message: AIChatMessage }
  | { type: 'edit'; message: AIChatMessage; query: string };

function isReplayImage(file: AIChatMessageFile): boolean {
  return (
    file.kind === 'image' ||
    file.filtered_reason === 'model_without_vision' ||
    file.mime_type?.toLowerCase().startsWith('image/') === true ||
    isImageExtension(file.extension) ||
    isImageExtension(file.name.split('.').pop())
  );
}

function getReplayImageCount(message: AIChatMessage): number {
  return messageFilesForReplay(message).filter(isReplayImage).length;
}

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

function normalizeSkillIds(skillIds: unknown) {
  return normalizeAIChatSkillIds(skillIds).sort();
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
): AIChatWorkflowQuestionAnswerInputs | null {
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
    question_answer_node_id: request.node_id?.trim() || undefined,
    question_answer_round: request.round,
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
  modelSelectorWarning = false,
  modelProps,
  supportsVisionOverride,
  isModelInitializing = false,
  onModelChange,
  beforeSend,
  inputTopNotice,
  variant = 'full',
  showModelSelector = true,
  modelUseCase = 'agent',
  modelAvailabilityUseCase,
  preferredModelUseCase,
  requireModel = true,
  showMemoryEntry = false,
  forcedUseMemory,
  enableUpload = true,
  fileUploadPolicy,
  uploadRequiresLogin,
  uploadScope,
  showFileLibraryPicker = true,
  allowWorkspaceSwitch = false,
  homeBrand,
  openingGuideBrand,
  homeTitle,
  initialInput,
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
  routeConversationId,
  isConversationTransitioning = false,
  conversationListTitle,
  conversationListDescription,
  defaultSidebarOpen = true,
  showAssistantModelMeta = true,
  showSkillEventDetails = true,
  enableMessageReplay = true,
  messageDensity = 'default',
  surface = 'aichat',
  runtimeSurface = 'work_chat',
  themeColor,
  enableToolGovernance = false,
  onTaskProposalAction,
  onTaskMutation,
  voiceTranscriber,
  speechSynthesizer,
  headerRightAction,
}: AIChatShellProps) {
  const router = useRouter();
  const t = useT('webapp');
  const connectorT = useT('connectors');
  const tGlobal = useT();
  const { locale } = useLocale();
  const isMobile = useIsMobile();
  // What this surface offers is decided in one place (ADR 0050).
  const surfaceProfile = resolveChatSurfaceProfile({
    surface,
    variant,
    runtimeSurface,
    enableToolGovernance,
    transport: {
      supportsToolGovernanceApproval: controller.supportsToolGovernanceApproval,
      supportsRuntimeInput: controller.supportsRuntimeInput,
    },
  });
  const isEmbedded = surfaceProfile.isEmbedded;
  const chatSurfaceBackground = surfaceProfile.background;
  const toolGovernanceApprovalScopeId = useId();
  const showEmbeddedConversationDrawer = isEmbedded && embeddedConversationMode === 'drawer';
  const themeStyle = useMemo<CSSProperties | undefined>(() => {
    const primary = themeColor ? CHAT_THEME_PRIMARY[themeColor] : undefined;
    return primary ? ({ '--primary': primary } as CSSProperties) : undefined;
  }, [themeColor]);
  const [input, setInput] = useState('');
  useEffect(() => {
    if (initialInput) setInput(initialInput);
  }, [initialInput]);
  const [queuedTurnsByConversation, setQueuedTurnsByConversation] = useState<
    Record<string, AIChatQueuedTurn[]>
  >({});
  const [guidingQueuedTurnIDs, setGuidingQueuedTurnIDs] = useState<Set<string>>(() => new Set());
  const [editingMessageId, setEditingMessageId] = useState<string | null>(null);
  const [editingQuery, setEditingQuery] = useState('');
  const [selectedModelProps, setSelectedModelProps] = useState<ModelSelectorModelProps | null>(
    null
  );
  const [pendingVisionReplayAction, setPendingVisionReplayAction] =
    useState<PendingVisionReplayAction | null>(null);
  const [sidebarOpen, setSidebarOpen] = useState(defaultSidebarOpen);
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const [embeddedAssetAuditOpen, setEmbeddedAssetAuditOpen] = useState(false);
  const [externalControlsPortal, setExternalControlsPortal] = useState<HTMLElement | null>(null);
  const [skillPreferenceOpen, setSkillPreferenceOpen] = useState(false);
  const [connectedAppsOpen, setConnectedAppsOpen] = useState(false);
  const [connectedAppsAnchorElement, setConnectedAppsAnchorElement] = useState<HTMLElement | null>(
    null
  );
  const [connectedAppsSummary, setConnectedAppsSummary] = useState<AIChatConnectedAppsSummary>({
    selectedConnectionCount: 0,
    hasAttentionRequired: false,
  });
  const [draftSkillPreferenceIds, setDraftSkillPreferenceIds] = useState<string[]>([]);
  const [submittedToolGovernanceDecisionKeys, setSubmittedToolGovernanceDecisionKeys] = useState<
    Set<string>
  >(() => new Set());
  const [pendingUserMessage, setPendingUserMessage] = useState<AIChatPendingUserMessage | null>(
    null
  );
  const [inputAreaHeight, setInputAreaHeight] = useState(160);
  const [toolGovernancePermissionTier, setToolGovernancePermissionTier] =
    useState<AIChatToolGovernancePermissionTier>('basic');
  const [toolGovernancePermissionTierLoaded, setToolGovernancePermissionTierLoaded] =
    useState(false);
  const topologyRef = useRef<{ key: string; topology: ChatMessageTopology } | null>(null);
  const lastErrorToastRef = useRef<string | null>(null);
  const autoSendingQueuedTurnRef = useRef<string | null>(null);
  const previousQueuedConversationIDRef = useRef<string | null>(null);

  const conversations = useStore(controller.store, state => state.conversations);
  const conversationPagination = useStore(controller.store, state => state.pagination);
  const activeConversationId = useStore(controller.store, state => state.activeConversationId);
  const activeConversation = useStore(controller.store, selectActiveConversation);
  const activeConversationRunning = useStore(controller.store, state =>
    shouldTreatConversationAsRunning(state, state.activeConversationId)
  );
  const activeMessages = useStore(controller.store, selectActiveMessages);
  const activeMessagePagination = useStore(controller.store, selectActiveMessagePagination);
  const isLoadingMessages = useStore(controller.store, state => state.isLoadingMessages);
  const selectedConversationId =
    routeConversationId === undefined ? activeConversationId : routeConversationId;
  const effectiveIsLoadingMessages = isLoadingMessages || isConversationTransitioning;
  const isLoadingConversationList = useStore(controller.store, state => state.isLoadingList);
  const isLoadingOlderMessages = useStore(controller.store, selectIsLoadingOlderMessages);
  const isRecoveringMessages = useStore(controller.store, selectIsRecoveringMessages);
  const isStopping = useStore(controller.store, selectIsStopping);
  const isSending = useStore(controller.store, state => state.isSending);
  const streamingByMessageId = useStore(controller.store, state => state.streamingByMessageId);
  const error = useStore(controller.store, state => state.error);
  const speechPlaybackErrorMessages = useMemo(
    () => ({
      timeout: t('consoleChat.voice.errors.playbackTimeout'),
      balance: t('consoleChat.voice.errors.playbackBalance'),
      quota: t('consoleChat.voice.errors.playbackQuota'),
      unavailable: t('consoleChat.voice.errors.playbackUnavailable'),
      failed: t('consoleChat.voice.errors.playbackFailed'),
    }),
    [t]
  );
  const speechPlayback = useAgentSpeechPlayback({
    synthesizer: speechSynthesizer,
    messages: activeMessages,
    conversationId: activeConversationId,
    isLoadingMessages: effectiveIsLoadingMessages,
    playbackErrorMessages: speechPlaybackErrorMessages,
  });
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const account = useAuthStore.use.user();
  const organization = useOrganizationStore.use.currentOrganization();
  const systemFeatures = useSystemFeatures();
  const { canManageModelConfig: isBillingAdmin } = useAccountCapabilities();
  const enableAIChatSkillPreference = surfaceProfile.skillAndConnectorPickers;
  const connectorsEnabled = enableAIChatSkillPreference;
  const legacyEnabled =
    enableAIChatSkillPreference && Boolean(systemFeatures.data?.enable_connectors);
  const enableAIChatConnectedApps = connectorsEnabled || legacyEnabled;
  const connectorConversationId =
    activeConversationId && !activeConversationId.startsWith('draft-') ? activeConversationId : '';
  const inheritConnectorDefaults = connectorsEnabled && !connectorConversationId;
  const { data: connectorDefaultsResponse, isError: connectorDefaultsError } =
    useChatConnectorPreferences(inheritConnectorDefaults);
  // Load on chat mount, not on picker open. Persisted chats and Agent surfaces
  // keep their explicit bindings and never acquire personal defaults.
  const connectorDraft = useChatConnectorDraft(
    JSON.stringify([account?.id, organization?.id, currentWorkspace?.id]),
    activeConversationId,
    {
      enabled: inheritConnectorDefaults,
      data: connectorDefaultsResponse?.data,
      isError: connectorDefaultsError,
    }
  );
  const connectorSelectedCount = connectorConversationId
    ? connectedAppsSummary.selectedConnectionCount
    : connectorDraft.draft.bindings.length;
  const effectiveRuntimeSurface: AIChatRuntimeSurface = surfaceProfile.runtimeSurface;
  const matchingModelProps = useMemo(() => {
    const matchesSelection = (candidate: ModelSelectorModelProps | null | undefined) =>
      Boolean(
        candidate &&
          candidate.provider === modelSelectorValue.provider &&
          candidate.model === modelSelectorValue.model
      );

    if (matchesSelection(modelProps)) return modelProps ?? null;
    if (matchesSelection(selectedModelProps)) return selectedModelProps;
    return null;
  }, [modelProps, modelSelectorValue.model, modelSelectorValue.provider, selectedModelProps]);
  const selectedModelSupportsVision =
    typeof supportsVisionOverride === 'boolean'
      ? supportsVisionOverride
      : matchingModelProps
        ? isVisionModel(matchingModelProps)
        : null;

  useEffect(() => {
    setConnectedAppsOpen(false);
  }, [connectorConversationId]);

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
        return isSkillSelectableForCaller(skill, 'aichat');
      }),
    [availableSkills]
  );

  useEffect(() => {
    if (!enableAIChatSkillPreference || !skillPreference) return;
    setDraftSkillPreferenceIds(skillPreference.enabled_skill_ids ?? []);
  }, [enableAIChatSkillPreference, skillPreference]);
  const enableToolGovernanceApprovals = surfaceProfile.toolGovernanceApprovals;
  const showToolGovernancePermissionControl = surfaceProfile.toolGovernancePermissionControl;
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
  const runtimeInputQueue = useMemo<AIChatRuntimeInput[]>(
    () => selectRuntimeInputQueue(messages, activeConversation?.active_message_id),
    [activeConversation?.active_message_id, messages]
  );
  const activeQueuedTurns = useMemo(
    () => (activeConversationId ? (queuedTurnsByConversation[activeConversationId] ?? []) : []),
    [activeConversationId, queuedTurnsByConversation]
  );
  const runtimeInputQueueFull =
    runtimeInputQueue.length + activeQueuedTurns.length >= MAX_QUEUED_CHAT_TURNS;
  useEffect(() => {
    const previousConversationID = previousQueuedConversationIDRef.current;
    previousQueuedConversationIDRef.current = activeConversationId;
    if (
      !previousConversationID ||
      !activeConversationId ||
      previousConversationID === activeConversationId ||
      !isDraftAIChatConversationId(previousConversationID)
    ) {
      return;
    }
    setQueuedTurnsByConversation(current => {
      const draftTurns = current[previousConversationID] ?? [];
      if (draftTurns.length === 0) return current;
      const next = { ...current };
      delete next[previousConversationID];
      next[activeConversationId] = draftTurns.reduce(
        (turns, turn) => enqueueQueuedChatTurn(turns, turn),
        next[activeConversationId] ?? []
      );
      return next;
    });
  }, [activeConversationId]);
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
  const isHome =
    !isConversationTransitioning && !activeConversationId && messages.length === 0 && !isSending;
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
    isLoadingMessages: effectiveIsLoadingMessages,
    isLoadingOlderMessages,
    isSending,
    bottomInsetHeight: inputAreaHeight,
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
      isRecoveringMessages ||
      isSending ||
      hasActiveStreamingMessage
    ) {
      return null;
    }
    const leafMessageId = activeConversation.current_leaf_message_id;
    if (!leafMessageId) return null;
    const leafMessage = activeMessages.find(message => message.id === leafMessageId) ?? null;
    if (!leafMessage || leafMessage.status !== 'waiting_question') return null;
    const request = leafMessage?.metadata?.user_input_request;
    if (!request?.questions?.some(question => question.question?.trim())) {
      return null;
    }
    return leafMessage;
  }, [
    activeConversation,
    activeMessages,
    hasActiveStreamingMessage,
    isRecoveringMessages,
    isSending,
  ]);
  const activeUserInputRequest = activeUserInputMessage?.metadata?.user_input_request ?? null;
  const activeWorkflowApprovalRequest = useMemo(
    () =>
      !isRecoveringMessages && surfaceProfile.workflowApprovals
        ? resolveActiveWorkflowApprovalRequest(
            activeConversation,
            activeMessages,
            isSending,
            hasActiveStreamingMessage,
            surface
          )
        : null,
    [
      activeConversation,
      activeMessages,
      hasActiveStreamingMessage,
      isRecoveringMessages,
      isSending,
      surface,
      surfaceProfile.workflowApprovals,
    ]
  );
  const canStopPendingWorkflowInteraction = Boolean(
    activeWorkflowApprovalRequest ||
      (activeUserInputMessage?.status === 'waiting_question' &&
        activeUserInputRequest?.source === AGENT_WORKFLOW_QUESTION_SOURCE)
  );
  const contextBlockedMessage = useMemo(() => {
    const leafMessageId = activeConversation?.current_leaf_message_id;
    if (!leafMessageId) return null;
    const leaf = activeMessages.find(message => message.id === leafMessageId) ?? null;
    return leaf?.status === 'error' &&
      leaf.metadata?.error_code === 'aichat.context.compaction_unavailable'
      ? leaf
      : null;
  }, [activeConversation?.current_leaf_message_id, activeMessages]);
  const contextBlocked = Boolean(contextBlockedMessage);
  // A turn paused for the person (an approval, a question, a page action or
  // more budget) is idle on the server but not finished, so nothing queued may
  // be sent over it.
  const leafWaitingForPerson = useMemo(() => {
    const leafId = activeConversation?.current_leaf_message_id;
    const leaf = leafId ? activeMessages.find(message => message.id === leafId) : undefined;
    return Boolean(leaf && AICHAT_WAITING_FOR_PERSON_STATUSES.has(leaf.status));
  }, [activeConversation?.current_leaf_message_id, activeMessages]);
  const messageActionsLocked = Boolean(
    activeWorkflowApprovalRequest || isConversationTransitioning || contextBlocked
  );
  const showResumeScrollButton = isAutoFollowPaused && (isSending || hasActiveStreamingMessage);
  const visiblePendingUserMessage =
    pendingUserMessage && isSending && !hasActiveStreamingMessage ? pendingUserMessage : null;
  const showPlanningPlaceholder = Boolean(
    visiblePendingUserMessage?.showAssistantPlanning && isSending && !hasActiveStreamingMessage
  );
  const showAssetAuditControl =
    showToolGovernancePermissionControl && Boolean(activeConversationId);
  const handleOpenAssetAudit = useCallback(() => {
    setMobileSidebarOpen(false);
    setEmbeddedAssetAuditOpen(true);
  }, []);
  const assetAuditButton = useMemo(
    () =>
      showAssetAuditControl ? (
        <AIChatAssetAuditButton
          conversationId={activeConversationId}
          refreshKey={assetAuditRefreshKey}
          className={embeddedControlButtonClassName}
          onOpen={isEmbedded ? handleOpenAssetAudit : undefined}
        />
      ) : null,
    [
      activeConversationId,
      assetAuditRefreshKey,
      handleOpenAssetAudit,
      isEmbedded,
      showAssetAuditControl,
    ]
  );
  const matchingRuntimeErrorMessage = useMemo(
    () =>
      contextBlockedMessage ??
      (error
        ? ([...activeMessages]
            .reverse()
            .find(message => message.status === 'error' && message.error === error) ?? null)
        : null),
    [activeMessages, contextBlockedMessage, error]
  );
  const resolvedRuntimeError = useMemo(() => {
    if (!error && !contextBlockedMessage) return null;
    const errorInput = matchingRuntimeErrorMessage
      ? getAIChatMessageErrorInput(matchingRuntimeErrorMessage)
      : { message: error ?? undefined };
    return resolveAIChatErrorMessage((key, values) => tGlobal(key as never, values), errorInput, {
      isAdmin: isBillingAdmin,
      workspaceId: currentWorkspace?.id,
    });
  }, [
    currentWorkspace?.id,
    error,
    contextBlockedMessage,
    isBillingAdmin,
    matchingRuntimeErrorMessage,
    tGlobal,
  ]);
  const showInlineRuntimeError =
    resolvedRuntimeError?.kind === 'billing' ||
    resolvedRuntimeError?.kind === 'server' ||
    resolvedRuntimeError?.kind === 'provider';

  useEffect(() => {
    if (!error) {
      lastErrorToastRef.current = null;
      return;
    }

    if (!resolvedRuntimeError || showInlineRuntimeError) {
      return;
    }

    const toastKey = `${resolvedRuntimeError.code ?? 'unknown'}:${error}`;

    if (lastErrorToastRef.current === toastKey) {
      return;
    }

    lastErrorToastRef.current = toastKey;
    const toastFn = resolvedRuntimeError.isBilling ? toast.warning : toast.error;
    toastFn(resolvedRuntimeError.title || resolvedRuntimeError.description, {
      id: resolvedRuntimeError.code ? `aichat-billing-${resolvedRuntimeError.code}` : undefined,
      description: resolvedRuntimeError.title ? resolvedRuntimeError.description : undefined,
      classNames: resolvedRuntimeError.isBilling ? workflowBillingToastClassNames : undefined,
      action:
        isBillingAdmin && resolvedRuntimeError.href && resolvedRuntimeError.actionLabel
          ? createElement(WorkflowBillingToastAction, {
              label: resolvedRuntimeError.actionLabel,
              onClick: () => router.push(resolvedRuntimeError.href as string),
            })
          : undefined,
    });
  }, [error, isBillingAdmin, resolvedRuntimeError, router, showInlineRuntimeError]);

  useEffect(() => {
    if (!pendingUserMessage) return;
    if (hasActiveStreamingMessage || !isSending) {
      setPendingUserMessage(null);
    }
  }, [hasActiveStreamingMessage, isSending, pendingUserMessage]);

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
        uploadScope?.type === 'console' || !uploadScope
          ? 'console'
          : `${uploadScope.type}:${uploadScope.webAppId}`,
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
        .map((text, index) => ({ text, prompt: text, key: `configured-${index}` }));
    }

    return [
      {
        text: t('chat.suggestions.email'),
        prompt: t('chat.suggestionPrompts.email'),
        key: 'email',
      },
      {
        text: t('chat.suggestions.meeting'),
        prompt: t('chat.suggestionPrompts.meeting'),
        key: 'meeting',
      },
      {
        text: t('chat.suggestions.report'),
        prompt: t('chat.suggestionPrompts.report'),
        key: 'report',
      },
      {
        text: t('chat.suggestions.polish'),
        prompt: t('chat.suggestionPrompts.polish'),
        key: 'polish',
      },
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

  const sendOrdinaryTurn = useCallback(
    async (query: string, files: AIChatMessageFile[] = [], useMemory = false): Promise<boolean> => {
      if (activeWorkflowApprovalRequest || contextBlocked) {
        return false;
      }
      if (!query.trim() || isSending) {
        return false;
      }
      if (requireModel && !modelSelectorValue.model) {
        toast.error(t('consoleChat.modelRequired'));
        return false;
      }
      const connectorSnapshot = inheritConnectorDefaults ? connectorDraft.capture() : undefined;
      if (inheritConnectorDefaults && !connectorSnapshot) return false;
      speechPlayback?.stop();
      if (beforeSend) {
        const canSend = await beforeSend();
        if (!canSend) return false;
      }
      if (connectorSnapshot && !connectorSnapshot.isCurrent()) return false;
      setConnectedAppsOpen(false);

      const pendingMessageId = Date.now();
      const pendingMessageCreatedAt = Math.floor(pendingMessageId / 1000);
      const showAssistantPlanning = Boolean(activeConversationId || messages.length > 0);
      setPendingUserMessage({
        id: `pending-user-${pendingMessageId}`,
        query,
        files,
        assistantModelName: modelSelectorValue.model,
        assistantCreatedAt: pendingMessageCreatedAt,
        showAssistantPlanning,
      });
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
        initialConnectorBindings:
          connectorsEnabled && !connectorConversationId ? connectorSnapshot?.bindings : undefined,
      });
      return true;
    },
    [
      activeWorkflowApprovalRequest,
      contextBlocked,
      activeConversationId,
      beforeSend,
      controller,
      forcedUseMemory,
      isSending,
      messages.length,
      modelSelectorValue,
      requireModel,
      speechPlayback,
      effectiveRuntimeSurface,
      t,
      toolGovernanceOperationContext,
      connectorsEnabled,
      connectorConversationId,
      connectorDraft,
      inheritConnectorDefaults,
    ]
  );

  const handleSend = useCallback(
    async (files: AIChatMessageFile[] = [], useMemory = false): Promise<boolean> => {
      if (activeWorkflowApprovalRequest || contextBlocked) return false;
      const query = input.trim();
      if (!query) return false;

      if (isSending) {
        if (!controller.supportsRuntimeInput || !activeConversationId) return false;
        if (files.length > 0) {
          toast.error(t('consoleChat.runtimeInput.textOnly'));
          return false;
        }
        if (runtimeInputQueueFull) {
          toast.error(t('consoleChat.runtimeInput.queueFull'));
          return false;
        }
        const queuedTurn: AIChatQueuedTurn = {
          id: crypto.randomUUID(),
          content: query,
          useMemory: forcedUseMemory ?? useMemory,
          createdAt: Date.now(),
        };
        setQueuedTurnsByConversation(current => ({
          ...current,
          [activeConversationId]: enqueueQueuedChatTurn(
            current[activeConversationId] ?? [],
            queuedTurn
          ),
        }));
        setInput('');
        return true;
      }

      setInput('');
      const sent = await sendOrdinaryTurn(query, files, useMemory);
      if (!sent) setInput(current => current || query);
      return sent;
    },
    [
      activeConversationId,
      activeWorkflowApprovalRequest,
      contextBlocked,
      controller.supportsRuntimeInput,
      forcedUseMemory,
      input,
      isSending,
      runtimeInputQueueFull,
      sendOrdinaryTurn,
      t,
    ]
  );

  const removeQueuedTurn = useCallback((conversationID: string, turnID: string) => {
    setQueuedTurnsByConversation(current => ({
      ...current,
      [conversationID]: removeQueuedChatTurn(current[conversationID] ?? [], turnID),
    }));
  }, []);

  const handleGuideQueuedTurn = useCallback(
    async (turn: AIChatQueuedTurn) => {
      if (!activeConversationId || guidingQueuedTurnIDs.has(turn.id)) return;
      setGuidingQueuedTurnIDs(current => new Set(current).add(turn.id));
      removeQueuedTurn(activeConversationId, turn.id);
      try {
        const result = await controller.submitRuntimeInput(turn.content);
        if (result.status === 'accepted') return;
        if (result.status === 'conversation_idle') {
          const sent = await sendOrdinaryTurn(turn.content, [], turn.useMemory);
          if (sent) return;
        } else {
          toast.error(t('consoleChat.runtimeInput.failed'));
        }
        setQueuedTurnsByConversation(current => ({
          ...current,
          [activeConversationId]: enqueueQueuedChatTurn(current[activeConversationId] ?? [], turn),
        }));
      } finally {
        setGuidingQueuedTurnIDs(current => {
          const next = new Set(current);
          next.delete(turn.id);
          return next;
        });
      }
    },
    [activeConversationId, controller, guidingQueuedTurnIDs, removeQueuedTurn, sendOrdinaryTurn, t]
  );

  const handleEditQueuedTurn = useCallback(
    (turn: AIChatQueuedTurn) => {
      if (!activeConversationId) return;
      removeQueuedTurn(activeConversationId, turn.id);
      setInput(current => (current.trim() ? `${turn.content}\n\n${current}` : turn.content));
    },
    [activeConversationId, removeQueuedTurn]
  );

  const handleStop = useCallback(() => {
    if (activeConversationId) {
      setQueuedTurnsByConversation(current => ({
        ...current,
        [activeConversationId]: [],
      }));
    }
    controller.stop();
  }, [activeConversationId, controller]);

  useEffect(() => {
    if (
      !activeConversationId ||
      activeConversationRunning ||
      isSending ||
      pendingUserMessage ||
      activeWorkflowApprovalRequest ||
      leafWaitingForPerson ||
      activeQueuedTurns.length === 0
    ) {
      return;
    }
    const nextTurn = activeQueuedTurns[0];
    if (autoSendingQueuedTurnRef.current === nextTurn.id) return;
    autoSendingQueuedTurnRef.current = nextTurn.id;
    void sendOrdinaryTurn(nextTurn.content, [], nextTurn.useMemory)
      .then(sent => {
        if (sent) removeQueuedTurn(activeConversationId, nextTurn.id);
      })
      .finally(() => {
        if (autoSendingQueuedTurnRef.current === nextTurn.id) {
          autoSendingQueuedTurnRef.current = null;
        }
      });
  }, [
    activeConversationId,
    activeConversationRunning,
    activeQueuedTurns,
    activeWorkflowApprovalRequest,
    isSending,
    leafWaitingForPerson,
    pendingUserMessage,
    removeQueuedTurn,
    sendOrdinaryTurn,
  ]);

  const handleUserInputRequestSubmit = useCallback(
    (query: string, _useMemory: boolean, answers?: Record<string, string>) => {
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
      const requestId = activeRequest?.request_id?.trim();
      if (!requestId || !answers || !controller.continueUserInput) {
        toast.error(t('consoleChat.userInputRequest.continuationUnavailable'));
        return;
      }
      void controller
        .continueUserInput(
          activeUserInputMessage.conversation_id,
          activeUserInputMessage.id,
          requestId,
          {
            answers,
            operation_context: toolGovernanceOperationContext,
          }
        )
        .catch(() => {
          toast.error(t('consoleChat.userInputRequest.continuationUnavailable'));
        });
    },
    [activeUserInputMessage, controller, isSending, t, toolGovernanceOperationContext]
  );

  const canRegenerateMessage = useCallback(
    (message: AIChatMessage) => {
      const branchCount = branchNavigationByMessageId.get(message.id)?.total ?? 1;
      const canReplaceRoot = canReplaceRootMessage(message);
      return (
        (!messageActionsLocked || message.id === contextBlockedMessage?.id) &&
        (canReplaceRoot || (Boolean(message.parent_id) && branchCount < MAX_AICHAT_BRANCHES))
      );
    },
    [
      branchNavigationByMessageId,
      canReplaceRootMessage,
      contextBlockedMessage?.id,
      messageActionsLocked,
    ]
  );

  const executeRegenerate = useCallback(
    async (message: AIChatMessage) => {
      const connectorSnapshot = inheritConnectorDefaults ? connectorDraft.capture() : undefined;
      if (inheritConnectorDefaults && !connectorSnapshot) return;
      speechPlayback?.stop();
      if (beforeSend && !(await beforeSend())) return;
      if (connectorSnapshot && !connectorSnapshot.isCurrent()) return;
      setConnectedAppsOpen(false);

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
          initialConnectorBindings:
            connectorsEnabled && !connectorConversationId ? connectorSnapshot?.bindings : undefined,
        }
      );
    },
    [
      beforeSend,
      controller,
      modelSelectorValue,
      speechPlayback,
      effectiveRuntimeSurface,
      toolGovernanceOperationContext,
      connectorsEnabled,
      connectorConversationId,
      connectorDraft,
      inheritConnectorDefaults,
    ]
  );

  const handleRegenerate = useCallback(
    async (message: AIChatMessage) => {
      if (!canRegenerateMessage(message)) return;
      if (requireModel && !modelSelectorValue.model) {
        toast.error(t('consoleChat.modelRequired'));
        return;
      }
      if (selectedModelSupportsVision === false && getReplayImageCount(message) > 0) {
        setPendingVisionReplayAction({ type: 'regenerate', message });
        return;
      }

      await executeRegenerate(message);
    },
    [
      canRegenerateMessage,
      executeRegenerate,
      modelSelectorValue.model,
      requireModel,
      selectedModelSupportsVision,
      t,
    ]
  );

  const resumeInterruptedTurn = controller.resumeInterruptedTurn;
  const handleResumeInterrupted = useCallback(
    (message: AIChatMessage) => {
      if (!resumeInterruptedTurn || isSending) return;
      void resumeInterruptedTurn(message.conversation_id, message.id).catch(() => {
        toast.error(t('consoleChat.errors.notResumable.title'));
      });
    },
    [isSending, resumeInterruptedTurn, t]
  );

  const handleRetryRuntimeError = useCallback(() => {
    if (!matchingRuntimeErrorMessage) return;
    void handleRegenerate(matchingRuntimeErrorMessage);
  }, [handleRegenerate, matchingRuntimeErrorMessage]);
  const canRetryRuntimeError = Boolean(
    matchingRuntimeErrorMessage && canRegenerateMessage(matchingRuntimeErrorMessage)
  );
  const runtimeErrorActionHref = isBillingAdmin
    ? resolvedRuntimeError?.href || '/dashboard/provider'
    : null;
  const runtimeErrorNotice =
    showInlineRuntimeError && resolvedRuntimeError ? (
      <AIChatRuntimeErrorNotice
        title={resolvedRuntimeError.title || resolvedRuntimeError.description}
        description={resolvedRuntimeError.description}
        retryLabel={
          contextBlocked ? t('consoleChat.contextCompactionBlocked.retry') : t('chat.retry')
        }
        configureLabel={
          runtimeErrorActionHref
            ? resolvedRuntimeError.actionLabel || t('consoleChat.configureModel')
            : undefined
        }
        isBilling={resolvedRuntimeError.isBilling}
        canRetry={canRetryRuntimeError}
        onRetry={handleRetryRuntimeError}
        onConfigure={runtimeErrorActionHref ? () => router.push(runtimeErrorActionHref) : undefined}
      />
    ) : null;
  const composerPrimaryNotice = runtimeErrorNotice || inputTopNotice;
  const composerTopAccessory =
    composerPrimaryNotice || runtimeInputQueue.length > 0 || activeQueuedTurns.length > 0 ? (
      <div className="space-y-2">
        {composerPrimaryNotice}
        <AIChatRuntimeInputQueue
          items={runtimeInputQueue}
          queuedTurns={activeQueuedTurns}
          guidingTurnIDs={guidingQueuedTurnIDs}
          onGuideTurn={controller.canSubmitRuntimeInput ? handleGuideQueuedTurn : undefined}
          onEditTurn={handleEditQueuedTurn}
        />
      </div>
    ) : null;

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

  const executeEdit = useCallback(
    (message: AIChatMessage, query: string) => {
      const canReplaceRoot = canReplaceRootMessage(message);
      speechPlayback?.stop();
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
          operationContext: toolGovernanceOperationContext,
        });
        return;
      }

      void controller.send({
        query,
        files: messageFilesForReplay(message),
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
      canReplaceRootMessage,
      controller,
      modelSelectorValue,
      speechPlayback,
      effectiveRuntimeSurface,
      toolGovernanceOperationContext,
    ]
  );

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
      if (selectedModelSupportsVision === false && getReplayImageCount(message) > 0) {
        setPendingVisionReplayAction({ type: 'edit', message, query });
        return;
      }

      executeEdit(message, query);
    },
    [
      branchNavigationByMessageId,
      canReplaceRootMessage,
      editingQuery,
      executeEdit,
      isSending,
      messageActionsLocked,
      modelSelectorValue.model,
      requireModel,
      selectedModelSupportsVision,
      t,
    ]
  );

  const handleConfirmVisionReplay = useCallback(() => {
    const action = pendingVisionReplayAction;
    if (!action) return;

    setPendingVisionReplayAction(null);
    if (action.type === 'regenerate') {
      void executeRegenerate(action.message);
      return;
    }
    executeEdit(action.message, action.query);
  }, [executeEdit, executeRegenerate, pendingVisionReplayAction]);

  const handleSwitchBranch = useCallback(
    (messageId: string) => {
      speechPlayback?.stop();
      setEditingMessageId(null);
      setEditingQuery('');
      controller.switchBranch(messageId);
    },
    [controller, speechPlayback]
  );

  const handleWorkflowApprovalSubmit = useCallback(
    (request: AIChatWorkflowApprovalRequest, payload: AIChatWorkflowApprovalSubmitPayload) => {
      if (!controller.continueWorkflowApproval) return;
      void controller
        .continueWorkflowApproval(request.conversationId, request.messageId, {
          approvalToken: request.approvalToken,
          inputs: payload.inputs,
          action: payload.action,
        })
        .then(() => onTaskMutation?.())
        .catch(error => {
          toast.error(
            error instanceof Error ? error.message : t('consoleChat.workflow.approvalSubmitFailed')
          );
          onTaskMutation?.();
        });
    },
    [controller, onTaskMutation, t]
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
          throw error;
        });
    },
    [controller]
  );
  const handleConnectorRuntimeDecision = useCallback(
    async (
      conversationId: string,
      messageId: string,
      command: ConnectorRuntimeConfirmationCommand,
      expectedScope?: string,
      rememberForSession = false
    ): Promise<ConnectorRuntimeDecisionResult> => {
      const currentScope = () =>
        JSON.stringify([
          useOrganizationStore.getState().currentOrganization?.id ?? '',
          useAuthStore.getState().user?.id ?? '',
        ]);
      const scope = expectedScope ?? currentScope();
      const result = await submitConnectorRuntimeDecision(
        {
          ...command,
          ...(rememberForSession ? { remember_for_session: true } : {}),
        },
        scope
      );
      if (currentScope() !== scope) return result;
      controller.store.getState().update(current =>
        applyConnectorRuntimeResolutionState(current, {
          conversation_id: conversationId,
          message_id: messageId,
          progress_id: result.route_receipt,
          state: result.state === 'transport_unconfirmed' ? 'unknown' : result.state,
          ...(result.state !== 'transport_unconfirmed' && result.decision_id
            ? { decision_id: result.decision_id }
            : {}),
          ...(result.state !== 'transport_unconfirmed' && result.error_code
            ? { error_code: result.error_code }
            : {}),
        })
      );
      try {
        await controller.recoverStreamingConversation(conversationId, {
          mode: 'active',
          refresh: true,
        });
      } catch {
        // The receipt is already consumed even when the provider outcome is unknown.
        // A later durable-event recovery is safe; repeating the operation is not.
      }
      return result;
    },
    [controller]
  );
  useEffect(() => {
    setSubmittedToolGovernanceDecisionKeys(new Set());
  }, [activeConversation?.id]);

  const activeToolGovernanceApprovalFallback = useMemo<ToolGovernancePendingApproval | null>(() => {
    if (
      !enableToolGovernanceApprovals ||
      !activeConversation ||
      activeConversation.runtime_status !== 'idle' ||
      isRecoveringMessages ||
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
    isRecoveringMessages,
    isSending,
    locale,
    skillDisplayById,
    streamingByMessageId,
    submittedToolGovernanceDecisionKeys,
    t,
    toolGovernanceApprovalScopeId,
  ]);

  const handleNewChat = useCallback(() => {
    speechPlayback?.stop();
    connectorDraft.reset();
    setConnectedAppsOpen(false);
    if (isHome) {
      toast.info(t('chat.alreadyInDraft'));
      setMobileSidebarOpen(false);
      setEmbeddedAssetAuditOpen(false);
      return;
    }
    if (onStartNewConversation) {
      onStartNewConversation();
    } else {
      controller.startNew();
    }
    setMobileSidebarOpen(false);
    setEmbeddedAssetAuditOpen(false);
  }, [controller, isHome, onStartNewConversation, speechPlayback, t, connectorDraft]);

  const handleLoadMoreConversations = useCallback(
    (page: number) => controller.refreshList({ page, append: true }),
    [controller]
  );

  const handleSelectConversation = useCallback(
    (id: string) => {
      speechPlayback?.stop();
      if (onSelectConversation) {
        onSelectConversation(id);
      } else {
        void controller.select(id);
      }
      setMobileSidebarOpen(false);
      setEmbeddedAssetAuditOpen(false);
    },
    [controller, onSelectConversation, speechPlayback]
  );

  const handleDeleteConversation = useCallback(
    (id: string) => {
      void controller.remove(id);
      setMobileSidebarOpen(false);
      setEmbeddedAssetAuditOpen(false);
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
        onError: () => {
          toast.error(t('consoleChat.skillPreferences.saveFailed'));
        },
      }
    );
  }, [draftSkillPreferenceIds, t, updateSkillPreference]);

  const embeddedConversationControls = useMemo(() => {
    if (!showEmbeddedConversationDrawer) return null;
    const controls = {
      openConversations: () => {
        setEmbeddedAssetAuditOpen(false);
        setMobileSidebarOpen(true);
      },
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
        conversationsLabel={t('consoleChat.historySubtitle')}
        newConversationLabel={t('chat.newConversation')}
        trailingAction={assetAuditButton}
      />
    );
  }, [
    assetAuditButton,
    handleNewChat,
    isHome,
    renderEmbeddedConversationControls,
    showEmbeddedConversationDrawer,
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

  const embeddedConversationOverlayOpen = showEmbeddedConversationDrawer && mobileSidebarOpen;
  const embeddedAssetAuditOverlayOpen = isEmbedded && embeddedAssetAuditOpen;
  const embeddedOverlayOpen = embeddedConversationOverlayOpen || embeddedAssetAuditOverlayOpen;

  return (
    <div
      className={cn('relative flex h-full w-full overflow-hidden', chatSurfaceBackground)}
      style={themeStyle}
    >
      {!isEmbedded ? (
        <div className="hidden md:block">
          <Sidebar
            activeId={selectedConversationId}
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
            pagination={conversationPagination}
            isLoadingList={isLoadingConversationList}
            onLoadMore={handleLoadMoreConversations}
            title={conversationListTitle}
            description={conversationListDescription}
          />
        </div>
      ) : null}

      <ToolGovernancePendingApprovalScopeProvider scopeId={toolGovernanceApprovalScopeId}>
        <main
          aria-hidden={embeddedOverlayOpen || undefined}
          inert={embeddedOverlayOpen}
          className={cn(
            'relative flex min-w-0 flex-1 flex-col overflow-hidden',
            chatSurfaceBackground
          )}
        >
          {!isEmbedded ? (
            <AIChatHeader
              isHome={isHome}
              title={activeConversation?.title || ''}
              position={surfaceProfile.headerPosition}
              onToggleSidebar={handleToggleSidebar}
              onStartNew={handleNewChat}
              rightAction={
                assetAuditButton || headerRightAction ? (
                  <div className="flex items-center justify-end gap-1">
                    {assetAuditButton}
                    {headerRightAction}
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
            readExecutionFile={controller.readExecutionFile}
            messages={messages}
            activeConversation={activeConversation}
            activeMessageCount={activeMessages.length}
            branchNavigationByMessageId={branchNavigationByMessageId}
            isLoadingMessages={effectiveIsLoadingMessages}
            isLoadingOlderMessages={isLoadingOlderMessages}
            isSending={isSending || messageActionsLocked}
            streamingByMessageId={streamingByMessageId}
            skillDisplayById={skillDisplayById}
            editingMessageId={editingMessageId}
            editingQuery={editingQuery}
            bottomRef={bottomRef}
            scrollViewportRef={scrollViewportRef}
            bottomSpacerHeight={
              activeUserInputRequest
                ? Math.max(inputAreaHeight - 48, 180)
                : Math.max(inputAreaHeight + 72, 180)
            }
            onScroll={handleMessagesScroll}
            onRegenerate={enableMessageReplay ? handleRegenerate : undefined}
            onResumeInterrupted={resumeInterruptedTurn ? handleResumeInterrupted : undefined}
            onToolGovernanceDecision={
              enableToolGovernanceApprovals && !isRecoveringMessages
                ? handleToolGovernanceDecision
                : undefined
            }
            onTaskProposalAction={onTaskProposalAction}
            onTaskMutation={onTaskMutation}
            onConnectorRuntimeDecision={handleConnectorRuntimeDecision}
            enableToolGovernanceApprovals={enableToolGovernanceApprovals}
            suppressPendingToolGovernanceApprovals={isRecoveringMessages}
            onSwitchBranch={handleSwitchBranch}
            onEditStart={enableMessageReplay ? handleEditStart : undefined}
            onEditChange={setEditingQuery}
            onEditCancel={handleEditCancel}
            onEditSubmit={handleEditSubmit}
            showAssistantModelMeta={showAssistantModelMeta}
            layout={isEmbedded ? 'embedded' : 'full'}
            messageDensity={messageDensity}
            showMemoryKey={surfaceProfile.memoryKey}
            // Published apps show status, not diagnostic argument/result panels.
            showSkillEventDetails={surfaceProfile.executionDetails && showSkillEventDetails}
            showWorkflowFailureDetails={surfaceProfile.executionDetails}
            showContextualOperationStatus={surfaceProfile.contextualOperationStatus}
            showPlanningPlaceholder={showPlanningPlaceholder}
            pendingUserMessage={visiblePendingUserMessage}
            speechPlayback={speechPlayback}
          />

          <AIChatHomeView
            isVisible={isHome && !effectiveIsLoadingMessages}
            suggestions={suggestions}
            onSelectSuggestion={setInput}
            brand={homeBrand}
            openingGuideBrand={openingGuideBrand}
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

          {controller.connectionState === 'disconnected' && activeConversationRunning ? (
            <div
              className="absolute inset-x-4 z-30 flex items-center justify-between gap-3 border-y bg-background/95 px-3 py-2 text-xs text-muted-foreground backdrop-blur"
              style={{ bottom: Math.max(inputAreaHeight + 8, 86) }}
            >
              <span>{t('consoleChat.streamDisconnected')}</span>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="h-7 shrink-0 px-2"
                onClick={() => {
                  if (!activeConversation?.id) return;
                  void controller.recoverStreamingConversation(activeConversation.id, {
                    mode: 'active',
                  });
                }}
              >
                <RefreshCw className="mr-1.5 size-3.5" />
                {t('consoleChat.reconnectStream')}
              </Button>
            </div>
          ) : null}

          <AIChatInputArea
            isEmbedded={isEmbedded}
            isHome={isHome}
            isLoadingMessages={effectiveIsLoadingMessages}
            input={input}
            modelSelectorValue={modelSelectorValue}
            modelSelectorWarning={modelSelectorWarning}
            modelProps={modelProps}
            supportsVisionOverride={supportsVisionOverride}
            isModelInitializing={isModelInitializing}
            modelMissing={modelMissing}
            isSending={isSending}
            allowRuntimeInput={surfaceProfile.runtimeInput}
            runtimeInputQueueFull={runtimeInputQueueFull}
            disabled={contextBlocked}
            canStop={canStopPendingWorkflowInteraction || activeConversationRunning}
            isStopping={isStopping}
            onInputChange={setInput}
            onSend={handleSend}
            activeUserInputRequest={activeUserInputRequest}
            onUserInputRequestSubmit={handleUserInputRequestSubmit}
            activeWorkflowApprovalRequest={activeWorkflowApprovalRequest}
            onWorkflowApprovalSubmit={handleWorkflowApprovalSubmit}
            onStop={handleStop}
            onModelChange={onModelChange}
            onModelPropsChange={setSelectedModelProps}
            onHeightChange={setInputAreaHeight}
            showModelSelector={showModelSelector}
            modelUseCase={modelUseCase}
            modelAvailabilityUseCase={modelAvailabilityUseCase}
            preferredModelUseCase={preferredModelUseCase}
            showMemoryEntry={showMemoryEntry}
            enableUpload={enableUpload}
            fileUploadPolicy={fileUploadPolicy}
            uploadRequiresLogin={uploadRequiresLogin}
            uploadScope={uploadScope}
            showFileLibraryPicker={showFileLibraryPicker}
            allowWorkspaceSwitch={allowWorkspaceSwitch}
            showConnectedApps={enableAIChatConnectedApps}
            connectedAppsLabel={
              connectorsEnabled
                ? !connectorConversationId && connectorDraft.attention === 'loadFailed'
                  ? connectorT('defaultsDraft.loadFailed')
                  : connectorT('picker.toolbarAction', {
                      count: connectorSelectedCount,
                    })
                : t('consoleChat.connectedApps.action', {
                    count: connectorSelectedCount,
                  })
            }
            connectedAppsSelectedCount={connectorSelectedCount}
            connectedAppsAttentionRequired={
              connectorConversationId
                ? connectedAppsSummary.hasAttentionRequired
                : Boolean(connectorDraft.attention && connectorDraft.attention !== 'loading')
            }
            connectedAppsDisabled={connectorsEnabled && !connectorConversationId && isSending}
            connectedAppsOpen={connectedAppsOpen}
            connectedAppsPopup="dialog"
            onOpenConnectedApps={anchorElement => {
              setConnectedAppsAnchorElement(anchorElement);
              setConnectedAppsOpen(open => !open);
            }}
            showSkillManagement={enableAIChatSkillPreference}
            skillManagementLabel={t('consoleChat.skillPreferences.action')}
            onOpenSkillManagement={() => handleSkillPreferenceOpenChange(true)}
            inputPlaceholder={inputPlaceholder}
            surface={surface}
            showToolGovernancePermissionControl={showToolGovernancePermissionControl}
            toolGovernancePermissionTier={toolGovernancePermissionTier}
            onToolGovernancePermissionTierChange={setToolGovernancePermissionTier}
            enableToolGovernanceApprovals={enableToolGovernanceApprovals && !isRecoveringMessages}
            activeConversationId={activeConversation?.id ?? null}
            activeToolGovernanceMessageId={
              activeConversation?.active_message_id ??
              activeConversation?.current_leaf_message_id ??
              null
            }
            activeToolGovernanceApprovalFallback={activeToolGovernanceApprovalFallback}
            topAccessory={composerTopAccessory}
            voiceTranscriber={voiceTranscriber}
            speechAutoPlay={
              speechPlayback
                ? {
                    enabled: speechPlayback.autoPlay,
                    onEnabledChange: speechPlayback.setAutoPlay,
                  }
                : undefined
            }
          />
        </main>
      </ToolGovernancePendingApprovalScopeProvider>

      {embeddedConversationOverlayOpen ? (
        <div
          className={cn(
            'absolute inset-0 z-40 flex min-h-0 min-w-0 overflow-hidden',
            chatSurfaceBackground
          )}
        >
          <Sidebar
            activeId={selectedConversationId}
            conversations={conversationSummaries}
            isOpen
            isHome={isHome}
            className="!w-full !border-r-0 !bg-transparent"
            onNewChat={handleNewChat}
            onSelect={handleSelectConversation}
            onDelete={handleDeleteConversation}
            onRename={handleRenameConversation}
            onClose={() => setMobileSidebarOpen(false)}
            alwaysShowHeader
            useBackNavigation
            search={searchConversations}
            searchKey={conversationSearchKey}
            title={conversationListTitle}
            description={conversationListDescription}
          />
        </div>
      ) : embeddedAssetAuditOverlayOpen ? (
        <div
          className={cn(
            'absolute inset-0 z-40 flex min-h-0 min-w-0 overflow-hidden',
            chatSurfaceBackground
          )}
        >
          <AIChatAssetAuditPanel
            conversationId={activeConversationId}
            refreshKey={assetAuditRefreshKey}
            onBack={() => setEmbeddedAssetAuditOpen(false)}
          />
        </div>
      ) : null}

      {!isEmbedded ? (
        <Sheet open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
          <SheetContent
            side="left"
            className="w-screen max-w-none p-0 sm:w-96 sm:max-w-sm"
            showClose={false}
          >
            <SheetTitle className="sr-only">
              {conversationListTitle || t('chat.conversations')}
            </SheetTitle>
            <SheetDescription className="sr-only">
              {conversationListDescription || t('consoleChat.conversationListDescription')}
            </SheetDescription>
            <Sidebar
              activeId={selectedConversationId}
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
              alwaysShowHeader
              search={searchConversations}
              searchKey={conversationSearchKey}
              pagination={conversationPagination}
              isLoadingList={isLoadingConversationList}
              onLoadMore={handleLoadMoreConversations}
              title={conversationListTitle}
              description={conversationListDescription}
            />
          </SheetContent>
        </Sheet>
      ) : null}

      {enableAIChatSkillPreference ? (
        <AIChatSkillPreferenceDialog
          open={skillPreferenceOpen}
          context={surfaceProfile.skillPreferenceContext}
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
      {enableAIChatConnectedApps ? (
        <AIChatConnectedAppsDialog
          open={connectedAppsOpen}
          anchorElement={connectedAppsAnchorElement}
          connectorsEnabled={connectorsEnabled}
          conversationId={connectorConversationId}
          draft={!connectorConversationId ? connectorDraft.draft : undefined}
          onDraftBindingsChange={connectorDraft.update}
          targetName={activeConversation?.title?.trim() || connectorT('picker.targetTypes.chat')}
          onOpenChange={setConnectedAppsOpen}
          onSummaryChange={setConnectedAppsSummary}
        />
      ) : null}

      <ConfirmDialog
        open={pendingVisionReplayAction !== null}
        onOpenChange={open => {
          if (!open) setPendingVisionReplayAction(null);
        }}
        title={t('consoleChat.attachments.visionReplayWarning.title')}
        description={
          <div className="space-y-3">
            <p>
              {t('consoleChat.attachments.visionReplayWarning.description', {
                count: pendingVisionReplayAction
                  ? getReplayImageCount(pendingVisionReplayAction.message)
                  : 0,
              })}
            </p>
            <p>{t('consoleChat.attachments.visionReplayWarning.recommendation')}</p>
          </div>
        }
        cancelText={t('consoleChat.attachments.visionReplayWarning.cancel')}
        confirmText={t('consoleChat.attachments.visionReplayWarning.confirm')}
        onConfirm={handleConfirmVisionReplay}
      />
    </div>
  );
}

function resolveActiveWorkflowApprovalRequest(
  conversation: AIChatConversation | null,
  messages: AIChatMessage[],
  isSending: boolean,
  hasActiveStreamingMessage: boolean,
  surface: AIChatShellSurface
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
    continuationStatus !== 'waiting_approval' &&
    !isActiveWorkflowApprovalRun(runForContinuation, continuationWorkflowRunId, continuationStatus)
  ) {
    return null;
  }
  const approval = runWithApproval?.approval ?? runForContinuation?.approval;
  const inlineApprovalForm =
    normalizeApprovalRuntimeForm(approval?.approval_form) ||
    normalizeApprovalRuntimeForm(stringRecordValue(continuation, 'approval_form'));
  const approvalToken =
    resolveApprovalToken(approval) ||
    stringFromRecord(continuation, 'approval_token') ||
    stringFromUnknown(inlineApprovalForm?.token);
  if (!approvalToken) return null;
  const canSubmitInline = isWorkflowApprovalInlineAllowed({
    surface,
    form: inlineApprovalForm,
    uiApprovalAllowed: workflowApprovalUIAccess(approval, continuation),
  });
  return {
    conversationId: conversation.id,
    messageId: leafMessage.id,
    workflowRunId: stringFromUnknown(runWithApproval?.workflow_run_id) || continuationWorkflowRunId,
    approvalToken,
    approvalUrl: stringFromUnknown(approval?.approval_url),
    approvalFormId:
      stringFromUnknown(approval?.approval_form_id) || stringFromUnknown(inlineApprovalForm?.id),
    approvalForm: inlineApprovalForm,
    canSubmitInline,
  };
}

function stringRecordValue(record: unknown, key: string): unknown {
  if (!record || typeof record !== 'object') return undefined;
  return (record as Record<string, unknown>)[key];
}

function workflowApprovalUIAccess(approval: unknown, continuation: unknown): boolean | undefined {
  for (const value of [approval, continuation]) {
    if (!value || typeof value !== 'object') continue;
    const allowed = (value as Record<string, unknown>).ui_approval_allowed;
    if (typeof allowed === 'boolean') return allowed;
  }
  return undefined;
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
