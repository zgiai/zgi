'use client';

import {
  createContext,
  useContext,
  useEffect,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from 'react';
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

export interface ToolGovernancePendingApproval {
  id: string;
  title: string;
  toolLabel?: string | null;
  actionSentence: string;
  assets: ToolGovernanceDisplayAsset[];
  riskLabel?: string | null;
  permissionLabel?: string | null;
  canSubmit: boolean;
  isHighImpact: boolean;
  createdAt?: number;
  onSubmitDecision?: (
    action: ToolGovernanceDecisionAction,
    rememberForSession: boolean
  ) => void | Promise<void>;
}

interface PendingApprovalEntry {
  approval: ToolGovernancePendingApproval;
  sequence: number;
  scopeId: string;
}

type PendingApprovalSubscriber = () => void;

const pendingApprovalEntries = new Map<string, PendingApprovalEntry>();
const pendingApprovalSubscribers = new Set<PendingApprovalSubscriber>();
let pendingApprovalSequence = 0;
const pendingApprovalSnapshots = new Map<string, ToolGovernancePendingApproval | null>();
const DEFAULT_PENDING_APPROVAL_SCOPE_ID = 'default';
const ToolGovernancePendingApprovalScopeContext = createContext(
  DEFAULT_PENDING_APPROVAL_SCOPE_ID
);

function resolvePendingApprovalSnapshot(scopeId: string): ToolGovernancePendingApproval | null {
  const entries = Array.from(pendingApprovalEntries.values()).filter(
    entry => entry.scopeId === scopeId
  );
  if (entries.length === 0) return null;
  entries.sort((left, right) => {
    const leftTime = left.approval.createdAt ?? 0;
    const rightTime = right.approval.createdAt ?? 0;
    if (leftTime !== rightTime) return rightTime - leftTime;
    return right.sequence - left.sequence;
  });
  return entries[0].approval;
}

function emitPendingApprovalChange(scopeId: string) {
  pendingApprovalSnapshots.set(scopeId, resolvePendingApprovalSnapshot(scopeId));
  pendingApprovalSubscribers.forEach(listener => listener());
}

function subscribePendingApproval(listener: PendingApprovalSubscriber) {
  pendingApprovalSubscribers.add(listener);
  return () => {
    pendingApprovalSubscribers.delete(listener);
  };
}

export function ToolGovernancePendingApprovalScopeProvider({
  children,
  scopeId,
}: {
  children: ReactNode;
  scopeId: string;
}) {
  return (
    <ToolGovernancePendingApprovalScopeContext.Provider value={scopeId}>
      {children}
    </ToolGovernancePendingApprovalScopeContext.Provider>
  );
}

export function useToolGovernancePendingApprovalScope() {
  return useContext(ToolGovernancePendingApprovalScopeContext);
}

export function publishToolGovernancePendingApproval(
  approval: ToolGovernancePendingApproval,
  scopeId = DEFAULT_PENDING_APPROVAL_SCOPE_ID
) {
  const sequence = (pendingApprovalSequence += 1);
  pendingApprovalEntries.set(`${scopeId}:${approval.id}`, { approval, sequence, scopeId });
  emitPendingApprovalChange(scopeId);
  return () => {
    const entryKey = `${scopeId}:${approval.id}`;
    if (pendingApprovalEntries.get(entryKey)?.sequence !== sequence) return;
    pendingApprovalEntries.delete(entryKey);
    emitPendingApprovalChange(scopeId);
  };
}

export function useActiveToolGovernancePendingApproval() {
  const scopeId = useToolGovernancePendingApprovalScope();
  return useSyncExternalStore(
    subscribePendingApproval,
    () => pendingApprovalSnapshots.get(scopeId) ?? null,
    () => null
  );
}

interface ToolGovernanceDecisionCardProps {
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

  if (compactAudit && !needsApproval) {
    const auditText = actionSentence || title;
    const isApproved = approvalStatus === 'approved' || isAllowed;
    const isRejected = approvalStatus === 'rejected';

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
          {isRejected || (!isApproved && !approvalStatus) ? (
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

export function ToolGovernanceApprovalPanel({
  approval,
}: {
  approval: ToolGovernancePendingApproval;
}) {
  const t = useT('webapp');
  const [rememberForSession, setRememberForSession] = useState(false);
  const [submittingAction, setSubmittingAction] = useState<ToolGovernanceDecisionAction | null>(
    null
  );
  const [submitError, setSubmitError] = useState<string | null>(null);
  const submitEnabled =
    approval.canSubmit && Boolean(approval.onSubmitDecision) && !submittingAction;
  const visibleAssets = approval.assets.slice(0, 3);
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
  }, [approval.id]);

  const submitDecision = async (action: ToolGovernanceDecisionAction) => {
    if (!submitEnabled || !approval.onSubmitDecision) return;
    setSubmittingAction(action);
    setSubmitError(null);
    try {
      await approval.onSubmitDecision(action, action === 'approve' ? rememberForSession : false);
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
            <div className="mt-2 inline-flex max-w-full rounded-md border bg-background/80 px-2 py-1 text-[11px] text-muted-foreground">
              <span className="truncate">{riskPermissionText}</span>
            </div>
          ) : null}
        </div>
      </div>

      <div className="mt-3 rounded-md border bg-background/75 p-2">
        <div className="mb-1.5 text-[11px] font-medium text-muted-foreground">
          {t('consoleChat.governance.approvalPanel.assets')}
        </div>
        {visibleAssets.length > 0 ? (
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
        ) : (
          <div className="text-xs text-muted-foreground">
            {t('consoleChat.governance.approvalPanel.noAssets')}
          </div>
        )}
      </div>

      <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
        <label className="flex min-w-0 items-center gap-2 text-[11px] text-muted-foreground">
          <Checkbox
            checked={rememberForSession}
            onCheckedChange={checked => setRememberForSession(checked === true)}
            disabled={Boolean(submittingAction)}
          />
          <span className="min-w-0 break-words">
            {t('consoleChat.governance.approvalPanel.rememberForSession')}
          </span>
        </label>
        <div className="flex shrink-0 items-center gap-2">
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
      {submitError ? <div className="mt-2 text-[11px] text-destructive">{submitError}</div> : null}
      {!submitEnabled && !submittingAction ? (
        <div className="mt-2 text-[11px] text-muted-foreground">
          {t('consoleChat.governance.actionsUnavailable')}
        </div>
      ) : null}
    </div>
  );
}
