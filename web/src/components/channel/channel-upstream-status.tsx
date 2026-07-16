'use client';

import { AlertTriangle, CircleCheck, CircleHelp, Gauge } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import type { UpstreamState } from '@/services/types/channel';
import { cn } from '@/lib/utils';

interface ChannelUpstreamStatusProps {
  state?: UpstreamState;
  provider?: string;
}

function formatAmount(value: string): string {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return value;
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 4 }).format(numeric);
}

export function ChannelUpstreamStatus({
  state,
  provider,
}: ChannelUpstreamStatusProps): JSX.Element {
  const t = useT('channels');
  const status = (() => {
    if (!state) {
      return { key: 'unknown', icon: CircleHelp, tone: 'text-muted-foreground bg-muted' };
    }
    if (state.block_reason === 'billing_unavailable') {
      return { key: 'billingUnavailable', icon: AlertTriangle, tone: 'text-red-700 bg-red-50' };
    }
    if (state.block_reason === 'quota_exhausted') {
      return { key: 'quotaExhausted', icon: AlertTriangle, tone: 'text-red-700 bg-red-50' };
    }
    if (state.availability === 'invalid_key') {
      return { key: 'invalidKey', icon: AlertTriangle, tone: 'text-red-700 bg-red-50' };
    }
    if (state.availability === 'exhausted') {
      return { key: 'exhausted', icon: AlertTriangle, tone: 'text-red-700 bg-red-50' };
    }
    if (state.balance_capability === 'permission_denied' && state.availability === 'unknown') {
      return { key: 'permissionDenied', icon: CircleHelp, tone: 'text-muted-foreground bg-muted' };
    }
    if (state.balance_capability === 'unsupported' && state.availability === 'unknown') {
      return { key: 'unsupported', icon: CircleHelp, tone: 'text-muted-foreground bg-muted' };
    }
    if (state.is_stale || state.last_check_status === 'failed') {
      return { key: 'stale', icon: CircleHelp, tone: 'text-amber-700 bg-amber-50' };
    }
    if (state.is_low) {
      return { key: 'low', icon: AlertTriangle, tone: 'text-amber-700 bg-amber-50' };
    }
    if (state.balance_capability === 'unknown' && state.availability === 'unknown') {
      return { key: 'unknown', icon: CircleHelp, tone: 'text-muted-foreground bg-muted' };
    }
    return { key: 'available', icon: CircleCheck, tone: 'text-emerald-700 bg-emerald-50' };
  })();
  const Icon = status.icon;
  const scopeKey =
    state?.balance_capability === 'unsupported'
      ? 'availability'
      : state?.balance_scope === 'key_limit' || provider?.toLowerCase() === 'openrouter'
        ? 'keyLimit'
        : 'accountBalance';
  const balances = state?.is_unlimited
    ? t('upstream.unlimited')
    : state?.balances
        ?.filter(item => item.remaining !== undefined)
        .map(item => `${formatAmount(item.remaining ?? '')} ${item.currency}`)
        .join(' / ');

  return (
    <div className="border-y py-2.5 text-xs">
      <div className="flex min-w-0 items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2 text-muted-foreground">
          <Gauge className="h-3.5 w-3.5 shrink-0" />
          <span className="truncate">{t('upstream.title')}</span>
        </div>
        <Badge
          variant="secondary"
          className={cn('h-5 shrink-0 rounded-sm px-1.5 text-[10px] font-medium', status.tone)}
        >
          <Icon className="mr-1 h-3 w-3" />
          {t(`upstream.status.${status.key}` as never)}
        </Badge>
      </div>
      <div className="mt-1.5 flex min-w-0 items-center justify-between gap-3">
        <span className="shrink-0 text-muted-foreground">
          {t(`upstream.scope.${scopeKey}` as never)}
        </span>
        <span className="min-w-0 truncate text-right font-medium text-foreground">
          {balances || t(`upstream.detail.${status.key}` as never)}
        </span>
      </div>
      {(state?.availability_observed_at || state?.balance_observed_at) && (
        <div className="mt-1 truncate text-right text-[11px] text-muted-foreground">
          {state.is_stale ? t('upstream.stalePrefix') : t('upstream.observedPrefix')}{' '}
          {new Date(
            state.availability_observed_at ?? state.balance_observed_at ?? ''
          ).toLocaleString()}
        </div>
      )}
      {state?.cooldown_until && state.would_guard && (
        <div className="mt-1 truncate text-right text-[11px] text-muted-foreground">
          {t('upstream.retryAfter', { time: new Date(state.cooldown_until).toLocaleString() })}
        </div>
      )}
      {state?.provider_error_code && (
        <div className="mt-1 truncate text-right font-mono text-[11px] text-muted-foreground">
          {t('upstream.errorCode', { code: state.provider_error_code })}
        </div>
      )}
      {(state?.shared_channel_count ?? 0) > 1 && (
        <div className="mt-1 text-[11px] leading-4 text-amber-700">
          {t('upstream.sharedCredential', { count: state?.shared_channel_count ?? 0 })}
        </div>
      )}
    </div>
  );
}
