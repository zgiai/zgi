'use client';

import * as React from 'react';
import Link from 'next/link';
import { AlertTriangle, ArrowRight, Ban, Gauge, SlidersHorizontal, Users } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
import { EndUserQuotaCard } from '@/features/runtime-workspaces/end-user-quota-card';
import { RuntimePageHeader } from '@/features/runtime-workspaces/runtime-page-header';
import {
  endUserRatio,
  pointsFormat,
  quotaWindows,
  UsageTrendChart,
} from '@/features/runtime-workspaces/end-user-usage';
import {
  useRuntimeEndUserSummary,
  useRuntimeEndUserUsageSeries,
} from '@/hooks/runtime-workspace/use-runtime-end-users';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { RuntimeApp } from '@/services/types/runtime-workspace';

const endUsersPath = '/console/workspace/runtime-workspaces/end-users';
const trendRanges = [7, 30, 90] as const;

function StatTile({
  label,
  value,
  hint,
  icon: Icon,
  tone,
  href,
}: {
  label: string;
  value: React.ReactNode;
  hint?: React.ReactNode;
  icon?: React.ComponentType<{ className?: string }>;
  tone?: 'danger' | 'warning';
  href?: string;
}) {
  const body = (
    <Card
      className={cn(
        'h-full border-border/80 transition-colors',
        href && 'hover:border-primary/40 hover:bg-muted/30'
      )}
    >
      <CardContent className="flex h-full flex-col gap-1 p-4">
        <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
          <span>{label}</span>
          {Icon ? (
            <Icon
              className={cn(
                'size-4',
                tone === 'danger' && 'text-destructive',
                tone === 'warning' && 'text-warning'
              )}
            />
          ) : null}
        </div>
        <div
          className={cn(
            'text-2xl font-semibold tabular-nums',
            tone === 'danger' && value !== 0 && 'text-destructive',
            tone === 'warning' && value !== 0 && 'text-warning'
          )}
        >
          {value}
        </div>
        {hint ? <div className="text-xs text-muted-foreground">{hint}</div> : null}
      </CardContent>
    </Card>
  );
  return href ? (
    <Link
      href={href}
      className="block rounded-xl focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      {body}
    </Link>
  ) : (
    body
  );
}

/**
 * @component RuntimeUsageOverview
 * @category Feature
 * @status Beta
 * @description How a Runtime Workspace's end users spend (ADR 0010): points
 * per window, how many are near or over their limits, daily usage by app,
 * and the end users who spend the most. Each count links to the filtered
 * end-user list.
 * @usage Rendered on the Runtime Workspace overview.
 * @example
 * <RuntimeUsageOverview runtimeWorkspaceId={id} apps={apps} />
 */
export function RuntimeUsageOverview({
  runtimeWorkspaceId,
  apps,
}: {
  runtimeWorkspaceId: string;
  apps: RuntimeApp[];
}) {
  const t = useT('workspace');
  const [days, setDays] = React.useState<(typeof trendRanges)[number]>(30);
  const summary = useRuntimeEndUserSummary(runtimeWorkspaceId);
  const series = useRuntimeEndUserUsageSeries(runtimeWorkspaceId, null, days);
  const appNames = React.useMemo(() => new Map(apps.map(app => [app.agent_id, app.name])), [apps]);
  const counts = summary.data?.end_users;
  const top = summary.data?.top ?? [];
  const topMax = Math.max(...top.map(user => user.usage[2]?.points ?? 0), 0);

  return (
    <section className="space-y-4">
      <div className="flex items-end justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">{t('runtime.usage.title')}</h2>
          <p className="mt-1 max-w-3xl text-sm leading-6 text-muted-foreground">
            {t('runtime.usage.description')}
          </p>
        </div>
        <Link
          href={endUsersPath}
          className="flex shrink-0 items-center gap-1 text-sm text-primary hover:underline"
        >
          {t('runtime.usage.viewEndUsers')}
          <ArrowRight className="size-3.5" />
        </Link>
      </div>

      {summary.isLoading ? (
        <div className="grid grid-cols-2 gap-3 @3xl/console:grid-cols-4">
          {Array.from({ length: 8 }).map((_, index) => (
            <Skeleton key={index} className="h-24" />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-3 @3xl/console:grid-cols-4">
          {quotaWindows.map((window, index) => {
            const usage = summary.data?.usage.find(item => item.window === window);
            return (
              <StatTile
                key={window}
                label={t('runtime.usage.pointsIn', {
                  window: t(`runtime.quota.windows.${window}`),
                })}
                value={pointsFormat.format(usage?.points ?? 0)}
                hint={t('runtime.usage.usersWithUsage', { count: usage?.end_users ?? 0 })}
                icon={index === 0 ? Gauge : undefined}
              />
            );
          })}
          <StatTile
            label={t('runtime.usage.endUsers')}
            value={counts?.total ?? 0}
            hint={t('runtime.usage.active7d', { count: counts?.active_7d ?? 0 })}
            icon={Users}
            href={endUsersPath}
          />
          <StatTile
            label={t('runtime.endUsers.quotaState.over')}
            value={counts?.over_quota ?? 0}
            hint={t('runtime.usage.overHint')}
            icon={AlertTriangle}
            tone="danger"
            href={`${endUsersPath}?quota_state=over`}
          />
          <StatTile
            label={t('runtime.endUsers.quotaState.near')}
            value={counts?.near_quota ?? 0}
            hint={t('runtime.usage.nearHint')}
            icon={AlertTriangle}
            tone="warning"
            href={`${endUsersPath}?quota_state=near`}
          />
          <StatTile
            label={t('runtime.endUsers.status.suspended')}
            value={counts?.suspended ?? 0}
            hint={t('runtime.usage.suspendedHint')}
            icon={Ban}
            href={`${endUsersPath}?status=suspended`}
          />
          <StatTile
            label={t('runtime.endUsers.quotaState.custom')}
            value={counts?.custom_quota ?? 0}
            hint={t('runtime.usage.customHint')}
            icon={SlidersHorizontal}
            href={`${endUsersPath}?quota_state=custom`}
          />
        </div>
      )}

      <div className="grid gap-5 @5xl/console:grid-cols-[minmax(0,5fr)_minmax(0,3fr)]">
        <Card className="border-border/80">
          <CardContent className="space-y-4 p-5">
            <div className="flex flex-col gap-3 @2xl/console:flex-row @2xl/console:items-start @2xl/console:justify-between">
              <div>
                <h3 className="text-base font-semibold">{t('runtime.usage.trend')}</h3>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t('runtime.usage.trendDescription')}
                </p>
              </div>
              <ToggleGroup
                type="single"
                variant="outline"
                value={String(days)}
                onValueChange={value => {
                  if (value) setDays(Number(value) as (typeof trendRanges)[number]);
                }}
              >
                {trendRanges.map(range => (
                  <ToggleGroupItem key={range} value={String(range)} className="px-3 text-xs">
                    {t('runtime.usage.lastDays', { count: range })}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </div>
            {series.isLoading ? (
              <Skeleton className="h-60 w-full" />
            ) : (
              <UsageTrendChart series={series.data} appNames={appNames} />
            )}
          </CardContent>
        </Card>

        <Card className="border-border/80">
          <CardContent className="space-y-4 p-5">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h3 className="text-base font-semibold">{t('runtime.usage.top')}</h3>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t('runtime.usage.topDescription')}
                </p>
              </div>
              <Link
                href={`${endUsersPath}?sort=usage_30d`}
                className="flex shrink-0 items-center gap-1 text-sm text-primary hover:underline"
              >
                {t('runtime.usage.viewAll')}
                <ArrowRight className="size-3.5" />
              </Link>
            </div>
            {top.length === 0 ? (
              <p className="py-8 text-center text-sm text-muted-foreground">
                {t('runtime.usage.noTop')}
              </p>
            ) : (
              <ol className="space-y-3">
                {top.map((user, index) => {
                  const month = user.usage.find(item => item.window === '30d');
                  const ratio = endUserRatio(user);
                  return (
                    <li key={user.subject_id}>
                      <Link
                        href={`${endUsersPath}?user=${encodeURIComponent(user.subject_id)}`}
                        className="group flex items-center gap-3 rounded-md px-1 py-0.5 hover:bg-muted/40"
                      >
                        <span className="w-5 shrink-0 text-right text-xs tabular-nums text-muted-foreground">
                          {index + 1}
                        </span>
                        <div className="min-w-0 flex-1 space-y-1">
                          <div className="flex items-center justify-between gap-2 text-sm">
                            <span className="truncate font-medium group-hover:underline">
                              {user.external_user_id ||
                                `${t('runtime.endUsers.unnamed')} · ${user.subject_id.slice(0, 8)}`}
                            </span>
                            <span className="shrink-0 tabular-nums">
                              {pointsFormat.format(month?.points ?? 0)}
                            </span>
                          </div>
                          <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
                            <div
                              className="h-full rounded-full bg-chart-1"
                              style={{
                                width: `${topMax > 0 ? Math.max(2, ((month?.points ?? 0) / topMax) * 100) : 0}%`,
                              }}
                            />
                          </div>
                        </div>
                        {ratio !== null && ratio >= 0.8 ? (
                          <span
                            className={cn(
                              'shrink-0 text-xs font-medium',
                              ratio >= 1 ? 'text-destructive' : 'text-warning'
                            )}
                          >
                            {Math.round(ratio * 100)}%
                          </span>
                        ) : null}
                      </Link>
                    </li>
                  );
                })}
              </ol>
            )}
          </CardContent>
        </Card>
      </div>
    </section>
  );
}

/**
 * @component RuntimeQuotaTab
 * @category Feature
 * @status Beta
 * @description The rules end users are held to (ADR 0010, ADR 0011): the
 * default limits, per-app caps, and plans, with how many end users hold
 * each plan.
 * @usage Rendered as the Quotas & plans section of the Runtime Workspace console.
 * @example
 * <RuntimeQuotaTab runtimeWorkspaceId={id} apps={apps} policyJson={policy} onSavePolicy={save} saving={false} />
 */
export function RuntimeQuotaTab({
  runtimeWorkspaceId,
  apps,
  policyJson,
  onSavePolicy,
  saving,
}: {
  runtimeWorkspaceId: string;
  apps: RuntimeApp[];
  policyJson?: string;
  onSavePolicy: (policy: Record<string, unknown>) => Promise<void>;
  saving: boolean;
}) {
  const t = useT('workspace');
  const summary = useRuntimeEndUserSummary(runtimeWorkspaceId);
  return (
    <section className="space-y-5">
      <RuntimePageHeader
        title={t('runtime.tabs.quota')}
        description={t('runtime.quota.pageDescription')}
      />
      <div className="max-w-4xl">
        <EndUserQuotaCard
          policyJson={policyJson}
          apps={apps.filter(app => app.status === 'active')}
          planCounts={Object.fromEntries(
            (summary.data?.plans ?? []).map(plan => [plan.key, plan.end_users])
          )}
          saving={saving}
          onSave={onSavePolicy}
        />
      </div>
    </section>
  );
}
