import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { generateClientId } from '@/utils/client-id';
import { Button } from '@/components/ui/button';
import { Send, Paperclip, SlidersHorizontal, Square, Upload, FolderOpen } from 'lucide-react';
import {
  useUploadConfig,
  useUploadMultipleWithProgress,
  useSupportedFileTypes,
} from '@/hooks/use-upload';
import type { AttachmentType, ChatAttachment } from '@/components/chat/types';
import { toast } from 'sonner';
import { cn } from '@/lib/utils';
import type { WorkflowFeatures } from '@/components/workflow/store/type';
import {
  buildFileInputAcceptAttribute,
  filterLowercaseExtensions,
  formatExtensionsForDisplay,
  getEffectiveChatUploadExtensions,
  isAllowedUploadExtension,
} from '@/utils/file-helpers';
import { useT } from '@/i18n';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import type {
  FormInputs,
  WorkflowFileUploadAccessMode,
  WorkflowInputFormHandle,
} from '@/components/workflow/common/workflow-input-form';
import { transformFilesToPayload } from '@/components/workflow/common/workflow-input-form';
import type { InputVar } from '@/components/workflow/types/input-var';
import AttachmentsPanel from './attachments-panel';
import ToolbarFormPanel from './toolbar-form-panel';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import dynamic from 'next/dynamic';
import type { FileItem } from '@/services/types/file';
import { useAuthStore } from '@/store/auth-store';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useRouter, usePathname } from 'next/navigation';

const FileSelectorDialog = dynamic(() => import('@/components/files/file-selector-dialog'), {
  ssr: false,
});

interface ToolbarFormSpec {
  variables: InputVar[];
  initialValues?: Record<string, unknown>;
  icon?: React.ReactNode;
  title?: string;
}

interface DraftValue {
  id: number;
  text: string;
}

interface UserInputProps {
  onSend: (payload: {
    query: string;
    files?: ChatAttachment[];
    inputs?: Record<string, unknown>;
  }) => void;
  /** Callback when stop button is clicked */
  onStop?: () => void;
  disabled?: boolean;
  sendDisabled?: boolean;
  /** Whether workflow is currently running - shows stop button instead of send */
  isRunning?: boolean;
  /** Whether stop action is in progress */
  isStopping?: boolean;
  placeholder?: string;
  enableUpload?: boolean;
  uploadFeature?: WorkflowFeatures['file_upload'];
  toolbarForm?: ToolbarFormSpec;
  /** Custom overlay content to show when input is disabled */
  disabledOverlay?: React.ReactNode;
  /** Explicit final whitelist of allowed extensions (skips fetching from server if provided) */
  allowedExtensions?: string[];
  /** Upload access policy used by anonymous webapp surfaces */
  uploadAccessMode?: WorkflowFileUploadAccessMode;
  /** Whether the system file selector should allow switching current workspace */
  allowWorkspaceSwitch?: boolean;
  leftActions?: React.ReactNode;
  topNotice?: React.ReactNode;
  draftValue?: DraftValue | null;
  variant?: 'default' | 'webapp';
}

interface AttachmentUIItem {
  id: string;
  filename: string;
  extension: string;
  progress: number;
  status: 'uploading' | 'uploaded' | 'error';
  error?: string;
  attachment?: ChatAttachment;
}

// Use shared extension constants for consistent filtering

const UserInput: React.FC<UserInputProps> = ({
  onSend,
  onStop,
  disabled,
  sendDisabled,
  isRunning,
  isStopping,
  placeholder,
  enableUpload,
  uploadFeature,
  toolbarForm,
  disabledOverlay,
  allowedExtensions,
  leftActions,
  topNotice,
  uploadAccessMode = 'enabled',
  allowWorkspaceSwitch = false,
  draftValue,
  variant = 'default',
}) => {
  const t = useT();
  const [value, setValue] = useState('');
  const isComposingRef = useRef<boolean>(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const [attachments, setAttachments] = useState<AttachmentUIItem[]>([]);
  const [sessionIds, setSessionIds] = useState<string[]>([]);
  const [isFormOpen, setIsFormOpen] = useState<boolean>(Boolean(toolbarForm));
  const didAutoOpenRef = useRef<boolean>(false);
  const firstSendClosedFormRef = useRef<boolean>(false);
  const floatingPanelsRef = useRef<HTMLDivElement | null>(null);
  const [floatingPanelsHeight, setFloatingPanelsHeight] = useState(0);

  // System file selector state
  const [isSystemSelectOpen, setIsSystemSelectOpen] = useState(false);
  const [loginDialogOpen, setLoginDialogOpen] = useState(false);
  const [loginDialogVariant, setLoginDialogVariant] = useState<'system-select' | 'upload'>(
    'system-select'
  );
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const router = useRouter();
  const pathname = usePathname();
  const isUploadLoginRequired = uploadAccessMode === 'login-required';

  useEffect(() => {
    // Auto-open only once when a toolbar form first becomes available
    if (toolbarForm && !didAutoOpenRef.current) {
      setIsFormOpen(true);
      didAutoOpenRef.current = true;
    }
  }, [toolbarForm]);

  // Compute initial form values from variable defaults (mirrors WorkflowInputForm logic)
  const computedInitialValues = useMemo<Record<string, unknown>>(() => {
    if (!toolbarForm?.variables) return {};
    const result: Record<string, unknown> = {};
    for (const v of toolbarForm.variables) {
      switch (v.type) {
        case 'checkbox':
          result[v.variable] = typeof v.default === 'boolean' ? v.default : false;
          break;
        case 'number': {
          const num = typeof v.default === 'string' ? Number(v.default) : undefined;
          result[v.variable] = Number.isFinite(num) ? num : undefined;
          break;
        }
        case 'file':
          result[v.variable] = undefined;
          break;
        case 'file-list':
          result[v.variable] = [];
          break;
        default:
          result[v.variable] = typeof v.default === 'string' ? v.default : '';
      }
    }
    return { ...result, ...(toolbarForm.initialValues ?? {}) };
  }, [toolbarForm?.variables, toolbarForm?.initialValues]);

  // Local form values managed inside UserInput
  const [formValues, setFormValues] = useState<Record<string, unknown>>(computedInitialValues);

  // Sync when computed values change (e.g., variables schema changes)
  useEffect(() => {
    setFormValues(prev => {
      const prevKeys = Object.keys(prev);
      const nextKeys = Object.keys(computedInitialValues);
      if (prevKeys.length !== nextKeys.length) return computedInitialValues;
      const same = nextKeys.every(key => {
        const a = prev[key];
        const b = computedInitialValues[key];
        if (Array.isArray(a) && Array.isArray(b)) {
          if (a.length !== b.length) return false;
          return a.every((item, index) => item === b[index]);
        }
        return a === b;
      });
      return same ? prev : computedInitialValues;
    });
  }, [computedInitialValues]);

  const handleFormValuesChange = useCallback((vals: FormInputs) => {
    setFormValues(vals as unknown as Record<string, unknown>);
  }, []);

  const appendErrorAttachments = useCallback(
    (items: Array<{ filename: string; extension?: string; error: string }>) => {
      if (items.length === 0) return;

      setAttachments(prev => {
        const existingErrorKeys = new Set(
          prev
            .filter(item => item.status === 'error')
            .map(item => `${item.filename}::${item.extension}::${item.error ?? ''}`)
        );

        const nextErrorItems: AttachmentUIItem[] = [];

        items.forEach((item, index) => {
          const normalizedExtension = (item.extension ?? item.filename.split('.').pop() ?? '')
            .toLowerCase()
            .replace(/^\./, '');
          const key = `${item.filename}::${normalizedExtension}::${item.error}`;
          if (existingErrorKeys.has(key)) {
            return;
          }

          existingErrorKeys.add(key);
          nextErrorItems.push({
            id: `error-${Date.now()}-${index}-${normalizedExtension || 'unknown'}`,
            filename: item.filename,
            extension: normalizedExtension,
            progress: 100,
            status: 'error',
            error: item.error,
          });
        });

        if (nextErrorItems.length === 0) return prev;
        return [...prev, ...nextErrorItems];
      });
    },
    []
  );

  const hasAllowedExtensionsOverride = allowedExtensions !== undefined;
  // Only fetch supported types if an explicit override is NOT provided
  const shouldFetchSupportedTypes =
    enableUpload === true && !hasAllowedExtensionsOverride && !isUploadLoginRequired;
  const { supportedTypes } = useSupportedFileTypes({ enabled: shouldFetchSupportedTypes });
  const { data: uploadConfig } = useUploadConfig({
    enabled: enableUpload === true && !isUploadLoginRequired,
  });
  const maxSizeMB = uploadConfig?.file_size_limit ?? 15;
  const supportedExtensionsFromServer = useMemo<string[]>(
    () =>
      Array.isArray(supportedTypes.allowed_extensions) ? supportedTypes.allowed_extensions : [],
    [supportedTypes.allowed_extensions]
  );
  const effectiveAllowedExtensions = useMemo<string[]>(() => {
    if (hasAllowedExtensionsOverride) {
      return filterLowercaseExtensions(allowedExtensions ?? []);
    }

    if (!uploadFeature) {
      return filterLowercaseExtensions(supportedExtensionsFromServer);
    }

    return getEffectiveChatUploadExtensions(
      uploadFeature.allowed_file_types ?? [],
      uploadFeature.allowed_file_extensions ?? [],
      supportedExtensionsFromServer
    );
  }, [
    allowedExtensions,
    hasAllowedExtensionsOverride,
    supportedExtensionsFromServer,
    uploadFeature,
  ]);
  const hasExtensionRestrictions = hasAllowedExtensionsOverride || Boolean(uploadFeature);
  const inputAccept = useMemo(
    () => buildFileInputAcceptAttribute(effectiveAllowedExtensions),
    [effectiveAllowedExtensions]
  );
  const invalidExtensionReason = useMemo(() => {
    if (effectiveAllowedExtensions.length === 0) {
      return t('ui.fileUpload.invalidExt');
    }

    return `${t('ui.fileUpload.invalidExt')} ${t('ui.fileUpload.allowedTypes', {
      types: formatExtensionsForDisplay(effectiveAllowedExtensions).join(', '),
    })}`;
  }, [effectiveAllowedExtensions, t]);
  const totalLimit = useMemo(() => {
    const configuredLimit = uploadConfig?.batch_count_limit ?? Number.POSITIVE_INFINITY;
    const featureLimit = uploadFeature?.number_limits ?? Number.POSITIVE_INFINITY;
    return Math.max(1, Math.min(configuredLimit, featureLimit));
  }, [uploadConfig?.batch_count_limit, uploadFeature?.number_limits]);
  const remainingSlots = useMemo(
    () =>
      Number.isFinite(totalLimit)
        ? Math.max(0, totalLimit - attachments.length)
        : Number.POSITIVE_INFINITY,
    [attachments.length, totalLimit]
  );
  const isFileAllowed = useCallback(
    (extension: string | null | undefined) => {
      if (!hasExtensionRestrictions && effectiveAllowedExtensions.length === 0) {
        return true;
      }

      return isAllowedUploadExtension(extension, effectiveAllowedExtensions);
    },
    [effectiveAllowedExtensions, hasExtensionRestrictions]
  );
  const {
    upload,
    isUploading,
    error: uploadError,
    response,
    progressMap,
  } = useUploadMultipleWithProgress();

  const formRef = useRef<WorkflowInputFormHandle>(null);
  const isQueryEmpty = value.trim().length === 0;

  useEffect(() => {
    if (!draftValue) return;
    const nextValue = draftValue.text.trim();
    if (!nextValue) return;

    setValue(nextValue);
    const frame = requestAnimationFrame(() => {
      const textarea = textareaRef.current;
      if (!textarea || textarea.disabled) return;
      textarea.focus();
      textarea.setSelectionRange(nextValue.length, nextValue.length);
    });

    return () => cancelAnimationFrame(frame);
  }, [draftValue]);

  const handleSend = useCallback(async () => {
    const trimmed = value.trim();
    if (!trimmed) {
      return;
    }
    // Prevent sending while uploading to guarantee valid file ids
    if (isUploading) return;
    // Block only sending while running; keep other controls active
    if (sendDisabled) return;

    // Validate toolbar form via ref if present
    if (toolbarForm && (toolbarForm.variables?.length ?? 0) > 0) {
      if (formRef.current) {
        const isValid = await formRef.current.validate();
        if (!isValid) {
          toast.error(t('agents.workflow.startForm.invalidInputs'));
          // Ensure form is open to see errors
          if (!isFormOpen) setIsFormOpen(true);
          return;
        }
      } else {
        // Fallback for case where ref might be missing (shouldn't happen with toolbarForm)
        const inputs = formValues ?? {};
        for (const v of toolbarForm.variables) {
          if (!v.required) continue;
          const val = (inputs as Record<string, unknown>)[v.variable];
          let invalid = false;
          switch (v.type) {
            case 'text-input':
            case 'paragraph':
            case 'select':
              if (typeof val !== 'string' || val.trim().length === 0) invalid = true;
              break;
            case 'number':
              if (typeof val !== 'number' || Number.isNaN(val)) invalid = true;
              break;
            case 'checkbox':
              if (typeof val !== 'boolean') invalid = true;
              break;
            case 'file':
              if (val === null || val === undefined) invalid = true;
              break;
            case 'file-list':
              if (!Array.isArray(val) || (val as unknown[]).length === 0) invalid = true;
              break;
            default:
              break;
          }
          if (invalid) {
            toast.error(t('agents.workflow.startForm.invalidInputs'));
            if (!isFormOpen) setIsFormOpen(true);
            return;
          }
        }
      }
    }

    const files: ChatAttachment[] = attachments
      .filter(a => a.status === 'uploaded' && a.attachment)
      .map(a => a.attachment as ChatAttachment);
    const inputs = toolbarForm
      ? transformFilesToPayload(formValues, toolbarForm.variables)
      : undefined;
    onSend({ query: trimmed, files: files.length > 0 ? files : undefined, inputs });
    // Clear input and attachments after send to reset UI state
    setValue('');
    setAttachments([]);
    setSessionIds([]);
    if (fileInputRef.current) {
      // Ensure native file input is also cleared
      fileInputRef.current.value = '';
    }
    // Close the toolbar form only on the first successful send in this component lifecycle
    if (toolbarForm && isFormOpen && !firstSendClosedFormRef.current) {
      setIsFormOpen(false);
      firstSendClosedFormRef.current = true;
    }
  }, [
    onSend,
    value,
    attachments,
    isUploading,
    sendDisabled,
    toolbarForm,
    formValues,
    isFormOpen,
    t,
  ]);

  const pickFiles = useCallback(() => {
    if (!enableUpload && !isUploadLoginRequired) return;
    if (isUploadLoginRequired) {
      setLoginDialogVariant('upload');
      setLoginDialogOpen(true);
      return;
    }
    const el = fileInputRef.current;
    if (!el) return;
    el.click();
  }, [enableUpload, isUploadLoginRequired]);

  const handleSystemSelect = useCallback(() => {
    if (!isAuthenticated) {
      setLoginDialogVariant('system-select');
      setLoginDialogOpen(true);
      return;
    }
    setIsSystemSelectOpen(true);
  }, [isAuthenticated]);

  const handleLoginConfirm = useCallback(() => {
    setLoginDialogOpen(false);
    const currentSearch = window.location.search;
    const currentUrl = currentSearch ? `${pathname}${currentSearch}` : pathname;
    router.push(`/login?redirect=${encodeURIComponent(currentUrl)}`);
  }, [pathname, router]);

  const handleSystemSelectConfirm = useCallback(
    (files: FileItem[]) => {
      const exceedCountReason = t('ui.fileUpload.exceedCount', { max: totalLimit });
      const disallowedReason = invalidExtensionReason;

      if (remainingSlots <= 0) {
        toast.error(exceedCountReason);
        appendErrorAttachments(
          files.map(file => ({
            filename: file.name,
            extension: file.extension,
            error: exceedCountReason,
          }))
        );
        return;
      }

      const allowedFiles = files.filter(file => isFileAllowed(file.extension));
      if (allowedFiles.length < files.length) {
        toast.error(disallowedReason);
        appendErrorAttachments(
          files
            .filter(file => !isFileAllowed(file.extension))
            .map(file => ({
              filename: file.name,
              extension: file.extension,
              error: disallowedReason,
            }))
        );
      }
      if (allowedFiles.length === 0) return;

      const limitedFiles = Number.isFinite(remainingSlots)
        ? allowedFiles.slice(0, remainingSlots)
        : allowedFiles;
      if (limitedFiles.length < allowedFiles.length) {
        toast.error(t('ui.fileUpload.partialDueToLimit', { count: limitedFiles.length }));
        appendErrorAttachments(
          allowedFiles.slice(limitedFiles.length).map(file => ({
            filename: file.name,
            extension: file.extension,
            error: exceedCountReason,
          }))
        );
      }

      const newItems: AttachmentUIItem[] = limitedFiles.map(file => ({
        id: file.id,
        filename: file.name,
        extension: file.extension,
        progress: 100,
        status: 'uploaded' as const,
        attachment: {
          type: file.mime_type.split('/')[0] as AttachmentType,
          transfer_method: 'local_file',
          url: file.source_url,
          upload_file_id: file.id,
        },
      }));

      setAttachments(prev => {
        const existingIds = new Set(prev.map(p => p.attachment?.upload_file_id).filter(Boolean));
        const uniqueNewItems = newItems.filter(
          item =>
            item.attachment?.upload_file_id && !existingIds.has(item.attachment.upload_file_id)
        );
        return [...prev, ...uniqueNewItems];
      });
    },
    [appendErrorAttachments, invalidExtensionReason, isFileAllowed, remainingSlots, t, totalLimit]
  );

  const handleFilesSelected = useCallback(
    (evt: React.ChangeEvent<HTMLInputElement>) => {
      const filesList = evt.target.files;
      if (!filesList || filesList.length === 0) return;
      const files: File[] = Array.from(filesList);
      const exceedCountReason = t('ui.fileUpload.exceedCount', { max: totalLimit });
      const disallowedReason = invalidExtensionReason;
      const tooLargeReason = t('ui.fileUpload.tooLarge', { max: maxSizeMB });
      if (remainingSlots <= 0) {
        toast.error(exceedCountReason);
        appendErrorAttachments(
          files.map(file => ({
            filename: file.name,
            error: exceedCountReason,
          }))
        );
        // Clear input value to avoid stale selection
        if (fileInputRef.current) fileInputRef.current.value = '';
        return;
      }

      const allowedFiles = files.filter(file => isFileAllowed(file.name.split('.').pop()));

      if (allowedFiles.length < files.length) {
        toast.error(disallowedReason);
        appendErrorAttachments(
          files
            .filter(file => !isFileAllowed(file.name.split('.').pop()))
            .map(file => ({
              filename: file.name,
              error: disallowedReason,
            }))
        );
      }
      if (allowedFiles.length === 0) {
        // Clear input to allow reselection
        if (fileInputRef.current) fileInputRef.current.value = '';
        return;
      }

      const sizeAllowedFiles = allowedFiles.filter(file => file.size <= maxSizeMB * 1024 * 1024);
      if (sizeAllowedFiles.length < allowedFiles.length) {
        toast.error(tooLargeReason);
        appendErrorAttachments(
          allowedFiles
            .filter(file => file.size > maxSizeMB * 1024 * 1024)
            .map(file => ({
              filename: file.name,
              error: tooLargeReason,
            }))
        );
      }
      if (sizeAllowedFiles.length === 0) {
        if (fileInputRef.current) fileInputRef.current.value = '';
        return;
      }

      const toUse = Number.isFinite(remainingSlots)
        ? sizeAllowedFiles.slice(0, Math.max(0, remainingSlots))
        : sizeAllowedFiles;
      if (toUse.length < sizeAllowedFiles.length) {
        toast.error(t('ui.fileUpload.partialDueToLimit', { count: toUse.length }));
        appendErrorAttachments(
          sizeAllowedFiles.slice(toUse.length).map(file => ({
            filename: file.name,
            error: exceedCountReason,
          }))
        );
      }
      const ids = toUse.map(() => generateClientId('attachment'));
      setSessionIds(ids);
      setAttachments(prev => {
        const newItems = toUse.map((f, idx) => ({
          id: ids[idx] ?? `${Date.now()}-${idx}`,
          filename: f.name,
          extension: (f.name.split('.').pop() || '').toLowerCase(),
          progress: 0,
          status: 'uploading' as const,
        }));
        return [...prev, ...newItems];
      });
      // Start upload session; progress will update via progressMap
      void upload(toUse, { is_temporary: true });
      // Clear input value so the same file can be selected again
      if (fileInputRef.current) fileInputRef.current.value = '';
    },
    [
      appendErrorAttachments,
      invalidExtensionReason,
      isFileAllowed,
      maxSizeMB,
      remainingSlots,
      t,
      totalLimit,
      upload,
    ]
  );

  // Reflect per-file progress based on current session
  useEffect(() => {
    if (sessionIds.length === 0) return;
    setAttachments(prev =>
      prev.map(item => {
        const idx = sessionIds.indexOf(item.id);
        if (idx === -1) return item;
        const p = progressMap[idx];
        return typeof p === 'number' ? { ...item, progress: p } : item;
      })
    );
  }, [progressMap, sessionIds]);

  // When upload completes, reflect status and attach server file info
  useEffect(() => {
    if (!response || sessionIds.length === 0) return;
    const idsSnapshot = [...sessionIds];
    const filesRes = response.files ?? [];
    // Success is determined by presence of server id; progress is not used as gate
    setAttachments(prev =>
      prev.map(item => {
        const idx = idsSnapshot.indexOf(item.id);
        if (idx === -1) return item;
        const info = filesRes[idx];
        if (!info || !info.id) return item;
        const attachment: ChatAttachment = {
          type: info.mime_type.split('/')[0] as AttachmentType,
          transfer_method: 'local_file',
          url: info.url ?? '',
          upload_file_id: info.id,
        };
        return { ...item, status: 'uploaded', progress: 100, attachment };
      })
    );
    // Clear current session markers immediately after applying success
    setSessionIds([]);
  }, [response, sessionIds]);

  // Reflect error state if upload fails
  useEffect(() => {
    if (!uploadError || sessionIds.length === 0) return;
    const idsSnapshot = [...sessionIds];
    setAttachments(prev =>
      prev.map(item =>
        idsSnapshot.includes(item.id)
          ? { ...item, status: 'error', error: uploadError, progress: 100 }
          : item
      )
    );
    setSessionIds([]);
  }, [uploadError, sessionIds]);

  const handleRemove = useCallback((id: string) => {
    setAttachments(prev => prev.filter(x => x.id !== id));
  }, []);

  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;

    textarea.style.height = 'auto';
    const maxHeight = variant === 'webapp' ? 144 : 160;
    textarea.style.height = `${Math.min(textarea.scrollHeight, maxHeight)}px`;
  }, [value, variant]);

  const hasUploadAction = Boolean(enableUpload);
  const useInlineUploadAction = variant === 'webapp' && !toolbarForm && !leftActions;

  const renderUploadAction = useCallback(
    (buttonClassName?: string) => {
      if (!hasUploadAction) return null;

      const buttonClasses = cn(
        'w-7 h-7 hover:text-highlight hover:bg-highlight/10',
        buttonClassName
      );

      if (isUploadLoginRequired) {
        return (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                isIcon
                variant="ghost"
                disabled={disabled}
                onClick={() => {
                  setLoginDialogVariant('upload');
                  setLoginDialogOpen(true);
                }}
                className={buttonClasses}
                aria-label={t('ui.fileUpload.uploadAria')}
              >
                <Paperclip size={16} />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top">{t('ui.fileUpload.uploadAria')}</TooltipContent>
          </Tooltip>
        );
      }

      return (
        <DropdownMenu>
          <Tooltip>
            <TooltipTrigger asChild>
              <DropdownMenuTrigger asChild>
                <Button
                  isIcon
                  variant="ghost"
                  disabled={disabled || isUploading}
                  className={buttonClasses}
                  aria-label={t('ui.fileUpload.uploadAria')}
                >
                  <Paperclip size={16} />
                </Button>
              </DropdownMenuTrigger>
            </TooltipTrigger>
            <TooltipContent side="top">{t('ui.fileUpload.uploadAria')}</TooltipContent>
          </Tooltip>
          <DropdownMenuContent align="start">
            <DropdownMenuItem onClick={pickFiles}>
              <Upload className="mr-2 h-4 w-4" />
              {t('ui.fileUpload.uploadFilesLabel')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handleSystemSelect}>
              <FolderOpen className="mr-2 h-4 w-4" />
              {t('ui.fileUpload.selectFromSystem')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      );
    },
    [
      disabled,
      handleSystemSelect,
      hasUploadAction,
      isUploadLoginRequired,
      isUploading,
      pickFiles,
      t,
    ]
  );

  // Show toolbar when at least one feature is enabled
  const toolbarVisible = useMemo<boolean>(() => {
    return Boolean((hasUploadAction && !useInlineUploadAction) || toolbarForm || leftActions);
  }, [hasUploadAction, leftActions, toolbarForm, useInlineUploadAction]);

  const shouldRenderFloatingPanels = useMemo<boolean>(() => {
    return attachments.length > 0 || Boolean(toolbarForm);
  }, [attachments.length, toolbarForm]);

  useEffect(() => {
    const element = floatingPanelsRef.current;

    if (!element) {
      setFloatingPanelsHeight(0);
      return;
    }

    const updateHeight = () => {
      setFloatingPanelsHeight(element.getBoundingClientRect().height);
    };

    updateHeight();

    if (typeof ResizeObserver === 'undefined') {
      return;
    }

    const observer = new ResizeObserver(() => {
      updateHeight();
    });

    observer.observe(element);

    return () => {
      observer.disconnect();
    };
  }, [shouldRenderFloatingPanels, attachments.length, toolbarForm, isFormOpen]);

  const noticeFloatingOffset = useMemo(() => {
    if (floatingPanelsHeight <= 0) {
      return 8;
    }

    return floatingPanelsHeight + 14;
  }, [floatingPanelsHeight]);

  const containerStyle = useMemo<React.CSSProperties>(
    () =>
      ({
        '--workflow-precheck-floating-offset': `${noticeFloatingOffset}px`,
      }) as React.CSSProperties,
    [noticeFloatingOffset]
  );

  return (
    <div
      className={cn(
        'w-full relative rounded-lg border shadow-sm',
        variant === 'webapp' ? 'bg-background' : 'bg-muted'
      )}
      style={containerStyle}
    >
      {topNotice}

      {/* Disabled overlay */}
      {disabled && disabledOverlay && (
        <div className="absolute inset-0 z-10 flex items-center justify-center rounded-lg bg-background/80 backdrop-blur-sm">
          {disabledOverlay}
        </div>
      )}

      {/* Floating panels above input: form (kept mounted) and attachments */}
      {shouldRenderFloatingPanels ? (
        <div ref={floatingPanelsRef} className="absolute bottom-full left-0 right-0 mb-1 space-y-2">
          {attachments.length > 0 && (
            <AttachmentsPanel
              items={attachments.map(a => ({
                id: a.id,
                filename: a.filename,
                extension: a.extension,
                progress: a.progress,
                status: a.status,
                error: a.error,
              }))}
              isUploading={isUploading}
              sessionIds={sessionIds}
              onRemove={handleRemove}
            />
          )}
          {toolbarForm && (
            <ToolbarFormPanel
              ref={formRef}
              toolbarForm={toolbarForm}
              isOpen={isFormOpen}
              onValuesChange={handleFormValuesChange}
              handleClose={() => setIsFormOpen(false)}
              fileUploadAccessMode={uploadAccessMode}
              allowWorkspaceSwitch={allowWorkspaceSwitch}
            />
          )}
        </div>
      ) : null}

      {/* Header toolbar */}
      {toolbarVisible && (
        <div className="flex items-center gap-1 px-2 h-9 border-b border-primary/10">
          {toolbarForm && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  isIcon
                  variant="ghost"
                  onClick={() => setIsFormOpen(prev => !prev)}
                  aria-pressed={isFormOpen}
                  className={cn(
                    'w-7 h-7 hover:text-highlight hover:bg-highlight/10',
                    isFormOpen ? 'bg-highlight/10 text-highlight' : undefined
                  )}
                  aria-label={
                    typeof toolbarForm.title === 'string' ? toolbarForm.title : 'Open Form'
                  }
                >
                  {toolbarForm.icon ?? <SlidersHorizontal size={16} />}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">
                {typeof toolbarForm.title === 'string' ? toolbarForm.title : 'Form'}
              </TooltipContent>
            </Tooltip>
          )}
          {!useInlineUploadAction && renderUploadAction()}
          {leftActions}
        </div>
      )}
      {/* Input and Action Row */}
      <div
        className={cn(
          'flex items-end gap-2 p-2',
          variant === 'webapp' ? 'min-h-[56px]' : 'min-h-[60px]'
        )}
      >
        {useInlineUploadAction ? (
          <div className="pb-1">
            {renderUploadAction(
              'h-9 w-9 rounded-md border border-border bg-background hover:bg-accent'
            )}
          </div>
        ) : null}
        <textarea
          ref={textareaRef}
          value={value}
          onChange={e => setValue(e.target.value)}
          placeholder={placeholder ?? ''}
          rows={1}
          className={cn(
            'flex-1 resize-none border-none bg-transparent text-sm focus:ring-0 scrollbar-hidden',
            variant === 'webapp'
              ? 'min-h-10 max-h-36 py-2 placeholder:text-muted-foreground/80'
              : 'min-h-[60px] max-h-40'
          )}
          disabled={disabled}
          onCompositionStart={() => {
            isComposingRef.current = true;
          }}
          onCompositionEnd={() => {
            isComposingRef.current = false;
          }}
          onKeyDown={e => {
            const native: unknown = (e as unknown as { nativeEvent?: unknown })?.nativeEvent;
            const isComposing =
              (native &&
                typeof native === 'object' &&
                (native as { isComposing?: boolean }).isComposing) ||
              isComposingRef.current ||
              (native &&
                typeof native === 'object' &&
                (native as { keyCode?: number }).keyCode === 229);
            if (e.key === 'Enter' && !e.shiftKey) {
              if (isComposing) return;
              e.preventDefault();
              handleSend();
            }
          }}
        />

        <div className="flex shrink-0">
          {!isRunning && (
            <Button
              isIcon
              disabled={disabled || isUploading || Boolean(sendDisabled) || isQueryEmpty}
              onClick={handleSend}
              className="rounded-full w-8 h-8 flex items-center justify-center shadow-lg"
              aria-label={t('webapp.chat.sendMessage')}
            >
              <Send size={18} className="text-primary-foreground" />
            </Button>
          )}
          {isRunning && (
            <Button
              isIcon
              variant="destructive"
              disabled={isStopping}
              onClick={onStop}
              className="rounded-full w-8 h-8 flex items-center justify-center shadow-lg"
              aria-label={t('webapp.chat.stopResponse')}
            >
              <Square size={14} className="text-primary-foreground fill-current" />
            </Button>
          )}
        </div>
      </div>
      {/* Hidden input for file selection */}
      {enableUpload && !isUploadLoginRequired && (
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept={inputAccept}
          className="hidden"
          onChange={handleFilesSelected}
        />
      )}

      {/* System File Selector Dialog */}
      {isSystemSelectOpen && (
        <FileSelectorDialog
          open={isSystemSelectOpen}
          onOpenChange={setIsSystemSelectOpen}
          onConfirm={handleSystemSelectConfirm}
          maxCount={Number.isFinite(remainingSlots) ? remainingSlots : undefined}
          acceptExt={effectiveAllowedExtensions}
          allowWorkspaceSwitch={allowWorkspaceSwitch}
        />
      )}

      {/* Login Confirm Dialog */}
      <ConfirmDialog
        open={loginDialogOpen}
        onOpenChange={setLoginDialogOpen}
        title={t('ui.fileUpload.loginRequiredTitle')}
        description={
          loginDialogVariant === 'upload'
            ? t('ui.fileUpload.loginRequiredForUploadDescription')
            : t('ui.fileUpload.loginRequiredDescription')
        }
        confirmText={t('ui.fileUpload.goToLogin')}
        cancelText={t('ui.fileUpload.cancelAction')}
        onConfirm={handleLoginConfirm}
      />
    </div>
  );
};

export default UserInput;
