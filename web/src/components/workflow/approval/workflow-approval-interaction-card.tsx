'use client';

import type { ReactNode } from 'react';
import { AlertCircle, Clock3, Loader2, Send, ShieldCheck } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { ApprovalRuntimeForm as ApprovalRuntimeFormData } from '@/services/approval.service';

import { ApprovalCompletedState } from './approval-completed-state';
import ApprovalRuntimeForm from './approval-runtime-form';

export type WorkflowApprovalInteractionMode =
  | 'form'
  | 'loading'
  | 'external'
  | 'submitted'
  | 'completed'
  | 'expired'
  | 'error';

interface WorkflowApprovalInteractionCardProps {
  mode: WorkflowApprovalInteractionMode;
  form?: ApprovalRuntimeFormData | null;
  error?: unknown;
  isSubmitting?: boolean;
  submittedAction?: string | null;
  onSubmit?: (payload: { inputs: Record<string, unknown>; action: string }) => void | Promise<void>;
  onRetry?: () => void;
  secondaryAction?: ReactNode;
  className?: string;
}

function CardFooter({ children }: { children?: ReactNode }) {
  if (!children) return null;
  return <div className="mt-4 flex justify-start border-t pt-3">{children}</div>;
}

/**
 * Shared approval presentation for Agent and Workflow runtimes. Tokens,
 * approval IDs and transport details deliberately stay outside this component.
 */
export function WorkflowApprovalInteractionCard({
  mode,
  form,
  error,
  isSubmitting = false,
  submittedAction,
  onSubmit,
  onRetry,
  secondaryAction,
  className,
}: WorkflowApprovalInteractionCardProps) {
  const t = useT();

  if (mode === 'completed' || mode === 'expired') {
    return (
      <ApprovalCompletedState
        compact
        variant={mode === 'expired' ? 'expired' : 'completed'}
        className={className}
      />
    );
  }

  if (mode === 'form' && form && onSubmit) {
    return (
      <div
        className={cn(
          'max-h-[45vh] overflow-y-auto rounded-xl border border-warning/30 bg-card p-3 shadow-sm',
          className
        )}
      >
        <div className="mb-3 flex items-start gap-2.5 px-1 text-sm">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-warning/15 text-warning-foreground">
            <ShieldCheck className="size-4" />
          </div>
          <div className="min-w-0 pt-0.5">
            <div className="font-medium text-foreground">
              {t('webapp.consoleChat.workflow.approvalPending')}
            </div>
            <div className="mt-1 text-xs leading-5 text-muted-foreground">
              {t('webapp.consoleChat.workflow.approvalInlineHint')}
            </div>
          </div>
        </div>
        <ApprovalRuntimeForm
          form={form}
          isSubmitting={isSubmitting}
          submittedAction={submittedAction}
          onSubmit={onSubmit}
          secondaryAction={secondaryAction}
        />
      </div>
    );
  }

  if (mode === 'error') {
    const message =
      error instanceof Error ? error.message : t('nodes.approval.runtime.loadFailedDescription');
    return (
      <div className={cn('rounded-xl border bg-card p-4 text-center shadow-sm', className)}>
        <div className="mx-auto flex size-10 items-center justify-center rounded-full bg-destructive/10 text-destructive ring-1 ring-destructive/20">
          <AlertCircle className="size-5" />
        </div>
        <div className="mt-3 text-sm font-medium text-foreground">
          {t('nodes.approval.runtime.loadFailed')}
        </div>
        <p className="mx-auto mt-1.5 max-w-md text-xs leading-5 text-muted-foreground">{message}</p>
        {onRetry ? (
          <Button type="button" size="sm" className="mt-3" onClick={onRetry}>
            {t('nodes.approval.runtime.retry')}
          </Button>
        ) : null}
        <CardFooter>{secondaryAction}</CardFooter>
      </div>
    );
  }

  const loading = mode === 'loading';
  const submitted = mode === 'submitted';
  const Icon = loading ? Loader2 : submitted ? Send : Clock3;
  const title = submitted
    ? t('nodes.approval.runtime.submitted')
    : loading
      ? t('webapp.consoleChat.workflow.loadingApprovalForm')
      : t('webapp.consoleChat.workflow.approvalPending');
  const description = submitted
    ? t('nodes.approval.runtime.waitingResume')
    : t('webapp.consoleChat.workflow.approvalConfiguredChannelHint');

  return (
    <div
      className={cn(
        'relative overflow-hidden rounded-xl border border-warning/30 bg-card px-5 py-5 text-center shadow-sm',
        className
      )}
    >
      <div className="mx-auto flex size-11 items-center justify-center rounded-full bg-warning/10 text-warning-foreground ring-1 ring-warning/20">
        <Icon className={cn('size-5', loading && 'animate-spin')} />
      </div>
      <div className="mt-3 text-sm font-semibold text-foreground">{title}</div>
      <p className="mx-auto mt-1.5 max-w-md text-xs leading-5 text-muted-foreground">
        {description}
      </p>
      {!loading ? (
        <div className="mt-3 inline-flex items-center gap-1.5 rounded-full border bg-muted/40 px-3 py-1 text-xs text-muted-foreground">
          <Clock3 className="size-3.5" />
          <span>{t('nodes.approval.runtime.waitingForReviewerStatus')}</span>
        </div>
      ) : null}
      <CardFooter>{secondaryAction}</CardFooter>
    </div>
  );
}

export default WorkflowApprovalInteractionCard;
