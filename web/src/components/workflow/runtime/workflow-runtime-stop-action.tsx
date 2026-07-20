'use client';

import { Loader2, Square } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

interface WorkflowRuntimeStopActionProps {
  onStop?: () => void | Promise<void>;
  isStopping?: boolean;
  disabled?: boolean;
  className?: string;
}

/**
 * A low-emphasis destructive action for approval and question cards.
 * The confirmation keeps stopping the whole run visually separate from
 * choosing an approval branch or answering a question.
 */
export function WorkflowRuntimeStopAction({
  onStop,
  isStopping = false,
  disabled = false,
  className,
}: WorkflowRuntimeStopActionProps) {
  const t = useT();

  if (!onStop) return null;

  return (
    <ConfirmDialog
      variant="danger"
      title={t('nodes.runtimeControl.stopTitle')}
      description={t('nodes.runtimeControl.stopDescription')}
      confirmText={t('nodes.runtimeControl.stopConfirm')}
      cancelText={t('nodes.runtimeControl.stopCancel')}
      loading={isStopping}
      onConfirm={() => void onStop()}
      trigger={
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={disabled || isStopping}
          className={cn(
            'h-8 border-destructive/20 bg-transparent px-2.5 text-destructive shadow-none',
            'hover:border-destructive/30 hover:bg-destructive/5 hover:text-destructive',
            className
          )}
        >
          {isStopping ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : (
            <Square className="size-3.5" />
          )}
          {isStopping ? t('nodes.runtimeControl.stopping') : t('nodes.runtimeControl.stopAction')}
        </Button>
      }
    />
  );
}
