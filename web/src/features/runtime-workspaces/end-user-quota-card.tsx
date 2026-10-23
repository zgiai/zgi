'use client';

import * as React from 'react';
import { Gauge, Loader2, Plus, RotateCcw, Trash2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useT } from '@/i18n';
import type { RuntimeApp } from '@/services/types/runtime-workspace';

const windows = ['24h', '7d', '30d'] as const;
type QuotaWindow = (typeof windows)[number];
type WindowPoints = Partial<Record<QuotaWindow, string>>;

interface QuotaLimit {
  window: QuotaWindow;
  points: number;
}

interface EndUserQuota {
  limits?: QuotaLimit[];
  apps?: Array<{ app_key: string; limits: QuotaLimit[] }>;
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
    return Number.isFinite(value) && value > 0 ? [{ window, points: value }] : [];
  });
}

/**
 * @component EndUserQuotaCard
 * @category Feature
 * @status Beta
 * @description Edits the points quotas of a Runtime Workspace's end users
 * (ADR 0010): a limit per rolling window for every end user, and optional caps
 * per app. Points may be fractional down to 0.001; empty fields mean no limit.
 * @usage Rendered on the Runtime Workspace quota page.
 * @example
 * <EndUserQuotaCard policyJson={policy} apps={apps} onSave={save} saving={false} />
 */
export function EndUserQuotaCard({
  policyJson,
  apps,
  onSave,
  saving,
}: {
  policyJson?: string;
  apps: RuntimeApp[];
  onSave: (policy: Record<string, unknown>) => Promise<void>;
  saving: boolean;
}) {
  const t = useT('workspace');
  const initial = React.useMemo(() => {
    const quota = (parsePolicy(policyJson).end_user_quota ?? {}) as EndUserQuota;
    return {
      limits: toWindowPoints(quota.limits),
      apps: (quota.apps ?? []).map(app => ({
        appKey: app.app_key,
        limits: toWindowPoints(app.limits),
      })),
    };
  }, [policyJson]);
  const [limits, setLimits] = React.useState<WindowPoints>(initial.limits);
  const [appCaps, setAppCaps] = React.useState(initial.apps);
  React.useEffect(() => {
    setLimits(initial.limits);
    setAppCaps(initial.apps);
  }, [initial]);

  const usedKeys = new Set(appCaps.map(cap => cap.appKey));
  const quotaOf = (
    defaults: WindowPoints,
    caps: Array<{ appKey: string; limits: WindowPoints }>
  ): EndUserQuota => {
    const quota: EndUserQuota = {};
    const defaultLimits = toLimits(defaults);
    if (defaultLimits.length > 0) quota.limits = defaultLimits;
    const appQuotas = caps
      .map(cap => ({ app_key: cap.appKey, limits: toLimits(cap.limits) }))
      .filter(cap => cap.app_key && cap.limits.length > 0);
    if (appQuotas.length > 0) quota.apps = appQuotas;
    return quota;
  };
  const dirty =
    JSON.stringify(quotaOf(limits, appCaps)) !==
    JSON.stringify(quotaOf(initial.limits, initial.apps));
  const save = async () => {
    const quota: EndUserQuota = {};
    const defaultLimits = toLimits(limits);
    if (defaultLimits.length > 0) quota.limits = defaultLimits;
    const caps = appCaps
      .map(cap => ({ app_key: cap.appKey, limits: toLimits(cap.limits) }))
      .filter(cap => cap.app_key && cap.limits.length > 0);
    if (caps.length > 0) quota.apps = caps;
    const policy = parsePolicy(policyJson);
    if (quota.limits || quota.apps) policy.end_user_quota = quota;
    else delete policy.end_user_quota;
    await onSave(policy);
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
            min={0.001}
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
        </section>

        <section className="space-y-3">
          <div className="flex items-center justify-between gap-2">
            <h4 className="text-sm font-medium">{t('runtime.quota.perApp')}</h4>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={apps.every(app => usedKeys.has(app.app_key))}
              onClick={() => {
                const next = apps.find(app => !usedKeys.has(app.app_key));
                if (next) setAppCaps([...appCaps, { appKey: next.app_key, limits: {} }]);
              }}
            >
              <Plus className="size-4" />
              {t('runtime.quota.addApp')}
            </Button>
          </div>
          {appCaps.length === 0 ? (
            <p className="text-xs text-muted-foreground">{t('runtime.quota.noAppCaps')}</p>
          ) : (
            appCaps.map((cap, index) => (
              <div key={`${cap.appKey}-${index}`} className="space-y-2 rounded-lg border p-3">
                <div className="flex items-center gap-2">
                  <Select
                    value={cap.appKey}
                    onValueChange={appKey =>
                      setAppCaps(
                        appCaps.map((item, i) => (i === index ? { ...item, appKey } : item))
                      )
                    }
                  >
                    <SelectTrigger className="h-9 min-w-0 flex-1">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {apps
                        .filter(app => app.app_key === cap.appKey || !usedKeys.has(app.app_key))
                        .map(app => (
                          <SelectItem key={app.app_key} value={app.app_key}>
                            {app.name}
                          </SelectItem>
                        ))}
                    </SelectContent>
                  </Select>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    aria-label={t('runtime.quota.removeApp')}
                    onClick={() => setAppCaps(appCaps.filter((_, i) => i !== index))}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
                {windowInputs(cap.limits, next =>
                  setAppCaps(
                    appCaps.map((item, i) => (i === index ? { ...item, limits: next } : item))
                  )
                )}
              </div>
            ))
          )}
        </section>

        <div className="flex items-center justify-end gap-2">
          {dirty ? (
            <>
              <Badge variant="outline" className="mr-auto border-warning/40 text-warning">
                {t('runtime.quota.unsaved')}
              </Badge>
              <Button
                type="button"
                variant="ghost"
                onClick={() => {
                  setLimits(initial.limits);
                  setAppCaps(initial.apps);
                }}
                disabled={saving}
              >
                <RotateCcw className="size-4" />
                {t('runtime.quota.discard')}
              </Button>
            </>
          ) : null}
          <Button type="button" onClick={() => void save()} disabled={saving || !dirty}>
            {saving ? <Loader2 className="size-4 animate-spin" /> : null}
            {t('runtime.quota.save')}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
