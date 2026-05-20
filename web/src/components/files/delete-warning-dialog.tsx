'use client';

import { AlertCircle } from 'lucide-react';
import { useT } from '@/i18n';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';

/**
 * Delete Warning Dialog Props
 */
export interface DeleteWarningDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  fileNames?: string[];
  onViewRelated?: () => void;
}

/**
 * Delete Warning Dialog Component
 * Shows a warning when files cannot be deleted due to associations
 */
export function DeleteWarningDialog({
  open,
  onOpenChange,
  onViewRelated,
}: DeleteWarningDialogProps) {
  const t = useT('files');

  const handleClose = () => {
    onOpenChange(false);
  };

  const handleViewRelated = () => {
    onViewRelated?.();
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle className="text-lg font-semibold">{t('delete.cannotDelete')}</DialogTitle>
        </DialogHeader>

        <DialogBody>
          {/* Warning Icon and Message */}
          <div className="flex gap-3 ">
            <div className="flex-shrink-0">
              <AlertCircle className="w-4 h-4 text-red-600 mt-1" />
            </div>
            <div className="flex-1 space-y-2">
              <p className="text-sm text-gray-700 leading-relaxed">
                {t('delete.associationWarning')}
              </p>
              <p className="text-sm text-gray-700 leading-relaxed">{t('delete.unlinkFirst')}</p>
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="gap-2 sm:gap-0">
          <Button variant="outline" onClick={handleClose}>
            {t('delete.understood')}
          </Button>
          {onViewRelated && <Button onClick={handleViewRelated}>{t('delete.viewRelated')}</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
