'use client';

import { useEffect, useId, useState } from 'react';
import { FileUp } from 'lucide-react';
import { useT } from '@/i18n';
import { Button, buttonVariants } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useUploadConfig } from '@/hooks/use-upload';
import type { FileItem, FileUploadProcessingMode } from '@/services/types/file';
import { cn } from '@/lib/utils';

interface ReplaceDocumentDialogProps {
  open: boolean;
  file: FileItem | null;
  loading?: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (
    file: FileItem,
    replacementFile: File,
    processingMode: FileUploadProcessingMode
  ) => void;
}

export function ReplaceDocumentDialog({
  open,
  file,
  loading = false,
  onOpenChange,
  onConfirm,
}: ReplaceDocumentDialogProps) {
  const { files: t, common } = useT();
  const { data: uploadConfig } = useUploadConfig({ enabled: open });
  const replacementFileInputId = useId();
  const [replacementFile, setReplacementFile] = useState<File | null>(null);
  const [processingMode, setProcessingMode] = useState<FileUploadProcessingMode>('process_now');

  const maxSizeMB = uploadConfig?.file_size_limit ?? 15;

  useEffect(() => {
    if (!open) return;
    setReplacementFile(null);
    setProcessingMode('process_now');
  }, [file?.id, open]);

  const handleConfirm = () => {
    if (!file || !replacementFile) return;
    onConfirm(file, replacementFile, processingMode);
  };

  const isFileTooLarge =
    replacementFile !== null && replacementFile.size > maxSizeMB * 1024 * 1024;
  const canSubmit = Boolean(file && replacementFile && !isFileTooLarge && !loading);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[520px] p-0 overflow-hidden">
        <DialogHeader className="border-b pb-4">
          <div className="flex items-start gap-3 pr-8">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <FileUp className="h-5 w-5" />
            </div>
            <div className="min-w-0">
              <DialogTitle className="text-lg font-semibold">
                {t('replaceDocument.title')}
              </DialogTitle>
              <DialogDescription className="mt-2 leading-6">
                {t('replaceDocument.description', { name: file?.name ?? '' })}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>
        <DialogBody className="space-y-5 py-5">
          <div className="space-y-2">
            <Label className="text-sm font-semibold">{t('replaceDocument.newFile')}</Label>
            <Input
              id={replacementFileInputId}
              type="file"
              className="sr-only"
              disabled={loading}
              onChange={event => setReplacementFile(event.target.files?.[0] ?? null)}
            />
            <div className="flex justify-center rounded-lg border border-input bg-background px-3 py-2">
              <label
                htmlFor={replacementFileInputId}
                className={cn(
                  buttonVariants({ variant: 'outline', size: 'default' }),
                  'cursor-pointer',
                  loading && 'pointer-events-none opacity-50'
                )}
              >
                {t('replaceDocument.chooseFile')}
              </label>
            </div>
            {replacementFile ? (
              <p className="text-xs leading-5 text-muted-foreground">
                {t('replaceDocument.selectedFile', { name: replacementFile.name })}
              </p>
            ) : null}
            {isFileTooLarge ? (
              <p className="text-xs leading-5 text-destructive">
                {t('replaceDocument.fileTooLarge', { max: maxSizeMB })}
              </p>
            ) : null}
          </div>

          <div className="space-y-2">
            <Label className="text-sm font-semibold">{t('upload.processingMode')}</Label>
            <Select
              value={processingMode}
              onValueChange={value => setProcessingMode(value as FileUploadProcessingMode)}
              disabled={loading}
            >
              <SelectTrigger className="bg-background">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="process_now">
                  {t('upload.processingModes.processNow.title')}
                </SelectItem>
                <SelectItem value="store_only">
                  {t('upload.processingModes.storeOnly.title')}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <p className="text-xs leading-5 text-muted-foreground">
            {processingMode === 'process_now'
              ? t('replaceDocument.processingHint')
              : t('replaceDocument.storeOnlyHint')}
          </p>
        </DialogBody>
        <DialogFooter className="border-t bg-muted/30 px-6 py-4">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {common('cancel')}
          </Button>
          <Button type="button" loading={loading} disabled={!canSubmit} onClick={handleConfirm}>
            {t('replaceDocument.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
