'use client';

import { useState, useEffect } from 'react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Label } from '@/components/ui/label';
import { RefreshCw } from 'lucide-react';

/**
 * Create text file data
 */
export interface CreateTextFileData {
  filename: string;
  content: string;
  folder_id?: string;
}

/**
 * Create text file dialog props
 */
export interface CreateTextFileDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (data: CreateTextFileData) => Promise<void>;
  folderId?: string;
  folderName?: string;
  isCreating?: boolean;
}

/**
 * Create Text File Dialog Component
 * Allows users to create a new text file with name and content
 */
export function CreateTextFileDialog({
  open,
  onOpenChange,
  onConfirm,
  folderId,
  folderName,
  isCreating = false,
}: CreateTextFileDialogProps) {
  const t = useT();

  // Local state
  const [filename, setFilename] = useState<string>('');
  const [content, setContent] = useState<string>('');

  // Calculate statistics
  const charCount = content.length;
  const byteSize = new Blob([content]).size;
  const fileSizeKB = (byteSize / 1024).toFixed(2);

  // Reset form when dialog opens/closes
  useEffect(() => {
    if (!open) {
      setTimeout(() => {
        setFilename('');
        setContent('');
      }, 200);
    }
  }, [open]);

  // Handle confirm
  const handleConfirm = async () => {
    if (!filename.trim() || !content.trim()) {
      return;
    }

    await onConfirm({
      filename: filename.trim(),
      content: content.trim(),
      folder_id: folderId,
    });
  };

  // Handle cancel
  const handleCancel = () => {
    onOpenChange(false);
  };

  // Check if form is valid
  const isValid = filename.trim().length > 0 && content.trim().length > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[640px] p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('files.text.createTitle')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-6 py-6">
          <form
            id="create-text-file-form"
            onSubmit={e => {
              e.preventDefault();
              handleConfirm();
            }}
            className="space-y-6"
          >
            {/* File Name Input */}
            <div className="space-y-2.5">
              <Label htmlFor="filename" className="text-sm font-semibold">
                {t('files.text.fileNameLabel')}
              </Label>
              <Input
                id="filename"
                className="h-11 shadow-sm"
                value={filename}
                onChange={e => setFilename(e.target.value)}
                placeholder={t('files.text.fileNamePlaceholder')}
                disabled={isCreating}
                autoFocus
              />
            </div>

            {/* Content Textarea */}
            <div className="space-y-2.5">
              <div className="flex items-center justify-between">
                <Label htmlFor="content" className="text-sm font-semibold">
                  {t('files.text.contentLabel')}
                </Label>
                {/* Statistics */}
                <div className="flex items-center gap-3 text-xs text-muted-foreground font-medium uppercase tracking-wider">
                  <span>
                    {charCount} {t('files.text.charCount')}
                  </span>
                  <span className="w-1 h-1 rounded-full bg-neutral-300" />
                  <span>{fileSizeKB} KB</span>
                </div>
              </div>
              <Textarea
                id="content"
                value={content}
                onChange={e => setContent(e.target.value)}
                placeholder={t('files.text.contentPlaceholder')}
                className="min-h-[320px] resize-none shadow-sm rounded-xl p-4"
                disabled={isCreating}
              />
            </div>
          </form>

          {/* Storage Location Info */}
          {folderName && (
            <div className="flex items-center gap-2 text-xs text-muted-foreground bg-neutral-50 px-3 py-2 rounded-lg w-fit">
              <span className="font-medium">{t('files.text.storageLocation')}:</span>
              <span className="font-bold text-primary">{folderName}</span>
            </div>
          )}
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6">
          <Button
            variant="ghost"
            onClick={handleCancel}
            disabled={isCreating}
            className="font-semibold"
          >
            {t('common.cancel')}
          </Button>
          <Button
            type="submit"
            form="create-text-file-form"
            disabled={!isValid || isCreating}
            size="lg"
            className="px-10 font-bold"
          >
            {isCreating && <RefreshCw className="mr-2 h-4 w-4 animate-spin" />}
            {t('files.text.saveFile')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
