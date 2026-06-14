'use client';

import { FileSearch } from 'lucide-react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import type { FileItem } from '@/services/types/file';

interface StartFileParseDialogProps {
  open: boolean;
  file: FileItem | null;
  loading?: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (file: FileItem) => void;
}

export function StartFileParseDialog({
  open,
  file,
  loading = false,
  onOpenChange,
  onConfirm,
}: StartFileParseDialogProps) {
  const { files: t, common } = useT();

  const handleConfirm = () => {
    if (!file) return;
    onConfirm(file);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[480px] p-0 overflow-hidden">
        <DialogHeader className="border-b pb-4">
          <div className="flex items-start gap-3 pr-8">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <FileSearch className="h-5 w-5" />
            </div>
            <div className="min-w-0">
              <DialogTitle className="text-lg font-semibold">
                {t('fileList.startParseDialog.title')}
              </DialogTitle>
              <DialogDescription className="mt-2 leading-6">
                {t('fileList.startParseDialog.description', { name: file?.name ?? '' })}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>
        <DialogBody className="space-y-3 py-5">
          <p className="text-xs leading-5 text-muted-foreground">
            {t('fileList.startParseDialog.providerHint')}
          </p>
        </DialogBody>
        <DialogFooter className="border-t bg-muted/30 px-6 py-4">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {common('cancel')}
          </Button>
          <Button type="button" loading={loading} onClick={handleConfirm}>
            {t('actions.startParse')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
