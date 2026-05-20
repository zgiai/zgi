'use client';

import React from 'react';
import { Loader2 } from 'lucide-react';
import { toast } from 'sonner';

import { ApprovalCompletedState } from '@/components/workflow/approval/approval-completed-state';
import ApprovalRuntimeForm from '@/components/workflow/approval/approval-runtime-form';
import { Button } from '@/components/ui/button';
import { useApprovalForm, useSubmitApprovalForm, fetchApprovalEvents } from '@/hooks';
import { useT } from '@/i18n';
import { isApprovalFormAlreadySubmittedError } from '@/services/approval.service';

interface ApprovalFormPageClientProps {
  token: string;
}

type PageStatus = 'ready' | 'submitted' | 'finished' | 'error';

/**
 * @component ApprovalFormPageClient
 * @category Feature
 * @status Beta
 * @description Standalone token page for reviewing and submitting workflow approval forms.
 * @usage Rendered by /approval/[token].
 * @example
 * <ApprovalFormPageClient token={token} />
 */
export function ApprovalFormPageClient({ token }: ApprovalFormPageClientProps) {
  const t = useT('nodes');
  const formQuery = useApprovalForm(token);
  const submitMutation = useSubmitApprovalForm(token);
  const [status, setStatus] = React.useState<PageStatus>('ready');
  const [submittedAction, setSubmittedAction] = React.useState<string | null>(null);
  const [lastSequence, setLastSequence] = React.useState(0);

  const submit = React.useCallback(
    async (payload: { inputs: Record<string, unknown>; action: string }) => {
      setSubmittedAction(payload.action);
      try {
        await submitMutation.mutateAsync(payload);
        setStatus('submitted');
        toast.success(t('approval.runtime.submitted'));
      } catch (error) {
        setStatus('error');
        toast.error(error instanceof Error ? error.message : t('approval.runtime.submitFailed'));
      }
    },
    [submitMutation, t]
  );

  React.useEffect(() => {
    if (status !== 'submitted') return;
    let cancelled = false;
    const timer = window.setInterval(async () => {
      try {
        const events = await fetchApprovalEvents(token, { after: lastSequence, limit: 100 });
        if (cancelled || events.length === 0) return;
        const maxSeq = events.reduce((max, event) => {
          const seq = typeof event.sequence === 'number' ? event.sequence : max;
          return Math.max(max, seq);
        }, lastSequence);
        setLastSequence(maxSeq);
        if (events.some(event => event.event === 'workflow_finished')) {
          setStatus('finished');
          window.clearInterval(timer);
        }
      } catch {
        // Polling failures are transient; keep the submitted state visible.
      }
    }, 2000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [lastSequence, status, token]);

  const isFormAlreadyCompleted = isApprovalFormAlreadySubmittedError(formQuery.error);

  return (
    <main className="min-h-screen bg-background px-4 py-8 text-foreground">
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
        {formQuery.isLoading ? (
          <div className="flex min-h-[320px] items-center justify-center rounded-lg border">
            <Loader2 className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : status === 'finished' || isFormAlreadyCompleted ? (
          <ApprovalCompletedState />
        ) : formQuery.error || !formQuery.data ? (
          <div className="rounded-lg border p-6 text-center">
            <h1 className="text-lg font-semibold">{t('approval.runtime.loadFailed')}</h1>
            <p className="mt-2 text-sm text-muted-foreground">
              {formQuery.error instanceof Error
                ? formQuery.error.message
                : t('approval.runtime.loadFailedDescription')}
            </p>
            <Button className="mt-4" onClick={() => formQuery.refetch()}>
              {t('approval.runtime.retry')}
            </Button>
          </div>
        ) : status === 'submitted' ? (
          <div className="rounded-lg border p-6 text-center">
            <Loader2 className="mx-auto size-5 animate-spin text-muted-foreground" />
            <h1 className="mt-4 text-lg font-semibold">{t('approval.runtime.submitted')}</h1>
            <p className="mt-2 text-sm text-muted-foreground">
              {t('approval.runtime.waitingResume')}
            </p>
          </div>
        ) : (
          <div className="rounded-lg border bg-card p-5 shadow-sm">
            <ApprovalRuntimeForm
              form={formQuery.data}
              onSubmit={submit}
              isSubmitting={submitMutation.isPending}
              submittedAction={submittedAction}
            />
          </div>
        )}
      </div>
    </main>
  );
}
