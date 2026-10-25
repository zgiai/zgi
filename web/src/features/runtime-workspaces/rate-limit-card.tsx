'use client';

import * as React from 'react';
import { Gauge, Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { useT } from '@/i18n';

interface RateLimitPolicy {
  requests_per_minute?: number;
  per_end_user_requests_per_minute?: number;
}

function parsePolicy(policyJson: string | undefined): Record<string, unknown> {
  try {
    const parsed = JSON.parse(policyJson || '{}');
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {};
  } catch {
    return {};
  }
}

function readLimits(policyJson: string | undefined) {
  const policy = parsePolicy(policyJson);
  const rateLimit = (policy.rate_limit ?? {}) as RateLimitPolicy;
  // A top-level requests_per_minute is the older spelling of the same limit.
  const workspace =
    rateLimit.requests_per_minute ||
    (typeof policy.requests_per_minute === 'number' ? policy.requests_per_minute : 0);
  return {
    workspace: workspace ? String(workspace) : '',
    endUser: rateLimit.per_end_user_requests_per_minute
      ? String(rateLimit.per_end_user_requests_per_minute)
      : '',
  };
}

function positive(value: string) {
  const number = Math.floor(Number(value));
  return Number.isFinite(number) && number > 0 ? number : 0;
}

/**
 * @component RuntimeRateLimitCard
 * @category Feature
 * @status Beta
 * @description Requests per minute allowed for the whole external access and
 * for each end user (ADR 0012). Empty means no limit.
 * @usage Rendered on the Runtime Workspace quota page.
 * @example
 * <RuntimeRateLimitCard policyJson={policy} onSave={save} saving={false} />
 */
export function RuntimeRateLimitCard({
  policyJson,
  onSave,
  saving,
}: {
  policyJson?: string;
  onSave: (policy: Record<string, unknown>) => Promise<void>;
  saving: boolean;
}) {
  const t = useT('workspace');
  const initial = React.useMemo(() => readLimits(policyJson), [policyJson]);
  const [workspace, setWorkspace] = React.useState(initial.workspace);
  const [endUser, setEndUser] = React.useState(initial.endUser);
  React.useEffect(() => {
    setWorkspace(initial.workspace);
    setEndUser(initial.endUser);
  }, [initial]);
  const dirty =
    positive(workspace) !== positive(initial.workspace) ||
    positive(endUser) !== positive(initial.endUser);

  const save = async () => {
    const policy = parsePolicy(policyJson);
    delete policy.requests_per_minute;
    const rateLimit: RateLimitPolicy = {};
    if (positive(workspace)) rateLimit.requests_per_minute = positive(workspace);
    if (positive(endUser)) rateLimit.per_end_user_requests_per_minute = positive(endUser);
    if (Object.keys(rateLimit).length > 0) policy.rate_limit = rateLimit;
    else delete policy.rate_limit;
    await onSave(policy);
  };

  const field = (label: string, hint: string, value: string, onChange: (value: string) => void) => (
    <label className="space-y-1">
      <span className="text-sm font-medium">{label}</span>
      <Input
        type="number"
        min={1}
        step={1}
        inputMode="numeric"
        placeholder={t('runtime.quota.unlimited')}
        value={value}
        onChange={event => onChange(event.target.value)}
        className="h-9"
      />
      <span className="block text-xs text-muted-foreground">{hint}</span>
    </label>
  );

  return (
    <Card className="border-border/80">
      <CardContent className="space-y-5 p-5">
        <div className="flex items-start gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
            <Gauge className="size-4" />
          </div>
          <div className="min-w-0">
            <h3 className="text-base font-semibold">{t('runtime.rateLimit.title')}</h3>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">
              {t('runtime.rateLimit.description')}
            </p>
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          {field(
            t('runtime.rateLimit.workspace'),
            t('runtime.rateLimit.workspaceHint'),
            workspace,
            setWorkspace
          )}
          {field(
            t('runtime.rateLimit.endUser'),
            t('runtime.rateLimit.endUserHint'),
            endUser,
            setEndUser
          )}
        </div>
        <div className="flex justify-end">
          <Button type="button" onClick={() => void save()} disabled={saving || !dirty}>
            {saving ? <Loader2 className="size-4 animate-spin" /> : null}
            {t('runtime.rateLimit.save')}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
