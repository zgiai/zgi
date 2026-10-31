'use client';

import type { FileUploadSelection } from '@/services/upload.service';
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type ClipboardEvent,
  type KeyboardEvent,
  type ReactNode,
} from 'react';
import dynamic from 'next/dynamic';
import { toast } from 'sonner';
import {
  type ModelSelectorModelProps,
  type ModelSelectorValue,
} from '@/components/common/model-selector';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Textarea } from '@/components/ui/textarea';
import WorkflowApprovalInteractionCard, {
  type WorkflowApprovalInteractionMode,
} from '@/components/workflow/approval/workflow-approval-interaction-card';
import { WorkflowRuntimeStopAction } from '@/components/workflow/runtime/workflow-runtime-stop-action';
import { useApprovalForm, useSubmitApprovalForm } from '@/hooks/workflow/use-approval-form';
import { useUploadConfig } from '@/hooks/use-upload';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { isUploadCanceledError, uploadService } from '@/services/upload.service';
import { isApprovalFormAlreadySubmittedError } from '@/services/approval.service';
import { useCurrentWorkspace } from '@/store/workspace-store';
import type { FileItem } from '@/services/types/file';
import type { AIChatMessageFile, AIChatUserInputRequest } from '@/services/types/aichat';
import type { AIChatToolGovernancePermissionTier } from '@/components/aichat/contextual/types';
import type { AvailableModelUseCase, ModelUseCase } from '@/services/types/model';
import {
  AUDIO_EXTENSIONS,
  IMAGE_EXTENSIONS,
  VIDEO_EXTENSIONS,
  buildFileInputAcceptAttribute,
  filterLowercaseExtensions,
  formatExtensionsForDisplay,
} from '@/utils/file-helpers';
import { ChevronLeft, ChevronRight, HelpCircle } from 'lucide-react';
import {
  AIChatAttachmentStrip,
  AIChatDragUploadOverlay,
} from '@/components/chat/variants/aichat/attachment-strip';
import {
  AICHAT_ATTACHMENT_LIMIT,
  AICHAT_DOCUMENT_EXTENSIONS,
  type AIChatAttachmentUploadKind,
  type AIChatInputAttachment,
} from '@/components/chat/variants/aichat/input-area-types';
import { AIChatInputToolbar } from '@/components/chat/variants/aichat/input-toolbar';
import {
  createAttachmentId,
  fileItemToAIChatMessageFile,
  getNormalizedExtension,
  getUploadedAIChatFiles,
  isImageExtension,
  toAIChatMessageFile,
  type AIChatComposerSurface,
} from '@/components/chat/variants/aichat/input-area-utils';
import type {
  AIChatModelValue,
  AIChatWorkflowApprovalRequest,
  AIChatWorkflowApprovalSubmitPayload,
} from '@/components/chat/variants/aichat/types';
import {
  isToolGovernancePendingApprovalDismissed,
  ToolGovernanceApprovalPanel,
  useActiveToolGovernancePendingApproval,
  useToolGovernancePendingApprovalScope,
  useToolGovernancePendingApprovalVersion,
  type ToolGovernancePendingApproval,
} from '@/components/chat/variants/aichat/tool-governance-decision-card';
import type { AIChatVoiceTranscriber } from '@/components/chat/variants/aichat/voice/pcm-audio';
import { useAgentVoiceInput } from '@/components/chat/variants/aichat/voice/use-agent-voice-input';
import { getVoiceInputErrorKey } from '@/components/chat/variants/aichat/voice/voice-input-errors';

export type AIChatUploadScope =
  | { type: 'console' }
  | { type: 'webapp'; webAppId: string }
  | { type: 'digital-human-webapp'; webAppId: string };

const FileSelectorDialog = dynamic(() => import('@/components/files/file-selector-dialog'), {
  ssr: false,
});

const COMPOSER_TEXTAREA_MIN_HEIGHT = 48;
const COMPOSER_TEXTAREA_LINE_HEIGHT = 20;
const COMPOSER_TEXTAREA_VERTICAL_PADDING = 16;
const COMPOSER_TEXTAREA_MAX_HEIGHT =
  COMPOSER_TEXTAREA_LINE_HEIGHT * 8 + COMPOSER_TEXTAREA_VERTICAL_PADDING;
const COMPOSER_TEXTAREA_EXPANDED_MIN_HEIGHT = 360;
const COMPOSER_TEXTAREA_EXPANDED_MAX_HEIGHT = 720;
const COMPOSER_TEXTAREA_EXPANDED_VIEWPORT_RATIO = 0.72;
const COMPOSER_EXPAND_VISIBLE_LINE_COUNT = 4;
// Some IMEs emit a non-composing Enter immediately after compositionend.
const COMPOSITION_END_ENTER_GRACE_MS = 100;

function getComposerExpandedMaxHeight(): number {
  if (typeof window === 'undefined') return COMPOSER_TEXTAREA_EXPANDED_MAX_HEIGHT;
  return Math.max(
    COMPOSER_TEXTAREA_EXPANDED_MIN_HEIGHT,
    Math.min(
      Math.round(window.innerHeight * COMPOSER_TEXTAREA_EXPANDED_VIEWPORT_RATIO),
      COMPOSER_TEXTAREA_EXPANDED_MAX_HEIGHT
    )
  );
}

function padDatePart(value: number): string {
  return String(value).padStart(2, '0');
}

function getTimestampForFilename(date = new Date()): string {
  return [
    date.getFullYear(),
    padDatePart(date.getMonth() + 1),
    padDatePart(date.getDate()),
    '-',
    padDatePart(date.getHours()),
    padDatePart(date.getMinutes()),
    padDatePart(date.getSeconds()),
  ].join('');
}

function getImageExtensionFromMimeType(mimeType: string): string {
  const subtype = mimeType.split('/')[1]?.toLowerCase() || 'png';
  if (subtype === 'jpeg') return 'jpg';
  if (subtype.includes('svg')) return 'svg';
  if (subtype.includes('png')) return 'png';
  return subtype.replace(/[^a-z0-9]/g, '') || 'png';
}

function renamePastedImageFile(file: File, index: number): File {
  const extension = getImageExtensionFromMimeType(file.type);
  const suffix = index > 0 ? `-${index + 1}` : '';
  return new File([file], `pasted-image-${getTimestampForFilename()}${suffix}.${extension}`, {
    type: file.type,
    lastModified: file.lastModified || Date.now(),
  });
}

function shouldRenamePastedImageFile(file: File): boolean {
  const name = file.name.trim().toLowerCase();
  return (
    file.type.startsWith('image/') && (!name || name === 'image.png' || name === 'clipboard.png')
  );
}

function getPastedFiles(event: ClipboardEvent<HTMLTextAreaElement>): File[] {
  const clipboard = event.clipboardData;
  const directFiles = Array.from(clipboard.files ?? []);
  if (directFiles.length > 0) {
    return directFiles.map((file, index) =>
      shouldRenamePastedImageFile(file) ? renamePastedImageFile(file, index) : file
    );
  }

  return Array.from(clipboard.items ?? [])
    .filter(item => item.kind === 'file')
    .map(item => item.getAsFile())
    .filter((file): file is File => Boolean(file))
    .map((file, index) =>
      file.type.startsWith('image/') ? renamePastedImageFile(file, index) : file
    );
}

function isComposingEnterEvent(event: KeyboardEvent<HTMLElement>): boolean {
  const nativeEvent = event.nativeEvent as globalThis.KeyboardEvent & {
    isComposing?: boolean;
  };

  return nativeEvent.isComposing === true || event.keyCode === 229;
}

function resizeComposerTextarea(
  textarea: HTMLTextAreaElement | null,
  maxHeight = COMPOSER_TEXTAREA_MAX_HEIGHT,
  minHeight = COMPOSER_TEXTAREA_MIN_HEIGHT,
  forceMaxHeight = false
): boolean {
  if (!textarea) return false;
  textarea.style.height = 'auto';
  const nextHeight = forceMaxHeight
    ? maxHeight
    : Math.min(Math.max(textarea.scrollHeight, minHeight), maxHeight);
  const isOverflowing = textarea.scrollHeight > maxHeight;
  textarea.style.height = `${nextHeight}px`;
  textarea.style.overflowY = isOverflowing ? 'auto' : 'hidden';
  return isOverflowing;
}

interface AIChatInputAreaProps {
  isEmbedded?: boolean;
  isHome: boolean;
  isLoadingMessages: boolean;
  input: string;
  modelSelectorValue: AIChatModelValue;
  modelSelectorWarning?: boolean;
  modelProps?: ModelSelectorModelProps | null;
  supportsVisionOverride?: boolean;
  isModelInitializing?: boolean;
  modelMissing: boolean;
  isSending: boolean;
  allowRuntimeInput?: boolean;
  runtimeInputQueueFull?: boolean;
  disabled?: boolean;
  canStop?: boolean;
  isStopping: boolean;
  onInputChange: (value: string) => void;
  onSend: (files: AIChatMessageFile[], useMemory: boolean) => boolean | Promise<boolean>;
  activeUserInputRequest?: AIChatUserInputRequest | null;
  onUserInputRequestSubmit?: (
    query: string,
    useMemory: boolean,
    answers?: Record<string, string>
  ) => void;
  activeWorkflowApprovalRequest?: AIChatWorkflowApprovalRequest | null;
  /** Rejects when the submission fails, so the form can be sent again. */
  onWorkflowApprovalSubmit?: (
    request: AIChatWorkflowApprovalRequest,
    payload: AIChatWorkflowApprovalSubmitPayload
  ) => void | Promise<void>;
  onStop: () => void;
  onModelChange: (value: ModelSelectorValue) => void;
  onModelPropsChange?: (model: ModelSelectorModelProps | null) => void;
  onHeightChange?: (height: number) => void;
  showModelSelector?: boolean;
  modelUseCase?: ModelUseCase;
  modelAvailabilityUseCase?: AvailableModelUseCase;
  preferredModelUseCase?: ModelUseCase;
  showMemoryEntry?: boolean;
  enableUpload?: boolean;
  fileUploadPolicy?: FileUploadSelection | null;
  uploadScope?: AIChatUploadScope;
  // Anonymous visitors see the upload buttons, which ask them to sign in.
  uploadRequiresLogin?: boolean;
  showFileLibraryPicker?: boolean;
  allowWorkspaceSwitch?: boolean;
  showConnectedApps?: boolean;
  connectedAppsLabel?: string;
  connectedAppsSelectedCount?: number;
  connectedAppsAttentionRequired?: boolean;
  connectedAppsDisabled?: boolean;
  connectedAppsOpen?: boolean;
  connectedAppsPopup?: 'menu' | 'dialog';
  onOpenConnectedApps?: (anchorElement: HTMLElement) => void;
  showSkillManagement?: boolean;
  skillManagementLabel?: string;
  onOpenSkillManagement?: () => void;
  inputPlaceholder?: string;
  surface?: AIChatComposerSurface;
  topAccessory?: ReactNode;
  showToolGovernancePermissionControl?: boolean;
  toolGovernancePermissionTier?: AIChatToolGovernancePermissionTier;
  onToolGovernancePermissionTierChange?: (tier: AIChatToolGovernancePermissionTier) => void;
  enableToolGovernanceApprovals?: boolean;
  activeConversationId?: string | null;
  activeToolGovernanceMessageId?: string | null;
  activeToolGovernanceApprovalFallback?: ToolGovernancePendingApproval | null;
  voiceTranscriber?: AIChatVoiceTranscriber;
  speechAutoPlay?: {
    enabled: boolean;
    onEnabledChange: (enabled: boolean) => void;
  };
}

function ToolGovernancePendingApprovalBridge({
  enabled,
  onApprovalChange,
}: {
  enabled: boolean;
  onApprovalChange: (approval: ToolGovernancePendingApproval | null) => void;
}) {
  if (!enabled) return null;
  return <ActiveToolGovernancePendingApprovalBridge onApprovalChange={onApprovalChange} />;
}

function ActiveToolGovernancePendingApprovalBridge({
  onApprovalChange,
}: {
  onApprovalChange: (approval: ToolGovernancePendingApproval | null) => void;
}) {
  const approval = useActiveToolGovernancePendingApproval();

  useEffect(() => {
    onApprovalChange(approval);
  }, [approval, onApprovalChange]);

  useEffect(() => () => onApprovalChange(null), [onApprovalChange]);

  return null;
}

/**
 * @component AIChatInputArea
 * @category Feature
 * @status Stable
 * @description Floating AIChat prompt composer with model selection and send action.
 * @usage Render at the bottom of AIChatShell or centered on the home view
 * @example
 * <AIChatInputArea input={input} onSend={send} />
 */
export function AIChatInputArea({
  isEmbedded = false,
  isHome,
  isLoadingMessages,
  input,
  modelSelectorValue,
  modelSelectorWarning = false,
  modelProps,
  isModelInitializing = false,
  modelMissing,
  isSending,
  allowRuntimeInput = false,
  runtimeInputQueueFull = false,
  disabled = false,
  canStop,
  isStopping,
  onInputChange,
  onSend,
  activeUserInputRequest,
  onUserInputRequestSubmit,
  activeWorkflowApprovalRequest,
  onWorkflowApprovalSubmit,
  onStop,
  onModelChange,
  onModelPropsChange,
  onHeightChange,
  showModelSelector = true,
  modelUseCase = 'agent',
  modelAvailabilityUseCase,
  preferredModelUseCase,
  showMemoryEntry = false,
  enableUpload = true,
  fileUploadPolicy,
  uploadScope = { type: 'console' },
  uploadRequiresLogin = false,
  showFileLibraryPicker = true,
  allowWorkspaceSwitch = false,
  showConnectedApps = false,
  connectedAppsLabel,
  connectedAppsSelectedCount = 0,
  connectedAppsAttentionRequired = false,
  connectedAppsDisabled = false,
  connectedAppsOpen = false,
  connectedAppsPopup = 'dialog',
  onOpenConnectedApps,
  showSkillManagement = false,
  skillManagementLabel,
  onOpenSkillManagement,
  inputPlaceholder,
  surface = 'aichat',
  topAccessory,
  showToolGovernancePermissionControl = false,
  toolGovernancePermissionTier = 'basic',
  onToolGovernancePermissionTierChange,
  enableToolGovernanceApprovals = false,
  activeConversationId = null,
  activeToolGovernanceMessageId = null,
  activeToolGovernanceApprovalFallback = null,
  voiceTranscriber,
  speechAutoPlay,
}: AIChatInputAreaProps) {
  const t = useT('webapp');
  const containerRef = useRef<HTMLDivElement | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const imageInputRef = useRef<HTMLInputElement | null>(null);
  const dragDepthRef = useRef(0);
  const isComposingRef = useRef(false);
  const ignoreEnterAfterCompositionRef = useRef(false);
  const compositionEndTimerRef = useRef<number | null>(null);
  // Upload attempts, keyed by the current attachment id. In-flight uploads can legitimately
  // run for minutes, and completed-but-unsent drafts stay cancellable until they are claimed.
  const uploadControllersRef = useRef<
    Map<string, { controller: AbortController; cancelAttempt?: () => Promise<void> }>
  >(new Map());
  const [attachments, setAttachments] = useState<AIChatInputAttachment[]>([]);
  const [isFileSelectorOpen, setIsFileSelectorOpen] = useState(false);
  const [isDraggingFiles, setIsDraggingFiles] = useState(false);
  const [selectedModelProps, setSelectedModelProps] = useState<ModelSelectorModelProps | null>(
    null
  );
  // Turns no longer ask for account memory: AI chat remembers through its
  // own memory (ADR 0021).
  const useMemory = false;
  const [isPreparingSend, setIsPreparingSend] = useState(false);
  const [isComposerExpanded, setIsComposerExpanded] = useState(false);
  const [isComposerOverflowing, setIsComposerOverflowing] = useState(false);
  const [questionAnswers, setQuestionAnswers] = useState<Record<string, string>>({});
  const [activeQuestionIndex, setActiveQuestionIndex] = useState(0);
  const [submittedApprovalAction, setSubmittedApprovalAction] = useState<string | null>(null);
  const [activeToolGovernanceApproval, setActiveToolGovernanceApproval] =
    useState<ToolGovernancePendingApproval | null>(null);
  const toolGovernancePendingApprovalScope = useToolGovernancePendingApprovalScope();
  useToolGovernancePendingApprovalVersion(toolGovernancePendingApprovalScope);

  const handleModelPropsChange = useCallback(
    (nextModel: ModelSelectorModelProps | null) => {
      setSelectedModelProps(nextModel);
      onModelPropsChange?.(nextModel);
    },
    [onModelPropsChange]
  );
  const canSubmitWorkflowApprovalInline = activeWorkflowApprovalRequest?.canSubmitInline === true;
  const activeApprovalForm = canSubmitWorkflowApprovalInline
    ? (activeWorkflowApprovalRequest?.approvalForm ?? null)
    : null;
  const approvalFormQuery = useApprovalForm(
    activeWorkflowApprovalRequest?.approvalToken,
    Boolean(
      canSubmitWorkflowApprovalInline &&
        activeWorkflowApprovalRequest?.approvalToken &&
        !activeApprovalForm
    )
  );
  const approvalForm = activeApprovalForm ?? approvalFormQuery.data ?? null;
  const approvalSubmitMutation = useSubmitApprovalForm(
    activeWorkflowApprovalRequest?.approvalToken
  );
  const workflowApprovalMode: WorkflowApprovalInteractionMode = submittedApprovalAction
    ? 'submitted'
    : !canSubmitWorkflowApprovalInline
      ? 'external'
      : approvalFormQuery.isLoading || approvalFormQuery.isFetching
        ? 'loading'
        : isApprovalFormAlreadySubmittedError(approvalFormQuery.error)
          ? 'completed'
          : approvalFormQuery.error
            ? 'error'
            : approvalForm
              ? 'form'
              : 'loading';

  const clearCompositionEndGuard = useCallback(() => {
    if (compositionEndTimerRef.current !== null) {
      window.clearTimeout(compositionEndTimerRef.current);
      compositionEndTimerRef.current = null;
    }
    ignoreEnterAfterCompositionRef.current = false;
  }, []);

  const handleCompositionStart = useCallback(() => {
    clearCompositionEndGuard();
    isComposingRef.current = true;
  }, [clearCompositionEndGuard]);

  const handleCompositionEnd = useCallback(() => {
    isComposingRef.current = false;
    ignoreEnterAfterCompositionRef.current = true;
    if (compositionEndTimerRef.current !== null) {
      window.clearTimeout(compositionEndTimerRef.current);
    }
    compositionEndTimerRef.current = window.setTimeout(() => {
      compositionEndTimerRef.current = null;
      ignoreEnterAfterCompositionRef.current = false;
    }, COMPOSITION_END_ENTER_GRACE_MS);
  }, []);

  const shouldIgnoreCompositionEnter = useCallback(
    (event: KeyboardEvent<HTMLElement>) => {
      if (isComposingRef.current || isComposingEnterEvent(event)) {
        return true;
      }
      if (!ignoreEnterAfterCompositionRef.current) {
        return false;
      }

      clearCompositionEndGuard();
      event.preventDefault();
      return true;
    },
    [clearCompositionEndGuard]
  );

  useEffect(() => {
    return () => clearCompositionEndGuard();
  }, [clearCompositionEndGuard]);

  const { data: uploadConfig } = useUploadConfig({
    // Visitors who have not signed in cannot read the upload config yet.
    enabled: enableUpload && !uploadRequiresLogin,
    scope: uploadScope.type === 'console' ? undefined : uploadScope,
  });
  const [loginPromptOpen, setLoginPromptOpen] = useState(false);
  const currentWorkspace = useCurrentWorkspace();
  // The application's own policy decides kinds and count; the server's
  // effective policy fills in when the application did not send one.
  const attachmentLimit =
    fileUploadPolicy?.number_limits || uploadConfig?.chat_upload_policy?.number_limit || AICHAT_ATTACHMENT_LIMIT;
  const turnMaxBytes = uploadConfig?.chat_upload_policy?.turn_max_bytes ?? 100 * 1024 * 1024;
  const policyExtensions = useMemo(
    () => chatPolicyExtensions(fileUploadPolicy, uploadConfig?.chat_upload_policy?.allowed_extensions),
    [uploadConfig?.chat_upload_policy?.allowed_extensions, fileUploadPolicy]
  );
  const allowedExtensions = useMemo(
    () => filterLowercaseExtensions(policyExtensions.filter(ext => !IMAGE_EXTENSIONS.includes(ext))),
    [policyExtensions]
  );
  const imageExtensions = useMemo(
    () => filterLowercaseExtensions(policyExtensions.filter(ext => IMAGE_EXTENSIONS.includes(ext))),
    [policyExtensions]
  );
  const allSelectableExtensions = useMemo(
    () => filterLowercaseExtensions([...allowedExtensions, ...imageExtensions]),
    [allowedExtensions, imageExtensions]
  );
  const inputAccept = useMemo(
    () => buildFileInputAcceptAttribute(allowedExtensions),
    [allowedExtensions]
  );
  const maxSizeMB = useMemo(() => {
    const limit = uploadConfig?.chat_upload_policy
      ? uploadConfig.chat_upload_policy.document_max_bytes / 1024 / 1024
      : uploadConfig?.file_size_limit;
    return Math.min(
      typeof limit === 'number' && limit > 0 ? limit : 50,
      turnMaxBytes / 1024 / 1024
    );
  }, [uploadConfig?.file_size_limit, uploadConfig?.chat_upload_policy, turnMaxBytes]);
  const imageMaxSizeMB = useMemo(() => {
    const limit = uploadConfig?.chat_upload_policy
      ? uploadConfig.chat_upload_policy.image_max_bytes / 1024 / 1024
      : uploadConfig?.image_file_size_limit;
    return Math.min(
      typeof limit === 'number' && limit > 0 ? limit : 10,
      turnMaxBytes / 1024 / 1024
    );
  }, [uploadConfig?.image_file_size_limit, uploadConfig?.chat_upload_policy, turnMaxBytes]);

  const audioMaxSizeMB = Math.min(
    (uploadConfig?.chat_upload_policy?.audio_max_bytes ?? 0) / 1024 / 1024 || uploadConfig?.audio_file_size_limit || 50,
    turnMaxBytes / 1024 / 1024
  );
  const videoMaxSizeMB = Math.min(
    (uploadConfig?.chat_upload_policy?.video_max_bytes ?? 0) / 1024 / 1024 || uploadConfig?.video_file_size_limit || 100,
    turnMaxBytes / 1024 / 1024
  );

  const isUploading = attachments.some(attachment => attachment.status === 'uploading');
  const hasUploadError = attachments.some(attachment => attachment.status === 'error');
  const canUseImage = imageExtensions.length > 0;
  const remainingSlots = Math.max(0, attachmentLimit - attachments.length);
  const acceptedTypesLabel = useMemo(
    () => formatExtensionsForDisplay(allSelectableExtensions).join(' / '),
    [allSelectableExtensions]
  );
  const uploadedFiles = useMemo(() => getUploadedAIChatFiles(attachments), [attachments]);
  const composerLineCount = useMemo(() => Math.max(1, input.split(/\r\n|\r|\n/).length), [input]);
  const showComposerExpandButton =
    isComposerExpanded ||
    isComposerOverflowing ||
    composerLineCount > COMPOSER_EXPAND_VISIBLE_LINE_COUNT;
  const canClickSend =
    Boolean(input.trim()) &&
    !modelMissing &&
    !isModelInitializing &&
    !isPreparingSend &&
    !isUploading &&
    !(isSending && runtimeInputQueueFull) &&
    !hasUploadError &&
    !disabled;
  const interactionDisabled = disabled || isSending;
  const activeQuestions = useMemo(
    () => (activeUserInputRequest?.questions ?? []).filter(question => question.question?.trim()),
    [activeUserInputRequest?.questions]
  );
  const requestKey = useMemo(
    () =>
      activeUserInputRequest?.request_id ||
      activeQuestions.map(question => question.id || question.question).join('|'),
    [activeQuestions, activeUserInputRequest?.request_id]
  );
  const hasActiveUserInputRequest = activeQuestions.length > 0;
  const hasActiveWorkflowApprovalRequest = Boolean(activeWorkflowApprovalRequest);
  const isCurrentToolGovernanceApproval = useCallback(
    (approval: ToolGovernancePendingApproval | null) => {
      if (!approval) return false;
      if (!activeConversationId || !activeToolGovernanceMessageId) return true;
      return (
        approval.conversationId === activeConversationId &&
        approval.messageId === activeToolGovernanceMessageId
      );
    },
    [activeConversationId, activeToolGovernanceMessageId]
  );
  const visibleActiveToolGovernanceApproval =
    activeToolGovernanceApproval &&
    isCurrentToolGovernanceApproval(activeToolGovernanceApproval) &&
    !isToolGovernancePendingApprovalDismissed(
      activeToolGovernanceApproval.id,
      toolGovernancePendingApprovalScope
    )
      ? activeToolGovernanceApproval
      : null;
  const visibleFallbackToolGovernanceApproval =
    activeToolGovernanceApprovalFallback &&
    isCurrentToolGovernanceApproval(activeToolGovernanceApprovalFallback) &&
    !isToolGovernancePendingApprovalDismissed(
      activeToolGovernanceApprovalFallback.id,
      toolGovernancePendingApprovalScope
    )
      ? activeToolGovernanceApprovalFallback
      : null;
  const effectiveToolGovernanceApproval = enableToolGovernanceApprovals
    ? (visibleFallbackToolGovernanceApproval ?? visibleActiveToolGovernanceApproval)
    : null;
  const hasActiveToolGovernanceApproval = Boolean(effectiveToolGovernanceApproval);
  const hasBlockingApproval = hasActiveWorkflowApprovalRequest || hasActiveToolGovernanceApproval;
  const activeQuestion = hasActiveUserInputRequest
    ? activeQuestions[Math.min(activeQuestionIndex, activeQuestions.length - 1)]
    : undefined;
  const activeQuestionKey = activeQuestion
    ? activeQuestion.id || `q${Math.min(activeQuestionIndex, activeQuestions.length - 1) + 1}`
    : '';
  const activeQuestionAnswer = activeQuestionKey ? (questionAnswers[activeQuestionKey] ?? '') : '';
  const canSubmitCurrentQuestion =
    Boolean(onUserInputRequestSubmit) &&
    Boolean(activeQuestion) &&
    Boolean(activeQuestionAnswer.trim()) &&
    !isUploading &&
    !hasUploadError &&
    !interactionDisabled;

  const handleVoiceInputError = useCallback(
    (error: unknown) => {
      const key = getVoiceInputErrorKey(error);
      if (key === 'cancelled') return;
      const message = {
        permissionDenied: t('consoleChat.voice.errors.permissionDenied'),
        unsupported: t('consoleChat.voice.errors.unsupported'),
        noSpeech: t('consoleChat.voice.errors.noSpeech'),
        timeout: t('consoleChat.voice.errors.timeout'),
        balance: t('consoleChat.voice.errors.balance'),
        quota: t('consoleChat.voice.errors.quota'),
        unavailable: t('consoleChat.voice.errors.unavailable'),
        failed: t('consoleChat.voice.errors.failed'),
      }[key];
      toast.error(message);
    },
    [t]
  );
  const voiceInput = useAgentVoiceInput({
    transcriber: voiceTranscriber,
    input,
    onInputChange,
    onError: handleVoiceInputError,
    disabled:
      isSending ||
      isPreparingSend ||
      isLoadingMessages ||
      isUploading ||
      hasBlockingApproval ||
      hasActiveUserInputRequest,
  });
  const voiceInputBusy = voiceInput.phase !== 'idle';

  useEffect(() => {
    setQuestionAnswers({});
    setActiveQuestionIndex(0);
  }, [requestKey]);

  useEffect(() => {
    setSubmittedApprovalAction(null);
  }, [activeWorkflowApprovalRequest?.approvalToken]);

  useEffect(() => {
    if (!enableToolGovernanceApprovals) {
      setActiveToolGovernanceApproval(null);
    }
  }, [enableToolGovernanceApprovals]);

  const questionKeyForIndex = useCallback(
    (index: number) => activeQuestions[index]?.id || `q${index + 1}`,
    [activeQuestions]
  );

  const handleQuestionAnswerChange = useCallback(
    (index: number, value: string) => {
      const key = questionKeyForIndex(index);
      setQuestionAnswers(current => ({
        ...current,
        [key]: value,
      }));
    },
    [questionKeyForIndex]
  );

  const buildQuestionAnswersQuery = useCallback(
    (answers: Record<string, string>) => {
      const lines = activeQuestions
        .map((question, index) => {
          const answer = answers[questionKeyForIndex(index)]?.trim();
          if (!answer) return '';
          return `${index + 1}. ${question.question.trim()}: ${answer}`;
        })
        .filter(Boolean);
      if (lines.length === 0) return '';
      return [t('consoleChat.userInputRequest.answerPrefix'), ...lines].filter(Boolean).join('\n');
    },
    [activeQuestions, questionKeyForIndex, t]
  );

  const sendQuestionAnswers = useCallback(
    (answers: Record<string, string>) => {
      const query = buildQuestionAnswersQuery(answers);
      if (!query.trim()) return;
      setQuestionAnswers({});
      setActiveQuestionIndex(0);
      onUserInputRequestSubmit?.(query, useMemory, answers);
    },
    [buildQuestionAnswersQuery, onUserInputRequestSubmit, useMemory]
  );

  const advanceQuestionOrSend = useCallback(
    (answers: Record<string, string>) => {
      if (activeQuestionIndex >= activeQuestions.length - 1) {
        sendQuestionAnswers(answers);
        return;
      }
      setActiveQuestionIndex(index => Math.min(index + 1, activeQuestions.length - 1));
    },
    [activeQuestionIndex, activeQuestions.length, sendQuestionAnswers]
  );

  const handleSubmitCurrentQuestion = useCallback(() => {
    if (!canSubmitCurrentQuestion || !activeQuestion) return;
    const index = Math.min(activeQuestionIndex, activeQuestions.length - 1);
    const key = questionKeyForIndex(index);
    const answer = activeQuestionAnswer.trim();
    if (!answer) return;
    const nextAnswers = {
      ...questionAnswers,
      [key]: answer,
    };
    setQuestionAnswers(nextAnswers);
    advanceQuestionOrSend(nextAnswers);
  }, [
    activeQuestion,
    activeQuestionAnswer,
    activeQuestionIndex,
    activeQuestions.length,
    advanceQuestionOrSend,
    canSubmitCurrentQuestion,
    questionAnswers,
    questionKeyForIndex,
  ]);

  const handleSelectQuestionOption = useCallback(
    (value: string) => {
      if (!activeQuestion || isSending) return;
      const index = Math.min(activeQuestionIndex, activeQuestions.length - 1);
      const key = questionKeyForIndex(index);
      const answer = value.trim();
      if (!answer) return;
      const nextAnswers = {
        ...questionAnswers,
        [key]: answer,
      };
      setQuestionAnswers(nextAnswers);
      advanceQuestionOrSend(nextAnswers);
    },
    [
      activeQuestion,
      activeQuestionIndex,
      activeQuestions.length,
      advanceQuestionOrSend,
      isSending,
      questionAnswers,
      questionKeyForIndex,
    ]
  );

  const handlePreviousQuestion = useCallback(() => {
    setActiveQuestionIndex(index => Math.max(index - 1, 0));
  }, []);

  const handleNextQuestion = useCallback(() => {
    setActiveQuestionIndex(index => Math.min(index + 1, activeQuestions.length - 1));
  }, [activeQuestions.length]);

  const validateFile = useCallback(
    (file: File, kind: AIChatAttachmentUploadKind): string | null => {
      const extension = file.name.split('.').pop()?.toLowerCase() ?? '';
      const extensions = kind === 'image' ? imageExtensions : allowedExtensions;
      if (!extensions.includes(extension)) {
        return t('consoleChat.attachments.unsupportedType', {
          types: formatExtensionsForDisplay(extensions).join(', '),
        });
      }
      const sizeLimitMB =
        kind === 'image'
          ? imageMaxSizeMB
          : AUDIO_EXTENSIONS.includes(extension)
            ? audioMaxSizeMB
            : VIDEO_EXTENSIONS.includes(extension)
              ? videoMaxSizeMB
              : maxSizeMB;
      if (file.size > sizeLimitMB * 1024 * 1024) {
        return t('consoleChat.attachments.fileTooLarge', { max: sizeLimitMB });
      }
      return null;
    },
    [allowedExtensions, audioMaxSizeMB, imageExtensions, imageMaxSizeMB, maxSizeMB, t, videoMaxSizeMB]
  );

  const uploadOneFile = useCallback(
    async (file: File, localId: string, kind: AIChatAttachmentUploadKind) => {
      // A fresh controller per attempt so the request can be aborted when the user
      // removes the attachment or the composer unmounts.
      const controller = new AbortController();
      const attemptID = crypto.randomUUID();
      // The digital-human webapp upload endpoint has no attempt tracking, so its uploads
      // can only be aborted locally; never send its end users to a console endpoint.
      const cancelAttempt =
        uploadScope.type === 'webapp'
          ? () => uploadService.cancelWebAppUploadAttempt(uploadScope.webAppId, attemptID)
          : uploadScope.type === 'digital-human-webapp'
            ? undefined
            : () => uploadService.cancelUploadAttempt(attemptID);
      uploadControllersRef.current.set(localId, { controller, cancelAttempt });
      try {
        const onProgress = (progress: number) =>
          setAttachments(current =>
            current.map(attachment =>
              attachment.id === localId ? { ...attachment, progress } : attachment
            )
          );
        const response =
          uploadScope.type === 'webapp'
            ? await uploadService.uploadWebAppSingle(uploadScope.webAppId, file, {
                onProgress,
                signal: controller.signal,
                upload_attempt_id: attemptID,
              })
            : uploadScope.type === 'digital-human-webapp'
              ? await uploadService.uploadDigitalHumanWebAppSingle(uploadScope.webAppId, file, {
                  onProgress,
                  signal: controller.signal,
                })
              : await uploadService.uploadSingle(file, {
                  is_temporary: true,
                  source: 'chat',
                  workspace_id: currentWorkspace?.id,
                  onProgress,
                  signal: controller.signal,
                  upload_attempt_id: attemptID,
                });
        const uploadedFile = toAIChatMessageFile(response, kind);
        const upload = uploadControllersRef.current.get(localId);
        if (upload?.controller === controller) {
          // Keep the attempt handle attached to the uploaded attachment until the
          // message is sent or the user removes it. An upload can finish after the
          // abort request raced it, and a completed-but-unsent draft still needs a
          // cancellation signal instead of waiting for the server sweep.
          uploadControllersRef.current.delete(localId);
          uploadControllersRef.current.set(uploadedFile.id, upload);
        }
        setAttachments(current =>
          current.map(attachment =>
            attachment.id === localId
              ? {
                  ...attachment,
                  id: uploadedFile.id,
                  name: uploadedFile.name,
                  size: uploadedFile.size,
                  extension: uploadedFile.extension,
                  kind,
                  progress: 100,
                  status: 'uploaded',
                  file: uploadedFile,
                  sourceFile: undefined,
                  error: undefined,
                }
              : attachment
          )
        );
      } catch (error) {
        // A cancelled upload has no attachment left to mark as failed: the user removed it,
        // or the composer unmounted. Trust the controller, which cannot be misread, and
        // fall back to the error shape for cancellations raised elsewhere.
        if (controller.signal.aborted || isUploadCanceledError(error)) return;
        const upload = uploadControllersRef.current.get(localId);
        if (upload?.controller === controller) {
          void upload
            .cancelAttempt?.()
            .catch(cancelError => console.warn('Failed to cancel failed upload attempt', cancelError));
        }
        const message =
          error instanceof Error ? error.message : t('consoleChat.attachments.uploadFailed');
        setAttachments(current =>
          current.map(attachment =>
            attachment.id === localId
              ? {
                  ...attachment,
                  progress: 100,
                  status: 'error',
                  error: message,
                }
              : attachment
          )
        );
      } finally {
        // A successful upload is moved to the server file id above; failed or cancelled
        // attempts still release the local id without touching a newer retry.
        if (uploadControllersRef.current.get(localId)?.controller === controller) {
          uploadControllersRef.current.delete(localId);
        }
      }
    },
    [currentWorkspace?.id, t, uploadScope]
  );

  const enqueueFiles = useCallback(
    (files: File[], fallbackKind?: AIChatAttachmentUploadKind) => {
      if (files.length === 0) return;
      if (!enableUpload) return;
      if (uploadRequiresLogin) {
        setLoginPromptOpen(true);
        return;
      }
      if (isSending || isUploading) {
        toast.error(t('consoleChat.attachments.uploadUnavailable'));
        return;
      }

      const availableSlots = Math.max(0, attachmentLimit - attachments.length);
      if (availableSlots <= 0) {
        toast.error(t('consoleChat.attachments.exceedCount', { max: attachmentLimit }));
        return;
      }

      const selectedFiles = files.slice(0, availableSlots);
      if (selectedFiles.length < files.length) {
        toast.error(t('consoleChat.attachments.exceedCount', { max: attachmentLimit }));
      }

      let totalBytes = attachments.reduce((sum, file) => sum + file.size, 0);
      selectedFiles.forEach(file => {
        const fileExtension = getNormalizedExtension(file.name.split('.').pop());
        const kind =
          fallbackKind ?? (imageExtensions.includes(fileExtension) ? 'image' : 'document');
        const validationError = validateFile(file, kind);
        if (validationError) {
          toast.error(validationError);
          return;
        }
        if (file.size > turnMaxBytes - totalBytes) {
          toast.error(
            t('consoleChat.attachments.turnTooLarge', { max: turnMaxBytes / 1024 / 1024 })
          );
          return;
        }
        totalBytes += file.size;

        const localId = createAttachmentId();
        setAttachments(current => [
          ...current,
          {
            id: localId,
            name: file.name,
            size: file.size,
            extension: fileExtension,
            kind,
            progress: 0,
            status: 'uploading',
            sourceFile: file,
          },
        ]);
        void uploadOneFile(file, localId, kind);
      });
    },
    [
      attachments,
      attachmentLimit,
      turnMaxBytes,
      enableUpload,
      imageExtensions,
      isSending,
      isUploading,
      t,
      uploadOneFile,
      uploadRequiresLogin,
      validateFile,
    ]
  );

  const handleFilesSelected = useCallback(
    (event: ChangeEvent<HTMLInputElement>, kind: AIChatAttachmentUploadKind) => {
      const files = event.target.files ? Array.from(event.target.files) : [];
      event.target.value = '';
      enqueueFiles(files, kind);
    },
    [enqueueFiles]
  );

  const handleRemoveAttachment = useCallback((id: string) => {
    // Removing an attachment that is still uploading cancels its request instead of
    // leaving the user waiting on a response nobody wants.
    const upload = uploadControllersRef.current.get(id);
    upload?.controller.abort();
    void upload
      ?.cancelAttempt?.()
      .catch(error => console.warn('Failed to cancel upload attempt', error));
    uploadControllersRef.current.delete(id);
    setAttachments(current => current.filter(attachment => attachment.id !== id));
  }, []);

  // Abandon in-flight uploads when the composer goes away.
  useEffect(
    () => () => {
      uploadControllersRef.current.forEach(upload => {
        upload.controller.abort();
        void upload
          .cancelAttempt?.()
          .catch(error => console.warn('Failed to cancel upload attempt', error));
      });
      uploadControllersRef.current.clear();
    },
    []
  );

  const handleRetryAttachment = useCallback(
    (id: string) => {
      const attachment = attachments.find(item => item.id === id);
      if (!attachment?.sourceFile) {
        toast.error(t('consoleChat.attachments.retryUnavailable'));
        return;
      }
      // Only sending blocks a retry. Another attachment finishing its upload must
      // not serialize independent retries, otherwise every retry has to wait for
      // the previous one to settle.
      if (isSending || attachment.status === 'uploading') {
        toast.error(t('consoleChat.attachments.uploadUnavailable'));
        return;
      }

      const validationError = validateFile(attachment.sourceFile, attachment.kind);
      if (validationError) {
        toast.error(validationError);
        return;
      }

      setAttachments(current =>
        current.map(item =>
          item.id === id
            ? {
                ...item,
                progress: 0,
                status: 'uploading',
                error: undefined,
              }
            : item
        )
      );
      void uploadOneFile(attachment.sourceFile, id, attachment.kind);
    },
    [attachments, isSending, t, uploadOneFile, validateFile]
  );

  const handleImageUpload = useCallback(() => {
    if (uploadRequiresLogin) {
      setLoginPromptOpen(true);
      return;
    }
    imageInputRef.current?.click();
  }, [uploadRequiresLogin]);
  const handleDocumentUpload = useCallback(() => {
    if (uploadRequiresLogin) {
      setLoginPromptOpen(true);
      return;
    }
    fileInputRef.current?.click();
  }, [uploadRequiresLogin]);

  const handleSystemFilesConfirm = useCallback(
    (files: FileItem[]) => {
      if (files.length === 0) return;

      setAttachments(current => {
        const existingIds = new Set(
          current.map(attachment => attachment.file?.id || attachment.id).filter(Boolean)
        );
        const nextItems: AIChatInputAttachment[] = [];

        for (const file of files) {
          if (current.length + nextItems.length >= attachmentLimit) {
            toast.error(t('consoleChat.attachments.exceedCount', { max: attachmentLimit }));
            break;
          }
          if (existingIds.has(file.id)) {
            continue;
          }

          const extension = getNormalizedExtension(file.extension);
          const isImage = isImageExtension(extension);
          const allowedByType = isImage
            ? imageExtensions.includes(extension)
            : allowedExtensions.includes(extension);
          if (!allowedByType) {
            toast.error(
              t('consoleChat.attachments.unsupportedType', {
                types: formatExtensionsForDisplay(allSelectableExtensions).join(', '),
              })
            );
            continue;
          }
          const maxBytes = (isImage ? imageMaxSizeMB : maxSizeMB) * 1024 * 1024;
          if (file.size < 0 || file.size > maxBytes) {
            toast.error(t('consoleChat.attachments.fileTooLarge', { max: maxBytes / 1024 / 1024 }));
            continue;
          }
          if (
            file.size >
            turnMaxBytes - [...current, ...nextItems].reduce((sum, item) => sum + item.size, 0)
          ) {
            toast.error(
              t('consoleChat.attachments.turnTooLarge', { max: turnMaxBytes / 1024 / 1024 })
            );
            continue;
          }

          const messageFile = fileItemToAIChatMessageFile(file);
          nextItems.push({
            id: file.id,
            name: file.name,
            size: file.size,
            extension,
            kind: isImage ? 'image' : 'document',
            progress: 100,
            status: 'uploaded',
            file: messageFile,
            previewUrl: isImage ? file.source_url : undefined,
          });
          existingIds.add(file.id);
        }

        return nextItems.length > 0 ? [...current, ...nextItems] : current;
      });
    },
    [
      allSelectableExtensions,
      allowedExtensions,
      attachmentLimit,
      turnMaxBytes,
      imageMaxSizeMB,
      maxSizeMB,
      imageExtensions,
      t,
    ]
  );

  const handleSend = useCallback(async () => {
    if (!input.trim() || isPreparingSend || isUploading || hasUploadError || voiceInputBusy) return;
    // Attachments belong to a signed-in visitor; if the session was lost after
    // attaching, ask them to sign in instead of sending files the server rejects.
    if (uploadRequiresLogin && attachments.length > 0) {
      setLoginPromptOpen(true);
      return;
    }
    if (attachments.length > attachmentLimit) {
      toast.error(t('consoleChat.attachments.exceedCount', { max: attachmentLimit }));
      return;
    }
    if (attachments.reduce((sum, file) => sum + file.size, 0) > turnMaxBytes) {
      toast.error(t('consoleChat.attachments.turnTooLarge', { max: turnMaxBytes / 1024 / 1024 }));
      return;
    }
    if (
      attachments.some(
        file =>
          !allSelectableExtensions.includes(file.extension)
      )
    ) {
      toast.error(
        t('consoleChat.attachments.unsupportedType', { types: allSelectableExtensions.join(', ') })
      );
      return;
    }
    const oversized = attachments.find(
      file => file.size >
        (IMAGE_EXTENSIONS.includes(file.extension) ? imageMaxSizeMB : maxSizeMB) * 1024 * 1024
    );
    if (oversized) {
      toast.error(t('consoleChat.attachments.fileTooLarge', {
        max: IMAGE_EXTENSIONS.includes(oversized.extension) ? imageMaxSizeMB : maxSizeMB,
      }));
      return;
    }
    setIsPreparingSend(true);
    try {
      const sent = await onSend(uploadedFiles, useMemory);
      if (sent !== false) {
        uploadedFiles.forEach(file => uploadControllersRef.current.delete(file.id));
        setAttachments([]);
      }
    } finally {
      setIsPreparingSend(false);
    }
  }, [
    attachments,
    attachmentLimit,
    turnMaxBytes,
    t,
    allSelectableExtensions,
    imageMaxSizeMB,
    maxSizeMB,
    hasUploadError,
    input,
    isPreparingSend,
    isUploading,
    onSend,
    uploadRequiresLogin,
    uploadedFiles,
    useMemory,
    voiceInputBusy,
  ]);

  const handleWorkflowApprovalSubmit = useCallback(
    async (payload: { inputs: Record<string, unknown>; action: string }) => {
      if (!activeWorkflowApprovalRequest) return;
      setSubmittedApprovalAction(payload.action);
      // A submission that fails leaves the submitted state, so the person can
      // send it again instead of facing a form that waits for nothing.
      if (onWorkflowApprovalSubmit) {
        try {
          await onWorkflowApprovalSubmit(activeWorkflowApprovalRequest, payload);
        } catch {
          setSubmittedApprovalAction(null);
        }
        return;
      }
      try {
        await approvalSubmitMutation.mutateAsync(payload);
      } catch (error) {
        setSubmittedApprovalAction(null);
        toast.error(
          error instanceof Error ? error.message : t('consoleChat.workflow.approvalSubmitFailed')
        );
      }
    },
    [activeWorkflowApprovalRequest, approvalSubmitMutation, onWorkflowApprovalSubmit, t]
  );

  const handlePaste = useCallback(
    (event: ClipboardEvent<HTMLTextAreaElement>) => {
      const files = getPastedFiles(event);
      if (!enableUpload) {
        return;
      }
      if (files.length === 0) {
        return;
      }

      event.preventDefault();
      enqueueFiles(files);
    },
    [enableUpload, enqueueFiles]
  );

  const handleComposerInputChange = useCallback(
    (event: ChangeEvent<HTMLTextAreaElement>) => {
      onInputChange(event.target.value);
    },
    [onInputChange]
  );

  const handleComposerKeyDown = useCallback(
    (event: KeyboardEvent<HTMLTextAreaElement>) => {
      if (event.key === 'Escape' && (canStop ?? isSending) && !input.trim()) {
        event.preventDefault();
        event.stopPropagation();
        if (!isStopping) onStop();
        return;
      }
      if (event.key !== 'Enter' || event.shiftKey) return;
      if (shouldIgnoreCompositionEnter(event)) return;
      if (
        disabled ||
        (isSending && !allowRuntimeInput) ||
        isPreparingSend ||
        isModelInitializing ||
        isUploading ||
        hasUploadError ||
        voiceInputBusy
      ) {
        return;
      }
      event.preventDefault();
      void handleSend();
    },
    [
      allowRuntimeInput,
      canStop,
      disabled,
      handleSend,
      hasUploadError,
      input,
      isModelInitializing,
      isPreparingSend,
      isSending,
      isStopping,
      isUploading,
      onStop,
      shouldIgnoreCompositionEnter,
      voiceInputBusy,
    ]
  );

  const adjustComposerTextareaHeight = useCallback(() => {
    const isOverflowing = resizeComposerTextarea(
      textareaRef.current,
      isComposerExpanded ? getComposerExpandedMaxHeight() : COMPOSER_TEXTAREA_MAX_HEIGHT,
      isComposerExpanded ? COMPOSER_TEXTAREA_EXPANDED_MIN_HEIGHT : COMPOSER_TEXTAREA_MIN_HEIGHT,
      isComposerExpanded
    );
    setIsComposerOverflowing(current => (current === isOverflowing ? current : isOverflowing));
  }, [isComposerExpanded]);

  useLayoutEffect(() => {
    adjustComposerTextareaHeight();
  }, [adjustComposerTextareaHeight, input]);

  useEffect(() => {
    if (!input.trim() && isComposerExpanded) {
      setIsComposerExpanded(false);
    }
  }, [input, isComposerExpanded]);

  useEffect(() => {
    window.addEventListener('resize', adjustComposerTextareaHeight);
    return () => {
      window.removeEventListener('resize', adjustComposerTextareaHeight);
    };
  }, [adjustComposerTextareaHeight]);

  useLayoutEffect(() => {
    const container = containerRef.current;
    if (!container || typeof ResizeObserver === 'undefined') return;

    let observedWidth = Math.ceil(container.getBoundingClientRect().width);
    let frameId: number | null = null;
    const resizeObserver = new ResizeObserver(entries => {
      const nextWidth = Math.ceil(entries[0]?.contentRect.width ?? 0);
      if (nextWidth === observedWidth) return;
      observedWidth = nextWidth;
      if (frameId !== null) window.cancelAnimationFrame(frameId);
      frameId = window.requestAnimationFrame(() => {
        frameId = null;
        adjustComposerTextareaHeight();
      });
    });
    resizeObserver.observe(container);

    return () => {
      resizeObserver.disconnect();
      if (frameId !== null) window.cancelAnimationFrame(frameId);
    };
  }, [adjustComposerTextareaHeight]);

  useEffect(() => {
    if (!isComposerExpanded) return;

    const frameId = window.requestAnimationFrame(() => {
      adjustComposerTextareaHeight();
      const textarea = textareaRef.current;
      if (!textarea) return;
      textarea.focus();
      const end = textarea.value.length;
      textarea.setSelectionRange(end, end);
    });

    return () => {
      window.cancelAnimationFrame(frameId);
    };
  }, [adjustComposerTextareaHeight, isComposerExpanded]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container || !onHeightChange) return;

    const updateHeight = () => {
      onHeightChange(Math.ceil(container.getBoundingClientRect().height));
    };
    updateHeight();

    const resizeObserver = new ResizeObserver(updateHeight);
    resizeObserver.observe(container);

    return () => {
      resizeObserver.disconnect();
    };
  }, [onHeightChange]);

  useEffect(() => {
    if (!enableUpload) return;
    const hasDraggedFiles = (event: DragEvent) =>
      Array.from(event.dataTransfer?.types ?? []).includes('Files');

    const resetDragState = () => {
      dragDepthRef.current = 0;
      setIsDraggingFiles(false);
    };

    const handleDragEnter = (event: DragEvent) => {
      if (!hasDraggedFiles(event)) return;
      event.preventDefault();
      dragDepthRef.current += 1;
      setIsDraggingFiles(true);
    };

    const handleDragOver = (event: DragEvent) => {
      if (!hasDraggedFiles(event)) return;
      event.preventDefault();
      if (event.dataTransfer) {
        event.dataTransfer.dropEffect =
          interactionDisabled || isUploading || remainingSlots <= 0 ? 'none' : 'copy';
      }
    };

    const handleDragLeave = (event: DragEvent) => {
      if (!hasDraggedFiles(event)) return;
      event.preventDefault();
      dragDepthRef.current = Math.max(0, dragDepthRef.current - 1);
      if (dragDepthRef.current === 0) {
        setIsDraggingFiles(false);
      }
    };

    const handleDrop = (event: DragEvent) => {
      if (!hasDraggedFiles(event)) return;
      event.preventDefault();
      event.stopPropagation();
      resetDragState();
      enqueueFiles(Array.from(event.dataTransfer?.files ?? []));
    };

    window.addEventListener('dragenter', handleDragEnter);
    window.addEventListener('dragover', handleDragOver);
    window.addEventListener('dragleave', handleDragLeave);
    window.addEventListener('drop', handleDrop);

    return () => {
      window.removeEventListener('dragenter', handleDragEnter);
      window.removeEventListener('dragover', handleDragOver);
      window.removeEventListener('dragleave', handleDragLeave);
      window.removeEventListener('drop', handleDrop);
    };
  }, [enableUpload, enqueueFiles, interactionDisabled, isUploading, remainingSlots]);

  return (
    <>
      <ToolGovernancePendingApprovalBridge
        enabled={enableToolGovernanceApprovals}
        onApprovalChange={setActiveToolGovernanceApproval}
      />
      <ConfirmDialog
        open={loginPromptOpen}
        onOpenChange={setLoginPromptOpen}
        title={t('consoleChat.attachments.loginRequiredTitle')}
        description={t('consoleChat.attachments.loginRequiredDescription')}
        confirmText={t('consoleChat.attachments.goToLogin')}
        onConfirm={() => {
          setLoginPromptOpen(false);
          const current = `${window.location.pathname}${window.location.search}`;
          window.location.assign(`/login?redirect=${encodeURIComponent(current)}`);
        }}
      />
      {enableUpload && !hasBlockingApproval && isDraggingFiles ? (
        <AIChatDragUploadOverlay
          isSending={interactionDisabled}
          isUploading={isUploading}
          remainingSlots={remainingSlots}
          attachmentLimit={attachmentLimit}
          acceptedTypesLabel={acceptedTypesLabel}
        />
      ) : null}
      <div
        ref={containerRef}
        className={cn(
          'pointer-events-none absolute inset-x-0 z-20 transition-[top,transform,padding,background-color,box-shadow] duration-300 ease-in-out',
          isEmbedded ? 'px-4' : 'px-4 sm:px-6 lg:px-8',
          isHome && !isLoadingMessages && surface === 'aichat'
            ? 'top-[62%] -translate-y-1/2 pb-0 pt-0 sm:top-[56%]'
            : isEmbedded
              ? cn(
                  'top-full -translate-y-full pb-2 pt-2',
                  surface === 'agent-draft' ? 'bg-background' : 'bg-bg-canvas'
                )
              : 'top-full -translate-y-full bg-background pb-1 shadow-[0_-18px_36px_hsl(var(--background))]'
        )}
      >
        <div
          className={cn(
            'pointer-events-none mx-auto w-full transition-[max-width] duration-300 ease-in-out',
            isEmbedded
              ? 'max-w-full'
              : surface === 'agent-draft'
                ? 'max-w-[560px]'
                : surface === 'digital-human' && isHome && !isLoadingMessages
                  ? 'max-w-3xl xl:max-w-4xl 2xl:max-w-5xl'
                  : isHome && !isLoadingMessages
                    ? 'max-w-3xl'
                    : 'max-w-4xl'
          )}
        >
          {modelMissing && !hasBlockingApproval && !hasActiveUserInputRequest ? (
            <div className="pointer-events-auto mb-2 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {t('consoleChat.modelRequired')}
            </div>
          ) : null}
          {!hasBlockingApproval && topAccessory ? (
            <div className="pointer-events-auto mb-2">{topAccessory}</div>
          ) : null}
          <div
            className={cn(
              'pointer-events-auto',
              hasActiveWorkflowApprovalRequest
                ? 'border-0 bg-transparent p-0 shadow-none'
                : 'border bg-background p-2 focus-within:border-primary/40',
              !hasActiveWorkflowApprovalRequest &&
                (isEmbedded ? 'rounded-lg' : 'rounded-2xl shadow-sm')
            )}
          >
            {hasActiveWorkflowApprovalRequest && activeWorkflowApprovalRequest ? (
              <WorkflowApprovalInteractionCard
                mode={workflowApprovalMode}
                form={approvalForm}
                error={approvalFormQuery.error}
                isSubmitting={approvalSubmitMutation.isPending || isSending}
                submittedAction={submittedApprovalAction}
                onSubmit={handleWorkflowApprovalSubmit}
                onRetry={() => void approvalFormQuery.refetch()}
                secondaryAction={
                  canStop ? (
                    <WorkflowRuntimeStopAction
                      onStop={onStop}
                      isStopping={isStopping}
                      disabled={approvalSubmitMutation.isPending || isSending}
                    />
                  ) : null
                }
              />
            ) : hasActiveToolGovernanceApproval && effectiveToolGovernanceApproval ? (
              <ToolGovernanceApprovalPanel approval={effectiveToolGovernanceApproval} />
            ) : hasActiveUserInputRequest && activeQuestion ? (
              <div className="mb-2 rounded-xl border bg-muted/30 px-3 py-3">
                <div className="mb-3 flex items-start gap-2 text-sm">
                  <HelpCircle className="mt-0.5 size-4 shrink-0 text-primary" />
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="font-medium text-foreground">
                        {t('consoleChat.userInputRequest.title')}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        {t('consoleChat.userInputRequest.progress', {
                          current: Math.min(activeQuestionIndex + 1, activeQuestions.length),
                          total: activeQuestions.length,
                        })}
                      </div>
                    </div>
                    <div className="text-xs text-muted-foreground">
                      {t('consoleChat.userInputRequest.description')}
                    </div>
                  </div>
                </div>
                <div className="space-y-3">
                  {activeUserInputRequest?.message?.trim() ? (
                    <div className="whitespace-pre-wrap break-words text-sm text-muted-foreground">
                      {activeUserInputRequest.message.trim()}
                    </div>
                  ) : null}
                  <div className="text-sm font-medium text-foreground">
                    {activeQuestion.question}
                  </div>
                  {(activeQuestion.options ?? []).filter(option => option.label?.trim()).length >
                  0 ? (
                    <div className="flex flex-wrap gap-1.5">
                      {(activeQuestion.options ?? [])
                        .filter(option => option.label?.trim())
                        .map(option => (
                          <Button
                            key={option.label}
                            type="button"
                            variant={
                              activeQuestionAnswer.trim() === option.label ? 'default' : 'outline'
                            }
                            size="sm"
                            className="h-auto max-w-full justify-start whitespace-normal rounded-md px-2.5 py-1.5 text-left text-xs"
                            title={option.description || option.label}
                            disabled={isSending}
                            onClick={() => handleSelectQuestionOption(option.label)}
                          >
                            <span className="min-w-0">
                              <span className="block break-words">{option.label}</span>
                              {option.description ? (
                                <span className="mt-0.5 block break-words opacity-80">
                                  {option.description}
                                </span>
                              ) : null}
                            </span>
                          </Button>
                        ))}
                    </div>
                  ) : null}
                  <input
                    type="text"
                    value={activeQuestionAnswer}
                    onChange={event =>
                      handleQuestionAnswerChange(activeQuestionIndex, event.target.value)
                    }
                    onKeyDown={event => {
                      if (event.key === 'Enter') {
                        if (shouldIgnoreCompositionEnter(event)) return;
                        event.preventDefault();
                        handleSubmitCurrentQuestion();
                      }
                    }}
                    onCompositionStart={handleCompositionStart}
                    onCompositionEnd={handleCompositionEnd}
                    placeholder={t('consoleChat.userInputRequest.freeAnswerPlaceholder')}
                    className="h-9 w-full rounded-md border bg-background px-2.5 text-sm outline-none transition-colors placeholder:text-muted-foreground focus:border-primary/50"
                    disabled={isSending}
                    autoFocus
                  />
                </div>
                <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center gap-1">
                    <Button
                      type="button"
                      variant="ghost"
                      isIcon
                      className="size-8"
                      disabled={activeQuestionIndex <= 0}
                      title={t('consoleChat.userInputRequest.previous')}
                      onClick={handlePreviousQuestion}
                    >
                      <ChevronLeft className="size-4" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      isIcon
                      className="size-8"
                      disabled={activeQuestionIndex >= activeQuestions.length - 1}
                      title={t('consoleChat.userInputRequest.next')}
                      onClick={handleNextQuestion}
                    >
                      <ChevronRight className="size-4" />
                    </Button>
                  </div>
                  <Button
                    type="button"
                    size="sm"
                    className="rounded-md"
                    disabled={!canSubmitCurrentQuestion}
                    onClick={handleSubmitCurrentQuestion}
                  >
                    {activeQuestionIndex >= activeQuestions.length - 1
                      ? t('consoleChat.userInputRequest.finish')
                      : t('consoleChat.userInputRequest.next')}
                  </Button>
                </div>
              </div>
            ) : null}
            {!hasBlockingApproval && !hasActiveUserInputRequest ? (
              <>
                <AIChatAttachmentStrip
                  attachments={attachments}
                  onRemove={handleRemoveAttachment}
                  onRetry={handleRetryAttachment}
                />
                <div
                  className={cn(
                    'rounded-xl transition-colors duration-200',
                    isComposerExpanded && 'mb-2 border-b border-border/60 bg-muted/20 pb-2'
                  )}
                >
                  <Textarea
                    ref={textareaRef}
                    value={input}
                    disabled={disabled}
                    rows={1}
                    onChange={handleComposerInputChange}
                    onPaste={handlePaste}
                    onCompositionStart={handleCompositionStart}
                    onCompositionEnd={handleCompositionEnd}
                    onKeyDown={handleComposerKeyDown}
                    aria-keyshortcuts="Enter Shift+Enter Escape"
                    placeholder={
                      isSending && allowRuntimeInput
                        ? t('consoleChat.runtimeInput.placeholder')
                        : inputPlaceholder || t('chat.enterCommand')
                    }
                    className={cn(
                      'min-h-12 resize-none overflow-y-hidden border-0 bg-transparent py-2 pl-3 pr-4 text-sm shadow-none focus-visible:ring-0',
                      '[scrollbar-width:thin] [scrollbar-color:rgba(113,113,122,0.45)_transparent]',
                      '[&::-webkit-scrollbar]:w-1.5 [&::-webkit-scrollbar-button]:hidden [&::-webkit-scrollbar-corner]:bg-transparent',
                      '[&::-webkit-scrollbar-track]:my-2 [&::-webkit-scrollbar-track]:rounded-full [&::-webkit-scrollbar-track]:bg-transparent',
                      '[&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-muted-foreground/35 hover:[&::-webkit-scrollbar-thumb]:bg-muted-foreground/55',
                      isComposerExpanded ? 'leading-6' : 'leading-5'
                    )}
                    style={{
                      minHeight: isComposerExpanded
                        ? COMPOSER_TEXTAREA_EXPANDED_MIN_HEIGHT
                        : COMPOSER_TEXTAREA_MIN_HEIGHT,
                      maxHeight: isComposerExpanded
                        ? `min(${COMPOSER_TEXTAREA_EXPANDED_VIEWPORT_RATIO * 100}vh, ${COMPOSER_TEXTAREA_EXPANDED_MAX_HEIGHT}px)`
                        : COMPOSER_TEXTAREA_MAX_HEIGHT,
                      scrollbarGutter: 'stable',
                    }}
                  />
                </div>
              </>
            ) : null}
            <input
              ref={fileInputRef}
              type="file"
              multiple
              hidden
              disabled={disabled}
              accept={inputAccept}
              onChange={event => handleFilesSelected(event, 'document')}
            />
            <input
              ref={imageInputRef}
              type="file"
              multiple
              hidden
              disabled={disabled}
              accept={buildFileInputAcceptAttribute(imageExtensions)}
              onChange={event => handleFilesSelected(event, 'image')}
            />
            {!hasBlockingApproval ? (
              <AIChatInputToolbar
                modelSelectorValue={modelSelectorValue}
                modelProps={selectedModelProps ?? modelProps}
                modelSelectorWarning={modelSelectorWarning}
                isModelInitializing={isModelInitializing}
                modelMissing={modelMissing}
                modelCapabilityFilter={undefined}
                hasImageAttachment={attachments.some(file => file.kind === 'image')}
                isSending={interactionDisabled}
                allowRuntimeInput={allowRuntimeInput && !disabled}
                hasInput={Boolean(input.trim())}
                runtimeInputQueueFull={runtimeInputQueueFull}
                canStop={canStop}
                isUploading={isUploading || isPreparingSend}
                isStopping={isStopping}
                canSend={!hasActiveUserInputRequest && canClickSend && !voiceInputBusy}
                canUseImage={canUseImage}
                remainingSlots={remainingSlots}
                attachmentLimit={attachmentLimit}
                maxSizeMB={maxSizeMB}
                imageMaxSizeMB={imageMaxSizeMB}
                allowedExtensions={allowedExtensions}
                imageExtensions={imageExtensions}
                showModelSelector={showModelSelector}
                modelUseCase={modelUseCase}
                modelAvailabilityUseCase={modelAvailabilityUseCase}
                preferredModelUseCase={preferredModelUseCase}
                showMemoryEntry={showMemoryEntry}
                showComposerExpandButton={!hasActiveUserInputRequest && showComposerExpandButton}
                isComposerExpanded={isComposerExpanded}
                showToolGovernancePermissionControl={showToolGovernancePermissionControl}
                toolGovernancePermissionTier={toolGovernancePermissionTier}
                onToolGovernancePermissionTierChange={onToolGovernancePermissionTierChange}
                enableUpload={!hasActiveUserInputRequest && enableUpload}
                showFileLibraryPicker={showFileLibraryPicker}
                showConnectedApps={showConnectedApps}
                connectedAppsLabel={connectedAppsLabel}
                connectedAppsSelectedCount={connectedAppsSelectedCount}
                connectedAppsAttentionRequired={connectedAppsAttentionRequired}
                connectedAppsDisabled={connectedAppsDisabled}
                connectedAppsOpen={connectedAppsOpen}
                connectedAppsPopup={connectedAppsPopup}
                showSkillManagement={showSkillManagement}
                skillManagementLabel={skillManagementLabel}
                surface={surface}
                onModelChange={onModelChange}
                onModelPropsChange={handleModelPropsChange}
                onUploadDocument={handleDocumentUpload}
                onUploadImage={handleImageUpload}
                onSelectFromFiles={() => setIsFileSelectorOpen(true)}
                onOpenConnectedApps={onOpenConnectedApps}
                onOpenSkillManagement={onOpenSkillManagement}
                onToggleComposerExpanded={() => setIsComposerExpanded(current => !current)}
                onSend={handleSend}
                onStop={onStop}
                voiceInput={voiceTranscriber ? voiceInput : undefined}
                speechAutoPlay={speechAutoPlay}
              />
            ) : null}
          </div>
          <div
            className={cn(
              'pt-1 text-center text-[11px] text-muted-foreground',
              isHome ? 'opacity-0' : 'opacity-100'
            )}
          >
            {t('chat.aiDisclaimer')}
          </div>
          {isFileSelectorOpen ? (
            <FileSelectorDialog
              open={isFileSelectorOpen}
              onOpenChange={setIsFileSelectorOpen}
              onConfirm={handleSystemFilesConfirm}
              maxCount={remainingSlots}
              acceptExt={canUseImage ? allSelectableExtensions : allowedExtensions}
              allowWorkspaceSwitch={allowWorkspaceSwitch}
            />
          ) : null}
        </div>
      </div>
    </>
  );
}

/**
 * The extensions a chat may attach. An application's own selection decides;
 * without one, the server's effective policy (or the default document and
 * image lists) applies.
 */
function chatPolicyExtensions(policy: FileUploadSelection | null | undefined, serverExtensions?: string[]): string[] {
  if (!policy) {
    return serverExtensions?.length ? serverExtensions : [...AICHAT_DOCUMENT_EXTENSIONS, ...IMAGE_EXTENSIONS];
  }
  const extensions: string[] = [];
  for (const kind of policy.allowed_file_types) {
    if (kind === 'document') extensions.push(...AICHAT_DOCUMENT_EXTENSIONS);
    if (kind === 'image') extensions.push(...IMAGE_EXTENSIONS);
    if (kind === 'audio') extensions.push(...AUDIO_EXTENSIONS);
    if (kind === 'video') extensions.push(...VIDEO_EXTENSIONS);
    if (kind === 'custom') extensions.push(...(policy.allowed_file_extensions ?? []));
  }
  return extensions;
}
