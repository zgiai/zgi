'use client';

import Link from 'next/link';
import { useMemo } from 'react';
import { useLocale } from 'next-intl';
import { useQuery } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { useDashboardStats } from '@/hooks/dashboard/use-dashboard';
import { APP_NAME } from '@/lib/config';
import { Skeleton } from '@/components/ui/skeleton';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useAuthStore } from '@/store/auth-store';
import { useCurrentWorkspace, useIsOrganizationMode } from '@/store/workspace-store';
import { dashboardService } from '@/services';
import { AlertCircle, ArrowRight, CheckCircle2, Circle, RefreshCw } from 'lucide-react';

type ModelType = 'text-chat' | 'embedding' | 'rerank' | 'vision' | 'image-gen';

interface ModelCapability {
  type: ModelType;
  label: string;
  description: string;
  priority: 'required' | 'recommended';
}

interface ResourceRow {
  key: string;
  label: string;
  value: number | undefined;
  href: string;
  action: string;
}

type RecentWorkType = 'conversation' | 'agent' | 'dataset' | 'database';

interface RecentWorkItem {
  id: string;
  type: RecentWorkType;
  title: string;
  href: string;
  timestamp: number;
}

const systemRows = [
  { label: 'API', value: 'Online' },
  { label: 'Database', value: 'Connected' },
];
const RECENT_WORK_LIMIT = 4;

async function getRecentWorkItems(): Promise<RecentWorkItem[]> {
  const response = await dashboardService.getRecentWork();
  return (response.data?.items ?? []).map(item => ({
    id: item.id,
    type: item.type,
    title: item.title,
    href: getRecentWorkHref(item.type, item.resource_id, item.parent_id),
    timestamp: item.updated_at * 1000,
  }));
}

function getRecentWorkHref(type: RecentWorkType, resourceId: string, parentId?: string) {
  if (type === 'conversation') {
    return parentId ? `/console/agents/${parentId}/logs` : '/console/agents';
  }
  if (type === 'agent') {
    return `/console/agents/${resourceId}/workflow`;
  }
  if (type === 'dataset') {
    return `/console/dataset/${resourceId}`;
  }
  return `/console/db/${resourceId}`;
}

function getModelCount(stats: ReturnType<typeof useDashboardStats>['data'], type: ModelType) {
  return stats?.data?.models.by_usecase?.[type] ?? 0;
}

function StatusDot({
  configured,
  priority,
  isLoading,
}: {
  configured: boolean;
  priority: ModelCapability['priority'];
  isLoading: boolean;
}) {
  const isMissing = !isLoading && !configured;

  return (
    <span aria-hidden="true" data-missing={isMissing ? 'true' : undefined}>
      {isLoading ? (
        <Circle className="mt-0.5 size-4 text-muted-foreground" />
      ) : configured ? (
        <CheckCircle2 className="mt-0.5 size-4 text-success" />
      ) : priority === 'required' ? (
        <AlertCircle className="mt-0.5 size-4 text-destructive" />
      ) : (
        <Circle className="mt-0.5 size-4 text-muted-foreground" />
      )}
    </span>
  );
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div className="mb-3 text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
      {children}
    </div>
  );
}

export default function ConsolePage() {
  const t = useT();
  const locale = useLocale();
  const { user } = useAuthStore();
  const currentWorkspace = useCurrentWorkspace();
  const isOrganizationMode = useIsOrganizationMode();
  const { data: statsData, isLoading, refetch, isRefetching } = useDashboardStats();
  const {
    data: recentWorkItems = [],
    isLoading: isRecentWorkLoading,
    isFetching: isRecentWorkFetching,
    refetch: refetchRecentWork,
  } = useQuery({
    queryKey: ['console', 'recent-work'],
    queryFn: getRecentWorkItems,
    staleTime: 60 * 1000,
    retry: false,
  });
  const stats = statsData?.data;
  const isAdminOrOwner = ['owner', 'admin'].includes(user?.organization_role || '');
  const workspaceLabel = isOrganizationMode
    ? t('navigation.personalSpace')
    : currentWorkspace?.name || t('navigation.switchWorkspace');
  const visibleRecentWorkItems = recentWorkItems.slice(0, RECENT_WORK_LIMIT);
  const dateFormatter = useMemo(
    () =>
      new Intl.DateTimeFormat(locale, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      }),
    [locale]
  );

  const handleRefresh = () => {
    void refetch();
    void refetchRecentWork();
  };

  const modelCapabilities: ModelCapability[] = [
    {
      type: 'text-chat',
      label: t('dashboard.stats.models.text-chat'),
      description: t('dashboard.stats.consoleHome.capabilities.textChatDescription'),
      priority: 'required',
    },
    {
      type: 'embedding',
      label: t('dashboard.stats.models.embedding'),
      description: t('dashboard.stats.consoleHome.capabilities.embeddingDescription'),
      priority: 'required',
    },
    {
      type: 'rerank',
      label: t('dashboard.stats.models.rerank'),
      description: t('dashboard.stats.consoleHome.capabilities.rerankDescription'),
      priority: 'recommended',
    },
    {
      type: 'vision',
      label: t('dashboard.stats.models.vision'),
      description: t('dashboard.stats.consoleHome.capabilities.visionDescription'),
      priority: 'recommended',
    },
    {
      type: 'image-gen',
      label: t('dashboard.stats.models.image-gen'),
      description: t('dashboard.stats.consoleHome.capabilities.imageGenDescription'),
      priority: 'recommended',
    },
  ];

  const configuredCount = modelCapabilities.filter(
    capability => getModelCount(statsData, capability.type) > 0
  ).length;
  const requiredMissing = modelCapabilities.filter(
    capability =>
      capability.priority === 'required' && getModelCount(statsData, capability.type) === 0
  );
  const isReady = requiredMissing.length === 0;

  const resourceRows: ResourceRow[] = [
    {
      key: 'agents',
      label: t('dashboard.stats.resources.agents'),
      value: stats?.resources.agents,
      href: '/console/agents',
      action: stats?.resources.agents
        ? t('dashboard.stats.consoleHome.actions.open')
        : t('dashboard.stats.consoleHome.actions.create'),
    },
    {
      key: 'datasets',
      label: t('dashboard.stats.resources.datasets'),
      value: stats?.resources.datasets,
      href: '/console/dataset',
      action: stats?.resources.datasets
        ? t('dashboard.stats.consoleHome.actions.open')
        : t('dashboard.stats.consoleHome.actions.add'),
    },
    {
      key: 'databases',
      label: t('dashboard.stats.resources.datasources'),
      value: stats?.resources.data_sources,
      href: '/console/db',
      action: stats?.resources.data_sources
        ? t('dashboard.stats.consoleHome.actions.open')
        : t('dashboard.stats.consoleHome.actions.create'),
    },
  ];

  const nextAction = !isReady
    ? {
        eyebrow: t('dashboard.stats.consoleHome.requiredSetup'),
        title: t('dashboard.stats.consoleHome.nextActions.configureRoutingTitle'),
        description: t('dashboard.stats.consoleHome.nextActions.configureRoutingDescription'),
        href: isAdminOrOwner ? '/dashboard/provider' : '/console/settings',
        label: isAdminOrOwner
          ? t('dashboard.stats.consoleHome.actions.configureModels')
          : t('dashboard.stats.consoleHome.actions.contactAdmin'),
      }
    : (stats?.resources.datasets ?? 0) === 0
      ? {
          eyebrow: t('dashboard.stats.consoleHome.nextAction'),
          title: t('dashboard.stats.consoleHome.nextActions.createKnowledgeTitle'),
          description: t('dashboard.stats.consoleHome.nextActions.createKnowledgeDescription'),
          href: '/console/dataset',
          label: t('dashboard.stats.consoleHome.actions.createKnowledge'),
        }
      : (stats?.resources.agents ?? 0) === 0
        ? {
            eyebrow: t('dashboard.stats.consoleHome.nextAction'),
            title: t('dashboard.stats.consoleHome.nextActions.createAgentTitle'),
            description: t('dashboard.stats.consoleHome.nextActions.createAgentDescription'),
            href: '/console/agents',
            label: t('dashboard.stats.consoleHome.actions.createAgent'),
          }
        : {
            eyebrow: t('dashboard.stats.consoleHome.continue'),
            title: t('dashboard.stats.consoleHome.nextActions.startChatTitle'),
            description: t('dashboard.stats.consoleHome.nextActions.startChatDescription'),
            href: '/console/work/chat',
            label: t('dashboard.stats.consoleHome.actions.openChat'),
          };

  const secondaryActions = resourceRows.filter(row => row.href !== nextAction.href).slice(0, 2);

  return (
    <div className="min-h-full bg-bg-canvas px-6 py-6 text-foreground md:px-8 lg:px-10">
      <div className="mx-auto max-w-7xl">
        <header className="mb-6 flex flex-col gap-4 border-b border-border/70 pb-5 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <div className="mb-2 text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
              {APP_NAME} / {workspaceLabel}
            </div>
            <h1 className="text-3xl font-semibold tracking-tight text-foreground">
              {t('dashboard.stats.consoleHome.title')}
            </h1>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
              {t('dashboard.stats.consoleHome.intro')}
            </p>
            <div className="mt-3 flex flex-wrap items-center gap-2 text-xs font-medium">
              <Badge variant={isLoading ? 'subtle' : isReady ? 'success' : 'warning'}>
                <span
                  className={`size-1.5 rounded-full ${
                    isLoading ? 'bg-muted-foreground' : isReady ? 'bg-success' : 'bg-warning'
                  }`}
                  aria-hidden="true"
                />
                {isLoading
                  ? t('dashboard.stats.consoleHome.checking')
                  : isReady
                    ? t('dashboard.stats.consoleHome.ready')
                    : t('dashboard.stats.consoleHome.missingCount', {
                        count: requiredMissing.length,
                      })}
              </Badge>
              <div className="inline-flex items-center gap-2 rounded-full border border-border/80 bg-muted/30 px-3 py-1 text-muted-foreground">
                <span className="size-2 rounded-full bg-success" aria-hidden="true" />
                <span>{t('dashboard.stats.consoleHome.connected')}</span>
              </div>
              <div className="inline-flex items-center gap-2 rounded-full border border-border/80 bg-muted/30 px-3 py-1 text-muted-foreground">
                <span>{t('navigation.current')} {t('navigation.workspace')}:</span>
                <span className="text-foreground">{workspaceLabel}</span>
              </div>
              <div className="inline-flex items-center gap-2 rounded-full border border-border/80 bg-muted/30 px-3 py-1 text-muted-foreground">
                <span>{t('dashboard.stats.consoleHome.recentWork')}</span>
                <span className="text-foreground">
                  {isRecentWorkLoading ? '-' : visibleRecentWorkItems.length}
                </span>
              </div>
            </div>
          </div>
          <Button
            type="button"
            variant="outline"
            size="default"
            onClick={handleRefresh}
            disabled={isLoading || isRefetching || isRecentWorkFetching}
            className="self-start"
          >
            <RefreshCw
              className={`size-4 ${isRefetching || isRecentWorkFetching ? 'animate-spin' : ''}`}
            />
            {t('dashboard.stats.consoleHome.refresh')}
          </Button>
        </header>

        <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_360px]">
          <main className="min-w-0 space-y-5">
            <section className="grid gap-5 xl:grid-cols-[minmax(0,1.25fr)_minmax(280px,0.75fr)]">
              <Card className="border-border/80 shadow-sm">
                <CardHeader className="pb-4">
                  <SectionLabel>{nextAction.eyebrow}</SectionLabel>
                  <CardTitle className="text-2xl leading-8">{nextAction.title}</CardTitle>
                  <CardDescription className="max-w-2xl leading-6">
                    {nextAction.description}
                  </CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid gap-3 sm:grid-cols-2">
                    <div className="rounded-lg border border-border/70 bg-muted/20 p-4">
                      <div className="text-2xl font-semibold tracking-tight">
                        {isLoading ? '-' : configuredCount}
                        <span className="text-base font-medium text-muted-foreground">/5</span>
                      </div>
                      <div className="mt-1 text-sm text-muted-foreground">
                        {t('dashboard.stats.consoleHome.capabilitiesConfigured')}
                      </div>
                    </div>
                    <div className="rounded-lg border border-border/70 bg-muted/20 p-4">
                      <div className="text-2xl font-semibold tracking-tight">
                        {isRecentWorkLoading ? '-' : visibleRecentWorkItems.length}
                      </div>
                      <div className="mt-1 text-sm text-muted-foreground">
                        {t('dashboard.stats.consoleHome.recentWork')}
                      </div>
                    </div>
                  </div>
                  <div className="flex flex-wrap gap-3">
                    <Button asChild size="default">
                      <Link href={nextAction.href}>
                        {nextAction.label}
                        <ArrowRight className="size-4" />
                      </Link>
                    </Button>
                    {secondaryActions.map(action => (
                      <Button key={action.key} asChild variant="outline" size="default">
                        <Link href={action.href}>{action.action} {action.label}</Link>
                      </Button>
                    ))}
                  </div>
                </CardContent>
              </Card>

              <Card className="border-border/80 shadow-sm">
                <CardHeader>
                  <SectionLabel>{t('dashboard.stats.consoleHome.resources')}</SectionLabel>
                  <CardTitle className="text-lg">
                    {t('dashboard.stats.consoleHome.resources')}
                  </CardTitle>
                  <CardDescription>
                    {t('dashboard.stats.consoleHome.noCriticalIssues')}
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="divide-y divide-border/70 rounded-lg border border-border/70">
                    {resourceRows.map(row => (
                      <Link
                        key={row.key}
                        href={row.href}
                        className="grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-3 px-4 py-3 transition-colors hover:bg-muted/40"
                      >
                        <span className="truncate text-sm font-medium text-foreground">
                          {row.label}
                        </span>
                        <span className="text-lg font-semibold leading-none text-foreground">
                          {isLoading || row.value === undefined ? '-' : row.value}
                        </span>
                        <span className="text-xs font-medium text-primary">{row.action}</span>
                      </Link>
                    ))}
                  </div>
                </CardContent>
              </Card>
            </section>

            <Card className="border-border/80 shadow-sm">
              <CardHeader className="pb-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <SectionLabel>{t('dashboard.stats.consoleHome.systemReadiness')}</SectionLabel>
                    <CardTitle className="text-xl">
                      {isLoading ? (
                        <Skeleton className="h-6 w-48" />
                      ) : isReady ? (
                        t('dashboard.stats.consoleHome.readyTitle')
                      ) : (
                        t('dashboard.stats.consoleHome.incompleteTitle')
                      )}
                    </CardTitle>
                    <CardDescription className="mt-2 max-w-2xl leading-6">
                      {isReady
                        ? t('dashboard.stats.consoleHome.readyDescription')
                        : t('dashboard.stats.consoleHome.incompleteDescription')}
                    </CardDescription>
                  </div>
                  <Badge variant={isLoading ? 'subtle' : isReady ? 'success' : 'warning'}>
                    <span
                      className={`size-1.5 rounded-full ${
                        isLoading ? 'bg-muted-foreground' : isReady ? 'bg-success' : 'bg-warning'
                      }`}
                      aria-hidden="true"
                    />
                    {isLoading
                      ? t('dashboard.stats.consoleHome.checking')
                      : isReady
                        ? t('dashboard.stats.consoleHome.ready')
                        : t('dashboard.stats.consoleHome.missingCount', {
                            count: requiredMissing.length,
                          })}
                  </Badge>
                </div>
              </CardHeader>
              <CardContent>
                <div className="grid gap-3 md:grid-cols-3">
                  {systemRows.map(row => (
                    <div
                      key={row.label}
                      className="rounded-lg border border-border/70 bg-muted/20 p-4"
                    >
                      <div className="text-sm font-medium text-foreground">
                        {row.label === 'API'
                          ? t('dashboard.stats.consoleHome.api')
                          : t('dashboard.stats.consoleHome.database')}
                      </div>
                      <div className="mt-1 text-sm text-muted-foreground">
                        {row.value === 'Online'
                          ? t('dashboard.stats.consoleHome.online')
                          : t('dashboard.stats.consoleHome.connected')}
                      </div>
                    </div>
                  ))}
                  <div className="rounded-lg border border-border/70 bg-muted/20 p-4">
                    <div className="text-sm font-medium text-foreground">
                      {t('dashboard.stats.consoleHome.requiredModels')}
                    </div>
                    <div className="mt-1 text-sm text-muted-foreground">
                      {isLoading
                        ? '-'
                        : isReady
                          ? t('dashboard.stats.consoleHome.ready')
                          : t('dashboard.stats.consoleHome.missingCount', {
                              count: requiredMissing.length,
                            })}
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card className="border-border/80 shadow-sm">
              <CardHeader>
                <SectionLabel>{t('dashboard.stats.consoleHome.setupChecklist')}</SectionLabel>
                <CardTitle className="text-lg">
                  {t('dashboard.stats.consoleHome.requiredModels')}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="grid gap-4 md:grid-cols-2">
                  {(['required', 'recommended'] as const).map(priority => (
                    <div key={priority} className="min-w-0">
                      <h3 className="mb-2 text-sm font-semibold text-foreground">
                        {priority === 'required'
                          ? t('dashboard.stats.consoleHome.required')
                          : t('dashboard.stats.consoleHome.recommended')}
                      </h3>
                      <div className="divide-y divide-border/70 rounded-lg border border-border/70">
                        {modelCapabilities
                          .filter(capability => capability.priority === priority)
                          .map(capability => {
                            const modelCount = getModelCount(statsData, capability.type);
                            const configured = modelCount > 0;

                            return (
                              <Link
                                key={capability.type}
                                href={isAdminOrOwner ? '/dashboard/provider' : '/console/settings'}
                                className="grid gap-3 px-4 py-3 transition-colors hover:bg-muted/40 sm:grid-cols-[auto_1fr_auto]"
                              >
                                <StatusDot
                                  configured={configured}
                                  priority={capability.priority}
                                  isLoading={isLoading}
                                />
                                <div className="min-w-0">
                                  <div className="truncate text-sm font-medium text-foreground">
                                    {capability.label}
                                  </div>
                                  <div className="mt-1 line-clamp-2 text-sm leading-5 text-muted-foreground">
                                    {capability.description}
                                  </div>
                                </div>
                                <span className="self-start text-xs font-medium text-muted-foreground">
                                  {isLoading
                                    ? t('dashboard.stats.consoleHome.checking')
                                    : configured
                                      ? t('dashboard.stats.consoleHome.ready')
                                      : t('dashboard.stats.consoleHome.missing')}
                                </span>
                              </Link>
                            );
                          })}
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>

            <Card className="border-border/80 shadow-sm">
              <CardHeader>
                <SectionLabel>{t('dashboard.stats.consoleHome.continue')}</SectionLabel>
                <CardTitle className="text-lg">
                  {t('dashboard.stats.consoleHome.recentWork')}
                </CardTitle>
                <CardDescription>
                  {visibleRecentWorkItems.length > 0
                    ? t('dashboard.stats.consoleHome.nextActions.startChatDescription')
                    : t('dashboard.stats.consoleHome.noRecentDescription')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                {isRecentWorkLoading ? (
                  <div className="space-y-3">
                    <Skeleton className="h-10 w-full" />
                    <Skeleton className="h-10 w-full" />
                    <Skeleton className="h-10 w-full" />
                  </div>
                ) : visibleRecentWorkItems.length > 0 ? (
                  <div className="divide-y divide-border/70 rounded-lg border border-border/70">
                    {visibleRecentWorkItems.map(item => (
                      <Link
                        key={item.id}
                        href={item.href}
                        className="grid gap-3 px-4 py-3 transition-colors hover:bg-muted/40 md:grid-cols-[96px_minmax(0,1fr)_auto_auto]"
                      >
                        <span className="text-xs font-medium text-muted-foreground">
                          {t(`dashboard.stats.consoleHome.recentTypes.${item.type}`)}
                        </span>
                        <span className="min-w-0 truncate text-sm font-medium text-foreground">
                          {item.title ||
                            t(`dashboard.stats.consoleHome.recentFallback.${item.type}`)}
                        </span>
                        <span className="text-sm text-muted-foreground">
                          {t('dashboard.stats.consoleHome.updatedAt', {
                            time: dateFormatter.format(item.timestamp),
                          })}
                        </span>
                        <span className="text-xs font-medium text-primary">
                          {t('dashboard.stats.consoleHome.actions.open')}
                        </span>
                      </Link>
                    ))}
                  </div>
                ) : (
                  <div className="rounded-lg border border-dashed border-border/70 px-5 py-6">
                    <h3 className="text-base font-semibold text-foreground">
                      {t('dashboard.stats.consoleHome.noRecentTitle')}
                    </h3>
                    <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
                      {t('dashboard.stats.consoleHome.noRecentDescription')}
                    </p>
                  </div>
                )}
              </CardContent>
            </Card>
          </main>

          <aside className="space-y-5">
            <Card className="border-border/80 shadow-sm">
              <CardHeader>
                <CardTitle className="text-lg">
                  {t('dashboard.stats.consoleHome.needsAttention')}
                </CardTitle>
              </CardHeader>
              <CardContent>
                {isReady ? (
                  <p className="text-sm leading-6 text-muted-foreground">
                    {t('dashboard.stats.consoleHome.noCriticalIssues')}
                  </p>
                ) : (
                  <div className="space-y-2">
                    {requiredMissing.map(capability => (
                      <Link
                        key={capability.type}
                        href={isAdminOrOwner ? '/dashboard/provider' : '/console/settings'}
                        className="flex items-start gap-2 rounded-lg border border-destructive/20 bg-destructive/5 px-3 py-2 text-sm text-foreground hover:bg-destructive/10"
                      >
                        <AlertCircle className="mt-0.5 size-4 shrink-0 text-destructive" />
                        <span>
                          {t('dashboard.stats.consoleHome.missingItem', {
                            label: capability.label,
                          })}
                        </span>
                      </Link>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </aside>
        </div>
      </div>
    </div>
  );
}
