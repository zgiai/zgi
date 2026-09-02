'use client';

import { Bot, BookOpen, Database, FileText, RefreshCw, Sparkles } from 'lucide-react';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useWorkspaceHome } from '@/hooks/console/use-workspace-home';
import { useCurrentUser } from '@/store/auth-store';
import { OnboardingChecklist } from './onboarding-checklist';
import { WorkspaceQuickActions } from './quick-actions';
import { WorkspaceRecentWork } from './recent-work';

export function WorkspaceHome() {
  const t = useT('dashboard');
  const user = useCurrentUser();
  const home = useWorkspaceHome();
  const organizationRoleLabel = home.organizationRole
    ? {
        owner: t('stats.consoleHome.organizationRoles.owner'),
        admin: t('stats.consoleHome.organizationRoles.admin'),
        normal: t('stats.consoleHome.organizationRoles.normal'),
      }[home.organizationRole]
    : null;
  const resources = [
    {
      label: t('stats.consoleHome.workspaceOverview.stats.agents'),
      value: home.stats?.resources.agents ?? 0,
      icon: Bot,
    },
    {
      label: t('stats.consoleHome.workspaceOverview.stats.datasets'),
      value: home.stats?.resources.datasets ?? 0,
      icon: BookOpen,
    },
    {
      label: t('stats.consoleHome.workspaceOverview.stats.dataSources'),
      value: home.stats?.resources.data_sources ?? 0,
      icon: Database,
    },
    {
      label: t('stats.consoleHome.workspaceOverview.stats.files'),
      value: home.stats?.resources.files ?? 0,
      icon: FileText,
    },
  ];
  const hasCoreModel = (home.stats?.models.by_usecase?.['text-chat'] ?? 0) > 0;

  return (
    <div className="min-h-full bg-bg-canvas px-5 py-6 text-foreground md:px-8 lg:px-10">
      <div className="mx-auto max-w-7xl">
        <header className="mb-6 flex flex-col gap-4 border-b border-border/70 pb-5 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <div className="mb-2 flex flex-wrap items-center gap-2">
              <Badge variant="subtle">
                {home.currentWorkspace?.name ?? t('stats.consoleHome.noWorkspaceName')}
              </Badge>
              {organizationRoleLabel ? (
                <Badge variant="outline">{organizationRoleLabel}</Badge>
              ) : null}
            </div>
            <h1 className="text-2xl font-semibold tracking-tight md:text-3xl">
              {t('stats.welcome', { name: user?.name || user?.email || '' })}
            </h1>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">
              {t('stats.consoleHome.intro')}
            </p>
          </div>
          <Button
            type="button"
            variant="outline"
            onClick={home.refresh}
            disabled={home.isRefreshing}
            className="self-start"
          >
            <RefreshCw className={home.isRefreshing ? 'size-4 animate-spin' : 'size-4'} />
            {t('stats.consoleHome.refresh')}
          </Button>
        </header>

        <div className="mb-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {resources.map(resource => {
            const Icon = resource.icon;
            return (
              <Card key={resource.label} className="border-border/80 shadow-sm">
                <CardContent className="flex items-center gap-3 p-4">
                  <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                    <Icon className="size-4" />
                  </span>
                  <span>
                    <span className="block text-xs text-muted-foreground">{resource.label}</span>
                    {home.isStatsLoading ? (
                      <Skeleton className="mt-1 h-5 w-10" />
                    ) : home.hasStatsError ? (
                      <span className="mt-0.5 block text-xl font-semibold text-muted-foreground">
                        —
                      </span>
                    ) : (
                      <span className="mt-0.5 block text-xl font-semibold tabular-nums">
                        {resource.value}
                      </span>
                    )}
                  </span>
                </CardContent>
              </Card>
            );
          })}
        </div>

        {home.hasError ? (
          <div className="mb-5 flex items-center justify-between gap-4 rounded-lg border border-warning/25 bg-warning/10 px-4 py-3 text-sm text-warning">
            <span>{t('stats.consoleHome.onboarding.loadError')}</span>
            <Button type="button" variant="outline" size="sm" onClick={home.refresh}>
              {t('stats.consoleHome.refresh')}
            </Button>
          </div>
        ) : null}

        <div className="grid gap-5 xl:grid-cols-[minmax(0,1.55fr)_minmax(320px,0.75fr)]">
          <main className="min-w-0 space-y-5">
            <Card className="overflow-hidden border-border/80 bg-gradient-to-br from-primary/[0.08] via-card to-card shadow-sm">
              <CardContent className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between md:p-6">
                <div className="flex items-start gap-3">
                  <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    <Sparkles className="size-5" />
                  </span>
                  {home.isReadinessLoading ? (
                    <div className="min-w-0 flex-1" aria-label={t('stats.consoleHome.checking')}>
                      <Skeleton className="h-5 w-44" />
                      <Skeleton className="mt-2 h-4 w-full max-w-lg" />
                    </div>
                  ) : home.hasReadinessError ? (
                    <div>
                      <div className="text-base font-semibold">
                        {t('stats.consoleHome.readinessUnavailableTitle')}
                      </div>
                      <p className="mt-1 max-w-2xl text-sm leading-6 text-muted-foreground">
                        {t('stats.consoleHome.readinessUnavailableDescription')}
                      </p>
                    </div>
                  ) : (
                    <div>
                      <div className="text-base font-semibold">
                        {hasCoreModel
                          ? t('stats.consoleHome.readyTitle')
                          : t('stats.consoleHome.incompleteTitle')}
                      </div>
                      <p className="mt-1 max-w-2xl text-sm leading-6 text-muted-foreground">
                        {hasCoreModel
                          ? t('stats.consoleHome.readyDescription')
                          : home.canManageModelConfig
                            ? t('stats.consoleHome.incompleteDescription')
                            : t('stats.consoleHome.modelConfigManagedByAdmin')}
                      </p>
                    </div>
                  )}
                </div>
                {home.isReadinessLoading ? (
                  <Skeleton className="h-6 w-20 rounded-full" />
                ) : home.hasReadinessError ? (
                  <Badge variant="subtle">{t('stats.consoleHome.unavailable')}</Badge>
                ) : (
                  <Badge variant={hasCoreModel ? 'success' : 'warning'}>
                    {hasCoreModel
                      ? t('stats.consoleHome.ready')
                      : t('stats.consoleHome.needsAttention')}
                  </Badge>
                )}
              </CardContent>
            </Card>

            <OnboardingChecklist
              checklist={home.checklist}
              canManageModelConfig={home.canManageModelConfig}
              access={home.actionAccess}
              isLoading={home.isChecklistLoading}
              hasError={home.hasStatsError}
            />
            <WorkspaceRecentWork
              items={home.recentWork}
              isLoading={home.isRecentWorkLoading}
              hasError={home.hasRecentWorkError}
              hasWorkspace={home.contextStatus === 'ready' && Boolean(home.currentWorkspace)}
            />
          </main>

          <aside className="min-w-0">
            <WorkspaceQuickActions access={home.actionAccess} />
          </aside>
        </div>
      </div>
    </div>
  );
}
