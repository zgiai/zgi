'use client';

import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

interface UnsavedChangesConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  discardText: string;
  cancelText: string;
  confirmText: string;
  disabled?: boolean;
  onDiscard: () => void;
  onConfirm: () => void;
}

export function UnsavedChangesConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  discardText,
  cancelText,
  confirmText,
  disabled = false,
  onDiscard,
  onConfirm,
}: UnsavedChangesConfirmDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="sm" className="p-0">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter className="flex-col gap-2 border-t bg-muted/40 sm:flex-row sm:justify-end">
          <Button variant="outline" onClick={onDiscard} disabled={disabled}>
            {discardText}
          </Button>
          <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={disabled}>
            {cancelText}
          </Button>
          <Button onClick={onConfirm} disabled={disabled}>
            {confirmText}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
