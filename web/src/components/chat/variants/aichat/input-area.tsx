'use client';

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type ClipboardEvent,
  type KeyboardEvent,
} from 'react';
import dynamic from 'next/dynamic';
import { toast } from 'sonner';
import {
  type ModelSelectorModelProps,
  type ModelSelectorValue,
} from '@/components/common/model-selector';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import ApprovalRuntimeForm from '@/components/workflow/approval/approval-runtime-form';
import { useApprovalForm, useSubmitApprovalForm } from '@/hooks/workflow/use-approval-form';
import { useUploadConfig } from '@/hooks/use-upload';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { uploadService } from '@/services/upload.service';
import type { FileItem } from '@/services/types/file';
import type { AIChatMessageFile, AIChatUserInputRequest } from '@/services/types/aichat';
import {
  IMAGE_EXTENSIONS,
  buildFileInputAcceptAttribute,
  filterLowercaseExtensions,
  formatExtensionsForDisplay,
} from '@/utils/file-helpers';
import { ChevronLeft, ChevronRight, ExternalLink, HelpCircle, Loader2 } from 'lucide-react';
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
  isVisionModel,
  type ScopedTranslatorWithHas,
  tAttachmentForSurface,
  toAIChatMessageFile,
  type AIChatComposerSurface,
} from '@/components/chat/variants/aichat/input-area-utils';
import type {
  AIChatModelValue,
  AIChatWorkflowApprovalRequest,
  AIChatWorkflowApprovalSubmitPayload,
} from '@/components/chat/variants/aichat/types';

export type AIChatUploadScope = { type: 'console' } | { type: 'webapp'; webAppId: string };

const FileSelectorDialog = dynamic(() => import('@/components/files/file-selector-dialog'), {
  ssr: false,
});

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

interface AIChatInputAreaProps {
  isHome: boolean;
  isLoadingMessages: boolean;
  input: string;
  modelSelectorValue: AIChatModelValue;
  modelProps?: ModelSelectorModelProps | null;
  supportsVisionOverride?: boolean;
  isModelInitializing?: boolean;
  modelMissing: boolean;
  isSending: boolean;
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
  onWorkflowApprovalSubmit?: (
    request: AIChatWorkflowApprovalRequest,
    payload: AIChatWorkflowApprovalSubmitPayload
  ) => void;
  onStop: () => void;
  onModelChange: (value: ModelSelectorValue) => void;
  onHeightChange?: (height: number) => void;
  showModelSelector?: boolean;
  showMemoryToggle?: boolean;
  enableUpload?: boolean;
  uploadScope?: AIChatUploadScope;
  showFileLibraryPicker?: boolean;
  allowWorkspaceSwitch?: boolean;
  inputPlaceholder?: string;
  surface?: AIChatComposerSurface;
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
  isHome,
  isLoadingMessages,
  input,
  modelSelectorValue,
  modelProps,
  supportsVisionOverride,
  isModelInitializing = false,
  modelMissing,
  isSending,
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
  onHeightChange,
  showModelSelector = true,
  showMemoryToggle = true,
  enableUpload = true,
  uploadScope = { type: 'console' },
  showFileLibraryPicker = true,
  allowWorkspaceSwitch = false,
  inputPlaceholder,
  surface = 'aichat',
}: AIChatInputAreaProps) {
  const t = useT('webapp');
  const containerRef = useRef<HTMLDivElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const imageInputRef = useRef<HTMLInputElement | null>(null);
  const dragDepthRef = useRef(0);
  const isComposingRef = useRef(false);
  const [attachments, setAttachments] = useState<AIChatInputAttachment[]>([]);
  const [isFileSelectorOpen, setIsFileSelectorOpen] = useState(false);
  const [isDraggingFiles, setIsDraggingFiles] = useState(false);
  const [selectedModelProps, setSelectedModelProps] = useState<ModelSelectorModelProps | null>(
    null
  );
  const [useMemory, setUseMemory] = useState(false);
  const [isPreparingSend, setIsPreparingSend] = useState(false);
  const [questionAnswers, setQuestionAnswers] = useState<Record<string, string>>({});
  const [activeQuestionIndex, setActiveQuestionIndex] = useState(0);
  const [ignoredUserInputRequestKey, setIgnoredUserInputRequestKey] = useState<string | null>(null);
  const [submittedApprovalAction, setSubmittedApprovalAction] = useState<string | null>(null);
  const activeApprovalForm = activeWorkflowApprovalRequest?.approvalForm ?? null;
  const approvalFormQuery = useApprovalForm(
    activeWorkflowApprovalRequest?.approvalToken,
    Boolean(activeWorkflowApprovalRequest?.approvalToken && !activeApprovalForm)
  );
  const approvalForm = activeApprovalForm ?? approvalFormQuery.data ?? null;
  const approvalSubmitMutation = useSubmitApprovalForm(
    activeWorkflowApprovalRequest?.approvalToken
  );
  const { data: uploadConfig } = useUploadConfig({
    enabled: enableUpload,
    scope: uploadScope.type === 'webapp' ? uploadScope : undefined,
  });
  const allowedExtensions = useMemo(
    () => filterLowercaseExtensions([...AICHAT_DOCUMENT_EXTENSIONS]),
    []
  );
  const imageExtensions = useMemo(() => filterLowercaseExtensions([...IMAGE_EXTENSIONS]), []);
  const allSelectableExtensions = useMemo(
    () => filterLowercaseExtensions([...allowedExtensions, ...imageExtensions]),
    [allowedExtensions, imageExtensions]
  );
  const inputAccept = useMemo(
    () => buildFileInputAcceptAttribute(allowedExtensions),
    [allowedExtensions]
  );
  const maxSizeMB = useMemo(() => {
    const limit = uploadConfig?.file_size_limit;
    return typeof limit === 'number' && limit > 0 ? limit : 100;
  }, [uploadConfig?.file_size_limit]);
  const imageMaxSizeMB = useMemo(() => {
    const limit = uploadConfig?.image_file_size_limit;
    return typeof limit === 'number' && limit > 0 ? limit : 50;
  }, [uploadConfig?.image_file_size_limit]);

  const isUploading = attachments.some(attachment => attachment.status === 'uploading');
  const hasUploadError = attachments.some(attachment => attachment.status === 'error');
  const hasImageAttachment = attachments.some(attachment => attachment.kind === 'image');
  const effectiveModelProps = modelProps ?? selectedModelProps;
  const canUseImage =
    typeof supportsVisionOverride === 'boolean'
      ? supportsVisionOverride
      : isVisionModel(effectiveModelProps);
  const modelCapabilityFilter = useMemo(
    () => (hasImageAttachment ? { features_vision: true } : undefined),
    [hasImageAttachment]
  );
  const remainingSlots = Math.max(0, AICHAT_ATTACHMENT_LIMIT - attachments.length);
  const acceptedTypesLabel = useMemo(
    () => formatExtensionsForDisplay(allSelectableExtensions).join(' / '),
    [allSelectableExtensions]
  );
  const uploadedFiles = useMemo(() => getUploadedAIChatFiles(attachments), [attachments]);
  const canClickSend =
    Boolean(input.trim()) &&
    !modelMissing &&
    !isModelInitializing &&
    !isPreparingSend &&
    !isUploading &&
    !hasUploadError;
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
  const hasActiveUserInputRequest =
    activeQuestions.length > 0 && ignoredUserInputRequestKey !== requestKey;
  const hasActiveWorkflowApprovalRequest = Boolean(activeWorkflowApprovalRequest?.approvalToken);
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
    !modelMissing &&
    !isModelInitializing &&
    !isUploading &&
    !hasUploadError &&
    !isSending;

  useEffect(() => {
    setQuestionAnswers({});
    setActiveQuestionIndex(0);
    setIgnoredUserInputRequestKey(null);
  }, [requestKey]);

  useEffect(() => {
    setSubmittedApprovalAction(null);
  }, [activeWorkflowApprovalRequest?.approvalToken]);

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

  const handleIgnoreUserInputRequest = useCallback(() => {
    setIgnoredUserInputRequestKey(requestKey);
    setActiveQuestionIndex(0);
  }, [requestKey]);

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
      const sizeLimitMB = kind === 'image' ? imageMaxSizeMB : maxSizeMB;
      if (file.size > sizeLimitMB * 1024 * 1024) {
        return t('consoleChat.attachments.fileTooLarge', { max: sizeLimitMB });
      }
      if (kind === 'image' && !canUseImage) {
        return tAttachmentForSurface(
          t as unknown as ScopedTranslatorWithHas,
          surface,
          'imageVisionRequired'
        );
      }
      return null;
    },
    [allowedExtensions, canUseImage, imageExtensions, imageMaxSizeMB, maxSizeMB, surface, t]
  );

  const uploadOneFile = useCallback(
    async (file: File, localId: string, kind: AIChatAttachmentUploadKind) => {
      try {
        const onProgress = (progress: number) =>
          setAttachments(current =>
            current.map(attachment =>
              attachment.id === localId ? { ...attachment, progress } : attachment
            )
          );
        const response =
          uploadScope.type === 'webapp'
            ? await uploadService.uploadWebAppSingle(uploadScope.webAppId, file, { onProgress })
            : await uploadService.uploadSingle(file, {
                is_temporary: true,
                onProgress,
              });
        const uploadedFile = toAIChatMessageFile(response, kind);
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
      }
    },
    [t, uploadScope]
  );

  const enqueueFiles = useCallback(
    (files: File[], fallbackKind?: AIChatAttachmentUploadKind) => {
      if (files.length === 0) return;
      if (!enableUpload) return;
      if (isSending || isUploading) {
        toast.error(t('consoleChat.attachments.uploadUnavailable'));
        return;
      }

      const availableSlots = Math.max(0, AICHAT_ATTACHMENT_LIMIT - attachments.length);
      if (availableSlots <= 0) {
        toast.error(t('consoleChat.attachments.exceedCount', { max: AICHAT_ATTACHMENT_LIMIT }));
        return;
      }

      const selectedFiles = files.slice(0, availableSlots);
      if (selectedFiles.length < files.length) {
        toast.error(t('consoleChat.attachments.exceedCount', { max: AICHAT_ATTACHMENT_LIMIT }));
      }

      selectedFiles.forEach(file => {
        const fileExtension = getNormalizedExtension(file.name.split('.').pop());
        const kind =
          fallbackKind ?? (imageExtensions.includes(fileExtension) ? 'image' : 'document');
        const validationError = validateFile(file, kind);
        if (validationError) {
          toast.error(validationError);
          return;
        }

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
      attachments.length,
      enableUpload,
      imageExtensions,
      isSending,
      isUploading,
      t,
      uploadOneFile,
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
    setAttachments(current => current.filter(attachment => attachment.id !== id));
  }, []);

  const handleRetryAttachment = useCallback(
    (id: string) => {
      const attachment = attachments.find(item => item.id === id);
      if (!attachment?.sourceFile) {
        toast.error(t('consoleChat.attachments.retryUnavailable'));
        return;
      }
      if (isSending || isUploading) {
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
    [attachments, isSending, isUploading, t, uploadOneFile, validateFile]
  );

  const handleImageUpload = useCallback(() => {
    if (!canUseImage) {
      toast.info(
        tAttachmentForSurface(
          t as unknown as ScopedTranslatorWithHas,
          surface,
          'imageVisionRequired'
        )
      );
      return;
    }
    imageInputRef.current?.click();
  }, [canUseImage, surface, t]);

  const handleSystemFilesConfirm = useCallback(
    (files: FileItem[]) => {
      if (files.length === 0) return;

      setAttachments(current => {
        const existingIds = new Set(
          current.map(attachment => attachment.file?.id || attachment.id).filter(Boolean)
        );
        const nextItems: AIChatInputAttachment[] = [];

        for (const file of files) {
          if (current.length + nextItems.length >= AICHAT_ATTACHMENT_LIMIT) {
            toast.error(t('consoleChat.attachments.exceedCount', { max: AICHAT_ATTACHMENT_LIMIT }));
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
          if (isImage && !canUseImage) {
            toast.error(
              tAttachmentForSurface(
                t as unknown as ScopedTranslatorWithHas,
                surface,
                'imageVisionRequired'
              )
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
    [allSelectableExtensions, allowedExtensions, canUseImage, imageExtensions, surface, t]
  );

  const handleSend = useCallback(async () => {
    if (!input.trim() || isPreparingSend || isUploading || hasUploadError) return;
    setIsPreparingSend(true);
    try {
      const sent = await onSend(uploadedFiles, useMemory);
      if (sent !== false) {
        setAttachments([]);
      }
    } finally {
      setIsPreparingSend(false);
    }
  }, [hasUploadError, input, isPreparingSend, isUploading, onSend, uploadedFiles, useMemory]);

  const handleWorkflowApprovalSubmit = useCallback(
    async (payload: { inputs: Record<string, unknown>; action: string }) => {
      if (!activeWorkflowApprovalRequest) return;
      setSubmittedApprovalAction(payload.action);
      if (onWorkflowApprovalSubmit) {
        onWorkflowApprovalSubmit(activeWorkflowApprovalRequest, payload);
        return;
      }
      try {
        await approvalSubmitMutation.mutateAsync(payload);
      } catch (error) {
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
          isSending || isUploading || remainingSlots <= 0 ? 'none' : 'copy';
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
  }, [enableUpload, enqueueFiles, isSending, isUploading, remainingSlots]);

  return (
    <>
      {enableUpload && !hasActiveWorkflowApprovalRequest && isDraggingFiles ? (
        <AIChatDragUploadOverlay
          isSending={isSending}
          isUploading={isUploading}
          remainingSlots={remainingSlots}
          attachmentLimit={AICHAT_ATTACHMENT_LIMIT}
          acceptedTypesLabel={acceptedTypesLabel}
        />
      ) : null}
      <div
        ref={containerRef}
        className={cn(
          'pointer-events-none absolute inset-x-0 z-20 px-4 transition-[top,transform,padding,background-color,box-shadow] duration-300 ease-in-out sm:px-6 lg:px-8',
          isHome && !isLoadingMessages
            ? surface === 'agent-draft'
              ? 'top-[58%] -translate-y-1/2 pb-0 pt-0 sm:top-1/2'
              : 'top-[58%] -translate-y-1/2 pb-0 pt-0 sm:top-1/2'
            : 'top-full -translate-y-full bg-background pb-1 shadow-[0_-18px_36px_hsl(var(--background))]'
        )}
      >
        <div
          className={cn(
            'pointer-events-auto mx-auto w-full transition-[max-width] duration-300 ease-in-out',
            surface === 'agent-draft'
              ? 'max-w-[560px]'
              : isHome && !isLoadingMessages
                ? 'max-w-3xl'
                : 'max-w-4xl'
          )}
        >
          {modelMissing && !hasActiveWorkflowApprovalRequest ? (
            <div className="mb-2 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {t('consoleChat.modelRequired')}
            </div>
          ) : null}
          <div className="rounded-2xl border bg-background p-2 shadow-sm focus-within:border-primary/40">
            {hasActiveWorkflowApprovalRequest && activeWorkflowApprovalRequest ? (
              <div className="rounded-xl border bg-card p-3 shadow-sm">
                <div className="mb-3 flex flex-wrap items-start justify-between gap-2 text-sm">
                  <div className="min-w-0">
                    <div className="font-medium text-foreground">
                      {t('consoleChat.workflow.approvalPending')}
                    </div>
                    <div className="mt-0.5 text-xs text-muted-foreground">
                      {activeWorkflowApprovalRequest.approvalFormId
                        ? t('consoleChat.workflow.formId', {
                            id: activeWorkflowApprovalRequest.approvalFormId,
                          })
                        : t('consoleChat.workflow.approvalInputLocked')}
                    </div>
                  </div>
                  {activeWorkflowApprovalRequest.approvalUrl ? (
                    <a
                      className="inline-flex shrink-0 items-center gap-1 text-xs text-primary underline-offset-2 hover:underline"
                      href={activeWorkflowApprovalRequest.approvalUrl}
                      target="_blank"
                      rel="noreferrer"
                    >
                      {t('consoleChat.workflow.openApproval')}
                      <ExternalLink className="size-3" />
                    </a>
                  ) : null}
                </div>
                {approvalFormQuery.isLoading ? (
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Loader2 className="size-3.5 animate-spin" />
                    <span>{t('consoleChat.workflow.loadingApprovalForm')}</span>
                  </div>
                ) : approvalForm ? (
                  <ApprovalRuntimeForm
                    form={approvalForm}
                    isSubmitting={approvalSubmitMutation.isPending || isSending}
                    submittedAction={submittedApprovalAction}
                    onSubmit={handleWorkflowApprovalSubmit}
                  />
                ) : (
                  <div className="text-xs text-destructive">
                    {t('consoleChat.workflow.approvalFormLoadFailed')}
                  </div>
                )}
              </div>
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
                        if (isComposingRef.current || isComposingEnterEvent(event)) return;
                        event.preventDefault();
                        handleSubmitCurrentQuestion();
                      }
                    }}
                    onCompositionStart={() => {
                      isComposingRef.current = true;
                    }}
                    onCompositionEnd={() => {
                      isComposingRef.current = false;
                    }}
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
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="rounded-md text-muted-foreground"
                      onClick={handleIgnoreUserInputRequest}
                    >
                      {t('consoleChat.userInputRequest.ignore')}
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
            {!hasActiveWorkflowApprovalRequest && !hasActiveUserInputRequest ? (
              <>
                <AIChatAttachmentStrip
                  attachments={attachments}
                  onRemove={handleRemoveAttachment}
                  onRetry={handleRetryAttachment}
                />
                <Textarea
                  value={input}
                  onChange={event => onInputChange(event.target.value)}
                  onPaste={handlePaste}
                  onCompositionStart={() => {
                    isComposingRef.current = true;
                  }}
                  onCompositionEnd={() => {
                    isComposingRef.current = false;
                  }}
                  onKeyDown={event => {
                    if (event.key === 'Enter' && !event.shiftKey) {
                      if (isComposingRef.current || isComposingEnterEvent(event)) return;
                      if (
                        isSending ||
                        isPreparingSend ||
                        isModelInitializing ||
                        isUploading ||
                        hasUploadError
                      ) {
                        return;
                      }
                      event.preventDefault();
                      void handleSend();
                    }
                  }}
                  placeholder={inputPlaceholder || t('chat.enterCommand')}
                  className="max-h-36 min-h-12 resize-none border-0 bg-transparent px-3 py-2 shadow-none focus-visible:ring-0"
                />
              </>
            ) : null}
            <input
              ref={fileInputRef}
              type="file"
              multiple
              hidden
              accept={inputAccept}
              onChange={event => handleFilesSelected(event, 'document')}
            />
            <input
              ref={imageInputRef}
              type="file"
              multiple
              hidden
              accept={buildFileInputAcceptAttribute(imageExtensions)}
              onChange={event => handleFilesSelected(event, 'image')}
            />
            {!hasActiveWorkflowApprovalRequest ? (
              <AIChatInputToolbar
                modelSelectorValue={modelSelectorValue}
                isModelInitializing={isModelInitializing}
                modelMissing={modelMissing}
                modelCapabilityFilter={modelCapabilityFilter}
                hasImageAttachment={hasImageAttachment}
                isSending={isSending}
                canStop={canStop}
                isUploading={isUploading || isPreparingSend}
                isStopping={isStopping}
                canSend={!hasActiveUserInputRequest && canClickSend}
                canUseImage={canUseImage}
                remainingSlots={remainingSlots}
                attachmentLimit={AICHAT_ATTACHMENT_LIMIT}
                maxSizeMB={maxSizeMB}
                imageMaxSizeMB={imageMaxSizeMB}
                allowedExtensions={allowedExtensions}
                imageExtensions={imageExtensions}
                showModelSelector={showModelSelector}
                showMemoryToggle={showMemoryToggle}
                enableUpload={!hasActiveUserInputRequest && enableUpload}
                showFileLibraryPicker={showFileLibraryPicker}
                surface={surface}
                onModelChange={onModelChange}
                onModelPropsChange={setSelectedModelProps}
                onUploadDocument={() => fileInputRef.current?.click()}
                onUploadImage={handleImageUpload}
                onSelectFromFiles={() => setIsFileSelectorOpen(true)}
                onMemoryEnabledChange={setUseMemory}
                onSend={handleSend}
                onStop={onStop}
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
