'use client';

import Link from 'next/link';
import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { endOfDay, getUnixTime, startOfDay, subDays } from 'date-fns';
import {
  Activity,
  ArrowRight,
  Brain,
  Building2,
  KeyRound,
  RadioTower,
  ShieldAlert,
  Users,
} from 'lucide-react';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useDashboardStats } from '@/hooks/dashboard/use-dashboard';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useModelUsage } from '@/hooks/statistics';
import { useJoinedWorkspaces } from '@/hooks/workspace/use-joined-workspaces';
import { organizationService } from '@/services/organization.service';
import { providerService } from '@/services/provider.service';
import { channelService } from '@/services/channel.service';
import { getOrganizationDisplayName } from '@/utils/organization-display';

function SectionEyebrow({ children }: { children: React.ReactNode }) {
  return (
    <div className="mb-3 text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
      {children}
    </div>
  );
}

export default function DashboardPage() {
  const t = useT('dashboard');
  const tCommon = useT('common');
  const { organizations, currentOrganization, isLoading: isOrgLoading } = useOrganizations(true);
  const { total: workspaceTotal, isLoading: isWorkspaceLoading } = useJoinedWorkspaces({
    syncToStore: false,
  });
  const { data: dashboardStatsData, isLoading: isStatsLoading, refetch: refetchStats } =
    useDashboardStats();

  const { data: memberSummary, isLoading: isMembersLoading, refetch: refetchMembers } = useQuery({
    queryKey: ['dashboard', 'organization-members-summary', currentOrganization?.id],
    enabled: Boolean(currentOrganization?.id),
    queryFn: async () => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return organizationService.getDepartmentMembersWithParams(currentOrganization.id, {
        page: '1',
        limit: '1',
      });
    },
    staleTime: 5 * 60 * 1000,
    retry: false,
  });

  const { data: providerSummary, isLoading: isProvidersLoading, refetch: refetchProviders } =
    useQuery({
      queryKey: ['dashboard', 'providers-summary'],
      queryFn: async () => providerService.getProviders({ page: 1, limit: 1 }),
      staleTime: 5 * 60 * 1000,
      retry: false,
    });

  const { data: channelSummary, isLoading: isChannelsLoading, refetch: refetchChannels } =
    useQuery({
      queryKey: ['dashboard', 'channels-summary'],
      queryFn: async () => channelService.getChannels({ page: 1, page_size: 1 }),
      staleTime: 5 * 60 * 1000,
      retry: false,
    });

  const usageParams = useMemo(() => {
    const now = new Date();
    const startDate = startOfDay(subDays(now, 7));
    const endDate = endOfDay(now);

    return {
      start_time: getUnixTime(startDate),
      end_time: getUnixTime(endDate),
    };
  }, []);

  const {
    data: usageSummaryData,
    isLoading: isUsageLoading,
    refetch: refetchUsageSummary,
  } = useModelUsage(usageParams);

  const stats = dashboardStatsData?.data;
  const configuredModelCount = useMemo(() => {
    if (!stats?.models.by_usecase) return 0;
    return Object.values(stats.models.by_usecase).filter(value => (value ?? 0) > 0).length;
  }, [stats]);

  const requiredHealth = useMemo(() => {
    const missing: string[] = [];
    if ((stats?.models.by_usecase?.['text-chat'] ?? 0) === 0) {
      missing.push(t('items.llmProviders'));
    }
    if ((stats?.models.by_usecase?.embedding ?? 0) === 0) {
      missing.push(t('items.modelSettings'));
    }
    if ((channelSummary?.data?.total ?? 0) === 0) {
      missing.push(t('items.channel'));
    }
    return missing;
  }, [channelSummary?.data?.total, stats?.models.by_usecase, t]);

  const isHealthy = requiredHealth.length === 0;
  const isLoading =
    isOrgLoading ||
    isWorkspaceLoading ||
    isStatsLoading ||
    isMembersLoading ||
    isProvidersLoading ||
    isChannelsLoading;

  const topUsageModel = usageSummaryData?.by_model?.[0];
  const currentOrganizationName = getOrganizationDisplayName(currentOrganization);

  const handleRefresh = () => {
    void refetchStats();
    void refetchMembers();
    void refetchProviders();
    void refetchChannels();
    void refetchUsageSummary();
  };

  const summaryCards = [
    {
      key: 'organizations',
      label: t('items.organization'),
      value: organizations.length,
      description: currentOrganizationName || '-',
      icon: Building2,
    },
    {
      key: 'workspaces',
      label: t('items.workspaces'),
      value: workspaceTotal,
      description: t('organization.workspaceManagement.title'),
      icon: Building2,
    },
    {
      key: 'members',
      label: t('items.contacts'),
      value: memberSummary?.total ?? 0,
      description: t('organization.contacts.title'),
      icon: Users,
    },
    {
      key: 'providers',
      label: t('items.llmProviders'),
      value: providerSummary?.data?.total ?? 0,
      description: t('items.channel'),
      icon: Brain,
    },
    {
      key: 'channels',
      label: t('items.channel'),
      value: channelSummary?.data?.total ?? 0,
      description: t('usage.cards.modelCount', { count: configuredModelCount }),
      icon: RadioTower,
    },
    {
      key: 'keys',
      label: t('items.apiKeys'),
      value: stats?.resources.agents ?? 0,
      description: t('stats.resources.agents'),
      icon: KeyRound,
    },
  ];

  const quickLinks = [
    {
      key: 'contacts',
      title: t('items.contacts'),
      description: t('organization.contacts.subtitle'),
      href: '/dashboard/organization/contacts',
      icon: Users,
    },
    {
      key: 'workspaces',
      title: t('items.workspaces'),
      description: t('organization.workspaceManagement.description'),
      href: '/dashboard/organization/workspaces',
      icon: Building2,
    },
    {
      key: 'permissions',
      title: t('items.permissions'),
      description: t('organization.permissions.subtitle'),
      href: '/dashboard/organization/permissions',
      icon: ShieldAlert,
    },
    {
      key: 'providers',
      title: t('items.llmProviders'),
      description: t('groups.llm'),
      href: '/dashboard/provider',
      icon: Brain,
    },
    {
      key: 'channels',
      title: t('items.channel'),
      description: t('items.channel'),
      href: '/dashboard/channel',
      icon: RadioTower,
    },
    {
      key: 'usage',
      title: t('usage.title'),
      description: t('usage.subtitle'),
      href: '/dashboard/usage/overview',
      icon: Activity,
    },
  ];

  return (
    <div className="min-h-full bg-bg-canvas px-6 py-6 text-foreground md:px-8 lg:px-10">
      <div className="mx-auto max-w-7xl">
        <header className="mb-6 flex flex-col gap-4 border-b border-border/70 pb-5 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <SectionEyebrow>{t('stats.quickActions.management')}</SectionEyebrow>
            <h1 className="text-3xl font-semibold tracking-tight text-foreground">
              {t('items.dashboard')}
            </h1>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">
              {t('usage.subtitle')}
            </p>
            <div className="mt-3 flex flex-wrap items-center gap-2 text-xs font-medium">
              <Badge variant={isLoading ? 'subtle' : isHealthy ? 'success' : 'warning'}>
                {isLoading ? tCommon('loading') : isHealthy ? t('stats.consoleHome.ready') : t('stats.consoleHome.missingCount', { count: requiredHealth.length })}
              </Badge>
              {currentOrganization ? (
                <div className="inline-flex items-center gap-2 rounded-full border border-border/80 bg-muted/30 px-3 py-1 text-muted-foreground">
                  <span>{t('items.organization')}:</span>
                  <span className="text-foreground">{currentOrganizationName}</span>
                </div>
              ) : null}
            </div>
          </div>
          <Button type="button" variant="outline" onClick={handleRefresh} className="self-start">
            {tCommon('refresh')}
          </Button>
        </header>

        <div className="grid gap-5 xl:grid-cols-[minmax(0,1.3fr)_minmax(320px,0.7fr)]">
          <main className="min-w-0 space-y-5">
            <Card className="border-border/80 shadow-sm">
              <CardHeader className="pb-4">
                <SectionEyebrow>{t('stats.consoleHome.systemReadiness')}</SectionEyebrow>
                <CardTitle className="text-xl">
                  {isLoading
                    ? tCommon('loading')
                    : isHealthy
                      ? t('stats.consoleHome.readyTitle')
                      : t('stats.consoleHome.incompleteTitle')}
                </CardTitle>
                <CardDescription className="leading-6">
                  {isHealthy
                    ? t('stats.consoleHome.readyDescription')
                    : t('stats.consoleHome.incompleteDescription')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                {isLoading ? (
                  <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                    {Array.from({ length: 6 }).map((_, index) => (
                      <Skeleton key={`summary-${index}`} className="h-28 rounded-lg" />
                    ))}
                  </div>
                ) : (
                  <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                    {summaryCards.map(card => {
                      const Icon = card.icon;
                      return (
                        <div
                          key={card.key}
                          className="rounded-lg border border-border/70 bg-muted/20 p-4"
                        >
                          <div className="flex items-center justify-between gap-3">
                            <span className="text-sm font-medium text-foreground">{card.label}</span>
                            <Icon className="h-4 w-4 text-muted-foreground" />
                          </div>
                          <div className="mt-3 text-2xl font-semibold tracking-tight text-foreground">
                            {card.value}
                          </div>
                          <div className="mt-1 text-sm text-muted-foreground">{card.description}</div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </CardContent>
            </Card>

            <Card className="border-border/80 shadow-sm">
              <CardHeader>
                <SectionEyebrow>{t('stats.quickActions.title')}</SectionEyebrow>
                <CardTitle className="text-lg">{t('stats.resources.overview')}</CardTitle>
                <CardDescription>{t('stats.quickActions.management')}</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                  {quickLinks.map(link => {
                    const Icon = link.icon;
                    return (
                      <Link key={link.key} href={link.href}>
                        <Card className="h-full border-border/80 transition-all hover:border-primary/40 hover:shadow-sm">
                          <CardHeader className="p-4 pb-3">
                            <div className="flex items-start gap-3">
                              <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border bg-muted/30">
                                <Icon className="h-4 w-4 text-muted-foreground" />
                              </div>
                              <div className="min-w-0">
                                <CardTitle className="text-base">{link.title}</CardTitle>
                                <CardDescription className="mt-1 line-clamp-2 text-xs leading-5">
                                  {link.description}
                                </CardDescription>
                              </div>
                            </div>
                          </CardHeader>
                          <CardContent className="px-4 pb-4 pt-0">
                            <div className="flex items-center gap-1 text-xs font-medium text-primary">
                              {t('stats.consoleHome.actions.open')}
                              <ArrowRight className="h-3.5 w-3.5" />
                            </div>
                          </CardContent>
                        </Card>
                      </Link>
                    );
                  })}
                </div>
              </CardContent>
            </Card>
          </main>

          <aside className="space-y-5">
            <Card className="border-border/80 shadow-sm">
              <CardHeader>
                <SectionEyebrow>{t('stats.consoleHome.needsAttention')}</SectionEyebrow>
                <CardTitle className="text-lg">{t('stats.consoleHome.needsAttention')}</CardTitle>
                <CardDescription>
                  {isHealthy
                    ? t('stats.consoleHome.noCriticalIssues')
                    : t('stats.consoleHome.requiredSetup')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                {isLoading ? (
                  <div className="space-y-2">
                    <Skeleton className="h-12 rounded-lg" />
                    <Skeleton className="h-12 rounded-lg" />
                  </div>
                ) : isHealthy ? (
                  <div className="rounded-lg border border-border/70 bg-muted/20 px-4 py-3 text-sm text-muted-foreground">
                    {t('stats.consoleHome.noCriticalIssues')}
                  </div>
                ) : (
                  <div className="space-y-2">
                    {requiredHealth.map(item => (
                      <Link
                        key={item}
                        href={item === t('items.channel') ? '/dashboard/channel' : '/dashboard/provider'}
                        className="flex items-start gap-2 rounded-lg border border-warning/30 bg-warning/5 px-3 py-2 text-sm text-foreground hover:bg-warning/10"
                      >
                        <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
                        <span>{t('stats.consoleHome.missingItem', { label: item })}</span>
                      </Link>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>

            <Card className="border-border/80 shadow-sm">
              <CardHeader>
                <SectionEyebrow>{t('groups.usage')}</SectionEyebrow>
                <CardTitle className="text-lg">{t('usage.title')}</CardTitle>
                <CardDescription>{t('usage.subtitle')}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="rounded-lg border border-border/70 bg-muted/20 p-4">
                    <div className="text-sm font-medium text-foreground">{t('stats.models.title')}</div>
                    <div className="mt-2 text-2xl font-semibold tracking-tight text-foreground">
                      {isLoading ? '-' : stats?.models.total ?? 0}
                    </div>
                    <div className="mt-1 text-sm text-muted-foreground">{t('stats.models.configStatus')}</div>
                  </div>
                  <div className="rounded-lg border border-border/70 bg-muted/20 p-4">
                    <div className="text-sm font-medium text-foreground">{t('stats.resources.overview')}</div>
                    <div className="mt-2 text-2xl font-semibold tracking-tight text-foreground">
                      {isLoading ? '-' : (stats?.resources.agents ?? 0) + (stats?.resources.datasets ?? 0) + (stats?.resources.data_sources ?? 0)}
                    </div>
                    <div className="mt-1 text-sm text-muted-foreground">{t('stats.resources.total')}</div>
                  </div>
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="rounded-lg border border-border/70 bg-muted/20 p-4">
                    <div className="text-sm font-medium text-foreground">{t('usage.cards.totalTokens')}</div>
                    <div className="mt-2 text-2xl font-semibold tracking-tight text-foreground">
                      {isUsageLoading ? '-' : usageSummaryData?.summary.total_tokens ?? 0}
                    </div>
                    <div className="mt-1 text-sm text-muted-foreground">{t('usage.dateRange.last7Days')}</div>
                  </div>
                  <div className="rounded-lg border border-border/70 bg-muted/20 p-4">
                    <div className="text-sm font-medium text-foreground">{t('usage.cards.totalPoints')}</div>
                    <div className="mt-2 text-2xl font-semibold tracking-tight text-foreground">
                      {isUsageLoading ? '-' : usageSummaryData?.summary.total_points ?? 0}
                    </div>
                    <div className="mt-1 text-sm text-muted-foreground">{t('usage.cards.pointsConsumptionHint')}</div>
                  </div>
                </div>
                <div className="rounded-lg border border-border/70 bg-muted/20 p-4">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <div className="text-sm font-medium text-foreground">{t('usage.cards.topModels')}</div>
                      <div className="mt-1 text-sm text-muted-foreground">
                        {topUsageModel
                          ? `${topUsageModel.model_name} · ${topUsageModel.total_tokens} tokens`
                          : t('usage.cards.noModels')}
                      </div>
                    </div>
                    <Activity className="h-4 w-4 text-muted-foreground" />
                  </div>
                </div>
                <Button asChild variant="outline" className="w-full">
                  <Link href="/dashboard/usage/overview">
                    {t('usage.exportReport')}
                    <ArrowRight className="h-4 w-4" />
                  </Link>
                </Button>
              </CardContent>
            </Card>
          </aside>
        </div>
      </div>
    </div>
  );
}
