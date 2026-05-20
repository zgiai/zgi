'use client';

import { CheckCircle2, Clock3 } from 'lucide-react';

import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

interface ApprovalCompletedStateProps {
  className?: string;
  compact?: boolean;
  variant?: 'completed' | 'expired';
}

/**
 * @component ApprovalCompletedState
 * @category Feature
 * @status Beta
 * @description Shared completed-state presentation for approval forms that have already been handled.
 * @usage Render when the approval form load API reports that the approval has already finished.
 * @example
 * <ApprovalCompletedState compact />
 */
export function ApprovalCompletedState({
  className,
  compact = false,
  variant = 'completed',
}: ApprovalCompletedStateProps) {
  const t = useT();
  const TitleTag = compact ? 'div' : 'h1';
  const Icon = variant === 'expired' ? Clock3 : CheckCircle2;

  return (
    <div
      className={cn(
        'relative overflow-hidden rounded-xl border bg-card text-center shadow-sm',
        compact ? 'px-4 py-5' : 'mx-auto w-full max-w-xl px-6 py-10',
        className
      )}
    >
      <div
        className={cn(
        'mx-auto flex items-center justify-center rounded-full ring-1',
        variant === 'expired'
          ? 'bg-amber-500/10 text-amber-600 ring-amber-500/20'
          : 'bg-primary/10 text-primary ring-primary/20',
        compact ? 'size-10' : 'size-12'
      )}
    >
        <Icon className={compact ? 'size-5' : 'size-6'} />
      </div>
      <TitleTag
        className={cn(
          'mt-4 font-semibold tracking-normal text-foreground',
          compact ? 'text-base' : 'text-xl'
        )}
      >
        {t(
          variant === 'expired'
            ? 'nodes.approval.runtime.expired'
            : 'nodes.approval.runtime.alreadyCompleted'
        )}
      </TitleTag>
      <p
        className={cn(
          'mx-auto mt-2 max-w-sm leading-6 text-muted-foreground',
          compact ? 'text-xs' : 'text-sm'
        )}
      >
        {t(
          variant === 'expired'
            ? 'nodes.approval.runtime.expiredDescription'
            : 'nodes.approval.runtime.finishedDescription'
        )}
      </p>
    </div>
  );
}
