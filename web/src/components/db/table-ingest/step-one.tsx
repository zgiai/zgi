'use client';

// Step 1: Select files from file manager
// English comments only as required. Strict types, no any.

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { AlertCircle, FileCheck2, FolderOpen, ListChecks, ShieldCheck, Trash2 } from 'lucide-react';
import FileSelectorDialog from '@/components/files/file-selector-dialog';
import type { FileItem } from '@/services/types/file';
import { useT } from '@/i18n';
import { formatFileSize } from '@/utils/format';
import { FileIcon } from '@/components/ui/file-icon';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { formatExtensionsForDisplay } from '@/utils/file-helpers';
import { toast } from 'sonner';
import {
  isTableIngestImageFile,
  isTableIngestSupportedFile,
  TABLE_INGEST_DOCUMENT_EXTENSIONS,
} from '@/components/db/table-ingest/file-support';

export type ModelVisionCapabilityStatus = 'checking' | 'vision' | 'textOnly';

export interface IngestStepOneProps {
  onNext: (files: FileItem[]) => void;
  onFilesChange?: (files: FileItem[]) => void;
  modelSelected: boolean;
  modelSupportsVision?: boolean;
  modelVisionCapabilityStatus?: ModelVisionCapabilityStatus;
  initialFiles?: FileItem[];
  acceptExt?: string[];
}

const StepOne: React.FC<IngestStepOneProps> = ({
  onNext,
  onFilesChange,
  modelSelected,
  modelSupportsVision = false,
  modelVisionCapabilityStatus,
  initialFiles = [],
  acceptExt = [...TABLE_INGEST_DOCUMENT_EXTENSIONS],
}) => {
  const t = useT('dbs');
  const MAX_COUNT = 5;

  const [dialogOpen, setDialogOpen] = useState(false);
  const [selected, setSelected] = useState<FileItem[]>(initialFiles);

  useEffect(() => {
    setSelected(initialFiles);
  }, [initialFiles]);

  const count = selected.length;
  const hasImageFile = useMemo(() => selected.some(file => isTableIngestImageFile(file)), [selected]);
  const effectiveVisionStatus: ModelVisionCapabilityStatus =
    modelVisionCapabilityStatus ?? (modelSupportsVision ? 'vision' : 'textOnly');
  const visionCapabilityChecking = hasImageFile && effectiveVisionStatus === 'checking';
  const visionModelRequired = hasImageFile && effectiveVisionStatus === 'textOnly';
  const nextDisabled =
    count === 0 || !modelSelected || visionCapabilityChecking || visionModelRequired;
  const supportedDesc = useMemo(
    () =>
      modelSupportsVision
        ? t('tableIngest.stepOne.supportedDescWithImages')
        : t('tableIngest.stepOne.supportedDescDocumentsOnly'),
    [modelSupportsVision, t]
  );
  const acceptedTypesLabel = useMemo(
    () => formatExtensionsForDisplay(acceptExt).join(' / '),
    [acceptExt]
  );

  const openDialog = useCallback(() => setDialogOpen(true), []);

  const removeFile = useCallback((fileId: string) => {
    setSelected(prev => {
      const next = prev.filter(f => f.id !== fileId);
      onFilesChange?.(next);
      return next;
    });
  }, [onFilesChange]);

  const onConfirmFiles = useCallback(
    (files: FileItem[]) => {
      const acceptedFiles = files.filter(file => isTableIngestSupportedFile(file));
      const rejectedUnsupportedCount = files.length - acceptedFiles.length;

      if (rejectedUnsupportedCount > 0) {
        toast.error(
          t('tableIngest.stepOne.unsupportedFileSkipped', {
            types: acceptedTypesLabel,
          })
        );
      }

      setSelected(acceptedFiles);
      onFilesChange?.(acceptedFiles);
    },
    [acceptedTypesLabel, onFilesChange, t]
  );

  const handleNext = useCallback(() => {
    if (nextDisabled) return;
    onNext(selected);
  }, [nextDisabled, onNext, selected]);

  return (
    <div className="h-0 grow flex flex-col gap-4">
      {/* Banner description */}
      <Card className="p-4 border-highlight bg-highlight/10">
        <div className="text-sm font-medium text-highlight">
          {t('tableIngest.stepOne.bannerTitle')}
        </div>
        <div className="mt-1 text-sm text-highlight/90">
          {t('tableIngest.stepOne.bannerText', { desc: supportedDesc })}
        </div>
        <div className="mt-3 grid gap-2 md:grid-cols-3">
          {[
            {
              icon: <FileCheck2 className="h-4 w-4" />,
              label: t('tableIngest.stepOne.pipeline.fileManager'),
            },
            {
              icon: <ListChecks className="h-4 w-4" />,
              label: t('tableIngest.stepOne.pipeline.review'),
            },
            {
              icon: <ShieldCheck className="h-4 w-4" />,
              label: t('tableIngest.stepOne.pipeline.commit'),
            },
          ].map(item => (
            <div
              key={item.label}
              className="flex items-center gap-2 rounded-md border border-highlight/20 bg-background/80 px-3 py-2 text-xs text-foreground"
            >
              {item.icon}
              <span>{item.label}</span>
            </div>
          ))}
        </div>
      </Card>

      {/* Select area */}
      <div className="flex flex-col h-0 grow gap-4">
        {/* choose files area */}
        <div
          onClick={openDialog}
          className="rounded-md border-2 border-dashed cursor-pointer hover:bg-muted/80 flex items-center justify-center min-h-[240px]"
        >
          <div className="text-center space-y-3">
            <div className="flex items-center justify-center">
              <FolderOpen className="h-10 w-10 text-muted-foreground" />
            </div>
            <div className="text-base font-medium">{t('tableIngest.stepOne.chooseFromFiles')}</div>
            <div className="text-sm text-muted-foreground">{supportedDesc}</div>
          </div>
        </div>

        {/* selected files list */}
        <div className="flex flex-col gap-2 h-0 grow">
          {count !== 0 && (
            <>
              <div className="flex items-center justify-between mb-2">
                <div className="text-base font-bold">
                  {t('tableIngest.stepOne.selectedTitle', { count })}
                </div>
              </div>
              <div className="space-y-2 overflow-auto">
                {selected.map(file => {
                  const isImage = isTableIngestImageFile(file);
                  return (
                    <div
                      key={file.id}
                      className="flex items-center justify-between rounded-md border shadow-sm px-3 py-2"
                    >
                    <div className="flex items-center gap-3 overflow-hidden">
                      <FileIcon filename={file.name} className="shrink-0" />
                      <div className="min-w-0">
                        <div className="flex min-w-0 items-center gap-2">
                          <div className="truncate text-sm font-medium">{file.name}</div>
                          {isImage && effectiveVisionStatus !== 'vision' ? (
                            <Badge variant="secondary" className="shrink-0 text-warning">
                              {effectiveVisionStatus === 'checking'
                                ? t('tableIngest.stepOne.visionCheckingBadge')
                                : t('tableIngest.stepOne.needsVisionBadge')}
                            </Badge>
                          ) : null}
                        </div>
                        <div className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
                          <span>{formatFileSize(file.size)}</span>
                          {isImage ? <span>{t('tableIngest.stepOne.imageFileHint')}</span> : null}
                        </div>
                      </div>
                    </div>
                    <Button
                      variant="ghost"
                      isIcon
                      onClick={() => removeFile(file.id)}
                      aria-label={t('tableIngest.stepOne.removeFileAria')}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                    </div>
                  );
                })}
              </div>
            </>
          )}
          {visionCapabilityChecking && (
            <Alert>
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>
                {t('tableIngest.stepOne.visionCapabilityChecking')}
              </AlertDescription>
            </Alert>
          )}
          {visionModelRequired && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>{t('tableIngest.stepOne.visionModelRequired')}</AlertDescription>
            </Alert>
          )}
        </div>
        <div className="flex justify-center">
          <Button onClick={handleNext} disabled={nextDisabled}>
            {t('tableIngest.stepOne.startRecognition', { count })}
          </Button>
        </div>
      </div>

      {/* Dialog for selecting files from file manager */}
      <FileSelectorDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onConfirm={onConfirmFiles}
        initSelectedFiles={selected}
        maxCount={MAX_COUNT}
        acceptExt={acceptExt}
        footerExtra={
          effectiveVisionStatus !== 'vision' ? (
            <div className="text-xs text-muted-foreground">
              {effectiveVisionStatus === 'checking'
                ? t('tableIngest.stepOne.visionCapabilityChecking')
                : t('tableIngest.stepOne.imageModelLocked')}
            </div>
          ) : undefined
        }
      />
    </div>
  );
};

export default StepOne;
