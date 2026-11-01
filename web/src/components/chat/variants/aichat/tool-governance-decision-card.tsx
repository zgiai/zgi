'use client';

import {
  createContext,
  useContext,
  useEffect,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from 'react';
import { CheckCircle2, ChevronDown, Loader2, ShieldAlert } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { isAIChatContinuationLikelyStarted } from '@/components/chat/variants/aichat/error-utils';

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

export interface ToolGovernancePendingApproval {
  id: string;
  conversationId?: string;
  messageId?: string;
  title: string;
  toolLabel?: string | null;
  actionSentence: string;
  assets: ToolGovernanceDisplayAsset[];
  riskLabel?: string | null;
  permissionLabel?: string | null;
  canSubmit: boolean;
  isHighImpact: boolean;
  /**
   * Whether the host can remember the approval for the session. A one-shot
   * host (a DigitalHuman confirmation) sets false; the option is then hidden.
   */
  allowRememberForSession?: boolean;
  createdAt?: number;
  onSubmitDecision?: (
    action: ToolGovernanceDecisionAction,
    rememberForSession: boolean
  ) => void | Promise<void>;
}

interface PendingApprovalEntry {
  approval: ToolGovernancePendingApproval;
  sequence: number;
}

/**
 * The approvals waiting in one chat shell. Each shell creates its own and it
 * goes away with the shell, so approvals, dismissals and subscribers never
 * leak between chats, conversations or mounts (ADR 0050).
 */
export interface ToolGovernancePendingApprovalScope {
  publish: (approval: ToolGovernancePendingApproval) => () => void;
  dismiss: (approval: ToolGovernancePendingApproval) => () => void;
  isDismissed: (approvalId: string) => boolean;
  subscribe: (listener: () => void) => () => void;
  getSnapshot: () => ToolGovernancePendingApproval | null;
  /**
   * Changes when a dismissal is made or undone, for readers of isDismissed.
   * Publishing does not change it: the timeline publishes on every render, and
   * a reader that re-rendered on each publish would loop.
   */
  getVersion: () => number;
}

export function createToolGovernancePendingApprovalScope(): ToolGovernancePendingApprovalScope {
  const entries = new Map<string, PendingApprovalEntry>();
  const dismissed = new Set<string>();
  const listeners = new Set<() => void>();
  let sequence = 0;
  let version = 0;
  let snapshot: ToolGovernancePendingApproval | null = null;

  const emit = () => {
    const latest = Array.from(entries.values()).sort((left, right) => {
      const leftTime = left.approval.createdAt ?? 0;
      const rightTime = right.approval.createdAt ?? 0;
      if (leftTime !== rightTime) return rightTime - leftTime;
      return right.sequence - left.sequence;
    })[0];
    snapshot = latest?.approval ?? null;
    listeners.forEach(listener => listener());
  };

  const scope: ToolGovernancePendingApprovalScope = {
    publish: approval => {
      if (dismissed.has(approval.id)) return () => undefined;
      const entrySequence = (sequence += 1);
      entries.set(approval.id, { approval, sequence: entrySequence });
      emit();
      return () => {
        if (entries.get(approval.id)?.sequence !== entrySequence) return;
        entries.delete(approval.id);
        emit();
      };
    },
    dismiss: approval => {
      dismissed.add(approval.id);
      entries.delete(approval.id);
      version += 1;
      emit();
      return () => {
        dismissed.delete(approval.id);
        version += 1;
        scope.publish(approval);
      };
    },
    isDismissed: approvalId => dismissed.has(approvalId),
    subscribe: listener => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    getSnapshot: () => snapshot,
    getVersion: () => version,
  };
  return scope;
}

// Components rendered outside a chat shell share this one.
const defaultPendingApprovalScope = createToolGovernancePendingApprovalScope();
const ToolGovernancePendingApprovalScopeContext = createContext(defaultPendingApprovalScope);

export function ToolGovernancePendingApprovalScopeProvider({
  children,
  scope,
}: {
  children: ReactNode;
  scope: ToolGovernancePendingApprovalScope;
}) {
  return (
    <ToolGovernancePendingApprovalScopeContext.Provider value={scope}>
      {children}
    </ToolGovernancePendingApprovalScopeContext.Provider>
  );
}

export function useToolGovernancePendingApprovalScope() {
  return useContext(ToolGovernancePendingApprovalScopeContext);
}

export function publishToolGovernancePendingApproval(
  approval: ToolGovernancePendingApproval,
  scope: ToolGovernancePendingApprovalScope = defaultPendingApprovalScope
) {
  return scope.publish(approval);
}

export function dismissToolGovernancePendingApproval(
  approval: ToolGovernancePendingApproval,
  scope: ToolGovernancePendingApprovalScope = defaultPendingApprovalScope
) {
  return scope.dismiss(approval);
}

export function isToolGovernancePendingApprovalDismissed(
  approvalId: string,
  scope: ToolGovernancePendingApprovalScope = defaultPendingApprovalScope
) {
  return scope.isDismissed(approvalId);
}

export function useActiveToolGovernancePendingApproval() {
  const scope = useToolGovernancePendingApprovalScope();
  return useSyncExternalStore(scope.subscribe, scope.getSnapshot, () => null);
}

/** Re-renders when the scope's approvals or dismissals change. */
export function useToolGovernancePendingApprovalVersion(scope: ToolGovernancePendingApprovalScope) {
  return useSyncExternalStore(scope.subscribe, scope.getVersion, () => 0);
}

interface ToolGovernanceDecisionCardProps {
  submissionKey?: string;
  title: string;
  toolLabel?: string | null;
  actionSentence?: string | null;
  notice?: string | null;
  reason?: string;
  assets: ToolGovernanceDisplayAsset[];
  summaryRows: ToolGovernanceDisplayRow[];
  details: ToolGovernanceDisplayRow[];
  needsApproval: boolean;
  approvalStatus?: string | null;
  isHighImpact: boolean;
  isAllowed: boolean;
  canSubmit: boolean;
  compactAudit?: boolean;
  onSubmitDecision?: (
    action: ToolGovernanceDecisionAction,
    rememberForSession: boolean
  ) => void | Promise<void>;
}

export function ToolGovernanceDecisionCard({
  submissionKey,
  title,
  toolLabel,
  actionSentence,
  notice,
  reason,
  assets,
  summaryRows,
  details,
  needsApproval,
  approvalStatus,
  isHighImpact,
  isAllowed,
  canSubmit,
  compactAudit = false,
  onSubmitDecision,
}: ToolGovernanceDecisionCardProps) {
  const t = useT('webapp');
  void toolLabel;
  const [rememberForSession, setRememberForSession] = useState(false);
  const [submittingAction, setSubmittingAction] = useState<ToolGovernanceDecisionAction | null>(
    null
  );
  const [resolvedAction, setResolvedAction] = useState<ToolGovernanceDecisionAction | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [isOpen, setIsOpen] = useState(needsApproval);
  const effectiveApprovalStatus =
    approvalStatus ??
    (resolvedAction === 'approve' ? 'approved' : resolvedAction === 'reject' ? 'rejected' : null);
  const effectiveNeedsApproval = needsApproval && !effectiveApprovalStatus;
  const canExpand =
    summaryRows.length > 0 ||
    assets.length > 0 ||
    details.length > 0 ||
    Boolean(reason) ||
    effectiveNeedsApproval ||
    Boolean(effectiveApprovalStatus);
  const submitEnabled =
    canSubmit &&
    Boolean(onSubmitDecision) &&
    !submittingAction &&
    !effectiveApprovalStatus;
  const actionsUnavailable =
    effectiveNeedsApproval && !submittingAction && (!canSubmit || !onSubmitDecision)
      ? t('consoleChat.governance.actionsUnavailable')
      : null;
  const approvalStatusLabel =
    effectiveApprovalStatus === 'approved'
      ? t('consoleChat.governance.approved')
      : effectiveApprovalStatus === 'rejected'
        ? t('consoleChat.governance.rejected')
        : null;

  useEffect(() => {
    setRememberForSession(false);
    setSubmittingAction(null);
    setSubmitError(null);
    setResolvedAction(null);
  }, [submissionKey, needsApproval, approvalStatus]);

  const submitDecision = async (action: ToolGovernanceDecisionAction) => {
    if (!submitEnabled || !onSubmitDecision) return;
    setSubmittingAction(action);
    setSubmitError(null);
    try {
      await onSubmitDecision(action, action === 'approve' ? rememberForSession : false);
      setResolvedAction(action);
    } catch (error) {
      if (isAIChatContinuationLikelyStarted(error)) {
        setResolvedAction(action);
        return;
      }
      const message =
        error instanceof Error && error.message
          ? error.message
          : t('consoleChat.governance.submitFailed');
      setSubmitError(message);
      setResolvedAction(null);
    } finally {
      setSubmittingAction(null);
    }
  };

  if (compactAudit && !effectiveNeedsApproval) {
    const auditText = actionSentence || title;
    const isApproved = effectiveApprovalStatus === 'approved' || isAllowed;
    const isRejected = effectiveApprovalStatus === 'rejected';

    return (
      <div
        className={cn(
          'flex min-h-8 w-full min-w-0 items-center gap-2 rounded-md border px-2.5 py-1.5 text-xs',
          isRejected
            ? 'border-destructive/25 bg-destructive/5'
            : isApproved
              ? 'border-emerald-500/25 bg-emerald-500/5'
              : isHighImpact
                ? 'border-destructive/25 bg-destructive/5'
                : 'border-warning/30 bg-warning/5'
        )}
      >
        <span
          className={cn(
            'flex size-5 shrink-0 items-center justify-center rounded-full border bg-background',
            isRejected || (!isApproved && isHighImpact)
              ? 'border-destructive/35 text-destructive'
              : isApproved
                ? 'border-emerald-500/30 text-emerald-600'
                : 'border-warning/40 text-warning'
          )}
        >
          {isRejected || (!isApproved && !effectiveApprovalStatus) ? (
            <ShieldAlert className="size-3.5" />
          ) : (
            <CheckCircle2 className="size-3.5" />
          )}
        </span>
        <span className="min-w-0 flex-1 truncate font-medium">{auditText}</span>
      </div>
    );
  }

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
          {notice ? (
            <div
              className={cn(
                'rounded-md border p-2 leading-relaxed text-muted-foreground',
                isHighImpact
                  ? 'border-destructive/20 bg-destructive/5'
                  : 'border-border bg-muted/30'
              )}
            >
              {notice}
            </div>
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
          {effectiveNeedsApproval ? (
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

export function ToolGovernanceApprovalPanel({
  approval,
}: {
  approval: ToolGovernancePendingApproval;
}) {
  const t = useT('webapp');
  const pendingApprovalScope = useToolGovernancePendingApprovalScope();
  const [rememberForSession, setRememberForSession] = useState(false);
  const [submittingAction, setSubmittingAction] = useState<ToolGovernanceDecisionAction | null>(
    null
  );
  const [resolvedAction, setResolvedAction] = useState<ToolGovernanceDecisionAction | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const allowRememberForSession = approval.allowRememberForSession !== false;
  const submitEnabled =
    approval.canSubmit &&
    Boolean(approval.onSubmitDecision) &&
    !submittingAction &&
    !resolvedAction;
  const resolvedLabel =
    resolvedAction === 'approve'
      ? t('consoleChat.governance.approved')
      : resolvedAction === 'reject'
        ? t('consoleChat.governance.rejected')
        : null;
  const visibleAssets = approval.assets.slice(0, 6);
  const hiddenAssetCount = Math.max(0, approval.assets.length - visibleAssets.length);
  const riskPermissionText =
    approval.riskLabel && approval.permissionLabel
      ? t('consoleChat.governance.approvalPanel.riskAndPermission', {
          risk: approval.riskLabel,
          permission: approval.permissionLabel,
        })
      : approval.riskLabel
        ? t('consoleChat.governance.approvalPanel.riskOnly', { risk: approval.riskLabel })
        : approval.permissionLabel
          ? t('consoleChat.governance.approvalPanel.permissionOnly', {
              permission: approval.permissionLabel,
            })
          : null;

  useEffect(() => {
    setRememberForSession(false);
    setSubmittingAction(null);
    setSubmitError(null);
    setResolvedAction(null);
  }, [approval.id]);

  const submitDecision = async (action: ToolGovernanceDecisionAction) => {
    if (!submitEnabled || !approval.onSubmitDecision) return;
    setSubmittingAction(action);
    setSubmitError(null);
    let restoreDismissedApproval: (() => void) | null = null;
    try {
      setResolvedAction(action);
      restoreDismissedApproval = dismissToolGovernancePendingApproval(
        approval,
        pendingApprovalScope
      );
      await approval.onSubmitDecision(
        action,
        action === 'approve' && allowRememberForSession ? rememberForSession : false
      );
    } catch (error) {
      if (isAIChatContinuationLikelyStarted(error)) {
        return;
      }
      restoreDismissedApproval?.();
      const message =
        error instanceof Error && error.message
          ? error.message
          : t('consoleChat.governance.submitFailed');
      setSubmitError(message);
      setResolvedAction(null);
    } finally {
      setSubmittingAction(null);
    }
  };

  return (
    <div
      className={cn(
        'rounded-xl border p-3 shadow-sm',
        approval.isHighImpact
          ? 'border-destructive/35 bg-destructive/5'
          : 'border-warning/35 bg-warning/5'
      )}
    >
      <div className="flex items-start gap-2.5">
        <span
          className={cn(
            'mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full border bg-background',
            approval.isHighImpact
              ? 'border-destructive/35 text-destructive'
              : 'border-warning/40 text-warning'
          )}
        >
          <ShieldAlert className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <div className="text-xs font-medium text-muted-foreground">
              {t('consoleChat.governance.approvalPanel.title')}
            </div>
          </div>
          <div className="mt-1 break-words text-sm font-medium text-foreground">
            {approval.actionSentence}
          </div>
          {riskPermissionText ? (
            <div className="mt-1.5 text-[11px] text-muted-foreground">
              <span className="truncate">{riskPermissionText}</span>
            </div>
          ) : null}
        </div>
      </div>

      {approval.assets.length > 0 ? (
        <div className="mt-3 rounded-md border bg-background/75 p-2">
          <div className="mb-1.5 text-[11px] font-medium text-muted-foreground">
            {t('consoleChat.governance.approvalPanel.assets')}
          </div>
          <div className="space-y-1.5">
            {visibleAssets.map(asset => (
              <div key={asset.key} className="min-w-0 rounded-sm bg-muted/45 px-2 py-1">
                <div className="truncate text-xs font-medium text-foreground">{asset.name}</div>
                {asset.meta ? (
                  <div className="mt-0.5 truncate text-[11px] text-muted-foreground">
                    {asset.meta}
                  </div>
                ) : null}
              </div>
            ))}
            {hiddenAssetCount > 0 ? (
              <div className="px-2 text-[11px] text-muted-foreground">
                {t('consoleChat.governance.approvalPanel.moreAssets', {
                  count: hiddenAssetCount,
                })}
              </div>
            ) : null}
          </div>
        </div>
      ) : null}

      <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
        {allowRememberForSession ? (
          <label className="flex min-w-0 items-center gap-2 text-[11px] text-muted-foreground">
            <Checkbox
              checked={rememberForSession}
              onCheckedChange={checked => setRememberForSession(checked === true)}
              disabled={Boolean(submittingAction || resolvedAction)}
            />
            <span className="min-w-0 break-words">
              {t('consoleChat.governance.approvalPanel.rememberForSession')}
            </span>
          </label>
        ) : null}
        <div className="ml-auto flex shrink-0 items-center gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={!submitEnabled}
            onClick={() => void submitDecision('reject')}
          >
            {submittingAction === 'reject' ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : null}
            {t('consoleChat.governance.approvalPanel.reject')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant={approval.isHighImpact ? 'destructive' : 'default'}
            disabled={!submitEnabled}
            onClick={() => void submitDecision('approve')}
          >
            {submittingAction === 'approve' ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : null}
            {t('consoleChat.governance.approvalPanel.approve')}
          </Button>
        </div>
      </div>
      {resolvedLabel ? (
        <div className="mt-2 text-[11px] text-muted-foreground">{resolvedLabel}</div>
      ) : null}
      {submitError ? <div className="mt-2 text-[11px] text-destructive">{submitError}</div> : null}
      {!resolvedLabel && !submitEnabled && !submittingAction ? (
        <div className="mt-2 text-[11px] text-muted-foreground">
          {t('consoleChat.governance.actionsUnavailable')}
        </div>
      ) : null}
    </div>
  );
}
