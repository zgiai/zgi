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
import type { FileParseProviderKey, FileUploadProcessingMode } from '@/services/types/file';
import {
  buildFileInputAcceptAttribute,
  filterLowercaseExtensions,
  formatExtensionsForDisplay,
} from '@/utils/file-helpers';
import { generateClientId } from '@/utils/client-id';
import { FileList } from '@/components/common/file-upload/file-list';
import { Button } from '@/components/ui/button';
import {
  calculateFileHash,
  getExistingFileKeys,
  getFileFallbackKey,
  hasAnyFileKey,
} from './file-dedup';

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface ManualFileUploadProps {
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
  /** Callback when files are selected (before upload) */
  onFilesChange?: (files: File[]) => void;
  /** Callback when queue status counts change */
  onQueueStateChange?: (state: {
    failedCount: number;
    pendingCount: number;
    totalCount: number;
    uploadingCount: number;
  }) => void;
  /** Callback after files are successfully uploaded */
  onUploadComplete?: (files: UploadedFile[]) => void;
  /** Translation namespace for the selected-file queue summary */
  queueSummaryNamespace?: 'files' | 'ui';
  /** Folder ID to upload files to */
  folderId?: string;
  /** Workspace id */
  workspaceId?: string;
  /** File asset processing mode for uploaded documents */
  processingMode?: FileUploadProcessingMode;
  /** Content parse provider for uploaded documents */
  parseProvider?: FileParseProviderKey;
}

export interface ManualFileUploadRef {
  /** Manually trigger upload for all pending files */
  uploadAll: () => Promise<UploadedFile[]>;
  /** Get all pending files (not yet uploaded) */
  getPendingFiles: () => File[];
  /** Get all failed files */
  getFailedFiles: () => File[];
  /** Get all successfully uploaded files */
  getUploadedFiles: () => UploadedFile[];
  /** Clear all files */
  clearAll: () => void;
  /** Check if upload is in progress */
  getIsUploading: () => boolean;
}

interface UploadItem {
  id: string;
  file: File;
  contentHash?: string;
  progress: number; // 0-100
  status: 'pending' | 'uploading' | 'success' | 'error';
  serverFile?: UploadedFile;
  errorMsg?: string;
}

const genId = () => generateClientId('upload');

/* -------------------------------------------------------------------------- */
/* Component                                                                   */
/* -------------------------------------------------------------------------- */

export const ManualFileUpload = forwardRef<ManualFileUploadRef, ManualFileUploadProps>(
  (
    {
      maxCount = 5,
      maxSizeMB = 15,
      acceptExt = [],
      containerClassName,
      tableWrapperClassName,
      dropZoneClassName,
      dropContent,
      onFilesChange,
      onQueueStateChange,
      onUploadComplete,
      queueSummaryNamespace,
      folderId,
      workspaceId,
      processingMode,
      parseProvider,
    },
    ref
  ) => {
    // Internationalization
    const t = useT('ui');

    const [items, setItems] = useState<UploadItem[]>([]);
    const [isUploading, setIsUploading] = useState(false);
    const isFull = items.length >= maxCount;
    const inputAccept = buildFileInputAcceptAttribute(acceptExt);

    // Keep latest callbacks in ref to avoid effect dependency loop
    const onFilesChangeRef = useRef<typeof onFilesChange>();
    const onQueueStateChangeRef = useRef<typeof onQueueStateChange>();
    const onUploadCompleteRef = useRef<typeof onUploadComplete>();

    useEffect(() => {
      onFilesChangeRef.current = onFilesChange;
    }, [onFilesChange]);

    useEffect(() => {
      onQueueStateChangeRef.current = onQueueStateChange;
    }, [onQueueStateChange]);

    useEffect(() => {
      onUploadCompleteRef.current = onUploadComplete;
    }, [onUploadComplete]);

    // Notify parent when files list changes
    useEffect(() => {
      if (onFilesChangeRef.current) {
        const files = items.map(it => it.file);
        onFilesChangeRef.current(files);
      }
      onQueueStateChangeRef.current?.({
        failedCount: items.filter(it => it.status === 'error').length,
        pendingCount: items.filter(it => it.status === 'pending').length,
        totalCount: items.length,
        uploadingCount: items.filter(it => it.status === 'uploading').length,
      });
    }, [items]);

    // Expose methods to parent via ref
    useImperativeHandle(
      ref,
      () => ({
        uploadAll: async () => {
          const pendingItems = items.filter(it => it.status === 'pending');

          if (pendingItems.length === 0) {
            return [];
          }

          setIsUploading(true);

          try {
            // Start upload for all pending items
            const uploadPromises = pendingItems.map(item => {
              return new Promise<UploadedFile | null>(resolve => {
                setItems(prev =>
                  prev.map(it => (it.id === item.id ? { ...it, status: 'uploading' as const } : it))
                );

                uploadService
                  .uploadSingle(item.file, {
                    folder_id: folderId,
                    workspace_id: workspaceId,
                    processing_mode: processingMode,
                    parse_provider: parseProvider,
                    onProgress: p =>
                      setItems(prev =>
                        prev.map(it => (it.id === item.id ? { ...it, progress: p } : it))
                      ),
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

                    setItems(prev =>
                      prev.map(it =>
                        it.id === item.id
                          ? ({
                              ...it,
                              progress: 100,
                              status: 'success',
                              serverFile: uploaded,
                            } as UploadItem)
                          : it
                      )
                    );

                    resolve(uploaded);
                  })
                  .catch((err: Error) => {
                    setItems(prev =>
                      prev.map(it =>
                        it.id === item.id
                          ? { ...it, progress: 100, status: 'error', errorMsg: err.message }
                          : it
                      )
                    );
                    resolve(null);
                  });
              });
            });

            const results = await Promise.all(uploadPromises);
            const successfulFiles = results.filter((file): file is UploadedFile => file !== null);

            // Notify parent of successful uploads
            if (onUploadCompleteRef.current && successfulFiles.length > 0) {
              onUploadCompleteRef.current(successfulFiles);
            }

            return successfulFiles;
          } finally {
            setIsUploading(false);
          }
        },

        getPendingFiles: () => {
          return items.filter(it => it.status === 'pending').map(it => it.file);
        },

        getFailedFiles: () => {
          return items.filter(it => it.status === 'error').map(it => it.file);
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

        getIsUploading: () => {
          return isUploading || items.some(it => it.status === 'uploading');
        },
      }),
      [items, folderId, workspaceId, processingMode, parseProvider, isUploading]
    );

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

        setItems(prev => [...prev, ...newItems]);
      },
      [items, maxCount, validateFile, t]
    );

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
            prev.map(it =>
              it.id === id ? { ...it, status: 'error', errorMsg } : it
            )
          );
          return;
        }

        // File is valid, reset to pending status (parent will trigger upload)
        setItems(prev =>
          prev.map(it =>
            it.id === id ? { ...it, status: 'pending', progress: 0, errorMsg: undefined } : it
          )
        );
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
      <div className={cn('space-y-4', containerClassName)}>
        {/* Upload zone */}
        <div
          onDragEnter={handleDrag}
          onDragOver={handleDrag}
          onDragLeave={handleDrag}
          onDrop={handleDrop}
          className={cn(
            'flex flex-col items-center justify-center border-2 border-dashed rounded-md px-6 py-4 text-center transition-colors',
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
              <UploadCloudIcon className="w-9 h-9 text-primary mb-2" />
              <p className="text-base text-muted-foreground">{t('fileUpload.dropHere')}</p>
              <Button
                type="button"
                className="mt-3 px-6"
                disabled={isFull}
                onClick={event => {
                  event.preventDefault();
                  event.stopPropagation();
                  if (!isFull) inputRef.current?.click();
                }}
              >
                {t('fileUpload.clickUpload')}
              </Button>
              {acceptExt.length > 0 && (
                <p className="text-sm text-muted-foreground mt-1">
                  {t('fileUpload.allowedTypesLabel')}
                  <span className="font-semibold text-primary">
                    {formatExtensionsForDisplay(filterLowercaseExtensions(acceptExt)).join(' , ')}
                  </span>
                </p>
              )}
              {maxSizeMB && (
                <p className="text-sm text-muted-foreground mt-0.5">
                  {t('fileUpload.maxSizeLabel')}{' '}
                  <span className="font-semibold text-primary mr-0.5">{maxSizeMB}</span> MB
                </p>
              )}
              {maxCount && (
                <p className="text-sm text-muted-foreground mt-0.5">
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
            queueSummaryNamespace={queueSummaryNamespace}
            tableWrapperClassName={tableWrapperClassName}
          />
        )}
      </div>
    );
  }
);

ManualFileUpload.displayName = 'ManualFileUpload';
