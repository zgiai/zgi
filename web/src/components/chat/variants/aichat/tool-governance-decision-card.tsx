'use client';

import { useState } from 'react';
import { toast } from 'sonner';
import { CheckCircle2, ChevronDown, Loader2, ShieldAlert } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';

export type ToolGovernanceDecisionAction = 'approve' | 'reject';

export interface ToolGovernanceDisplayRow {
  key: string;
  label: string;
  value: string;
}

export interface ToolGovernanceDisplayAsset {
  key: string;
  name: string;
  meta?: string;
}

interface ToolGovernanceDecisionCardProps {
  title: string;
  toolLabel?: string | null;
  reason?: string;
  assets: ToolGovernanceDisplayAsset[];
  summaryRows: ToolGovernanceDisplayRow[];
  details: ToolGovernanceDisplayRow[];
  needsApproval: boolean;
  approvalStatus?: string | null;
  isHighImpact: boolean;
  isAllowed: boolean;
  canSubmit: boolean;
  onSubmitDecision?: (
    action: ToolGovernanceDecisionAction,
    rememberForSession: boolean
  ) => void | Promise<void>;
}

export function ToolGovernanceDecisionCard({
  title,
  toolLabel,
  reason,
  assets,
  summaryRows,
  details,
  needsApproval,
  approvalStatus,
  isHighImpact,
  isAllowed,
  canSubmit,
  onSubmitDecision,
}: ToolGovernanceDecisionCardProps) {
  const t = useT('webapp');
  const [rememberForSession, setRememberForSession] = useState(false);
  const [submittingAction, setSubmittingAction] = useState<ToolGovernanceDecisionAction | null>(
    null
  );
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [isOpen, setIsOpen] = useState(needsApproval);
  const canExpand =
    summaryRows.length > 0 ||
    assets.length > 0 ||
    details.length > 0 ||
    Boolean(reason) ||
    needsApproval ||
    Boolean(approvalStatus);
  const submitEnabled = canSubmit && Boolean(onSubmitDecision) && !submittingAction;
  const actionsUnavailable =
    needsApproval && !submittingAction && (!canSubmit || !onSubmitDecision)
      ? t('consoleChat.governance.actionsUnavailable')
      : null;
  const approvalStatusLabel =
    approvalStatus === 'approved'
      ? t('consoleChat.governance.approved')
      : approvalStatus === 'rejected'
        ? t('consoleChat.governance.rejected')
        : null;

  const submitDecision = async (action: ToolGovernanceDecisionAction) => {
    if (!submitEnabled || !onSubmitDecision) return;
    setSubmittingAction(action);
    setSubmitError(null);
    try {
      await onSubmitDecision(action, action === 'approve' ? rememberForSession : false);
      toast.success(
        action === 'approve'
          ? t('consoleChat.governance.approveSucceeded')
          : t('consoleChat.governance.rejectSucceeded')
      );
    } catch (error) {
      const message =
        error instanceof Error && error.message
          ? error.message
          : t('consoleChat.governance.submitFailed');
      setSubmitError(message);
      toast.error(message);
    } finally {
      setSubmittingAction(null);
    }
  };

  return (
    <div
      className={cn(
        'rounded-md border text-xs text-foreground',
        isHighImpact
          ? 'border-destructive/35 bg-destructive/10'
          : isAllowed
            ? 'border-emerald-500/30 bg-emerald-500/5'
            : 'border-warning/40 bg-warning/10'
      )}
    >
      <button
        type="button"
        className="flex min-h-8 w-full min-w-0 items-center gap-2 px-2.5 py-1.5 text-left"
        onClick={() => canExpand && setIsOpen(open => !open)}
        aria-expanded={isOpen}
      >
        <span
          className={cn(
            'flex size-5 shrink-0 items-center justify-center rounded-full border bg-background',
            isHighImpact
              ? 'border-destructive/40 text-destructive'
              : isAllowed
                ? 'border-emerald-500/30 text-emerald-600'
                : 'border-warning/40 text-warning'
          )}
        >
          {isAllowed ? <CheckCircle2 className="size-3.5" /> : <ShieldAlert className="size-3.5" />}
        </span>
        <span className="min-w-0 flex-1 truncate font-medium">{title}</span>
        {toolLabel ? (
          <span className="max-w-44 shrink-0 truncate text-muted-foreground">{toolLabel}</span>
        ) : null}
        {canExpand ? (
          <ChevronDown
            className={cn('size-3.5 shrink-0 text-muted-foreground transition-transform', {
              'rotate-180': isOpen,
            })}
          />
        ) : null}
      </button>
      {isOpen ? (
        <div
          className={cn(
            'space-y-2 border-t bg-background/70 px-2.5 py-2',
            isHighImpact
              ? 'border-destructive/15'
              : isAllowed
                ? 'border-emerald-500/15'
                : 'border-warning/20'
          )}
        >
          {summaryRows.length > 0 ? (
            <dl className="grid gap-1.5 rounded-md bg-background/80 p-2 text-[11px] sm:grid-cols-2">
              {summaryRows.map(row => (
                <div key={row.key} className="min-w-0">
                  <dt className="text-muted-foreground">{row.label}</dt>
                  <dd className="mt-0.5 truncate font-medium text-foreground">{row.value}</dd>
                </div>
              ))}
            </dl>
          ) : null}
          {reason ? (
            <div className="rounded-md border border-warning/20 bg-warning/5 p-2 text-muted-foreground">
              <span className="font-medium text-foreground">
                {t('consoleChat.governance.fields.reason')}
              </span>
              <span className="ml-1 break-words">{reason}</span>
            </div>
          ) : null}
          {assets.length > 0 ? (
            <div className="rounded-md bg-background/80 p-2 text-[11px]">
              <div className="mb-1 font-medium text-foreground">
                {t('consoleChat.governance.fields.assets')}
              </div>
              <div className="space-y-1.5">
                {assets.map(asset => (
                  <div key={asset.key} className="min-w-0 rounded-sm bg-muted/40 px-2 py-1">
                    <div className="truncate font-medium text-foreground">{asset.name}</div>
                    {asset.meta ? (
                      <div className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">
                        {asset.meta}
                      </div>
                    ) : null}
                  </div>
                ))}
              </div>
            </div>
          ) : null}
          {details.length > 0 ? (
            <dl className="grid gap-1 rounded-md bg-background/80 p-2 text-[11px]">
              {details.map(row => (
                <div key={row.key} className="grid grid-cols-[104px_minmax(0,1fr)] gap-2">
                  <dt className="text-muted-foreground">{row.label}</dt>
                  <dd className="min-w-0 max-h-40 overflow-auto whitespace-pre-wrap break-all font-mono text-foreground/80">
                    {row.value}
                  </dd>
                </div>
              ))}
            </dl>
          ) : null}
          {needsApproval ? (
            <div className="space-y-2">
              <label className="flex items-center gap-2 text-[11px] text-muted-foreground">
                <Checkbox
                  checked={rememberForSession}
                  onCheckedChange={checked => setRememberForSession(checked === true)}
                  disabled={Boolean(submittingAction)}
                />
                <span>{t('consoleChat.governance.rememberForSession')}</span>
              </label>
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  size="xs"
                  variant="outline"
                  disabled={!submitEnabled}
                  onClick={() => void submitDecision('approve')}
                >
                  {submittingAction === 'approve' ? (
                    <Loader2 className="mr-1 size-3 animate-spin" />
                  ) : null}
                  {t('consoleChat.governance.approve')}
                </Button>
                <Button
                  type="button"
                  size="xs"
                  variant="outline"
                  disabled={!submitEnabled}
                  onClick={() => void submitDecision('reject')}
                >
                  {submittingAction === 'reject' ? (
                    <Loader2 className="mr-1 size-3 animate-spin" />
                  ) : null}
                  {t('consoleChat.governance.reject')}
                </Button>
              </div>
              {submitError ? (
                <div className="text-[11px] text-destructive">{submitError}</div>
              ) : null}
              {actionsUnavailable ? (
                <div className="text-[11px] text-muted-foreground">{actionsUnavailable}</div>
              ) : null}
            </div>
          ) : approvalStatusLabel ? (
            <div className="text-[11px] text-muted-foreground">{approvalStatusLabel}</div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
