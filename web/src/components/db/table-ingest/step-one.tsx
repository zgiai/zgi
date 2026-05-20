'use client';

// Step 1: Select files from file manager
// English comments only as required. Strict types, no any.

import React, { useCallback, useMemo, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { FileCheck2, FolderOpen, ListChecks, ShieldCheck, Trash2 } from 'lucide-react';
import FileSelectorDialog from '@/components/files/file-selector-dialog';
import type { FileItem } from '@/services/types/file';
import { useT } from '@/i18n';
import { formatFileSize } from '@/utils/format';
import { FileIcon } from '@/components/ui/file-icon';

export interface IngestStepOneProps {
  onNext: (files: FileItem[]) => void;
  modelSelected: boolean;
  initialFiles?: FileItem[];
}

const StepOne: React.FC<IngestStepOneProps> = ({ onNext, modelSelected, initialFiles = [] }) => {
  const t = useT('dbs');
  const MAX_COUNT = 5;

  const [dialogOpen, setDialogOpen] = useState(false);
  const [selected, setSelected] = useState<FileItem[]>(initialFiles);

  const count = selected.length;
  const supportedDesc = useMemo(() => t('tableIngest.stepOne.supportedDesc'), [t]);

  const openDialog = useCallback(() => setDialogOpen(true), []);

  const removeFile = useCallback((fileId: string) => {
    setSelected(prev => prev.filter(f => f.id !== fileId));
  }, []);

  const onConfirmFiles = useCallback((files: FileItem[]) => {
    setSelected(files);
  }, []);

  const handleNext = useCallback(() => {
    if (selected.length === 0 || !modelSelected) return;
    onNext(selected);
  }, [onNext, selected, modelSelected]);

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
                {selected.map(file => (
                  <div
                    key={file.id}
                    className="flex items-center justify-between rounded-md border shadow-sm px-3 py-2"
                  >
                    <div className="flex items-center gap-3 overflow-hidden">
                      <FileIcon filename={file.name} className="shrink-0" />
                      <div className="min-w-0">
                        <div className="truncate text-sm font-medium">{file.name}</div>
                        <div className="text-xs text-muted-foreground">
                          {formatFileSize(file.size)}
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
                ))}
              </div>
            </>
          )}
        </div>
        <div className="flex justify-center">
          <Button onClick={handleNext} disabled={count === 0 || !modelSelected}>
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
      />
    </div>
  );
};

export default StepOne;
