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
  variant?: 'default' | 'warning' | 'danger';
  contentClassName?: string;
  footerClassName?: string;
  cancelClassName?: string;
  confirmClassName?: string;
  titleClassName?: string;
  descriptionClassName?: string;
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
  titleClassName,
  descriptionClassName,
}: ConfirmDialogProps) {
  const [openInternal, setOpenInternal] = useState(false);
  const open = openProp !== undefined ? openProp : openInternal;
  const setOpen = onOpenChangeProp !== undefined ? onOpenChangeProp : setOpenInternal;
  const isDanger = variant === 'danger';

  // Handle confirm action then close dialog
  const handleConfirm = () => {
    onConfirm();
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent
        size="sm"
        className={cn('p-0 overflow-hidden', isDanger && 'max-w-md rounded-2xl', contentClassName)}
      >
        <DialogHeader>
          <DialogTitle className={cn('text-xl font-bold tracking-tight', titleClassName)}>
            {title}
          </DialogTitle>
        </DialogHeader>

        {description && (
          <DialogBody>
            <DialogDescription
              className={cn('text-sm text-muted-foreground font-medium', descriptionClassName)}
            >
              {description}
            </DialogDescription>
          </DialogBody>
        )}

        <DialogFooter
          className={cn(
            'bg-muted/50 px-6 pb-6 pt-4 border-t gap-3',
            isDanger && 'justify-end border-t-0 bg-white px-6 py-5',
            footerClassName
          )}
        >
          <Button
            variant={isDanger ? 'outline' : 'ghost'}
            size="xl"
            className={cn(
              'px-6 font-semibold',
              isDanger && 'rounded-xl border-slate-200 bg-white hover:bg-slate-50',
              cancelClassName
            )}
            onClick={() => {
              onCancel?.();
              setOpen(false);
            }}
          >
            {cancelText}
          </Button>
          <Button
            variant={variant === 'warning' ? 'destructive' : isDanger ? 'outline' : 'default'}
            onClick={handleConfirm}
            disabled={loading}
            size="xl"
            className={cn(
              'px-6 font-semibold',
              isDanger &&
                'rounded-xl border-red-200 bg-red-50 text-red-600 hover:border-red-200 hover:bg-red-100 hover:text-red-700',
              confirmClassName
            )}
          >
            {loading && <Loader2 className="animate-spin h-4 w-4 mr-2" />}
            {confirmText}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
