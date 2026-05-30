'use client';

import React, { useState } from 'react';
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';

export interface ConfirmDialogProps {
  /** Element to trigger the dialog (button, menu item, etc.) */
  trigger?: React.ReactNode;
  /** Dialog title */
  title: React.ReactNode;
  /** Optional description below the title */
  description?: React.ReactNode;
  /** Text for the confirm action button */
  confirmText?: string;
  /** Text for the cancel action button */
  cancelText?: string;
  /** Callback fired when confirming */
  onConfirm: () => void;
  /** Loading state for confirm action */
  loading?: boolean;
  /** Controlled open state (optional) */
  open?: boolean;
  /** Controlled open change handler (optional) */
  onOpenChange?: (open: boolean) => void;
  /** Optional cancel handler */
  onCancel?: () => void;
  /** Variant of the dialog (optional) */
  variant?: 'default' | 'warning';
  contentClassName?: string;
  footerClassName?: string;
  cancelClassName?: string;
  confirmClassName?: string;
}

export function ConfirmDialog({
  trigger,
  title,
  description,
  confirmText = 'Confirm',
  cancelText = 'Cancel',
  onConfirm,
  loading = false,
  open: openProp,
  onOpenChange: onOpenChangeProp,
  variant = 'default',
  onCancel,
  contentClassName,
  footerClassName,
  cancelClassName,
  confirmClassName,
}: ConfirmDialogProps) {
  const [openInternal, setOpenInternal] = useState(false);
  const open = openProp !== undefined ? openProp : openInternal;
  const setOpen = onOpenChangeProp !== undefined ? onOpenChangeProp : setOpenInternal;

  // Handle confirm action then close dialog
  const handleConfirm = () => {
    onConfirm();
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent size="sm" className={cn('p-0 overflow-hidden', contentClassName)}>
        <DialogHeader>
          <DialogTitle className="text-xl font-bold tracking-tight">{title}</DialogTitle>
        </DialogHeader>

        {description && (
          <DialogBody>
            <DialogDescription className="text-sm text-muted-foreground font-medium">
              {description}
            </DialogDescription>
          </DialogBody>
        )}

        <DialogFooter className={cn('bg-muted/50 px-6 pb-6 pt-4 border-t gap-3', footerClassName)}>
          <Button
            variant="ghost"
            size="xl"
            className={cn('px-6 font-semibold', cancelClassName)}
            onClick={() => {
              onCancel?.();
              setOpen(false);
            }}
          >
            {cancelText}
          </Button>
          <Button
            variant={variant === 'warning' ? 'destructive' : 'default'}
            onClick={handleConfirm}
            disabled={loading}
            size="xl"
            className={cn('px-6 font-semibold', confirmClassName)}
          >
            {loading && <Loader2 className="animate-spin h-4 w-4 mr-2" />}
            {confirmText}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
