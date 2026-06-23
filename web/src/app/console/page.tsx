'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useLocale } from 'next-intl';
import { useMemo, useState, type ComponentType, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  AlertCircle,
  AppWindow,
  ArrowRight,
  BookOpen,
  Bot,
  CheckCircle2,
  Circle,
  Database,
  FileText,
  Image as ImageIcon,
  Loader2,
  MessageSquare,
  RefreshCw,
  Settings,
  Users,
} from 'lucide-react';
import { toast } from 'sonner';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useRunnableWebApps } from '@/hooks/agent/use-runnable-webapps';
import type { RunnableWebAppResolvedItem } from '@/hooks/agent/use-runnable-webapps';
import { DASHBOARD_KEYS } from '@/hooks/query-keys';
import { useDashboardStats } from '@/hooks/dashboard/use-dashboard';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import { useUpdateCurrentWorkspace } from '@/hooks/workspace/use-update-current-workspace';
import { useT } from '@/i18n';
import { APP_NAME, ICON_BG } from '@/lib/config';
import { dashboardService } from '@/services';
import type { DashboardRecentWorkType } from '@/services/types/dashboard';
import { useCurrentWorkspace, useWorkspaces, type Workspace } from '@/store/workspace-store';
import { getRecentWorkHref } from '@/utils/console-recent-work';
import { getErrorMessage } from '@/utils/error-notifications';

type ModelType = 'text-chat' | 'embedding' | 'rerank' | 'vision' | 'image-gen';

interface ModelCapability {
  type: ModelType;
  label: string;
  description: string;
  priority: 'required' | 'recommended';
}

interface ProductEntry {
  key: string;
  title: string;
  description: string;
  href: string;
  icon: ComponentType<{ className?: string }>;
  enabled: boolean;
}

interface WorkspaceAssetEntry {
  key: string;
  label: string;
  value: number | undefined;
  icon: ComponentType<{ className?: string }>;
}

interface RecentWorkItem {
  id: string;
  type: DashboardRecentWorkType;
  title: string;
  href: string;
  timestamp: number;
  workspaceId?: string;
  workspaceName?: string;
}

const RECENT_WORK_LIMIT = 5;

async function getRecentWorkItems(limit = RECENT_WORK_LIMIT): Promise<RecentWorkItem[]> {
  const response = await dashboardService.getRecentWork({
    scope: 'overview',
    limit,
  });

  return (response.data?.items ?? []).map(item => ({
    id: item.id,
    type: item.type,
    title: item.title,
    href: getRecentWorkHref(item.type, item.resource_id, item.parent_id),
    timestamp: item.updated_at * 1000,
    workspaceId: item.workspace_id,
    workspaceName: item.workspace_name,
  }));
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

function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <div className="mb-3 text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
      {children}
    </div>
  );
}

function toRunnableAppPreview(item: RunnableWebAppResolvedItem) {
  let iconType: 'image' | 'text' = item.icon_type === 'image' ? 'image' : 'text';
  let src = '';
  let textIcon = (item.meta_data.title || 'A').slice(0, 2).toUpperCase();
  let iconBackground = ICON_BG;
  const icon = item.meta_data.icon || '';

  if (item.icon_type === 'image') {
    src = item.meta_data.icon_url || icon;
  } else {
    try {
      const parsed = JSON.parse(icon || '{}') as { icon?: string; icon_background?: string };
      textIcon = parsed.icon || textIcon;
      iconBackground = parsed.icon_background || iconBackground;
    } catch {
      iconType = 'text';
    }
  }

  return {
    iconType,
    src,
    textIcon,
    iconBackground,
  };
}

export default function ConsolePage() {
  const t = useT();
  const locale = useLocale();
  const router = useRouter();
  const currentWorkspace = useCurrentWorkspace();
  const workspaces = useWorkspaces();
  const updateCurrentWorkspace = useUpdateCurrentWorkspace();
  const [openingRecentWorkId, setOpeningRecentWorkId] = useState<string | null>(null);
  const {
    capabilities,
    canUseOrganizationScope,
    canAccessOrganizationDashboard,
    canManageModelConfig,
  } = useAccountCapabilities();
  const {
    data: statsData,
    isLoading: isModelStatsLoading,
    refetch: refetchModelStats,
    isRefetching: isModelStatsRefetching,
  } = useDashboardStats();
  const {
    items: runnableApps,
    isLoading: isRunnableAppsLoading,
    isFetching: isRunnableAppsFetching,
    refetch: refetchRunnableApps,
    canUseResourceList: canUseRunnableApps,
  } = useRunnableWebApps({ workspaceId: null });
  const {
    data: recentWorkItems = [],
    isLoading: isRecentWorkLoading,
    isFetching: isRecentWorkFetching,
    refetch: refetchRecentWork,
  } = useQuery({
    queryKey: DASHBOARD_KEYS.recentWork('overview'),
    queryFn: () => getRecentWorkItems(RECENT_WORK_LIMIT),
    enabled: canUseOrganizationScope,
    staleTime: 60 * 1000,
    retry: false,
  });

  const canOpenModelConfig = canAccessOrganizationDashboard && canManageModelConfig;
  const productSurfaces = capabilities?.organization.product_surfaces;
  const resources = statsData?.data?.resources;
  const visibleWorkspaceCount = resources?.workspaces ?? 0;
  const hasVisibleWorkspaceAssets = visibleWorkspaceCount > 0;

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

  const productEntries: ProductEntry[] = [
    {
      key: 'chat',
      title: t('navigation.chat'),
      description: t('dashboard.stats.consoleHome.productEntries.chatDescription'),
      href: '/console/work/chat',
      icon: MessageSquare,
      enabled: canUseOrganizationScope && productSurfaces?.chat !== false,
    },
    {
      key: 'image',
      title: t('navigation.image'),
      description: t('dashboard.stats.consoleHome.productEntries.imageDescription'),
      href: '/console/work/image',
      icon: ImageIcon,
      enabled: canUseOrganizationScope && productSurfaces?.image !== false,
    },
    {
      key: 'app-center',
      title: t('dashboard.stats.consoleHome.productEntries.appCenterTitle'),
      description: t('dashboard.stats.consoleHome.productEntries.appCenterDescription'),
      href: '/console/work/app',
      icon: AppWindow,
      enabled: canUseOrganizationScope && productSurfaces?.app !== false,
    },
  ];

  const visibleRunnableApps = useMemo(() => runnableApps.slice(0, 4), [runnableApps]);
  const visibleRecentWorkItems = useMemo(
    () => recentWorkItems.slice(0, RECENT_WORK_LIMIT),
    [recentWorkItems]
  );
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

  const workspaceAssetEntries: WorkspaceAssetEntry[] = [
    {
      key: 'workspaces',
      label: t('dashboard.stats.consoleHome.workspaceOverview.stats.workspaces'),
      value: resources?.workspaces,
      icon: Users,
    },
    {
      key: 'agents',
      label: t('dashboard.stats.consoleHome.workspaceOverview.stats.agents'),
      value: resources?.agents,
      icon: Bot,
    },
    {
      key: 'datasets',
      label: t('dashboard.stats.consoleHome.workspaceOverview.stats.datasets'),
      value: resources?.datasets,
      icon: BookOpen,
    },
    {
      key: 'dataSources',
      label: t('dashboard.stats.consoleHome.workspaceOverview.stats.dataSources'),
      value: resources?.data_sources,
      icon: Database,
    },
    {
      key: 'files',
      label: t('dashboard.stats.consoleHome.workspaceOverview.stats.files'),
      value: resources?.files,
      icon: FileText,
    },
  ];

  const handleRefresh = () => {
    void refetchModelStats();
    if (canUseRunnableApps) {
      void refetchRunnableApps();
    }
    if (canUseOrganizationScope) {
      void refetchRecentWork();
    }
  };

  const handleOpenRecentWork = async (item: RecentWorkItem) => {
    if (openingRecentWorkId) {
      return;
    }

    setOpeningRecentWorkId(item.id);
    try {
      if (item.workspaceId && item.workspaceId !== currentWorkspace?.id) {
        const targetWorkspace: Workspace = workspaces.find(workspace => workspace.id === item.workspaceId) ?? {
          id: item.workspaceId,
          name:
            item.workspaceName ||
            t('dashboard.stats.consoleHome.workspaceOverview.unknownWorkspace'),
        };

        await updateCurrentWorkspace.mutateAsync(targetWorkspace);
      }

      router.push(item.href);
    } catch (error) {
      toast.error(
        getErrorMessage(error) || t('dashboard.stats.consoleHome.workspaceOverview.openRecentFailed')
      );
    } finally {
      setOpeningRecentWorkId(null);
    }
  };

  const isRefreshDisabled =
    isModelStatsLoading ||
    isModelStatsRefetching ||
    isRunnableAppsLoading ||
    isRunnableAppsFetching ||
    isRecentWorkFetching;

  const isRefreshSpinning =
    isModelStatsRefetching || isRunnableAppsFetching || isRecentWorkFetching;

  return (
    <div className="min-h-full bg-bg-canvas px-6 py-6 text-foreground md:px-8 lg:px-10">
      <div className="mx-auto max-w-7xl">
        <header className="mb-6 flex flex-col gap-4 border-b border-border/70 pb-5 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <div className="mb-2 text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
              {APP_NAME} / {t('dashboard.stats.consoleHome.title')}
            </div>
            <h1 className="text-3xl font-semibold tracking-tight text-foreground">
              {t('dashboard.stats.consoleHome.title')}
            </h1>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
              {t('dashboard.stats.consoleHome.intro')}
            </p>
            <div className="mt-3 flex flex-wrap items-center gap-2 text-xs font-medium">
              <Badge variant={isModelStatsLoading ? 'subtle' : isReady ? 'success' : 'warning'}>
                <span
                  className={`size-1.5 rounded-full ${
                    isModelStatsLoading
                      ? 'bg-muted-foreground'
                      : isReady
                        ? 'bg-success'
                        : 'bg-warning'
                  }`}
                  aria-hidden="true"
                />
                {isModelStatsLoading
                  ? t('dashboard.stats.consoleHome.checking')
                  : isReady
                    ? t('dashboard.stats.consoleHome.ready')
                    : t('dashboard.stats.consoleHome.missingCount', {
                        count: requiredMissing.length,
                      })}
              </Badge>
              <div className="inline-flex items-center gap-2 rounded-full border border-border/80 bg-muted/30 px-3 py-1 text-muted-foreground">
                <Users className="size-3.5" />
                <span>{t('dashboard.stats.consoleHome.permissionScope')}</span>
                <span className="max-w-[180px] truncate text-foreground">
                  {isModelStatsLoading
                    ? t('dashboard.stats.consoleHome.checking')
                    : t('dashboard.stats.consoleHome.visibleWorkspaceCount', {
                        count: visibleWorkspaceCount,
                      })}
                </span>
              </div>
            </div>
          </div>
          <Button
            type="button"
            variant="outline"
            size="default"
            onClick={handleRefresh}
            disabled={isRefreshDisabled}
            className="self-start"
          >
            <RefreshCw className={`size-4 ${isRefreshSpinning ? 'animate-spin' : ''}`} />
            {t('dashboard.stats.consoleHome.refresh')}
          </Button>
        </header>

        <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_360px]">
          <main className="min-w-0 space-y-5">
            <section className="grid gap-4 md:grid-cols-3">
              {productEntries.map(entry => {
                const Icon = entry.icon;
                const content = (
                  <Card className="group h-full border-border/80 shadow-sm transition-colors hover:border-primary/40">
                    <CardHeader className="p-4">
                      <div className="mb-4 flex size-10 items-center justify-center rounded-lg border border-border/70 bg-muted/30">
                        <Icon className="size-5 text-muted-foreground" />
                      </div>
                      <CardTitle className="text-base">{entry.title}</CardTitle>
                      <CardDescription className="line-clamp-2 min-h-10 text-sm leading-5">
                        {entry.description}
                      </CardDescription>
                    </CardHeader>
                    <CardContent className="flex items-center justify-between px-4 pb-4 pt-0">
                      <Badge variant={entry.enabled ? 'subtle' : 'outline'} className="text-xs">
                        {entry.enabled
                          ? t('dashboard.stats.consoleHome.available')
                          : t('dashboard.stats.consoleHome.unavailable')}
                      </Badge>
                      <span className="inline-flex items-center gap-1 text-xs font-medium text-primary">
                        {t('dashboard.stats.consoleHome.actions.open')}
                        <ArrowRight className="size-3.5" />
                      </span>
                    </CardContent>
                  </Card>
                );

                return entry.enabled ? (
                  <Link key={entry.key} href={entry.href}>
                    {content}
                  </Link>
                ) : (
                  <div key={entry.key} className="opacity-75">
                    {content}
                  </div>
                );
              })}
            </section>

            {isModelStatsLoading || hasVisibleWorkspaceAssets ? (
              <Card className="border-border/80 shadow-sm">
                <CardHeader className="pb-4">
                  <SectionLabel>{t('dashboard.stats.consoleHome.workspaceOverview.eyebrow')}</SectionLabel>
                  <CardTitle className="text-xl">
                    {t('dashboard.stats.consoleHome.workspaceOverview.title')}
                  </CardTitle>
                  <CardDescription className="mt-2 max-w-2xl leading-6">
                    {t('dashboard.stats.consoleHome.workspaceOverview.description')}
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  {isModelStatsLoading ? (
                    <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
                      {Array.from({ length: 5 }).map((_, index) => (
                        <div
                          key={`workspace-asset-skeleton-${index}`}
                          className="rounded-lg border border-border/70 p-4"
                        >
                          <Skeleton className="mb-4 size-9 rounded-lg" />
                          <Skeleton className="h-6 w-12" />
                          <Skeleton className="mt-2 h-4 w-20" />
                        </div>
                      ))}
                    </section>
                  ) : (
                    <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
                      {workspaceAssetEntries.map(entry => {
                        const Icon = entry.icon;

                        return (
                          <div
                            key={entry.key}
                            className="rounded-lg border border-border/70 bg-background p-4"
                          >
                            <div className="mb-4 flex size-9 items-center justify-center rounded-lg border border-border/70 bg-muted/30">
                              <Icon className="size-4 text-muted-foreground" />
                            </div>
                            <div className="text-2xl font-semibold tracking-tight">
                              {entry.value ?? 0}
                            </div>
                            <div className="mt-1 text-sm text-muted-foreground">
                              {entry.label}
                            </div>
                          </div>
                        );
                      })}
                    </section>
                  )}
                </CardContent>
              </Card>
            ) : null}

            {isModelStatsLoading || hasVisibleWorkspaceAssets ? (
              <Card className="border-border/80 shadow-sm">
                <CardHeader className="pb-4">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div>
                      <SectionLabel>
                        {t('dashboard.stats.consoleHome.workspaceOverview.recentEyebrow')}
                      </SectionLabel>
                      <CardTitle className="text-xl">
                        {t('dashboard.stats.consoleHome.workspaceOverview.recentTitle')}
                      </CardTitle>
                      <CardDescription className="mt-2 max-w-2xl leading-6">
                        {t('dashboard.stats.consoleHome.workspaceOverview.recentDescription')}
                      </CardDescription>
                    </div>
                    <Badge variant="subtle" className="w-fit">
                      {t('dashboard.stats.consoleHome.workspaceOverview.recentCount', {
                        count: visibleRecentWorkItems.length,
                      })}
                    </Badge>
                  </div>
                </CardHeader>
                <CardContent>
                  {isRecentWorkLoading ? (
                    <div className="space-y-3">
                      {Array.from({ length: 4 }).map((_, index) => (
                        <Skeleton
                          key={`console-recent-work-skeleton-${index}`}
                          className="h-11 w-full"
                        />
                      ))}
                    </div>
                  ) : visibleRecentWorkItems.length > 0 ? (
                    <div className="divide-y divide-border/70 rounded-lg border border-border/70">
                      {visibleRecentWorkItems.map(item => {
                        const isOpening = openingRecentWorkId === item.id;

                        return (
                          <button
                            key={item.id}
                            type="button"
                            onClick={() => void handleOpenRecentWork(item)}
                            disabled={Boolean(openingRecentWorkId)}
                            className="grid w-full gap-3 px-4 py-3 text-left transition-colors hover:bg-muted/40 disabled:cursor-wait disabled:opacity-70 md:grid-cols-[96px_minmax(0,1fr)_150px_auto_auto]"
                          >
                            <span className="text-xs font-medium text-muted-foreground">
                              {t(`dashboard.stats.consoleHome.recentTypes.${item.type}`)}
                            </span>
                            <span className="min-w-0 truncate text-sm font-medium text-foreground">
                              {item.title ||
                                t(`dashboard.stats.consoleHome.recentFallback.${item.type}`)}
                            </span>
                            <span className="min-w-0 truncate text-xs text-muted-foreground">
                              {item.workspaceName ||
                                t('dashboard.stats.consoleHome.workspaceOverview.unknownWorkspace')}
                            </span>
                            <span className="text-sm text-muted-foreground">
                              {t('dashboard.stats.consoleHome.updatedAt', {
                                time: dateFormatter.format(item.timestamp),
                              })}
                            </span>
                            <span className="inline-flex items-center justify-end gap-1 text-xs font-medium text-primary">
                              {isOpening ? (
                                <Loader2 className="size-3.5 animate-spin" />
                              ) : (
                                <ArrowRight className="size-3.5" />
                              )}
                              {t('dashboard.stats.consoleHome.actions.open')}
                            </span>
                          </button>
                        );
                      })}
                    </div>
                  ) : (
                    <div className="rounded-lg border border-dashed border-border/70 px-5 py-6">
                      <h3 className="text-base font-semibold text-foreground">
                        {t('dashboard.stats.consoleHome.workspaceOverview.emptyRecentTitle')}
                      </h3>
                      <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
                        {t(
                          'dashboard.stats.consoleHome.workspaceOverview.emptyRecentDescription'
                        )}
                      </p>
                    </div>
                  )}
                </CardContent>
              </Card>
            ) : null}

            <Card className="border-border/80 shadow-sm">
              <CardHeader className="pb-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <SectionLabel>{t('dashboard.stats.consoleHome.runnableApps')}</SectionLabel>
                    <CardTitle className="text-xl">
                      {t('dashboard.stats.consoleHome.runnableAppsTitle')}
                    </CardTitle>
                    <CardDescription className="mt-2 max-w-2xl leading-6">
                      {t('dashboard.stats.consoleHome.runnableAppsDescription')}
                    </CardDescription>
                  </div>
                  <Button asChild variant="outline" size="sm">
                    <Link href="/console/work/app">
                      {t('dashboard.stats.consoleHome.actions.openAppCenter')}
                      <ArrowRight className="size-3.5" />
                    </Link>
                  </Button>
                </div>
              </CardHeader>
              <CardContent>
                {isRunnableAppsLoading ? (
                  <div className="grid gap-3 md:grid-cols-2">
                    {Array.from({ length: 4 }).map((_, index) => (
                      <div
                        key={`runnable-app-skeleton-${index}`}
                        className="rounded-lg border border-border/70 p-3"
                      >
                        <div className="flex items-center gap-3">
                          <Skeleton className="size-9 rounded-md" />
                          <div className="min-w-0 flex-1 space-y-2">
                            <Skeleton className="h-4 w-40" />
                            <Skeleton className="h-3 w-28" />
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                ) : !canUseRunnableApps ? (
                  <div className="rounded-lg border border-dashed border-border/70 px-5 py-6">
                    <h3 className="text-base font-semibold text-foreground">
                      {t('dashboard.stats.consoleHome.runnableAppsUnavailableTitle')}
                    </h3>
                    <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
                      {t('dashboard.stats.consoleHome.runnableAppsUnavailableDescription')}
                    </p>
                  </div>
                ) : visibleRunnableApps.length > 0 ? (
                  <div className="grid gap-3 md:grid-cols-2">
                    {visibleRunnableApps.map(item => {
                      const preview = toRunnableAppPreview(item);

                      return (
                        <Link
                          key={item.web_app_id}
                          href={`/console/work/app/${item.web_app_id}`}
                          className="rounded-lg border border-border/70 p-3 transition-colors hover:bg-muted/40"
                        >
                          <div className="flex items-start gap-3">
                            <IconPreview
                              iconType={preview.iconType}
                              src={preview.src}
                              icon={preview.textIcon}
                              iconBackground={preview.iconBackground}
                              editable={false}
                              size="sm"
                            />
                            <div className="min-w-0 flex-1">
                              <div className="truncate text-sm font-medium text-foreground">
                                {item.meta_data.title}
                              </div>
                              <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                                {item.meta_data.desc ||
                                  t('dashboard.stats.consoleHome.runnableAppFallbackDescription')}
                              </div>
                            </div>
                            <ArrowRight className="mt-1 size-3.5 shrink-0 text-muted-foreground" />
                          </div>
                        </Link>
                      );
                    })}
                  </div>
                ) : (
                  <div className="rounded-lg border border-dashed border-border/70 px-5 py-6">
                    <h3 className="text-base font-semibold text-foreground">
                      {t('dashboard.stats.consoleHome.noRunnableAppsTitle')}
                    </h3>
                    <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
                      {t('dashboard.stats.consoleHome.noRunnableAppsDescription')}
                    </p>
                  </div>
                )}
              </CardContent>
            </Card>

            <Card className="border-border/80 shadow-sm">
              <CardHeader className="pb-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <SectionLabel>{t('dashboard.stats.consoleHome.systemReadiness')}</SectionLabel>
                    <CardTitle className="text-xl">
                      {isModelStatsLoading ? (
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
                  {canOpenModelConfig ? (
                    <Button asChild size="sm">
                      <Link href="/dashboard/provider">
                        <Settings className="size-3.5" />
                        {t('dashboard.stats.consoleHome.actions.configureModels')}
                      </Link>
                    </Button>
                  ) : (
                    <p className="max-w-44 text-right text-xs leading-5 text-muted-foreground">
                      {t('dashboard.stats.consoleHome.modelConfigManagedByAdmin')}
                    </p>
                  )}
                </div>
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
                            const row = (
                              <div className="grid gap-3 px-4 py-3 sm:grid-cols-[auto_1fr_auto]">
                                <StatusDot
                                  configured={configured}
                                  priority={capability.priority}
                                  isLoading={isModelStatsLoading}
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
                                  {isModelStatsLoading
                                    ? t('dashboard.stats.consoleHome.checking')
                                    : configured
                                      ? t('dashboard.stats.consoleHome.ready')
                                      : t('dashboard.stats.consoleHome.missing')}
                                </span>
                              </div>
                            );

                            return canOpenModelConfig ? (
                              <Link
                                key={capability.type}
                                href="/dashboard/provider"
                                className="block transition-colors hover:bg-muted/40"
                              >
                                {row}
                              </Link>
                            ) : (
                              <div key={capability.type}>{row}</div>
                            );
                          })}
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          </main>

          <aside className="space-y-5">
            <Card className="border-border/80 shadow-sm">
              <CardHeader>
                <CardTitle className="text-lg">
                  {t('dashboard.stats.consoleHome.modelAvailability')}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  <div className="rounded-lg border border-border/70 bg-muted/20 p-4">
                    <div className="text-2xl font-semibold tracking-tight">
                      {isModelStatsLoading ? '-' : configuredCount}
                      <span className="text-base font-medium text-muted-foreground">/5</span>
                    </div>
                    <div className="mt-1 text-sm text-muted-foreground">
                      {t('dashboard.stats.consoleHome.capabilitiesConfigured')}
                    </div>
                  </div>
                  {isReady ? (
                    <p className="text-sm leading-6 text-muted-foreground">
                      {t('dashboard.stats.consoleHome.noCriticalIssues')}
                    </p>
                  ) : (
                    <div className="space-y-2">
                      {requiredMissing.map(capability => (
                        <div
                          key={capability.type}
                          className="flex items-start gap-2 rounded-lg border border-destructive/20 bg-destructive/5 px-3 py-2 text-sm text-foreground"
                        >
                          <AlertCircle className="mt-0.5 size-4 shrink-0 text-destructive" />
                          <span>
                            {t('dashboard.stats.consoleHome.missingItem', {
                              label: capability.label,
                            })}
                          </span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>
          </aside>
        </div>
      </div>
    </div>
  );
}
