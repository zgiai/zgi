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
import { Textarea } from '@/components/ui/textarea';
import { useUploadConfig } from '@/hooks/use-upload';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { uploadService } from '@/services/upload.service';
import type { FileItem } from '@/services/types/file';
import type { AIChatMessageFile } from '@/services/types/aichat';
import {
  IMAGE_EXTENSIONS,
  buildFileInputAcceptAttribute,
  filterLowercaseExtensions,
  formatExtensionsForDisplay,
} from '@/utils/file-helpers';
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
  toAIChatMessageFile,
} from '@/components/chat/variants/aichat/input-area-utils';
import type { AIChatModelValue } from '@/components/chat/variants/aichat/types';

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

function isComposingEnterEvent(event: KeyboardEvent<HTMLTextAreaElement>): boolean {
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
  modelMissing: boolean;
  isSending: boolean;
  isStopping: boolean;
  onInputChange: (value: string) => void;
  onSend: (files: AIChatMessageFile[], useMemory: boolean) => void;
  onStop: () => void;
  onModelChange: (value: ModelSelectorValue) => void;
  onHeightChange?: (height: number) => void;
  showModelSelector?: boolean;
  showMemoryToggle?: boolean;
  enableUpload?: boolean;
  uploadScope?: AIChatUploadScope;
  showFileLibraryPicker?: boolean;
  inputPlaceholder?: string;
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
  modelMissing,
  isSending,
  isStopping,
  onInputChange,
  onSend,
  onStop,
  onModelChange,
  onHeightChange,
  showModelSelector = true,
  showMemoryToggle = true,
  enableUpload = true,
  uploadScope = { type: 'console' },
  showFileLibraryPicker = true,
  inputPlaceholder,
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
  const canUseImage = isVisionModel(selectedModelProps);
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
  const canClickSend = Boolean(input.trim()) && !modelMissing && !isUploading && !hasUploadError;

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
        return t('consoleChat.attachments.imageVisionRequired');
      }
      return null;
    },
    [allowedExtensions, canUseImage, imageExtensions, imageMaxSizeMB, maxSizeMB, t]
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
      toast.info(t('consoleChat.attachments.imageVisionRequired'));
      return;
    }
    imageInputRef.current?.click();
  }, [canUseImage, t]);

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
            toast.error(t('consoleChat.attachments.imageVisionRequired'));
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
    [allSelectableExtensions, allowedExtensions, canUseImage, imageExtensions, t]
  );

  const handleSend = useCallback(() => {
    if (!input.trim() || isUploading || hasUploadError) return;
    onSend(uploadedFiles, useMemory);
    setAttachments([]);
  }, [hasUploadError, input, isUploading, onSend, uploadedFiles, useMemory]);

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
      {enableUpload && isDraggingFiles ? (
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
            ? 'top-[58%] -translate-y-1/2 pb-0 pt-0 sm:top-1/2'
            : 'top-full -translate-y-full bg-background pb-1 shadow-[0_-18px_36px_hsl(var(--background))]'
        )}
      >
        <div
          className={cn(
            'pointer-events-auto mx-auto w-full transition-[max-width] duration-300 ease-in-out',
            isHome && !isLoadingMessages ? 'max-w-3xl' : 'max-w-4xl'
          )}
        >
          {modelMissing ? (
            <div className="mb-2 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {t('consoleChat.modelRequired')}
            </div>
          ) : null}
          <div className="rounded-2xl border bg-background p-2 shadow-sm focus-within:border-primary/40">
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
                  if (isSending || isUploading || hasUploadError) return;
                  event.preventDefault();
                  handleSend();
                }
              }}
              placeholder={inputPlaceholder || t('chat.enterCommand')}
              className="max-h-36 min-h-12 resize-none border-0 bg-transparent px-3 py-2 shadow-none focus-visible:ring-0"
            />
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
            <AIChatInputToolbar
              modelSelectorValue={modelSelectorValue}
              modelMissing={modelMissing}
              modelCapabilityFilter={modelCapabilityFilter}
              hasImageAttachment={hasImageAttachment}
              isSending={isSending}
              isUploading={isUploading}
              isStopping={isStopping}
              canSend={canClickSend}
              canUseImage={canUseImage}
              remainingSlots={remainingSlots}
              attachmentLimit={AICHAT_ATTACHMENT_LIMIT}
              maxSizeMB={maxSizeMB}
              imageMaxSizeMB={imageMaxSizeMB}
              allowedExtensions={allowedExtensions}
              imageExtensions={imageExtensions}
              showModelSelector={showModelSelector}
              showMemoryToggle={showMemoryToggle}
              enableUpload={enableUpload}
              showFileLibraryPicker={showFileLibraryPicker}
              onModelChange={onModelChange}
              onModelPropsChange={setSelectedModelProps}
              onUploadDocument={() => fileInputRef.current?.click()}
              onUploadImage={handleImageUpload}
              onSelectFromFiles={() => setIsFileSelectorOpen(true)}
              onMemoryEnabledChange={setUseMemory}
              onSend={handleSend}
              onStop={onStop}
            />
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
            />
          ) : null}
        </div>
      </div>
    </>
  );
}
