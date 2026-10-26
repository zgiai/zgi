'use client';

import * as React from 'react';
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  RuntimeEndUser,
  RuntimeEndUserPatch,
  RuntimeEndUserPlan,
  RuntimeEndUserQuota,
  RuntimeQuotaWindow,
  RuntimeUsageSeries,
  RuntimeWindowUsage,
} from '@/services/types/runtime-workspace';

export const quotaWindows: RuntimeQuotaWindow[] = ['24h', '7d', '30d'];

/** The share of a limit from which an end user is near it. */
export const nearQuotaRatio = 0.8;

export const pointsFormat = new Intl.NumberFormat(undefined, { maximumFractionDigits: 3 });

const compactPoints = new Intl.NumberFormat(undefined, {
  notation: 'compact',
  maximumFractionDigits: 1,
});

/** The share of its limit a usage has spent, or null without a limit. */
export function usageRatio(usage?: RuntimeWindowUsage): number | null {
  if (!usage || usage.limit === undefined || usage.limit <= 0) return null;
  return usage.points / usage.limit;
}

/** The largest share of any limit an end user has spent. */
export function endUserRatio(user: RuntimeEndUser): number | null {
  const ratios = user.usage.map(usageRatio).filter((value): value is number => value !== null);
  return ratios.length > 0 ? Math.max(...ratios) : null;
}

function ratioTone(ratio: number) {
  if (ratio >= 1) return 'bg-destructive';
  if (ratio >= nearQuotaRatio) return 'bg-warning';
  return 'bg-primary';
}

/**
 * @component UsageMeter
 * @category Feature
 * @status Beta
 * @description Shows points spent in a quota window and, when a limit
 * applies, a bar of the share spent that turns amber near the limit and red
 * once it is reached.
 * @usage Used in the end-user list and detail.
 * @example
 * <UsageMeter usage={{ window: '24h', points: 3, limit: 10, remaining: 7 }} />
 */
export function UsageMeter({
  usage,
  align = 'right',
  className,
}: {
  usage?: RuntimeWindowUsage;
  align?: 'left' | 'right';
  className?: string;
}) {
  if (!usage) return <span className="text-muted-foreground">—</span>;
  const ratio = usageRatio(usage);
  return (
    <div className={cn('min-w-24 space-y-1', align === 'right' && 'text-right', className)}>
      <div className="whitespace-nowrap tabular-nums">
        <span className={ratio !== null && ratio >= 1 ? 'font-medium text-destructive' : undefined}>
          {pointsFormat.format(usage.points)}
        </span>
        {usage.limit !== undefined ? (
          <span className="text-muted-foreground"> / {pointsFormat.format(usage.limit)}</span>
        ) : null}
      </div>
      {ratio !== null ? (
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
          <div
            className={cn('h-full rounded-full transition-[width]', ratioTone(ratio))}
            style={{ width: `${Math.min(100, Math.max(ratio * 100, usage.points > 0 ? 2 : 0))}%` }}
          />
        </div>
      ) : null}
    </div>
  );
}

const seriesColors = [
  'var(--chart-1)',
  'var(--chart-2)',
  'var(--chart-3)',
  'var(--chart-4)',
  'var(--chart-5)',
];
const otherColor = 'var(--muted-foreground)';
const maxSeriesApps = 5;

/**
 * @component UsageTrendChart
 * @category Feature
 * @status Beta
 * @description Daily usage of end users: points as bars stacked by app, with
 * apps beyond the five largest and usage without an app grouped; or the
 * daily count of active end users or model calls.
 * @usage Used on the quota page and in the end-user detail.
 * @example
 * <UsageTrendChart series={series} appNames={names} />
 */
export type UsageMetric = 'points' | 'end_users' | 'calls';

export function UsageTrendChart({
  series,
  appNames,
  height = 240,
  metric = 'points',
}: {
  series?: RuntimeUsageSeries;
  appNames: Map<string, string>;
  height?: number;
  /** Points are stacked by app; counts are one series. */
  metric?: UsageMetric;
}) {
  const t = useT('workspace');
  const { rows, keys } = React.useMemo(() => {
    const days = series?.days ?? [];
    if (metric !== 'points') {
      return {
        rows: days.map(day => ({ date: day.date, total: day[metric], [metric]: day[metric] })),
        keys: [{ key: metric, name: t(`runtime.usage.metrics.${metric}`), color: seriesColors[0] }],
      };
    }
    const totals = new Map<string, number>();
    for (const day of days) {
      for (const [appId, points] of Object.entries(day.apps)) {
        totals.set(appId, (totals.get(appId) ?? 0) + points);
      }
    }
    const ranked = [...totals.entries()].sort((a, b) => b[1] - a[1]).map(([appId]) => appId);
    const shown = ranked.slice(0, maxSeriesApps);
    const keys = shown.map((appId, index) => ({
      key: appId,
      name:
        appNames.get(appId) ??
        series?.apps.find(app => app.app_id === appId)?.app_key ??
        appId.slice(0, 8),
      color: seriesColors[index % seriesColors.length],
    }));
    const rows = days.map(day => {
      const row: Record<string, number | string> = { date: day.date, total: day.points };
      let attributed = 0;
      for (const appId of shown) {
        row[appId] = day.apps[appId] ?? 0;
        attributed += day.apps[appId] ?? 0;
      }
      row.other = Math.max(0, Math.round((day.points - attributed) * 1000) / 1000);
      return row;
    });
    if (rows.some(row => Number(row.other) > 0)) {
      keys.push({
        key: 'other',
        name:
          ranked.length > maxSeriesApps
            ? t('runtime.usage.otherApps')
            : t('runtime.usage.unattributed'),
        color: otherColor,
      });
    }
    return { rows, keys };
  }, [series, appNames, metric, t]);

  const formatDay = (value: string) => value.slice(5).replace('-', '/');

  return (
    <div className="space-y-3">
      <div style={{ height }}>
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={rows} margin={{ top: 4, right: 4, bottom: 0, left: -12 }}>
            <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="3 3" />
            <XAxis
              dataKey="date"
              tickFormatter={formatDay}
              tickLine={false}
              axisLine={false}
              fontSize={11}
              minTickGap={16}
              stroke="var(--muted-foreground)"
            />
            <YAxis
              tickFormatter={value =>
                Number(value) >= 1000
                  ? compactPoints.format(Number(value))
                  : pointsFormat.format(Number(value))
              }
              tickLine={false}
              axisLine={false}
              fontSize={11}
              width={48}
              stroke="var(--muted-foreground)"
            />
            <Tooltip
              cursor={{ fill: 'var(--muted)', opacity: 0.5 }}
              content={({ active, payload, label }) => {
                if (!active || !payload?.length) return null;
                const total = Number(payload[0]?.payload?.total ?? 0);
                return (
                  <div className="min-w-40 rounded-md border bg-popover px-3 py-2 text-xs shadow-md">
                    <div className="mb-1 flex justify-between gap-4 font-medium">
                      <span>{String(label)}</span>
                      <span className="tabular-nums">{pointsFormat.format(total)}</span>
                    </div>
                    {payload
                      .filter(item => Number(item.value) > 0)
                      .reverse()
                      .map(item => (
                        <div key={String(item.dataKey)} className="flex justify-between gap-4">
                          <span className="flex min-w-0 items-center gap-1.5">
                            <span
                              className="size-2 shrink-0 rounded-sm"
                              style={{ background: item.color }}
                            />
                            <span className="truncate text-muted-foreground">{item.name}</span>
                          </span>
                          <span className="tabular-nums">
                            {pointsFormat.format(Number(item.value))}
                          </span>
                        </div>
                      ))}
                  </div>
                );
              }}
            />
            {keys.map((item, index) => (
              <Bar
                key={item.key}
                dataKey={item.key}
                name={item.name}
                stackId="points"
                fill={item.color}
                radius={index === keys.length - 1 ? [3, 3, 0, 0] : 0}
                maxBarSize={28}
              />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </div>
      {keys.length > 0 ? (
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
          {keys.map(item => (
            <span key={item.key} className="flex items-center gap-1.5">
              <span className="size-2 rounded-sm" style={{ background: item.color }} />
              {item.name}
            </span>
          ))}
        </div>
      ) : null}
    </div>
  );
}

export type QuotaOverrideMode = 'default' | 'custom' | 'unlimited';

export interface QuotaOverrideDraft {
  mode: QuotaOverrideMode;
  limits: Partial<Record<RuntimeQuotaWindow, string>>;
}

export function quotaDraftOf(user?: RuntimeEndUser | null): QuotaOverrideDraft {
  const limits: QuotaOverrideDraft['limits'] = {};
  for (const limit of user?.quota.limits ?? []) limits[limit.window] = String(limit.points);
  const source = user?.quota_source;
  return { mode: source === 'custom' || source === 'unlimited' ? source : 'default', limits };
}

/** The quota a draft sets: null returns to the workspace limits. */
export function quotaOfDraft(draft: QuotaOverrideDraft): RuntimeEndUserQuota | null | undefined {
  if (draft.mode === 'default') return null;
  if (draft.mode === 'unlimited') return { unlimited: true };
  const limits = quotaWindows.flatMap(window => {
    const value = Math.round(Number.parseFloat(draft.limits[window] ?? '') * 1000) / 1000;
    return Number.isFinite(value) && value >= 0 ? [{ window, points: value }] : [];
  });
  return limits.length > 0 ? { limits } : undefined;
}

/**
 * @component QuotaOverrideEditor
 * @category Feature
 * @status Beta
 * @description Chooses whether end users follow the workspace limits, have
 * their own, or have none, and edits their own limits per window.
 * @usage Used in the end-user detail and the bulk quota dialog.
 * @example
 * <QuotaOverrideEditor value={draft} onChange={setDraft} />
 */
export function QuotaOverrideEditor({
  value,
  onChange,
}: {
  value: QuotaOverrideDraft;
  onChange: (next: QuotaOverrideDraft) => void;
}) {
  const t = useT('workspace');
  return (
    <div className="space-y-3">
      <ToggleGroup
        type="single"
        variant="outline"
        value={value.mode}
        onValueChange={mode => {
          if (mode) onChange({ ...value, mode: mode as QuotaOverrideMode });
        }}
        className="justify-start"
      >
        {(['default', 'custom', 'unlimited'] as const).map(item => (
          <ToggleGroupItem key={item} value={item} className="px-3 text-xs">
            {t(`runtime.endUsers.quotaMode.${item}`)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      {value.mode === 'custom' ? (
        <div className="grid grid-cols-3 gap-2">
          {quotaWindows.map(window => (
            <label key={window} className="space-y-1">
              <span className="text-xs text-muted-foreground">
                {t(`runtime.quota.windows.${window}`)}
              </span>
              <Input
                type="number"
                min={0}
                step="any"
                inputMode="decimal"
                placeholder={t('runtime.quota.unlimited')}
                value={value.limits[window] ?? ''}
                onChange={event =>
                  onChange({ ...value, limits: { ...value.limits, [window]: event.target.value } })
                }
                className="h-9"
              />
            </label>
          ))}
        </div>
      ) : null}
    </div>
  );
}

/** The plans of a Runtime Workspace policy. */
export function parsePlans(policyJson?: string): RuntimeEndUserPlan[] {
  try {
    const parsed = JSON.parse(policyJson || '{}') as {
      end_user_quota?: { plans?: RuntimeEndUserPlan[] };
    };
    return Array.isArray(parsed?.end_user_quota?.plans) ? parsed.end_user_quota.plans : [];
  } catch {
    return [];
  }
}

export function planLabel(plan: { key: string; name?: string }) {
  return plan.name?.trim() || plan.key;
}

/**
 * @component PlanBadge
 * @category Feature
 * @status Beta
 * @description Shows the plan an end user holds, muted and struck through
 * once it expired or was removed from the policy.
 * @usage Used in the end-user list and detail.
 * @example
 * <PlanBadge plan={{ key: 'pro', name: 'Pro', active: true }} />
 */
export function PlanBadge({ plan }: { plan?: RuntimeEndUser['plan'] }) {
  const t = useT('workspace');
  if (!plan) return null;
  return (
    <Badge
      variant="outline"
      title={plan.active ? undefined : t('runtime.plans.inactive')}
      className={cn(
        'h-5 px-1.5 text-[11px] font-medium',
        plan.active
          ? 'border-primary/40 bg-primary/5 text-primary'
          : 'text-muted-foreground line-through'
      )}
    >
      {planLabel(plan)}
    </Badge>
  );
}

export interface PlanDraft {
  key: string;
  /** The last day the plan applies, as yyyy-mm-dd; empty never expires. */
  until: string;
}

function dateInputValue(value?: string) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 10);
}

export function planDraftOf(user?: RuntimeEndUser | null): PlanDraft {
  return { key: user?.plan?.key ?? '', until: dateInputValue(user?.plan?.expires_at) };
}

/** The plan a draft sets; it applies through the end of its last day. */
export function planOfDraft(draft: PlanDraft): RuntimeEndUserPatch['plan'] {
  if (!draft.key) return null;
  if (!draft.until) return { key: draft.key };
  const end = new Date(`${draft.until}T23:59:59`);
  return Number.isNaN(end.getTime())
    ? { key: draft.key }
    : { key: draft.key, expires_at: end.toISOString() };
}

/**
 * @component PlanAssignmentEditor
 * @category Feature
 * @status Beta
 * @description Picks a plan of the workspace for end users, or none, and the
 * last day it applies.
 * @usage Used in the end-user detail and the bulk plan dialog.
 * @example
 * <PlanAssignmentEditor plans={plans} value={draft} onChange={setDraft} />
 */
export function PlanAssignmentEditor({
  plans,
  value,
  onChange,
}: {
  plans: RuntimeEndUserPlan[];
  value: PlanDraft;
  onChange: (next: PlanDraft) => void;
}) {
  const t = useT('workspace');
  const today = dateInputValue(new Date().toISOString());
  const known = plans.some(plan => plan.key === value.key);
  return (
    <div className="grid grid-cols-2 gap-2">
      <label className="space-y-1">
        <span className="text-xs text-muted-foreground">{t('runtime.plans.plan')}</span>
        <Select
          value={value.key || 'none'}
          onValueChange={key => onChange({ ...value, key: key === 'none' ? '' : key })}
        >
          <SelectTrigger className="h-9">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="none">{t('runtime.plans.none')}</SelectItem>
            {plans.map(plan => (
              <SelectItem key={plan.key} value={plan.key}>
                {planLabel(plan)}
              </SelectItem>
            ))}
            {value.key && !known ? (
              <SelectItem value={value.key} disabled>
                {value.key}
              </SelectItem>
            ) : null}
          </SelectContent>
        </Select>
      </label>
      <label className="space-y-1">
        <span className="text-xs text-muted-foreground">{t('runtime.plans.until')}</span>
        <Input
          type="date"
          min={today}
          disabled={!value.key}
          value={value.until}
          onChange={event => onChange({ ...value, until: event.target.value })}
          className="h-9"
        />
      </label>
    </div>
  );
}
