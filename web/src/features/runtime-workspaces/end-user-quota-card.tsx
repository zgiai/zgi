'use client';

import * as React from 'react';
import Link from 'next/link';
import { Crown, Gauge, Loader2, Pencil, Plus, RotateCcw, Trash2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { AppCapsEditor } from '@/features/runtime-workspaces/app-caps-editor';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { RuntimeApp } from '@/services/types/runtime-workspace';

const windows = ['24h', '7d', '30d'] as const;
type QuotaWindow = (typeof windows)[number];
type WindowPoints = Partial<Record<QuotaWindow, string>>;
const planKeyPattern = /^[a-z0-9][a-z0-9_-]{0,63}$/;

interface QuotaLimit {
  window: QuotaWindow;
  points: number;
}

interface AppQuota {
  app_key: string;
  limits: QuotaLimit[];
}

interface PlanQuota {
  key: string;
  name?: string;
  limits: QuotaLimit[];
  apps?: AppQuota[];
}

interface EndUserQuota {
  limits?: QuotaLimit[];
  apps?: AppQuota[];
  plans?: PlanQuota[];
}

interface PlanDraft {
  id: string;
  key: string;
  name: string;
  limits: WindowPoints;
  /** Saved plans keep their key: end users refer to it. */
  saved: boolean;
  /** Lift the per-app caps for this plan, or keep the workspace's. */
  noAppCaps: boolean;
  /** Per-app caps of the plan set through the API, kept as they are. */
  apps?: AppQuota[];
}

function parsePolicy(policyJson: string | undefined): Record<string, unknown> {
  try {
    const parsed = JSON.parse(policyJson || '{}');
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {};
  } catch {
    return {};
  }
}

function toWindowPoints(limits: QuotaLimit[] | undefined): WindowPoints {
  const result: WindowPoints = {};
  for (const limit of limits ?? []) {
    if (windows.includes(limit.window)) result[limit.window] = String(limit.points);
  }
  return result;
}

function toLimits(points: WindowPoints): QuotaLimit[] {
  return windows.flatMap(window => {
    const value = Math.round(Number.parseFloat(points[window] ?? '') * 1000) / 1000;
    return Number.isFinite(value) && value >= 0 ? [{ window, points: value }] : [];
  });
}

/**
 * @component EndUserQuotaCard
 * @category Feature
 * @status Beta
 * @description Edits the points quotas of a Runtime Workspace's end users
 * (ADR 0010, ADR 0011): a limit per rolling window for every end user,
 * optional caps per app, and plans that end users may hold instead of the
 * defaults. Points may be fractional down to 0.001; empty fields mean no
 * limit.
 * @usage Rendered on the Runtime Workspace quota page.
 * @example
 * <EndUserQuotaCard policyJson={policy} apps={apps} onSave={save} saving={false} />
 */
export function EndUserQuotaCard({
  policyJson,
  apps,
  planCounts,
  onSave,
  saving,
}: {
  policyJson?: string;
  apps: RuntimeApp[];
  /** End users each plan applies to, by key. */
  planCounts?: Record<string, number>;
  onSave: (policy: Record<string, unknown>) => Promise<void>;
  saving: boolean;
}) {
  const t = useT('workspace');
  const tCommon = useT('common');
  const initial = React.useMemo(() => {
    const quota = (parsePolicy(policyJson).end_user_quota ?? {}) as EndUserQuota;
    return {
      limits: toWindowPoints(quota.limits),
      apps: (quota.apps ?? []).map(app => ({
        appKey: app.app_key,
        limits: toWindowPoints(app.limits),
      })),
      plans: (quota.plans ?? []).map(
        (plan): PlanDraft => ({
          id: plan.key,
          key: plan.key,
          name: plan.name ?? '',
          limits: toWindowPoints(plan.limits),
          saved: true,
          noAppCaps: Array.isArray(plan.apps) && plan.apps.length === 0,
          apps: plan.apps && plan.apps.length > 0 ? plan.apps : undefined,
        })
      ),
    };
  }, [policyJson]);
  const [limits, setLimits] = React.useState<WindowPoints>(initial.limits);
  const [appCaps, setAppCaps] = React.useState(initial.apps);
  const [plans, setPlans] = React.useState<PlanDraft[]>(initial.plans);
  const reset = React.useCallback(() => {
    setLimits(initial.limits);
    setAppCaps(initial.apps);
    setPlans(initial.plans);
  }, [initial]);
  React.useEffect(reset, [reset]);

  const quotaOf = (
    defaults: WindowPoints,
    caps: Array<{ appKey: string; limits: WindowPoints }>,
    planDrafts: PlanDraft[]
  ): EndUserQuota => {
    const quota: EndUserQuota = {};
    const defaultLimits = toLimits(defaults);
    if (defaultLimits.length > 0) quota.limits = defaultLimits;
    const appQuotas = caps
      .map(cap => ({ app_key: cap.appKey, limits: toLimits(cap.limits) }))
      .filter(cap => cap.app_key && cap.limits.length > 0);
    if (appQuotas.length > 0) quota.apps = appQuotas;
    if (planDrafts.length > 0) {
      quota.plans = planDrafts.map(plan => {
        const next: PlanQuota = { key: plan.key.trim(), limits: toLimits(plan.limits) };
        if (plan.name.trim()) next.name = plan.name.trim();
        if (plan.noAppCaps) next.apps = [];
        else if (plan.apps) next.apps = plan.apps;
        return next;
      });
    }
    return quota;
  };
  const current = quotaOf(limits, appCaps, plans);
  const dirty =
    JSON.stringify(current) !==
    JSON.stringify(quotaOf(initial.limits, initial.apps, initial.plans));
  const planKeys = plans.map(plan => plan.key.trim());
  const planError = (plan: PlanDraft, index: number) => {
    const key = plan.key.trim();
    if (!planKeyPattern.test(key)) return t('runtime.plans.keyInvalid');
    if (planKeys.indexOf(key) !== index) return t('runtime.plans.keyTaken');
    return null;
  };
  const valid = plans.every((plan, index) => planError(plan, index) === null);
  const save = async () => {
    const policy = parsePolicy(policyJson);
    if (current.limits || current.apps || current.plans) policy.end_user_quota = current;
    else delete policy.end_user_quota;
    await onSave(policy);
  };
  const [editing, setEditing] = React.useState<{ index: number | null; draft: PlanDraft } | null>(
    null
  );
  const editingError = (() => {
    if (!editing) return null;
    const key = editing.draft.key.trim();
    if (!planKeyPattern.test(key)) return t('runtime.plans.keyInvalid');
    if (plans.some((plan, index) => index !== editing.index && plan.key.trim() === key)) {
      return t('runtime.plans.keyTaken');
    }
    return null;
  })();
  const updateDraft = (patch: Partial<PlanDraft>) =>
    setEditing(current =>
      current ? { ...current, draft: { ...current.draft, ...patch } } : current
    );
  const applyDraft = () => {
    if (!editing || editingError) return;
    const draft = { ...editing.draft, key: editing.draft.key.trim() };
    setPlans(
      editing.index === null
        ? [...plans, draft]
        : plans.map((plan, index) => (index === editing.index ? draft : plan))
    );
    setEditing(null);
  };
  const limitSummary = (points: WindowPoints) => {
    const limits = toLimits(points);
    return limits.length === 0
      ? t('runtime.plans.unlimited')
      : limits
          .map(limit => `${t(`runtime.usage.windowShort.${limit.window}`)} ${limit.points}`)
          .join(' · ');
  };

  const windowInputs = (value: WindowPoints, onChange: (next: WindowPoints) => void) => (
    <div className="grid grid-cols-3 gap-2">
      {windows.map(window => (
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
            value={value[window] ?? ''}
            onChange={event => onChange({ ...value, [window]: event.target.value })}
            className="h-9"
          />
        </label>
      ))}
    </div>
  );

  return (
    <Card className="border-border/80">
      <CardContent className="space-y-5 p-5">
        <div className="flex items-start gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
            <Gauge className="size-4" />
          </div>
          <div className="min-w-0">
            <h3 className="text-base font-semibold">{t('runtime.quota.title')}</h3>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">
              {t('runtime.quota.description')}
            </p>
          </div>
        </div>

        <section className="space-y-2">
          <h4 className="text-sm font-medium">{t('runtime.quota.perEndUser')}</h4>
          {windowInputs(limits, setLimits)}
          <p className="text-xs text-muted-foreground">{t('runtime.quota.zeroHint')}</p>
        </section>

        <AppCapsEditor apps={apps} caps={appCaps} onChange={setAppCaps} />

        <section className="space-y-3">
          <div className="flex items-start justify-between gap-2">
            <div>
              <h4 className="text-sm font-medium">{t('runtime.plans.title')}</h4>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('runtime.plans.description')}
              </p>
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="shrink-0"
              onClick={() =>
                setEditing({
                  index: null,
                  draft: {
                    id: `new-${Date.now()}`,
                    key: '',
                    name: '',
                    limits: {},
                    saved: false,
                    noAppCaps: false,
                  },
                })
              }
            >
              <Plus className="size-4" />
              {t('runtime.plans.add')}
            </Button>
          </div>
          {plans.length === 0 ? (
            <p className="text-xs text-muted-foreground">{t('runtime.plans.empty')}</p>
          ) : (
            <div className="overflow-x-auto rounded-lg border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('runtime.plans.name')}</TableHead>
                    <TableHead>{t('runtime.plans.columns.limits')}</TableHead>
                    <TableHead>{t('runtime.plans.columns.appCaps')}</TableHead>
                    <TableHead className="text-right">
                      {t('runtime.plans.columns.holders')}
                    </TableHead>
                    <TableHead className="w-20" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {plans.map((plan, index) => (
                    <TableRow key={plan.id}>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <Crown className="size-4 shrink-0 text-primary" />
                          <div className="min-w-0">
                            <div className="truncate font-medium">{plan.name || plan.key}</div>
                            <code className="text-[11px] text-muted-foreground">{plan.key}</code>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell className="text-sm tabular-nums">
                        {limitSummary(plan.limits)}
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {plan.noAppCaps
                          ? t('runtime.plans.appCapsNone')
                          : plan.apps
                            ? t('runtime.plans.appCapsOwn', { count: plan.apps.length })
                            : t('runtime.plans.appCapsInherit')}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {plan.saved ? (
                          <Link
                            href={`/console/workspace/runtime-workspaces/end-users?plan=${encodeURIComponent(plan.key)}`}
                            className="text-primary hover:underline"
                          >
                            {planCounts?.[plan.key] ?? 0}
                          </Link>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex justify-end">
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            className="size-8 p-0"
                            aria-label={t('runtime.plans.edit')}
                            onClick={() => setEditing({ index, draft: { ...plan } })}
                          >
                            <Pencil className="size-4" />
                          </Button>
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            className="size-8 p-0 text-muted-foreground hover:text-destructive"
                            aria-label={t('runtime.plans.remove')}
                            onClick={() => setPlans(plans.filter((_, i) => i !== index))}
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
          {initial.plans.some(
            plan => !plans.some(next => next.key === plan.key) && (planCounts?.[plan.key] ?? 0) > 0
          ) ? (
            <p className="text-xs text-warning">{t('runtime.plans.removeWarning')}</p>
          ) : null}
        </section>

        <div
          className={cn(
            'flex items-center justify-end gap-2',
            dirty &&
              'sticky bottom-0 z-10 -mx-5 -mb-5 rounded-b-xl border-t bg-card/95 px-5 py-3 backdrop-blur supports-[backdrop-filter]:bg-card/80'
          )}
        >
          {dirty ? (
            <>
              <Badge variant="outline" className="mr-auto border-warning/40 text-warning">
                {t('runtime.quota.unsaved')}
              </Badge>
              <Button type="button" variant="ghost" onClick={reset} disabled={saving}>
                <RotateCcw className="size-4" />
                {t('runtime.quota.discard')}
              </Button>
            </>
          ) : null}
          <Button type="button" onClick={() => void save()} disabled={saving || !dirty || !valid}>
            {saving ? <Loader2 className="size-4 animate-spin" /> : null}
            {t('runtime.quota.save')}
          </Button>
        </div>
      </CardContent>

      <Sheet open={editing !== null} onOpenChange={open => (!open ? setEditing(null) : undefined)}>
        <SheetContent className="w-full overflow-y-auto sm:max-w-lg">
          {editing ? (
            <>
              <SheetHeader>
                <SheetTitle>
                  {editing.index === null
                    ? t('runtime.plans.createTitle')
                    : t('runtime.plans.editTitle')}
                </SheetTitle>
                <SheetDescription>{t('runtime.plans.description')}</SheetDescription>
              </SheetHeader>
              <div className="mt-6 space-y-5">
                <div className="grid grid-cols-2 gap-3">
                  <label className="space-y-1">
                    <span className="text-xs text-muted-foreground">{t('runtime.plans.name')}</span>
                    <Input
                      value={editing.draft.name}
                      maxLength={64}
                      placeholder={t('runtime.plans.namePlaceholder')}
                      onChange={event => updateDraft({ name: event.target.value })}
                      className="h-9"
                    />
                  </label>
                  <label className="space-y-1">
                    <span className="text-xs text-muted-foreground">{t('runtime.plans.key')}</span>
                    <Input
                      value={editing.draft.key}
                      maxLength={64}
                      disabled={editing.draft.saved}
                      placeholder="pro"
                      onChange={event => updateDraft({ key: event.target.value.toLowerCase() })}
                      className="h-9 font-mono"
                    />
                  </label>
                </div>
                {editingError && editing.draft.key ? (
                  <p className="text-xs text-destructive">{editingError}</p>
                ) : null}
                <section className="space-y-2">
                  <h4 className="text-sm font-medium">{t('runtime.plans.columns.limits')}</h4>
                  {windowInputs(editing.draft.limits, next => updateDraft({ limits: next }))}
                  <p className="text-xs text-muted-foreground">{t('runtime.quota.zeroHint')}</p>
                </section>
                <label className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={editing.draft.noAppCaps}
                    onCheckedChange={value =>
                      updateDraft({ noAppCaps: value === true, apps: undefined })
                    }
                  />
                  {editing.draft.apps
                    ? t('runtime.plans.ownAppCaps', { count: editing.draft.apps.length })
                    : t('runtime.plans.noAppCaps')}
                </label>
                <p className="text-xs text-muted-foreground">{t('runtime.plans.applyHint')}</p>
              </div>
              <SheetFooter className="mt-6">
                <Button type="button" variant="outline" onClick={() => setEditing(null)}>
                  {tCommon('cancel')}
                </Button>
                <Button type="button" disabled={Boolean(editingError)} onClick={applyDraft}>
                  {t('runtime.plans.apply')}
                </Button>
              </SheetFooter>
            </>
          ) : null}
        </SheetContent>
      </Sheet>
    </Card>
  );
}
