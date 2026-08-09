'use client';

import { useMemo, useState } from 'react';
import { endOfDay, getUnixTime, startOfDay, subDays } from 'date-fns';
import { RefreshCw } from 'lucide-react';

import { InvocationLogSection } from '@/components/usage/invocation-log-section';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { SearchInput } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useT } from '@/i18n';
import type { ModelUsageAppType } from '@/services/types/statistics';
import { getBillingDisplaySettings } from '@/utils/billing-display';
import { MODEL_USAGE_APP_TYPES } from '@/utils/model-usage-app-type';

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
  const [dateRange, setDateRange] = useState<DateRangeKey>('today');
  const [appType, setAppType] = useState<AppTypeFilter>('all');
  const [modelNameInput, setModelNameInput] = useState('');
  const [refreshKey, setRefreshKey] = useState(() => Date.now());
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
        <div>
          <h1 className="text-2xl font-semibold">{t('usage.invocations.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('usage.invocations.subtitle')}</p>
        </div>

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
              <Button
                variant="outline"
                className="gap-2"
                onClick={() => setRefreshKey(Date.now())}
              >
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
        />
      </div>
    </div>
  );
}
