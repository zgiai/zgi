'use client';

import React, {
  useCallback,
  useRef,
  useState,
  useEffect,
  forwardRef,
  useImperativeHandle,
} from 'react';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { uploadService } from '@/services';
import { UploadCloudIcon } from 'lucide-react';
import type { UploadedFile } from '@/services/types/dataset';
import {
  buildFileInputAcceptAttribute,
  filterLowercaseExtensions,
  formatExtensionsForDisplay,
} from '@/utils/file-helpers';
import { generateClientId } from '@/utils/client-id';
import { FileList } from '@/components/common/file-upload/file-list';
import type { FileItem } from '@/services/types/file';
import { Button } from '@/components/ui/button';
import { useRouter, usePathname } from 'next/navigation';
import { useAuthStore } from '@/store/auth-store';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import dynamic from 'next/dynamic';

const FileSelectorDialog = dynamic(() => import('@/components/files/file-selector-dialog'), {
  ssr: false,
});

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface AutoFileUploadProps {
  /** Maximum number of files allowed in total */
  maxCount?: number;
  /** Max single file size (MB) */
  maxSizeMB?: number;
  /** Allowed extensions like ['.jpg', '.png'] (case-insensitive) */
  acceptExt?: string[];
  /** Additional class for outer container */
  containerClassName?: string;
  /** Additional class for table wrapper */
  tableWrapperClassName?: string;
  /** Additional class for drop zone */
  dropZoneClassName?: string;
  /** Custom drop content (defaults to text) */
  dropContent?: React.ReactNode;
  /** Callback when uploaded files list changes (only successful ones) */
  onChange?: (files: UploadedFile[]) => void;
  /** External value for controlled mode - list of already uploaded files */
  value?: UploadedFile[];
  /** Whether the component is in controlled mode */
  controlled?: boolean;
  /** Folder ID to upload files to */
  folderId?: string;
  /** Workspace id */
  workspaceId?: string;
  /** Whether to show the system file selector button */
  showSystemSelect?: boolean;
  /** Whether to mark files as temporary */
  isTemporary?: boolean;
  /** Whether the system file selector should allow switching current workspace */
  allowWorkspaceSwitch?: boolean;
}

export interface AutoFileUploadRef {
  /** Manually trigger upload for all pending files */
  uploadAll: () => Promise<UploadedFile[]>;
  /** Get all pending files (not yet uploaded) */
  getPendingFiles: () => File[];
  /** Get all successfully uploaded files */
  getUploadedFiles: () => UploadedFile[];
  /** Clear all files */
  clearAll: () => void;
}

interface UploadItem {
  id: string;
  file: File;
  progress: number; // 0-100
  status: 'pending' | 'uploading' | 'success' | 'error';
  serverFile?: UploadedFile;
  controller?: AbortController;
  errorMsg?: string;
}

const genId = () => generateClientId('upload');

/* -------------------------------------------------------------------------- */
/* Component                                                                   */
/* -------------------------------------------------------------------------- */

export const AutoFileUpload = forwardRef<AutoFileUploadRef, AutoFileUploadProps>(
  (
    {
      maxCount = 5,
      maxSizeMB = 15,
      acceptExt = [],
      containerClassName,
      tableWrapperClassName,
      dropZoneClassName,
      dropContent,
      onChange,
      value = [],
      controlled = false,
      folderId,
      workspaceId,
      showSystemSelect = false,
      isTemporary = false,
      allowWorkspaceSwitch = false,
    },
    ref
  ) => {
    // Internationalization
    const t = useT('ui');
    const router = useRouter();
    const pathname = usePathname();
    const isAuthenticated = useAuthStore.use.isAuthenticated();

    const [items, setItems] = useState<UploadItem[]>([]);
    const isFull = items.length >= maxCount;
    const [isSystemSelectOpen, setIsSystemSelectOpen] = useState(false);
    const [isLoginDialogOpen, setIsLoginDialogOpen] = useState(false);
    const inputAccept = buildFileInputAcceptAttribute(acceptExt);

    const handleSystemSelectConfirm = useCallback((files: FileItem[]) => {
      const newItems: UploadItem[] = files.map(file => ({
        id: file.id,
        file: new File([], file.name, { type: file.mime_type }),
        progress: 100,
        status: 'success' as const,
        serverFile: {
          id: file.id,
          name: file.name,
          size: file.size,
          extension: file.extension,
          mime_type: file.mime_type,
          hash: file.hash,
          created_by: file.created_by,
          created_at: file.created_at,
          url: file.source_url,
        },
      }));

      setItems(prev => {
        const existingIds = new Set(
          prev.map(p => p.serverFile?.id).filter((id): id is string => !!id)
        );
        const uniqueNewItems = newItems.filter(
          item => item.serverFile?.id && !existingIds.has(item.serverFile.id)
        );
        return [...prev, ...uniqueNewItems];
      });
    }, []);

    // Keep latest onChange in ref to avoid effect dependency loop
    const onChangeRef = useRef<typeof onChange>();
    useEffect(() => {
      onChangeRef.current = onChange;
    }, [onChange]);

    // Expose methods to parent via ref
    useImperativeHandle(
      ref,
      () => ({
        uploadAll: async () => {
          // In auto-upload mode, files are already uploaded
          // Return empty array as there's nothing to upload manually
          return [];
        },
        getPendingFiles: () => {
          return items.filter(it => it.status === 'pending').map(it => it.file);
        },
        getUploadedFiles: () => {
          return items
            .filter(it => it.status === 'success' && it.serverFile)
            .map(it => it.serverFile)
            .filter((file): file is UploadedFile => file !== undefined);
        },
        clearAll: () => {
          setItems([]);
        },
      }),
      [items, folderId, workspaceId]
    );

    // Initialize/sync items from external value in controlled mode without dropping in-flight uploads
    useEffect(() => {
      if (!controlled) return;

      setItems(prev => {
        const nonSuccessItems = prev.filter(it => it.status !== 'success');
        const syncedSuccessItems: UploadItem[] = value.map(file => {
          const matched = prev.find(
            it => it.status === 'success' && (it.serverFile?.id === file.id || it.id === file.id)
          );

          if (matched) {
            return {
              ...matched,
              status: 'success',
              progress: 100,
              serverFile: file,
              file:
                matched.file?.name && matched.file.name.length > 0
                  ? matched.file
                  : new File([], file.name, { type: file.mime_type }),
            };
          }

          return {
            id: file.id,
            file: new File([], file.name, { type: file.mime_type }),
            progress: 100,
            status: 'success',
            serverFile: file,
          };
        });

        const next = [...nonSuccessItems, ...syncedSuccessItems];
        const isSame =
          prev.length === next.length &&
          prev.every((item, index) => {
            const target = next[index];
            if (!target) return false;
            return (
              item.id === target.id &&
              item.status === target.status &&
              item.progress === target.progress &&
              item.errorMsg === target.errorMsg &&
              item.serverFile?.id === target.serverFile?.id &&
              item.file?.name === target.file?.name &&
              item.file?.size === target.file?.size &&
              item.file?.type === target.file?.type
            );
          });

        return isSame ? prev : next;
      });
    }, [controlled, value]);

    // Notify parent after items change
    useEffect(() => {
      if (!onChangeRef.current) return;

      const successFiles = items
        .filter(it => it.status === 'success' && it.serverFile)
        .map(it => it.serverFile)
        .filter((file): file is UploadedFile => file !== undefined);

      // In controlled mode, only call onChange if the successFiles are different from current value
      if (controlled) {
        // Compare with current value to avoid infinite loops
        const currentIds = value.map(f => f.id).sort();
        const newIds = successFiles.map(f => f.id).sort();
        const hasChanged =
          currentIds.length !== newIds.length ||
          currentIds.some((id, index) => id !== newIds[index]);

        if (hasChanged) {
          onChangeRef.current?.(successFiles);
        }
      } else {
        // Call immediately in non-controlled mode
        onChangeRef.current(successFiles);
      }
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [items, controlled, value]);

    const inputRef = useRef<HTMLInputElement>(null);

    /* ---------------------------- handle selection --------------------------- */

    const validateFile = useCallback(
      (file: File): { isValid: boolean; errorMsg?: string } => {
        if (file.size > maxSizeMB * 1024 * 1024) {
          return {
            isValid: false,
            errorMsg: t('fileUpload.tooLarge', { max: maxSizeMB }),
          };
        }
        if (acceptExt.length) {
          const ext = file.name.split('.').pop()?.toLowerCase();
          if (!ext) {
            return {
              isValid: false,
              errorMsg: t('fileUpload.invalidExt'),
            };
          }

          // Support both formats: "txt" and ".txt"
          const normalizedAcceptExt = acceptExt.map(e => {
            const normalized = e.toLowerCase();
            return normalized.startsWith('.') ? normalized.slice(1) : normalized;
          });

          if (!normalizedAcceptExt.includes(ext)) {
            return {
              isValid: false,
              errorMsg: t('fileUpload.invalidExt'),
            };
          }
        }
        return { isValid: true };
      },
      [acceptExt, maxSizeMB, t]
    );

    const enqueueFiles = useCallback(
      (files: FileList | File[]) => {
        const currentCount = items.length;

        if (currentCount >= maxCount) {
          toast.error(t('fileUpload.exceedCount', { max: maxCount }));
          return;
        }

        const arr = Array.from(files);

        // Check if trying to add too many files
        const availableSlots = maxCount - currentCount;
        const filesToProcess = arr.slice(0, availableSlots);

        if (arr.length > availableSlots) {
          toast.error(t('fileUpload.exceedCount', { max: maxCount }));
        }

        // Process all files, including invalid ones
        const newItems: UploadItem[] = filesToProcess.map(file => {
          const validation = validateFile(file);

          if (!validation.isValid) {
            const errorMsg = validation.errorMsg ?? t('fileUpload.error');
            toast.error(
              t('fileUpload.invalidFileToast', {
                file: file.name,
                reason: errorMsg,
              })
            );
            // Create item with error status immediately
            return {
              id: genId(),
              file,
              progress: 0,
              status: 'error' as const,
              errorMsg,
            };
          }

          // Create valid item for upload
          return {
            id: genId(),
            file,
            progress: 0,
            status: 'pending' as const,
          };
        });

        if (!newItems.length) {
          return;
        }

        setItems(prev => [...prev, ...newItems]);

        // Only start upload for valid files
        newItems.filter(item => item.status === 'pending').forEach(startUpload);
      },
      [items, maxCount, validateFile, t]
    );

    /* ------------------------------- uploading ------------------------------- */

    const startUpload = (item: UploadItem) => {
      setItems(prev => prev.map(it => (it.id === item.id ? { ...it, status: 'uploading' } : it)));

      uploadService
        .uploadSingle(item.file, {
          folder_id: folderId,
          workspace_id: workspaceId,
          is_temporary: isTemporary,
          onProgress: p =>
            setItems(prev => prev.map(it => (it.id === item.id ? { ...it, progress: p } : it))),
        })
        .then(response => {
          const uploaded: UploadedFile = {
            id: response.id,
            name: response.name,
            size: response.size,
            extension: response.extension,
            mime_type: response.mime_type,
            hash: response.hash || '',
            created_by: response.created_by,
            created_at: response.created_at,
            url: response.url || '',
          };

          setItems(prev => {
            const next: UploadItem[] = prev.map(it =>
              it.id === item.id
                ? ({ ...it, progress: 100, status: 'success', serverFile: uploaded } as UploadItem)
                : it
            );
            return next;
          });
        })
        .catch((err: Error) => {
          setItems(prev =>
            prev.map(it =>
              it.id === item.id
                ? { ...it, progress: 100, status: 'error', errorMsg: err.message }
                : it
            )
          );
        });
    };

    const removeItem = (id: string) => {
      setItems(prev => {
        const next = prev.filter(it => it.id !== id);
        return next as UploadItem[];
      });
    };

    const retryItem = (id: string) => {
      const target = items.find(it => it.id === id);
      if (target) {
        // Re-validate the file before retry
        const validation = validateFile(target.file);

        if (!validation.isValid) {
          const errorMsg = validation.errorMsg ?? t('fileUpload.error');
          toast.error(
            t('fileUpload.invalidFileToast', {
              file: target.file.name,
              reason: errorMsg,
            })
          );
          // Update with new validation error
          setItems(prev =>
            prev.map(it => (it.id === id ? { ...it, status: 'error', errorMsg } : it))
          );
          return;
        }

        // File is valid, proceed with upload
        startUpload({ ...target, progress: 0, status: 'pending', errorMsg: undefined });
      }
    };

    /* ------------------------------ drag events ------------------------------ */

    const [dragActive, setDragActive] = useState(false);

    const handleDrag = (e: React.DragEvent<HTMLDivElement>) => {
      e.preventDefault();
      e.stopPropagation();
      if (isFull) return;
      if (e.type === 'dragenter' || e.type === 'dragover') {
        setDragActive(true);
      } else if (e.type === 'dragleave') {
        setDragActive(false);
      }
    };

    const handleDrop = (e: React.DragEvent<HTMLDivElement>) => {
      e.preventDefault();
      e.stopPropagation();
      setDragActive(false);
      if (isFull) return;
      if (e.dataTransfer.files && e.dataTransfer.files.length) {
        enqueueFiles(e.dataTransfer.files);
      }
    };

    return (
      <div className={cn('space-y-2', containerClassName)}>
        <div className="flex justify-between items-center">
          <p className="text-base">{t('fileUpload.uploadFilesLabel')}</p>
          {showSystemSelect && (
            <Button
              size="sm"
              type="button"
              onClick={e => {
                e.preventDefault();
                e.stopPropagation();
                if (isFull) return;
                if (!isAuthenticated) {
                  setIsLoginDialogOpen(true);
                  return;
                }
                setIsSystemSelectOpen(true);
              }}
              disabled={isFull}
            >
              {t('fileUpload.selectFromSystem')}
            </Button>
          )}
        </div>
        {/* Upload zone */}
        <div
          onDragEnter={handleDrag}
          onDragOver={handleDrag}
          onDragLeave={handleDrag}
          onDrop={handleDrop}
          className={cn(
            'flex flex-col items-center justify-center border-2 border-dashed rounded-md px-5 py-4 text-center transition-colors',
            dragActive ? 'border-primary bg-primary/5' : 'border-border',
            dropZoneClassName,
            isFull ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'
          )}
          onClick={() => {
            if (!isFull) inputRef.current?.click();
          }}
        >
          <input
            ref={inputRef}
            type="file"
            multiple
            hidden
            accept={inputAccept}
            disabled={isFull}
            onChange={e => {
              if (e.target.files) {
                enqueueFiles(e.target.files);
                e.target.value = '';
              }
            }}
          />
          {dropContent ?? (
            <div className="text-center select-none flex flex-col items-center">
              <UploadCloudIcon className="w-8 h-8 text-primary mb-2" />
              <p className="text-sm text-muted-foreground">{t('fileUpload.dropHere')}</p>
              {acceptExt.length > 0 && (
                <p className="text-xs text-muted-foreground mt-1">
                  {t('fileUpload.allowedTypesLabel')}
                  <span className="font-semibold text-primary">
                    {formatExtensionsForDisplay(filterLowercaseExtensions(acceptExt)).join(' , ')}
                  </span>
                </p>
              )}
              {maxSizeMB && (
                <p className="text-xs text-muted-foreground mt-0.5">
                  {t('fileUpload.maxSizeLabel')}{' '}
                  <span className="font-semibold text-primary mr-0.5">{maxSizeMB}</span> MB
                </p>
              )}
              {maxCount && (
                <p className="text-xs text-muted-foreground mt-0.5">
                  {t('fileUpload.maxCountLabel')}{' '}
                  <span className="font-semibold text-primary mr-0.5">{maxCount}</span>
                </p>
              )}
            </div>
          )}
        </div>

        {/* Table */}
        {items.length > 0 && (
          <FileList
            items={items.map(it => ({
              id: it.id,
              name: it.serverFile ? it.serverFile.name : it.file.name,
              size: it.serverFile ? it.serverFile.size : it.file.size,
              status: it.status,
              progress: it.progress,
              errorMsg: it.errorMsg,
            }))}
            onRetry={retryItem}
            onRemove={removeItem}
            tableWrapperClassName={tableWrapperClassName}
          />
        )}
        {showSystemSelect && isSystemSelectOpen && (
          <FileSelectorDialog
            open={isSystemSelectOpen}
            onOpenChange={setIsSystemSelectOpen}
            onConfirm={handleSystemSelectConfirm}
            maxCount={Math.max(0, maxCount - items.length)}
            acceptExt={acceptExt}
            allowWorkspaceSwitch={allowWorkspaceSwitch}
          />
        )}
        {showSystemSelect && (
          <ConfirmDialog
            open={isLoginDialogOpen}
            onOpenChange={setIsLoginDialogOpen}
            title={t('fileUpload.loginRequiredTitle')}
            description={t('fileUpload.loginRequiredDescription')}
            confirmText={t('fileUpload.goToLogin')}
            cancelText={t('fileUpload.cancelAction')}
            onConfirm={() => {
              const currentSearch = window.location.search;
              const currentUrl = currentSearch ? `${pathname}${currentSearch}` : pathname || '/';
              router.push(`/login?redirect=${encodeURIComponent(currentUrl)}`);
            }}
          />
        )}
      </div>
    );
  }
);

AutoFileUpload.displayName = 'AutoFileUpload';
