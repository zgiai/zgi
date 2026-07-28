'use client';

import { useEffect, useId, useState, type FormEvent, type ReactNode } from 'react';
import { AlertTriangle, Loader2 } from 'lucide-react';
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
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

interface DestructiveNameConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description: ReactNode;
  itemName: string;
  itemNameLabel: string;
  confirmationLabel: string;
  confirmationPlaceholder: string;
  confirmationMismatchText: string;
  confirmText: string;
  confirmingText: string;
  cancelText: string;
  onConfirm: () => void;
  loading?: boolean;
}

export function DestructiveNameConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  itemName,
  itemNameLabel,
  confirmationLabel,
  confirmationPlaceholder,
  confirmationMismatchText,
  confirmText,
  confirmingText,
  cancelText,
  onConfirm,
  loading = false,
}: DestructiveNameConfirmDialogProps) {
  const inputId = useId();
  const [confirmationValue, setConfirmationValue] = useState('');
  const isMatch = itemName.length > 0 && confirmationValue === itemName;
  const showMismatch = confirmationValue.length > 0 && !isMatch;

  useEffect(() => {
    if (!open) {
      setConfirmationValue('');
    }
  }, [open, itemName]);

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setConfirmationValue('');
    }
    onOpenChange(nextOpen);
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!isMatch || loading) return;
    onConfirm();
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent size="md" className="w-[calc(100%-2rem)] overflow-hidden p-0">
        <form onSubmit={handleSubmit}>
          <DialogHeader className="pr-12">
            <div className="flex items-start gap-3">
              <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-destructive/10 text-destructive">
                <AlertTriangle className="h-5 w-5" />
              </div>
              <div className="min-w-0">
                <DialogTitle className="text-lg">{title}</DialogTitle>
                <DialogDescription className="mt-2 leading-6">{description}</DialogDescription>
              </div>
            </div>
          </DialogHeader>

          <DialogBody className="space-y-4 px-6 pb-5 pt-0">
            <div className="rounded-lg border bg-muted/40 px-3.5 py-3">
              <div className="text-xs text-muted-foreground">{itemNameLabel}</div>
              <div
                className="mt-1 max-h-14 overflow-y-auto break-all pr-1 text-sm font-medium leading-5 text-foreground"
                title={itemName}
              >
                {itemName}
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor={inputId} className="text-sm font-medium">
                {confirmationLabel}
              </Label>
              <Input
                id={inputId}
                value={confirmationValue}
                onChange={event => setConfirmationValue(event.target.value)}
                placeholder={confirmationPlaceholder}
                autoComplete="off"
                autoFocus
                aria-invalid={showMismatch}
                aria-describedby={showMismatch ? `${inputId}-error` : undefined}
              />
              {showMismatch && (
                <p id={`${inputId}-error`} className="text-xs text-destructive" role="alert">
                  {confirmationMismatchText}
                </p>
              )}
            </div>
          </DialogBody>

          <DialogFooter className="border-t bg-muted/20 px-6 py-4">
            <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
              {cancelText}
            </Button>
            <Button type="submit" variant="destructive" disabled={!isMatch || loading}>
              {loading && <Loader2 className="h-4 w-4 animate-spin" />}
              {loading ? confirmingText : confirmText}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
