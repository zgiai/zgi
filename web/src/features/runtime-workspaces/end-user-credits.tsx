'use client';

import * as React from 'react';
import { Coins, Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { pointsFormat } from '@/features/runtime-workspaces/end-user-usage';
import { useRuntimeEndUserCredits } from '@/hooks/runtime-workspace/use-runtime-end-users';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { RuntimeCreditEntry } from '@/services/types/runtime-workspace';

function formatDate(value?: string, withTime = false): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: withTime ? 'short' : undefined,
  }).format(date);
}

function newExternalId() {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? `console-${crypto.randomUUID()}`
    : `console-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function entryKind(entry: RuntimeCreditEntry): 'usage' | 'takeBack' | 'grant' {
  if (entry.kind === 'usage') return 'usage';
  return entry.points < 0 ? 'takeBack' : 'grant';
}

/**
 * @component EndUserCreditsSection
 * @category Feature
 * @status Beta
 * @description Shows an end user's credits (ADR 0011): the balance, debt,
 * unexpired lots, and the ledger of grants and usage drawn beyond their
 * limits, and grants or takes back credits from the console.
 * @usage Rendered in the end-user detail sheet.
 * @example
 * <EndUserCreditsSection runtimeWorkspaceId={id} subjectId={subjectId} />
 */
export function EndUserCreditsSection({
  runtimeWorkspaceId,
  subjectId,
}: {
  runtimeWorkspaceId: string;
  subjectId: string;
}) {
  const t = useT('workspace');
  const { ledger, grant } = useRuntimeEndUserCredits(runtimeWorkspaceId, subjectId);
  const [points, setPoints] = React.useState('');
  const [reason, setReason] = React.useState('');
  const [until, setUntil] = React.useState('');
  // One ID per intended grant, so a repeated click records it once.
  const externalId = React.useRef(newExternalId());
  const amount = Math.round(Number.parseFloat(points) * 1000) / 1000;
  const valid = Number.isFinite(amount) && amount !== 0;
  const credits = ledger.data?.credits;
  const entries = ledger.data?.items ?? [];

  const submit = async () => {
    if (!valid) return;
    let expiresAt: string | undefined;
    if (amount > 0 && until) {
      const end = new Date(`${until}T23:59:59`);
      if (!Number.isNaN(end.getTime())) expiresAt = end.toISOString();
    }
    const result = await grant.mutateAsync({
      points: amount,
      external_id: externalId.current,
      reason: reason.trim() || undefined,
      expires_at: expiresAt,
    });
    externalId.current = newExternalId();
    setPoints('');
    setReason('');
    setUntil('');
    toast.success(
      t('runtime.credits.adjusted', { points: pointsFormat.format(result.credits.balance) })
    );
  };

  return (
    <section className="space-y-3">
      <div>
        <h3 className="text-sm font-medium">{t('runtime.credits.title')}</h3>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">
          {t('runtime.credits.description')}
        </p>
      </div>

      <div className="grid grid-cols-2 gap-2">
        <div className="rounded-lg border p-3">
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <Coins className="size-3.5" />
            {t('runtime.credits.balance')}
          </div>
          <div
            className={cn(
              'mt-1 text-base font-semibold tabular-nums',
              (credits?.balance ?? 0) < 0 && 'text-destructive'
            )}
          >
            {ledger.isLoading ? '—' : pointsFormat.format(credits?.balance ?? 0)}
          </div>
          {(credits?.debt ?? 0) > 0 ? (
            <div className="mt-0.5 text-xs text-destructive">
              {t('runtime.credits.debt', { points: pointsFormat.format(credits?.debt ?? 0) })}
            </div>
          ) : null}
        </div>
        <div className="rounded-lg border p-3">
          <div className="text-xs text-muted-foreground">{t('runtime.credits.lots')}</div>
          {(credits?.lots ?? []).length === 0 ? (
            <div className="mt-1 text-xs text-muted-foreground">{t('runtime.credits.noLots')}</div>
          ) : (
            <ul className="mt-1 space-y-0.5 text-xs">
              {(credits?.lots ?? []).map((lot, index) => (
                <li key={index} className="flex justify-between gap-2">
                  <span className="tabular-nums">{pointsFormat.format(lot.points)}</span>
                  <span className="text-muted-foreground">
                    {lot.expires_at
                      ? t('runtime.credits.expiresOn', { date: formatDate(lot.expires_at) })
                      : t('runtime.credits.noExpiry')}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>

      <div className="grid grid-cols-3 gap-2">
        <label className="space-y-1">
          <span className="text-xs text-muted-foreground">{t('runtime.credits.points')}</span>
          <Input
            type="number"
            step="any"
            inputMode="decimal"
            placeholder={t('runtime.credits.pointsPlaceholder')}
            value={points}
            onChange={event => setPoints(event.target.value)}
            className="h-9"
          />
        </label>
        <label className="space-y-1">
          <span className="text-xs text-muted-foreground">{t('runtime.credits.until')}</span>
          <Input
            type="date"
            disabled={!(amount > 0)}
            value={until}
            onChange={event => setUntil(event.target.value)}
            className="h-9"
          />
        </label>
        <label className="space-y-1">
          <span className="text-xs text-muted-foreground">{t('runtime.credits.reason')}</span>
          <Input
            maxLength={255}
            placeholder={t('runtime.credits.reasonPlaceholder')}
            value={reason}
            onChange={event => setReason(event.target.value)}
            className="h-9"
          />
        </label>
      </div>
      <div className="flex justify-end">
        <Button
          type="button"
          size="sm"
          disabled={!valid || grant.isPending}
          onClick={() => void submit()}
        >
          {grant.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
          {amount < 0 ? t('runtime.credits.takeBack') : t('runtime.credits.grant')}
        </Button>
      </div>

      {entries.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('runtime.credits.noEntries')}</p>
      ) : (
        <div className="divide-y rounded-lg border">
          {entries.slice(0, 20).map(entry => (
            <div key={entry.id} className="flex items-start gap-3 px-3 py-2 text-sm">
              <div className="min-w-0 flex-1">
                <div>{t(`runtime.credits.kinds.${entryKind(entry)}`)}</div>
                <div className="truncate text-xs text-muted-foreground">
                  {formatDate(entry.created_at, true)}
                  {entry.reason ? ` · ${entry.reason}` : ''}
                  {entry.expires_at
                    ? ` · ${t('runtime.credits.expiresOn', { date: formatDate(entry.expires_at) })}`
                    : ''}
                </div>
              </div>
              <div className="text-right tabular-nums">
                <div className={entry.points < 0 ? 'text-destructive' : 'text-success'}>
                  {entry.points > 0 ? '+' : ''}
                  {pointsFormat.format(entry.points)}
                </div>
                <div className="text-xs text-muted-foreground">
                  {t('runtime.credits.balanceAfter', {
                    points: pointsFormat.format(entry.balance_after),
                  })}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
