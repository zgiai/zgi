import { Clock3, Loader2, Send } from 'lucide-react';

import { useT } from '@/i18n';

interface ApprovalWaitingStateProps {
  loading?: boolean;
  submitted?: boolean;
}

export function ApprovalWaitingState({
  loading = false,
  submitted = false,
}: ApprovalWaitingStateProps) {
  const t = useT();
  const Icon = loading ? Loader2 : Send;

  return (
    <div className="relative overflow-hidden rounded-xl border bg-card px-5 py-5 text-center shadow-sm">
      <div className="mx-auto flex size-11 items-center justify-center rounded-full bg-amber-500/10 text-amber-600 ring-1 ring-amber-500/20">
        <Icon className={loading ? 'size-5 animate-spin' : 'size-5'} />
      </div>
      <div className="mt-3 text-sm font-semibold text-foreground">
        {submitted
          ? t('nodes.approval.runtime.submitted')
          : loading
            ? t('nodes.approval.runtime.paused')
            : t('nodes.approval.runtime.requestSubmitted')}
      </div>
      <p className="mx-auto mt-1.5 max-w-md text-xs leading-5 text-muted-foreground">
        {submitted
          ? t('nodes.approval.runtime.waitingResume')
          : t('nodes.approval.runtime.waitingForReviewer')}
      </p>
      <div className="mt-3 inline-flex items-center gap-1.5 rounded-full border bg-muted/40 px-3 py-1 text-xs text-muted-foreground">
        <Clock3 className="size-3.5" />
        <span>{t('nodes.approval.runtime.waitingForReviewerStatus')}</span>
      </div>
    </div>
  );
}
