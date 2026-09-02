'use client';

import Link from 'next/link';
import {
  ArrowRight,
  Atom,
  BookOpen,
  CircleAlert,
  Database,
  MessageSquare,
  Workflow,
} from 'lucide-react';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import type { DashboardRecentWorkItem, DashboardRecentWorkType } from '@/services/types/dashboard';
import { getRecentWorkNavigationHref } from '@/utils/console-recent-work';
import { formatDate } from '@/utils/format';

const recentWorkIcons = {
  conversation: MessageSquare,
  agent: Atom,
  workflow: Workflow,
  dataset: BookOpen,
  database: Database,
} satisfies Record<DashboardRecentWorkType, React.ElementType>;

function getRecentTypeLabel(
  type: DashboardRecentWorkType,
  t: ReturnType<typeof useT<'dashboard'>>
) {
  switch (type) {
    case 'conversation':
      return t('stats.consoleHome.recentTypes.conversation');
    case 'agent':
      return t('stats.consoleHome.recentTypes.agent');
    case 'workflow':
      return t('stats.consoleHome.recentTypes.workflow');
    case 'dataset':
      return t('stats.consoleHome.recentTypes.dataset');
    case 'database':
      return t('stats.consoleHome.recentTypes.database');
  }
}

export function WorkspaceRecentWork({
  items,
  isLoading,
  hasError,
  hasWorkspace,
}: {
  items: DashboardRecentWorkItem[];
  isLoading: boolean;
  hasError: boolean;
  hasWorkspace: boolean;
}) {
  const t = useT('dashboard');
  const hasUnavailableItems =
    !hasWorkspace &&
    items.some(
      item =>
        !getRecentWorkNavigationHref(
          hasWorkspace,
          item.type,
          item.resource_id,
          item.parent_id,
          item.workspace_id
        )
    );

  return (
    <Card className="border-border/80 shadow-sm">
      <CardHeader className="pb-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <CardTitle className="text-lg">
              {t('stats.consoleHome.workspaceOverview.recentTitle')}
            </CardTitle>
            <CardDescription className="mt-1.5 leading-6">
              {t('stats.consoleHome.workspaceOverview.recentDescription')}
            </CardDescription>
          </div>
          {!isLoading && !hasError ? (
            <Badge variant="subtle">
              {t('stats.consoleHome.workspaceOverview.recentCount', { count: items.length })}
            </Badge>
          ) : null}
        </div>
      </CardHeader>
      <CardContent>
        {hasUnavailableItems && !isLoading && !hasError ? (
          <div className="mb-3 flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 px-3 py-2.5 text-xs leading-5 text-warning">
            <CircleAlert className="mt-0.5 size-3.5 shrink-0" />
            <span>{t('stats.consoleHome.noWorkspaceHint')}</span>
          </div>
        ) : null}
        {hasError ? (
          <div className="rounded-lg border border-warning/20 bg-warning/10 px-6 py-8 text-center text-warning">
            <CircleAlert className="mx-auto size-6" />
            <div className="mt-3 text-sm font-medium">
              {t('stats.consoleHome.workspaceOverview.recentLoadError')}
            </div>
          </div>
        ) : isLoading ? (
          <div className="grid gap-3 md:grid-cols-2">
            {Array.from({ length: 4 }).map((_, index) => (
              <Skeleton key={index} className="h-20 rounded-lg" />
            ))}
          </div>
        ) : items.length === 0 ? (
          <div className="rounded-lg border border-dashed border-border px-6 py-10 text-center">
            <MessageSquare className="mx-auto size-6 text-muted-foreground" />
            <div className="mt-3 text-sm font-medium">
              {t('stats.consoleHome.workspaceOverview.emptyRecentTitle')}
            </div>
            <p className="mx-auto mt-1 max-w-lg text-xs leading-5 text-muted-foreground">
              {t('stats.consoleHome.workspaceOverview.emptyRecentDescription')}
            </p>
          </div>
        ) : (
          <div className="grid gap-3 md:grid-cols-2">
            {items.map(item => {
              const Icon = recentWorkIcons[item.type];
              const navigationHref = getRecentWorkNavigationHref(
                hasWorkspace,
                item.type,
                item.resource_id,
                item.parent_id,
                item.workspace_id
              );
              const content = (
                <>
                  <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                    <Icon className="size-4" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium text-foreground">
                        {item.title}
                      </span>
                      {navigationHref ? (
                        <ArrowRight className="ml-auto size-3.5 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
                      ) : null}
                    </span>
                    <span className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                      <span>{getRecentTypeLabel(item.type, t)}</span>
                      {item.workspace_name ? <span>· {item.workspace_name}</span> : null}
                      <span>· {formatDate(item.updated_at, 'MM-DD HH:mm')}</span>
                    </span>
                  </span>
                </>
              );

              return navigationHref ? (
                <Link
                  key={`${item.type}-${item.id}`}
                  href={navigationHref}
                  className="group flex items-start gap-3 rounded-lg border border-border/70 p-3 transition-colors hover:border-border-strong hover:bg-muted/40"
                >
                  {content}
                </Link>
              ) : (
                <div
                  key={`${item.type}-${item.id}`}
                  aria-disabled="true"
                  title={t('stats.consoleHome.noWorkspaceHint')}
                  className="flex cursor-not-allowed items-start gap-3 rounded-lg border border-border/70 p-3 opacity-70"
                >
                  {content}
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
