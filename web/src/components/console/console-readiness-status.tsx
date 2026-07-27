'use client';

import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';
import { AlertCircle, CheckCircle2, ChevronDown, Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useDashboardStats } from '@/hooks/dashboard/use-dashboard';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { contentParseService } from '@/services';

type RequiredModelType = 'text-chat' | 'embedding';

const REQUIRED_MODEL_TYPES: RequiredModelType[] = ['text-chat', 'embedding'];

export function ConsoleReadinessStatus() {
  const t = useT();
  const {
    isLoading: isCapabilitiesLoading,
    canUseOrganizationScope,
    canAccessOrganizationDashboard,
    canManageModelConfig,
  } = useAccountCapabilities();
  const { data: statsData, isLoading: isModelStatsLoading } = useDashboardStats();
  const { data: parserSettingsData, isSuccess: isParserSettingsSuccess } = useQuery({
    queryKey: ['content-parse', 'parser-settings'],
    queryFn: () => contentParseService.listParserSettings(),
    enabled: canUseOrganizationScope,
    staleTime: 60 * 1000,
    retry: false,
  });

  const missingModels = REQUIRED_MODEL_TYPES.filter(
    type => (statsData?.data?.models.by_usecase?.[type] ?? 0) === 0
  );
  const hasAvailableThirdPartyParser = (parserSettingsData?.data.items ?? []).some(
    item =>
      (item.provider_key === 'reducto' || item.provider_key === 'mineru') &&
      item.enabled &&
      item.configured &&
      item.status === 'available'
  );
  const showParserAttention =
    canUseOrganizationScope && isParserSettingsSuccess && !hasAvailableThirdPartyParser;
  const isLoading = isCapabilitiesLoading || isModelStatsLoading;
  const isReady = !isLoading && missingModels.length === 0 && !showParserAttention;
  const canOpenModelConfig = canAccessOrganizationDashboard && canManageModelConfig;
  const canOpenParserConfig = canAccessOrganizationDashboard && canManageModelConfig;
  const triggerLabel = isLoading
    ? t('dashboard.stats.consoleHome.checking')
    : missingModels.length > 0
      ? t('dashboard.stats.consoleHome.missingCount', { count: missingModels.length })
      : showParserAttention
        ? t('dashboard.stats.consoleHome.needsAttention')
        : t('dashboard.stats.consoleHome.ready');

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className={cn(
            'h-8 gap-1.5 rounded-full border px-2.5 text-xs font-medium',
            isLoading && 'border-border/70 text-muted-foreground',
            isReady && 'border-success/30 bg-success/5 text-success hover:bg-success/10',
            !isLoading &&
              !isReady &&
              'border-warning/40 bg-warning/5 text-warning hover:bg-warning/10'
          )}
          aria-label={`${t('dashboard.stats.consoleHome.systemReadiness')}: ${triggerLabel}`}
        >
          {isLoading ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : isReady ? (
            <CheckCircle2 className="size-3.5" />
          ) : (
            <AlertCircle className="size-3.5" />
          )}
          <span className="hidden sm:inline">{triggerLabel}</span>
          <ChevronDown className="size-3 text-current/70" />
        </Button>
      </DropdownMenuTrigger>

      <DropdownMenuContent align="end" className="w-80 p-2">
        <DropdownMenuLabel className="flex items-center justify-between gap-3 px-2 py-2">
          <span>{t('dashboard.stats.consoleHome.systemReadiness')}</span>
          <span
            className={cn(
              'text-[11px] font-medium',
              isReady ? 'text-success' : 'text-muted-foreground'
            )}
          >
            {triggerLabel}
          </span>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />

        {isLoading ? (
          <div className="flex items-center gap-2 px-2 py-4 text-sm text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
            {t('dashboard.stats.consoleHome.checking')}
          </div>
        ) : missingModels.length === 0 && !showParserAttention ? (
          <div className="flex gap-2 px-2 py-3 text-sm text-muted-foreground">
            <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-success" />
            <span>{t('dashboard.stats.consoleHome.noCriticalIssues')}</span>
          </div>
        ) : (
          <div className="space-y-3 px-2 py-2">
            {missingModels.length > 0 ? (
              <div className="space-y-2">
                {missingModels.map(type => (
                  <div key={type} className="flex items-start gap-2 text-sm">
                    <AlertCircle className="mt-0.5 size-4 shrink-0 text-destructive" />
                    <span>
                      {t('dashboard.stats.consoleHome.missingItem', {
                        label: t(`dashboard.stats.models.${type}`),
                      })}
                    </span>
                  </div>
                ))}
                {canOpenModelConfig ? (
                  <Button asChild variant="outline" size="sm" className="mt-1 w-full">
                    <Link href="/dashboard/provider">
                      {t('dashboard.stats.consoleHome.actions.configureModels')}
                    </Link>
                  </Button>
                ) : (
                  <p className="text-xs leading-5 text-muted-foreground">
                    {t('dashboard.stats.consoleHome.modelConfigManagedByAdmin')}
                  </p>
                )}
              </div>
            ) : null}

            {showParserAttention ? (
              <div
                className={cn(
                  'space-y-2 border-t border-border/70 pt-3',
                  missingModels.length === 0 && 'border-t-0 pt-0'
                )}
              >
                <div className="flex items-start gap-2 text-xs leading-5 text-muted-foreground">
                  <AlertCircle className="mt-0.5 size-4 shrink-0 text-warning" />
                  <span>{t('dashboard.stats.consoleHome.parserServiceMissing')}</span>
                </div>
                {canOpenParserConfig ? (
                  <Button asChild variant="ghost" size="sm" className="w-full">
                    <Link href="/dashboard/settings/parsers">
                      {t('dashboard.stats.consoleHome.actions.configureParserService')}
                    </Link>
                  </Button>
                ) : null}
              </div>
            ) : null}
          </div>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
