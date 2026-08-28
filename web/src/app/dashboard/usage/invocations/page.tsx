'use client';

import { useMemo, useState } from 'react';
import { endOfDay, getUnixTime, startOfDay, subDays } from 'date-fns';
import { ChevronDown, RefreshCw, ShieldCheck, Trash2 } from 'lucide-react';
import { toast } from 'sonner';

import { InvocationLogSection } from '@/components/usage/invocation-log-section';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { SearchInput } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import {
  useInvocationContentSettings,
  usePurgeInvocationContent,
  useUpdateInvocationContentSettings,
} from '@/hooks/statistics';
import { useT } from '@/i18n';
import type { ModelUsageAppType } from '@/services/types/statistics';
import { getBillingDisplaySettings } from '@/utils/billing-display';
import { MODEL_USAGE_APP_TYPES } from '@/utils/model-usage-app-type';
import { formatNumber } from '@/utils/format';
import { useAuthStore } from '@/store/auth-store';

type DateRangeKey = 'today' | 'last7Days' | 'last30Days';
type AppTypeFilter = 'all' | ModelUsageAppType;

const DATE_RANGE_DAYS: Record<DateRangeKey, number> = {
  today: 0,
  // The range includes today, so subtract one fewer day than the label count.
  last7Days: 6,
  last30Days: 29,
};

export default function InvocationLogPage() {
  const t = useT('dashboard');
  const tCommon = useT('common');
  const { currentOrganization } = useOrganizations(true);
  const user = useAuthStore.use.user();
  const canManageContent = ['owner', 'admin'].includes(
    currentOrganization?.organization_role || user?.organization_role || ''
  );
  const organizationId = currentOrganization?.id;
  const contentSettingsQuery = useInvocationContentSettings(organizationId, canManageContent);
  const updateContentSettings = useUpdateInvocationContentSettings(organizationId);
  const purgeContent = usePurgeInvocationContent(organizationId);
  const contentSettings = contentSettingsQuery.data?.data;
  const [dateRange, setDateRange] = useState<DateRangeKey>('today');
  const [appType, setAppType] = useState<AppTypeFilter>('all');
  const [modelNameInput, setModelNameInput] = useState('');
  const [refreshKey, setRefreshKey] = useState(() => Date.now());
  const [contentSettingsOpen, setContentSettingsOpen] = useState(false);
  const modelName = useDebouncedValue(modelNameInput.trim(), 300);

  const period = useMemo(() => {
    const now = new Date(refreshKey);
    return {
      startTime: getUnixTime(startOfDay(subDays(now, DATE_RANGE_DAYS[dateRange]))),
      endTime: getUnixTime(endOfDay(now)),
    };
  }, [dateRange, refreshKey]);
  const billingDisplay = useMemo(
    () => getBillingDisplaySettings(currentOrganization),
    [currentOrganization]
  );

  const resetFilters = () => {
    setDateRange('today');
    setAppType('all');
    setModelNameInput('');
  };
  const hasActiveFilters =
    dateRange !== 'today' || appType !== 'all' || Boolean(modelNameInput.trim());

  return (
    <div className="flex h-full flex-col overflow-auto">
      <div className="space-y-6 p-6">
        <Collapsible open={contentSettingsOpen} onOpenChange={setContentSettingsOpen}>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <h1 className="text-2xl font-semibold">{t('usage.invocations.title')}</h1>
              <p className="mt-1 text-sm text-muted-foreground">
                {t('usage.invocations.subtitle')}
              </p>
            </div>

            {canManageContent && contentSettings ? (
              <CollapsibleTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="-mr-2 shrink-0 gap-2 text-muted-foreground hover:text-foreground"
                >
                  <ShieldCheck className="h-4 w-4" />
                  <span>{t('usage.invocations.contentSettings.compactLabel')}</span>
                  <span className="rounded-md bg-muted px-1.5 py-0.5 text-xs font-normal text-muted-foreground">
                    {t(
                      contentSettings.enabled
                        ? 'usage.invocations.contentSettings.statusEnabled'
                        : 'usage.invocations.contentSettings.statusDisabled'
                    )}
                  </span>
                  <ChevronDown
                    className={`h-4 w-4 transition-transform ${contentSettingsOpen ? 'rotate-180' : ''}`}
                  />
                </Button>
              </CollapsibleTrigger>
            ) : null}
          </div>

          {canManageContent && contentSettings ? (
            <CollapsibleContent className="mt-4">
              <Card className="border-border/70 bg-muted/10 shadow-none">
                <CardContent className="space-y-4 p-4">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div className="min-w-0">
                      <p className="text-sm text-muted-foreground">
                        {t('usage.invocations.contentSettings.description', {
                          days: contentSettings.retention_days,
                          size: Math.round(contentSettings.max_bytes / 1024),
                        })}
                      </p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {t('usage.invocations.contentSettings.metadataUnaffected')}
                      </p>
                    </div>
                    <Switch
                      checked={contentSettings.enabled}
                      disabled={updateContentSettings.isPending}
                      aria-label={t('usage.invocations.contentSettings.title')}
                      onCheckedChange={enabled => {
                        updateContentSettings.mutate(
                          { enabled, retention_days: contentSettings.retention_days },
                          {
                            onSuccess: () =>
                              toast.success(
                                t(
                                  enabled
                                    ? 'usage.invocations.contentSettings.enabled'
                                    : 'usage.invocations.contentSettings.disabled'
                                )
                              ),
                            onError: () =>
                              toast.error(t('usage.invocations.contentSettings.updateFailed')),
                          }
                        );
                      }}
                    />
                  </div>

                  <div className="flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-end">
                    <div className="space-y-1.5">
                      <div className="text-sm font-medium">
                        {t('usage.invocations.contentSettings.retentionLabel')}
                      </div>
                      <Select
                        value={String(contentSettings.retention_days)}
                        disabled={updateContentSettings.isPending}
                        onValueChange={value => {
                          updateContentSettings.mutate(
                            { enabled: contentSettings.enabled, retention_days: Number(value) },
                            {
                              onSuccess: () =>
                                toast.success(
                                  t('usage.invocations.contentSettings.retentionUpdated')
                                ),
                              onError: () =>
                                toast.error(t('usage.invocations.contentSettings.updateFailed')),
                            }
                          );
                        }}
                      >
                        <SelectTrigger className="w-full sm:w-[160px]">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {Array.from(new Set([1, 7, 14, 30, contentSettings.retention_days]))
                            .sort((left, right) => left - right)
                            .map(days => (
                              <SelectItem key={days} value={String(days)}>
                                {t('usage.invocations.contentSettings.retentionDays', { days })}
                              </SelectItem>
                            ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="text-sm text-muted-foreground sm:ml-auto sm:self-center">
                      {t('usage.invocations.contentSettings.storedCount', {
                        count: `${formatNumber(contentSettings.stored_count, 0)}${
                          contentSettings.stored_count_capped ? '+' : ''
                        }`,
                      })}
                    </div>
                    <ConfirmDialog
                      variant="danger"
                      title={t('usage.invocations.contentSettings.purgeTitle')}
                      description={t('usage.invocations.contentSettings.purgeDescription')}
                      confirmText={t('usage.invocations.contentSettings.purgeConfirm')}
                      cancelText={tCommon('cancel')}
                      loading={purgeContent.isPending}
                      onConfirm={() => {
                        purgeContent.mutate(undefined, {
                          onSuccess: result => {
                            toast.success(
                              t(
                                result.data.has_more
                                  ? 'usage.invocations.contentSettings.purgedPartial'
                                  : 'usage.invocations.contentSettings.purged',
                                {
                                  count: formatNumber(result.data.deleted_count, 0),
                                }
                              )
                            );
                          },
                          onError: () =>
                            toast.error(t('usage.invocations.contentSettings.purgeFailed')),
                        });
                      }}
                      trigger={
                        <Button
                          type="button"
                          variant="outline"
                          className="gap-2 text-destructive hover:text-destructive"
                          disabled={purgeContent.isPending || contentSettings.stored_count === 0}
                        >
                          <Trash2 className="h-4 w-4" />
                          {t('usage.invocations.contentSettings.purgeAction')}
                        </Button>
                      }
                    />
                  </div>
                </CardContent>
              </Card>
            </CollapsibleContent>
          ) : null}
        </Collapsible>

        <Card className="border-border/80 shadow-sm">
          <CardContent className="flex flex-col gap-3 p-4 lg:flex-row lg:items-center">
            <Select value={dateRange} onValueChange={value => setDateRange(value as DateRangeKey)}>
              <SelectTrigger className="w-full sm:w-[150px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="today">{t('usage.dateRange.today')}</SelectItem>
                <SelectItem value="last7Days">{t('usage.dateRange.last7Days')}</SelectItem>
                <SelectItem value="last30Days">{t('usage.dateRange.last30Days')}</SelectItem>
              </SelectContent>
            </Select>

            <Select value={appType} onValueChange={value => setAppType(value as AppTypeFilter)}>
              <SelectTrigger className="w-full sm:w-[180px]">
                <SelectValue placeholder={t('usage.filters.appTypePlaceholder')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('usage.filters.appTypePlaceholder')}</SelectItem>
                {MODEL_USAGE_APP_TYPES.map(value => (
                  <SelectItem key={value} value={value}>
                    {t(`usage.appTypes.${value}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <SearchInput
              value={modelNameInput}
              onChange={event => setModelNameInput(event.target.value)}
              placeholder={t('usage.filters.modelNamePlaceholder')}
              className="w-full min-w-[220px] lg:max-w-[300px]"
            />

            <div className="flex gap-2 lg:ml-auto">
              {hasActiveFilters ? (
                <Button variant="ghost" onClick={resetFilters}>
                  {tCommon('resetFilters')}
                </Button>
              ) : null}
              <Button variant="outline" className="gap-2" onClick={() => setRefreshKey(Date.now())}>
                <RefreshCw className="h-4 w-4" />
                {tCommon('refresh')}
              </Button>
            </div>
          </CardContent>
        </Card>

        <InvocationLogSection
          startTime={period.startTime}
          endTime={period.endTime}
          appType={appType === 'all' ? undefined : appType}
          modelName={modelName || undefined}
          enabled
          billingDisplay={billingDisplay}
          refreshToken={refreshKey}
          canViewContent={canManageContent}
        />
      </div>
    </div>
  );
}
