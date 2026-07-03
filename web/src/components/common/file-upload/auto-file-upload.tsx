'use client';

import React, {
  useCallback,
  useRef,
  useState,
  useEffect,
  useLayoutEffect,
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
import {
  calculateFileHash,
  getExistingFileKeys,
  getFileFallbackKey,
  getUploadedFileKeys,
  hasAnyFileKey,
} from './file-dedup';
import type { FileParseProviderKey, FileUploadProcessingMode } from '@/services/types/file';

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
  /** Whether to show the allowed extensions hint below the upload button */
  showAllowedTypesHint?: boolean;
  /** Whether to set the native file input accept attribute */
  useNativeAccept?: boolean;
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
  /** File asset processing mode for uploaded documents */
  processingMode?: FileUploadProcessingMode;
  /** Content parse provider for uploaded documents */
  parseProvider?: FileParseProviderKey;
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
  /** Abort all in-flight uploads and remove them from the queue */
  cancelUploading: () => void;
}

interface UploadItem {
  id: string;
  file: File;
  contentHash?: string;
  progress: number; // 0-100
  status: 'pending' | 'uploading' | 'success' | 'error';
  serverFile?: UploadedFile;
  controller?: AbortController;
  errorMsg?: string;
}

const genId = () => generateClientId('upload');

function getSuccessfulUploadFiles(items: UploadItem[]): UploadedFile[] {
  return items
    .filter(it => it.status === 'success' && it.serverFile)
    .map(it => it.serverFile)
    .filter((file): file is UploadedFile => file !== undefined);
}

/* -------------------------------------------------------------------------- */
/* Component                                                                   */
/* -------------------------------------------------------------------------- */

export const AutoFileUpload = forwardRef<AutoFileUploadRef, AutoFileUploadProps>(
  (
    {
      maxCount = 5,
      maxSizeMB = 15,
      acceptExt = [],
      showAllowedTypesHint = true,
      useNativeAccept = true,
      containerClassName,
      tableWrapperClassName,
      dropZoneClassName,
      dropContent,
      onChange,
      value = [],
      controlled = false,
      folderId,
      workspaceId,
      processingMode,
      parseProvider,
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
    const uploadControllersRef = useRef<Map<string, AbortController>>(new Map());
    const itemsRef = useRef(items);
    const isFull = items.length >= maxCount;
    const [isSystemSelectOpen, setIsSystemSelectOpen] = useState(false);
    const [isLoginDialogOpen, setIsLoginDialogOpen] = useState(false);
    const inputAccept = useNativeAccept ? buildFileInputAcceptAttribute(acceptExt) : undefined;

    const handleSystemSelectConfirm = useCallback(
      (files: FileItem[]) => {
        const existingKeys = getExistingFileKeys(items);
        const uniqueNewItems: UploadItem[] = [];

        files.forEach(file => {
          const serverFile: UploadedFile = {
            id: file.id,
            name: file.name,
            size: file.size,
            extension: file.extension,
            mime_type: file.mime_type,
            hash: file.hash,
            created_by: file.created_by,
            created_at: file.created_at,
            url: file.source_url,
          };
          const keys = getUploadedFileKeys(serverFile);

          if (hasAnyFileKey(existingKeys, keys)) {
            toast.info(t('fileUpload.duplicateFile', { file: file.name }));
            return;
          }

          keys.forEach(key => existingKeys.add(key));
          uniqueNewItems.push({
            id: file.id,
            file: new File([], file.name, { type: file.mime_type }),
            contentHash: file.hash,
            progress: 100,
            status: 'success' as const,
            serverFile,
          });
        });

        if (!uniqueNewItems.length) {
          return;
        }

        setItems(prev => {
          const next = [...prev, ...uniqueNewItems];
          itemsRef.current = next;
          latestSuccessFilesRef.current = getSuccessfulUploadFiles(next);
          return next;
        });
      },
      [items, t]
    );

    // Keep latest onChange in ref to avoid effect dependency loop
    const onChangeRef = useRef<typeof onChange>();
    useLayoutEffect(() => {
      onChangeRef.current = onChange;
    }, [onChange]);
    const valueRef = useRef(value);
    const latestSuccessFilesRef = useRef(value);
    const controlledRef = useRef(controlled);
    useLayoutEffect(() => {
      valueRef.current = value;
      controlledRef.current = controlled;
    }, [controlled, value]);
    useLayoutEffect(() => {
      itemsRef.current = items;
      latestSuccessFilesRef.current = getSuccessfulUploadFiles(items);
    }, [items]);

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
          return itemsRef.current.filter(it => it.status === 'pending').map(it => it.file);
        },
        getUploadedFiles: () => {
          const latestSuccessFiles = latestSuccessFilesRef.current;
          if (latestSuccessFiles.length > 0) {
            return latestSuccessFiles;
          }
          return getSuccessfulUploadFiles(itemsRef.current);
        },
        clearAll: () => {
          uploadControllersRef.current.forEach(controller => controller.abort());
          uploadControllersRef.current.clear();
          itemsRef.current = [];
          latestSuccessFilesRef.current = [];
          setItems([]);
        },
        cancelUploading: () => {
          uploadControllersRef.current.forEach(controller => controller.abort());
          uploadControllersRef.current.clear();
          setItems(prev => {
            const next = prev.filter(it => it.status !== 'uploading');
            itemsRef.current = next;
            latestSuccessFilesRef.current = getSuccessfulUploadFiles(next);
            return next;
          });
        },
      }),
      []
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

        if (!isSame) {
          itemsRef.current = next;
          latestSuccessFilesRef.current = getSuccessfulUploadFiles(next);
        }

        return isSame ? prev : next;
      });
    }, [controlled, value]);

    // Notify parent after items change
    useLayoutEffect(() => {
      if (!onChangeRef.current) return;

      const successFiles = getSuccessfulUploadFiles(items);
      latestSuccessFilesRef.current = successFiles;

      // In controlled mode, compare against refs so external value reference changes do not
      // re-run this notification effect.
      if (controlledRef.current) {
        const currentIds = valueRef.current.map(f => f.id).sort();
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
    }, [items]);

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

    /* ------------------------------- uploading ------------------------------- */

    const startUpload = useCallback((item: UploadItem) => {
      const controller = new AbortController();
      uploadControllersRef.current.set(item.id, controller);
      setItems(prev => {
        const next: UploadItem[] = prev.map(it =>
          it.id === item.id ? { ...it, status: 'uploading' as const } : it
        );
        itemsRef.current = next;
        latestSuccessFilesRef.current = getSuccessfulUploadFiles(next);
        return next;
      });

      uploadService
        .uploadSingle(item.file, {
          folder_id: folderId,
          workspace_id: workspaceId,
          is_temporary: isTemporary,
          processing_mode: processingMode,
          parse_provider: parseProvider,
          signal: controller.signal,
          onProgress: p =>
            setItems(prev => {
              const next = prev.map(it => (it.id === item.id ? { ...it, progress: p } : it));
              itemsRef.current = next;
              latestSuccessFilesRef.current = getSuccessfulUploadFiles(next);
              return next;
            }),
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

          if (onChangeRef.current) {
            const existingFiles = controlledRef.current
              ? latestSuccessFilesRef.current
              : getSuccessfulUploadFiles(itemsRef.current);
            const nextFiles = [
              ...existingFiles.filter(file => file.id !== uploaded.id),
              uploaded,
            ];
            latestSuccessFilesRef.current = nextFiles;
            onChangeRef.current(nextFiles);
          }

          setItems(prev => {
            const next: UploadItem[] = prev.map(it =>
              it.id === item.id
                ? ({ ...it, progress: 100, status: 'success', serverFile: uploaded } as UploadItem)
                : it
            );
            itemsRef.current = next;
            latestSuccessFilesRef.current = getSuccessfulUploadFiles(next);
            return next;
          });
        })
        .catch((err: Error) => {
          if (controller.signal.aborted) return;

          setItems(prev => {
            const next: UploadItem[] = prev.map(it =>
              it.id === item.id
                ? { ...it, progress: 100, status: 'error' as const, errorMsg: err.message }
                : it
            );
            itemsRef.current = next;
            latestSuccessFilesRef.current = getSuccessfulUploadFiles(next);
            return next;
          });
        })
        .finally(() => {
          uploadControllersRef.current.delete(item.id);
        });
    }, [folderId, isTemporary, processingMode, parseProvider, workspaceId]);

    const enqueueFiles = useCallback(
      async (files: FileList | File[]) => {
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

        const existingKeys = getExistingFileKeys(items);
        const newItems: UploadItem[] = [];

        for (const file of filesToProcess) {
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
            newItems.push({
              id: genId(),
              file,
              progress: 0,
              status: 'error' as const,
              errorMsg,
            });
            continue;
          }

          const contentHash = await calculateFileHash(file);
          const duplicateKeys = [`hash:${contentHash}`, `local:${getFileFallbackKey(file)}`];
          if (hasAnyFileKey(existingKeys, duplicateKeys)) {
            toast.info(t('fileUpload.duplicateFile', { file: file.name }));
            continue;
          }

          duplicateKeys.forEach(key => existingKeys.add(key));
          newItems.push({
            id: genId(),
            file,
            contentHash,
            progress: 0,
            status: 'pending' as const,
          });
        }

        if (!newItems.length) {
          return;
        }

        setItems(prev => {
          const next = [...prev, ...newItems];
          itemsRef.current = next;
          latestSuccessFilesRef.current = getSuccessfulUploadFiles(next);
          return next;
        });

        // Only start upload for valid files
        newItems.filter(item => item.status === 'pending').forEach(startUpload);
      },
      [items, maxCount, startUpload, validateFile, t]
    );

    const removeItem = (id: string) => {
      uploadControllersRef.current.get(id)?.abort();
      uploadControllersRef.current.delete(id);
      setItems(prev => {
        const next = prev.filter(it => it.id !== id);
        itemsRef.current = next;
        latestSuccessFilesRef.current = getSuccessfulUploadFiles(next);
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
          setItems(prev => {
            const next: UploadItem[] = prev.map(it =>
              it.id === id ? { ...it, status: 'error' as const, errorMsg } : it
            );
            itemsRef.current = next;
            latestSuccessFilesRef.current = getSuccessfulUploadFiles(next);
            return next;
          });
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
            'flex flex-col items-center justify-center border-2 border-dashed rounded-md px-5 py-3 text-center transition-colors',
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
            aria-label={t('fileUpload.uploadAria')}
            data-testid="local-file-input"
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
              <Button
                type="button"
                size="sm"
                className="mt-3 px-5"
                disabled={isFull}
                onClick={event => {
                  event.preventDefault();
                  event.stopPropagation();
                  if (!isFull) inputRef.current?.click();
                }}
              >
                {t('fileUpload.clickUpload')}
              </Button>
              {showAllowedTypesHint && acceptExt.length > 0 && (
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
